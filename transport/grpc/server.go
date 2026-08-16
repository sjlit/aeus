package grpc

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"errors"

	"github.com/google/uuid"
	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/middleware"
	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/aeus/pkg/netutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmd "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type grpcMetadataCarrier struct {
	md grpcmd.MD
}

func (c grpcMetadataCarrier) Get(key string) string {
	vals := c.md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c grpcMetadataCarrier) Set(key, value string) {
	c.md.Set(key, value)
}

type Server struct {
	ctx         context.Context
	opts        *options
	uri         *url.URL
	serve       *grpc.Server
	listener    net.Listener
	listenerMu  sync.Mutex
	middlewares []middleware.Middleware
	Logger      logger.Logger
	tracer      telemetry.Tracer
	metrics     telemetry.MetricsRecorder
}

func (s *Server) createListener() (err error) {
	// Endpoint() may be called concurrently with Start() (and with itself).
	// The lazy listener + uri.Host initialization must be serialized,
	// otherwise two goroutines both pass the nil check and produce
	// a double-bind to the OS port plus a data race on uri.Host.
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	if s.listener == nil {
		if s.listener, err = net.Listen(s.opts.network, s.opts.address); err != nil {
			return
		}
		s.uri.Host = netutil.EffectiveAddr(s.opts.address, s.listener)
	}
	return
}

func (s *Server) unaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		ctx, span := s.startSpan(ctx, info.FullMethod)
		if span != nil {
			defer span.End()
		}

		next := func(ctx context.Context) error {
			var nerr error
			resp, nerr = handler(ctx, req)
			return nerr
		}
		h := middleware.Chain(s.middlewares...)(next)
		ctx, outgoing := s.attachMetadata(ctx, info.FullMethod)
		ctx = context.WithValue(ctx, requestValueContextKey{}, req)
		err = h(ctx)
		if err != nil {
			if span != nil && !middleware.IsAbort(err) {
				span.SetError(err)
			}
			err = mapGRPCError(err)
		}
		s.flushOutgoingMD(ctx, outgoing)
		s.recordMetrics(ctx, info.FullMethod, start, err)
		return
	}
}

func (s *Server) streamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		start := time.Now()
		ctx, span := s.startSpan(ss.Context(), info.FullMethod)
		if span != nil {
			defer span.End()
		}

		next := func(ctx context.Context) error {
			return handler(srv, ss)
		}
		h := middleware.Chain(s.middlewares...)(next)
		ctx, outgoing := s.attachMetadata(ctx, info.FullMethod)
		err = h(ctx)
		if err != nil {
			if span != nil && !middleware.IsAbort(err) {
				span.SetError(err)
			}
			err = mapGRPCError(err)
		}
		s.flushOutgoingMD(ctx, outgoing)
		s.recordMetrics(ctx, info.FullMethod, start, err)
		return
	}
}

// startSpan begins a server-side span if a tracer is configured.
// Returns the (possibly augmented) ctx and the span (nil if no tracer).
func (s *Server) startSpan(ctx context.Context, method string) (context.Context, telemetry.Span) {
	if s.tracer == nil {
		return ctx, nil
	}
	incoming, _ := grpcmd.FromIncomingContext(ctx)
	return s.tracer.Start(
		s.tracer.Extract(ctx, grpcMetadataCarrier{incoming.Copy()}),
		method,
		telemetry.WithSpanKind(telemetry.SpanKindServer),
	)
}

// attachMetadata reads the incoming gRPC MD, mirrors it into the framework
// Metadata tee, fills in request id / path / protocol, and returns the
// augmented ctx plus the outgoing MD so the caller can flush it via
// grpc.SetHeader.
func (s *Server) attachMetadata(ctx context.Context, fullMethod string) (context.Context, grpcmd.MD) {
	md := metadata.FromContext(ctx)
	if incoming, ok := grpcmd.FromIncomingContext(ctx); ok {
		md.TeeReader(&grpcMetadataReader{incoming})
	}
	outgoing, ok := grpcmd.FromOutgoingContext(ctx)
	if !ok {
		outgoing = make(grpcmd.MD)
	}
	md.TeeWriter(&grpcMetadataWriter{outgoing})
	if !md.Has(metadata.RequestID) {
		md.Set(metadata.RequestID, uuid.New().String())
	}
	md.Set(metadata.RequestPath, fullMethod)
	md.Set(metadata.RequestProtocol, Protocol)
	return metadata.NewContext(ctx, md), outgoing
}

// mapGRPCError converts a framework error into the gRPC status form:
//   - a *errs.Error becomes status.Error(code, msg);
//   - an Abort wrapper collapses to OK (the abort has already been written);
//   - any other error is returned unchanged.
func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}
	var ae *errs.Error
	if middleware.IsAbort(err) {
		if errors.As(err, &ae) {
			return status.Error(codes.Code(ae.GRPCStatus()), ae.Message)
		}
		return status.Error(codes.OK, "")
	}
	if errors.As(err, &ae) {
		return status.Error(codes.Code(ae.GRPCStatus()), ae.Message)
	}
	return err
}

// recordMetrics records request duration and total. err drives the status label.
func (s *Server) recordMetrics(ctx context.Context, method string, start time.Time, err error) {
	if s.metrics == nil {
		return
	}
	status := "success"
	if err != nil {
		status = "error"
	}
	s.metrics.RecordRequestDuration(ctx, Protocol, method, status, time.Since(start))
	s.metrics.RecordRequestTotal(ctx, Protocol, method, status)
}

// flushOutgoingMD writes the captured outgoing metadata to the response
// trailer if any was set during handler execution.
func (s *Server) flushOutgoingMD(ctx context.Context, outgoing grpcmd.MD) {
	if outgoing.Len() > 0 {
		_ = grpc.SetHeader(ctx, outgoing)
	}
}

func (s *Server) Use(middlewares ...middleware.Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *Server) SetTracer(t telemetry.Tracer) {
	s.tracer = t
}

func (s *Server) SetMetrics(r telemetry.MetricsRecorder) {
	s.metrics = r
}

func (s *Server) Start(ctx context.Context) (err error) {
	s.ctx = ctx
	if s.opts.logger != nil {
		s.Logger = s.opts.logger
	}
	if s.Logger == nil {
		s.Logger = logger.Default()
	}
	if err = s.createListener(); err != nil {
		return
	}
	s.Logger.Infof(ctx, "grpc server listen on: %s", s.uri.Host)
	reflection.Register(s.serve)
	return s.serve.Serve(s.listener)
}

func (s *Server) Endpoint(ctx context.Context) (string, error) {
	if err := s.createListener(); err != nil {
		return "", err
	}
	return s.uri.String(), nil
}

func (s *Server) RegisterService(sd *grpc.ServiceDesc, ss any) {
	s.serve.RegisterService(sd, ss)
}

func (s *Server) Stop(ctx context.Context) (err error) {
	if s.serve == nil {
		return nil
	}
	// GracefulStop blocks until every in-flight RPC finishes; without a
	// deadline a stuck RPC would hang shutdown indefinitely. When the
	// caller's ctx fires, fall back to Stop() (immediate cancel) so the
	// process can actually exit.
	done := make(chan struct{})
	go func() {
		s.serve.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		s.Logger.Infof(s.ctx, "grpc server stopped")
	case <-ctx.Done():
		s.serve.Stop()
		s.Logger.Warnf(ctx, "grpc server force-stopped: %v", ctx.Err())
		return ctx.Err()
	}
	return
}

func New(cbs ...Option) *Server {
	svr := &Server{
		opts: &options{
			network:  "tcp",
			grpcOpts: make([]grpc.ServerOption, 0, 10),
		},
		uri: &url.URL{
			Scheme: "grpc",
		},
		middlewares: make([]middleware.Middleware, 0, 10),
	}
	portStr := os.Getenv("GRPC_PORT")
	port, err := strconv.Atoi(portStr)
	if err != nil && portStr != "" {
		panic(fmt.Sprintf("invalid GRPC_PORT %q: %v", portStr, err))
	}
	svr.opts.address = fmt.Sprintf(":%d", port)
	for _, cb := range cbs {
		cb(svr.opts)
	}
	svr.opts.grpcOpts = append(svr.opts.grpcOpts, grpc.ChainUnaryInterceptor(svr.unaryServerInterceptor()))
	svr.opts.grpcOpts = append(svr.opts.grpcOpts, grpc.ChainStreamInterceptor(svr.streamServerInterceptor()))
	svr.serve = grpc.NewServer(svr.opts.grpcOpts...)
	return svr
}
