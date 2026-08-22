package cli

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/middleware"
	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/aeus/pkg/netutil"
)

type Server struct {
	ctx            context.Context
	router         *Router
	opts           *options
	listener       net.Listener
	listenerMu     sync.Mutex
	sequenceLocker sync.Mutex
	sequence       int64
	ctxMap         sync.Map
	uri            *url.URL
	exitFlag       int32
	middleware     []middleware.Middleware
	Logger         logger.Logger
	tracer         telemetry.Tracer
	metrics        telemetry.MetricsRecorder
}

func (svr *Server) Use(middlewares ...middleware.Middleware) {
	svr.middleware = append(svr.middleware, middlewares...)
}

func (svr *Server) SetTracer(t telemetry.Tracer) {
	svr.tracer = t
}

func (svr *Server) SetMetrics(r telemetry.MetricsRecorder) {
	svr.metrics = r
}

func (svr *Server) Handle(pathname string, desc string, cb HandleFunc) (err error) {
	return svr.router.Handle(pathname, svr.wrapCommand(pathname, desc, cb))
}

func (svr *Server) wrapCommand(pathname, desc string, cb HandleFunc) Command {
	h := func(ctx *Context) (err error) {
		return cb(ctx)
	}
	if desc == "" {
		desc = strings.Join(strings.Split(strings.TrimPrefix(pathname, "/"), "/"), " ")
	}
	return Command{
		Path:        pathname,
		Handle:      h,
		Description: desc,
	}
}

func (svr *Server) createListener() (err error) {
	// Endpoint() may be called concurrently with Start() (and with itself).
	// The lazy listener + uri.Host initialization must be serialized;
	// otherwise two goroutines both pass the nil check and produce a
	// double-bind to the OS port plus a data race on uri.Host. The http
	// and grpc transports already serialize this; CLI follows the same
	// shape for consistency.
	svr.listenerMu.Lock()
	defer svr.listenerMu.Unlock()
	if svr.listener != nil {
		return
	}
	if svr.listener, err = net.Listen(svr.opts.network, svr.opts.address); err == nil {
		svr.uri.Host = netutil.EffectiveAddr(svr.opts.address, svr.listener)
	}
	return
}

func (svr *Server) applyContext() *Context {
	if v := ctxPool.Get(); v != nil {
		if ctx, ok := v.(*Context); ok {
			return ctx
		}
	}
	return &Context{}
}

func (svr *Server) releaseContext(ctx *Context) {
	ctxPool.Put(ctx)
}

func (svr *Server) execute(ctx *Context, frame *Frame) (err error) {
	var (
		params map[string]string
		tokens []string
		args   []string
		r      *Router
	)
	cmd := string(frame.Data)
	tokens = strings.Fields(cmd)
	if frame.Timeout > 0 {
		childCtx, cancelFunc := context.WithTimeout(svr.ctx, time.Duration(frame.Timeout))
		ctx.setContext(childCtx)
		defer func() {
			cancelFunc()
		}()
	} else {
		ctx.setContext(svr.ctx)
	}
	var span telemetry.Span
	start := time.Now()
	if r, args, err = svr.router.Lookup(tokens); err != nil {
		if errs.Is(err, ErrNotFound) {
			err = ctx.Error(int(errs.CodeNotFound), fmt.Sprintf("Command %s not found", cmd))
		} else {
			err = ctx.Error(int(errs.CodeUnavailable), err.Error())
		}
	} else {
		if len(r.params) > len(args) {
			err = ctx.Error(int(errs.CodeUnavailable), r.Usage())
			return
		}
		if len(r.params) > 0 {
			params = make(map[string]string)
			for i, name := range r.params {
				params[name] = args[i]
			}
		}
		ctx.setArgs(args)
		ctx.setParam(params)
		if svr.tracer != nil {
			md := metadata.FromContext(ctx.ctx)
			ctx.ctx, span = svr.tracer.Start(
				svr.tracer.Extract(ctx.ctx, telemetry.MetadataCarrier{MD: md}),
				r.command.Path,
			)
			defer span.End()
		}
		h := func(c context.Context) error {
			return r.command.Handle(ctx)
		}
		next := middleware.Chain(svr.middleware...)(h)
		md := metadata.FromContext(ctx.ctx)
		md.Set(metadata.RequestPath, r.command.Path)
		md.Set(metadata.RequestProtocol, Protocol)
		md.TeeReader(&cliMetadataReader{ctx: ctx})
		md.TeeWriter(&cliMetadataWriter{ctx: ctx})
		ctx.ctx = metadata.NewContext(ctx.ctx, md)
		err = next(ctx.ctx)
		if svr.metrics != nil {
			status := "success"
			if err != nil {
				status = "error"
				if span != nil {
					span.SetError(err)
				}
			}
			svr.metrics.RecordRequestDuration(ctx.ctx, Protocol, r.command.Path, status, time.Since(start))
			svr.metrics.RecordRequestTotal(ctx.ctx, Protocol, r.command.Path, status)
		}
	}
	return
}

func (svr *Server) nextSequence() int64 {
	svr.sequenceLocker.Lock()
	defer svr.sequenceLocker.Unlock()
	if svr.sequence == math.MaxInt64 {
		svr.sequence = 1
	}
	svr.sequence++
	return svr.sequence
}

func (svr *Server) process(conn net.Conn) {
	var (
		err   error
		ctx   *Context
		frame *Frame
	)
	ctx = svr.applyContext()
	ctx.reset(svr.nextSequence(), conn)
	svr.ctxMap.Store(ctx.ID, ctx)
	defer func() {
		_ = conn.Close()
		svr.ctxMap.Delete(ctx.ID)
		svr.releaseContext(ctx)
	}()
readLoop:
	for {
		if frame, err = readFrame(conn); err != nil {
			break
		}
		//reset frame
		ctx.seq = frame.Seq
		switch frame.Type {
		case PacketTypeHandshake:
			if err = ctx.send(responsePayload{
				Type: PacketTypeHandshake,
				Data: &handshake{
					ID:         ctx.ID,
					Name:       "",
					Version:    "",
					OS:         runtime.GOOS,
					ServerTime: time.Now(),
					RemoteAddr: conn.RemoteAddr().String(),
				},
			}); err != nil {
				break readLoop
			}
		case PacketTypeCompleter:
			if err = ctx.send(responsePayload{
				Type: PacketTypeCompleter,
				Data: svr.router.Completer(strings.Fields(string(frame.Data))...),
			}); err != nil {
				break readLoop
			}
		case PacketTypeCommand:
			if err = svr.execute(ctx, frame); err != nil {
				break readLoop
			}
		default:
			// Unknown frame type — keep the connection alive and read the
			// next frame, but log so a misbehaving client doesn't hide
			// behind silent drops. Without this, a peer that speaks the
			// wrong protocol version can wedge a slot until the read
			// timeout fires with no audit trail.
			svr.Logger.Warnf(svr.ctx, "cli: ignoring unknown frame type=%d seq=%d from %s", frame.Type, frame.Seq, conn.RemoteAddr())
		}
	}
}

func (svr *Server) serve() (err error) {
	for {
		conn, err := svr.listener.Accept()
		if err != nil {
			if atomic.LoadInt32(&svr.exitFlag) == 1 {
				return nil
			}
			return err
		}
		go svr.process(conn)
	}
}

func (svr *Server) Start(ctx context.Context) (err error) {
	svr.ctx = ctx
	if svr.opts.logger != nil {
		svr.Logger = svr.opts.logger
	}
	if svr.Logger == nil {
		svr.Logger = logger.Default()
	}
	if err = svr.createListener(); err != nil {
		return
	}
	svr.Logger.Infof(ctx, "cli server listen on: %s", svr.uri.Host)
	err = svr.serve()
	return
}

func (svr *Server) Stop(ctx context.Context) (err error) {
	if !atomic.CompareAndSwapInt32(&svr.exitFlag, 0, 1) {
		return
	}
	if svr.listener != nil {
		if err = svr.listener.Close(); err != nil {
			svr.Logger.Warnf(ctx, "cli listener close error: %v", err)
		}
	}
	var closeErrs []error
	svr.ctxMap.Range(func(key, value any) bool {
		if ctx, ok := value.(*Context); ok {
			if e := ctx.Close(); e != nil {
				closeErrs = append(closeErrs, e)
			}
		}
		return true
	})
	if len(closeErrs) > 0 {
		err = closeErrs[0]
	}
	svr.Logger.Info(ctx, "cli server stopped")
	return
}

func New(cbs ...Option) *Server {
	srv := &Server{
		opts: &options{
			network: "tcp",
			address: ":0",
		},
		uri:    &url.URL{Scheme: "cli"},
		router: newRouter(""),
	}
	// Registered here rather than in Start so a server restart (a second
	// Start call) cannot hit the duplicate-registration error. The discard
	// is safe: the router is freshly created above, so this registration
	// cannot collide.
	_ = srv.Handle("/help", "Display help information", func(ctx *Context) (err error) {
		return ctx.Success(srv.router.String())
	})
	portStr := os.Getenv("CLI_PORT")
	port, err := strconv.Atoi(portStr)
	if err != nil && portStr != "" {
		panic(fmt.Sprintf("invalid CLI_PORT %q: %v", portStr, err))
	}
	srv.opts.address = fmt.Sprintf(":%d", port)
	for _, cb := range cbs {
		cb(srv.opts)
	}
	return srv
}
