package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/middleware"
	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/aeus/pkg/netutil"
)

type Server struct {
	ctx                     context.Context
	opts                    *options
	uri                     *url.URL
	serve                   *http.Server
	engine                  *gin.Engine
	listener                net.Listener
	listenerMu              sync.Mutex
	pprofListener           net.Listener
	pprofServe              *http.Server
	fs                      *filesystem
	mcpServer               *mcp.Server
	middlewares             []middleware.Middleware
	Logger                  logger.Logger
	autoCompress            bool
	routeRegisteredCallback OnRouteRegistered
	tracer                  telemetry.Tracer
	metrics                 telemetry.MetricsRecorder
}

// Engine returns the gin engine of the server.
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) SetTracer(t telemetry.Tracer) {
	s.tracer = t
}

func (s *Server) SetMetrics(r telemetry.MetricsRecorder) {
	s.metrics = r
}

// Endpoint returns the endpoint of the server.
func (s *Server) Endpoint(ctx context.Context) (string, error) {
	if err := s.createListener(); err != nil {
		return "", err
	}
	return s.uri.String(), nil
}

// Mcp returns the mcp server if enabled, otherwise returns nil.
func (s *Server) Mcp() *mcp.Server {
	if !s.opts.mcp.Enable {
		return nil
	}
	return s.mcpServer
}

// register wires a handler for a single HTTP method through the shared
// child-Context lifecycle. All public verbs (GET, POST, ...) are
// one-line wrappers around it; new verbs follow the same pattern.
func (s *Server) register(method, pattern string, h HandleFunc) {
	if s.routeRegisteredCallback != nil {
		s.routeRegisteredCallback(method, pattern)
	}
	s.engine.Handle(method, pattern, func(ctx *gin.Context) {
		childCtx := newContext(ctx)
		if err := h(childCtx); err != nil {
			ctx.Error(err)
		}
		putContext(childCtx)
	})
}

// GET registers a handler for HTTP GET requests on pattern.
func (s *Server) GET(pattern string, h HandleFunc) { s.register(http.MethodGet, pattern, h) }

// POST registers a handler for HTTP POST requests on pattern.
func (s *Server) POST(pattern string, h HandleFunc) { s.register(http.MethodPost, pattern, h) }

// PUT registers a handler for HTTP PUT requests on pattern.
func (s *Server) PUT(pattern string, h HandleFunc) { s.register(http.MethodPut, pattern, h) }

// HEAD registers a handler for HTTP HEAD requests on pattern.
func (s *Server) HEAD(pattern string, h HandleFunc) { s.register(http.MethodHead, pattern, h) }

// PATCH registers a handler for HTTP PATCH requests on pattern.
func (s *Server) PATCH(pattern string, h HandleFunc) { s.register(http.MethodPatch, pattern, h) }

// DELETE registers a handler for HTTP DELETE requests on pattern.
func (s *Server) DELETE(pattern string, h HandleFunc) { s.register(http.MethodDelete, pattern, h) }

func (s *Server) Use(middlewares ...middleware.Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *Server) Handle(method string, uri string, handler http.HandlerFunc) {
	s.engine.Handle(method, uri, func(ctx *gin.Context) {
		handler(ctx.Writer, ctx.Request)
	})
}

func (s *Server) Webroot(prefix string, autoCompress bool, fs http.FileSystem) {
	s.autoCompress = autoCompress
	s.fs = newFS(time.Now(), fs)
	s.fs.SetPrefix(prefix)
	s.fs.DenyAccessDirectory()
	s.fs.SetIndexFile("/index.html")
}

func (s *Server) shouldCompress(req *http.Request) bool {
	if !s.autoCompress {
		return false
	}
	if !strings.Contains(req.Header.Get(headerAcceptEncoding), "gzip") ||
		strings.Contains(req.Header.Get("Connection"), "Upgrade") {
		return false
	}

	// Check if the request path is excluded from compression
	extension := filepath.Ext(req.URL.Path)
	if slices.Contains(assetsExtensions, extension) {
		return true
	}
	return false
}

func (s *Server) staticHandle(ctx *gin.Context, fp http.File) bool {
	uri := path.Clean(ctx.Request.URL.Path)
	fi, err := fp.Stat()
	if err != nil {
		return false
	}
	if !fi.IsDir() {
		//https://github.com/gin-contrib/gzip
		if s.shouldCompress(ctx.Request) && fi.Size() > 8192 {
			gzWriter := newGzipWriter()
			gzWriter.Reset(ctx.Writer)
			ctx.Header(headerContentEncoding, "gzip")
			ctx.Writer.Header().Add(headerVary, headerAcceptEncoding)
			originalEtag := ctx.GetHeader("ETag")
			if originalEtag != "" && !strings.HasPrefix(originalEtag, "W/") {
				ctx.Header("ETag", "W/"+originalEtag)
			}
			ctx.Writer = &gzipWriter{ctx.Writer, gzWriter}
			defer func() {
				if ctx.Writer.Size() < 0 {
					gzWriter.Reset(io.Discard)
				}
				if closeErr := gzWriter.Close(); closeErr != nil {
					s.Logger.Warnf(ctx, "gzip close error: %v", closeErr)
				}
				if ctx.Writer.Size() > -1 {
					ctx.Header("Content-Length", strconv.Itoa(ctx.Writer.Size()))
				}
				putGzipWriter(gzWriter)
			}()
		}
	}
	http.ServeContent(ctx.Writer, ctx.Request, path.Base(uri), s.fs.modtime, fp)
	ctx.Abort()
	return true
}

func (s *Server) notFoundHandle(ctx *gin.Context) {
	if s.fs != nil && ctx.Request.Method == http.MethodGet {
		uri := path.Clean(ctx.Request.URL.Path)
		if fp, err := s.fs.Open(uri); err == nil {
			defer fp.Close()
			if s.staticHandle(ctx, fp) {
				return
			}
		} else {
			//if found compress file
			if fp, err := s.fs.Open(uri + ".gz"); err == nil {
				defer fp.Close()
				if s.staticHandle(ctx, fp) {
					return
				}
			}
		}
	}
	ctx.JSON(http.StatusNotFound, newResponse(int(errs.CodeNotFound), "Not Found", nil))
}

func (s *Server) CORSInterceptor() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "OPTIONS" {
			c.Writer.Header().Add("Vary", "Origin")
			c.Writer.Header().Add("Vary", "Access-Control-Request-Method")
			c.Writer.Header().Add("Vary", "Access-Control-Request-Headers")
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,PUT,PATCH,POST,DELETE")
			h := c.Request.Header.Get("Access-Control-Request-Headers")
			if h != "" {
				c.Writer.Header().Set("Access-Control-Allow-Headers", h)
			}
			c.AbortWithStatus(204)
			return
		} else {
			c.Writer.Header().Add("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			h := c.Request.Header.Get("Access-Control-Request-Headers")
			if h != "" {
				c.Writer.Header().Set("Access-Control-Allow-Headers", h)
			}
		}
		c.Next()
	}
}

func (s *Server) requestInterceptor() gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		start := time.Now()
		ctx := ginCtx.Request.Context()
		var span telemetry.Span

		if s.tracer != nil {
			md := metadata.FromContext(ctx)
			ctx, span = s.tracer.Start(
				s.tracer.Extract(ctx, telemetry.MetadataCarrier{MD: md}),
				fmt.Sprintf("%s %s", ginCtx.Request.Method, ginCtx.Request.URL.Path),
				telemetry.WithSpanKind(telemetry.SpanKindServer),
			)
			defer span.End()
			span.SetAttributes(
				telemetry.String("http.method", ginCtx.Request.Method),
				telemetry.String("http.url", ginCtx.Request.URL.String()),
				telemetry.String("http.client_ip", ginCtx.ClientIP()),
			)
		}

		next := func(ctx context.Context) error {
			ginCtx.Request = ginCtx.Request.WithContext(ctx)
			ginCtx.Next()
			if err := ginCtx.Errors.Last(); err != nil {
				return err.Err
			}
			return nil
		}
		handler := middleware.Chain(s.middlewares...)(next)
		md := metadata.FromContext(ctx)
		ginCtx.Request.Header.Set(metadata.RequestClientIP, ginCtx.ClientIP())
		md.TeeReader(&httpMetadataReader{
			r: ginCtx.Request.Header,
		})
		md.TeeWriter(&httpMetadataWriter{
			w: ginCtx.Writer,
		})
		if !md.Has(metadata.RequestID) {
			md.Set(metadata.RequestID, uuid.New().String())
		}
		md.Set(metadata.RequestProtocol, Protocol)
		md.Set(metadata.RequestPath, ginCtx.FullPath())
		md.Set(metadata.RequestMethod, ginCtx.Request.Method)
		ctx = metadata.NewContext(ctx, md)

		err := handler(ctx)
		status := fmt.Sprintf("%d", ginCtx.Writer.Status())
		if err != nil {
			if middleware.IsAbort(err) {
				status = "abort"
				var ae *errs.Error
				if errors.As(err, &ae) {
					ginCtx.AbortWithStatusJSON(ae.HTTPStatus(), newResponse(int(ae.Code), ae.Message, nil))
				} else {
					ginCtx.AbortWithStatusJSON(http.StatusOK, newResponse(int(errs.CodeOK), "", nil))
				}
			} else {
				status = "error"
				if span != nil {
					span.SetError(err)
				}
				if se, ok := err.(*errs.Error); ok {
					ginCtx.AbortWithStatusJSON(se.HTTPStatus(), newResponse(int(se.Code), se.Message, nil))
				} else {
					ginCtx.AbortWithStatusJSON(http.StatusInternalServerError, newResponse(int(errs.CodeUnavailable), err.Error(), nil))
				}
			}
		}

		if s.metrics != nil {
			d := time.Since(start)
			// Use FullPath() to get the route template (e.g. /api/users/:id)
			// instead of the raw URL path to avoid unbounded Prometheus cardinality.
			path := ginCtx.FullPath()
			if path == "" {
				path = ginCtx.Request.URL.Path
			}
			s.metrics.RecordRequestDuration(ctx, Protocol, path, status, d)
			s.metrics.RecordRequestTotal(ctx, Protocol, path, status)
		}
	}
}

func (s *Server) createListener() (err error) {
	// Endpoint() 可能被多个 goroutine 并发调用(如 Service.build 与调用方),
	// 惰性创建 listener 与读取 uri 必须互斥,否则存在 data race。
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

func (s *Server) getMcpServer(r *http.Request) *mcp.Server {
	return s.mcpServer
}

func (s *Server) verifyMcpToken(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	if token != s.opts.mcp.Authorization {
		return nil, errs.ErrPermissionDenied
	}
	return &auth.TokenInfo{
		Scopes:     []string{"access"},
		Expiration: time.Now().Add(time.Hour * 2),
	}, nil
}

func (s *Server) Start(ctx context.Context) (err error) {
	s.serve = &http.Server{
		Addr:    s.opts.address,
		Handler: s.engine,
	}
	if s.opts.logger != nil {
		s.Logger = s.opts.logger
	}
	if s.Logger == nil {
		s.Logger = logger.Default()
	}
	s.ctx = ctx
	if s.opts.debug {
		// pprof is never bound on the public listener: it lands on a
		// dedicated loopback-only listener when WithDebugAddr is set,
		// otherwise it is skipped with a startup warning. AEUS_DEBUG=1
		// alone does not expose the process.
		if s.opts.debugAddr == "" {
			s.Logger.Warnf(ctx, "pprof disabled: set transport/http.WithDebugAddr (e.g. \"127.0.0.1:6060\") to expose debug endpoints")
		} else if err = s.startPprof(ctx); err != nil {
			return
		}
	}
	if s.opts.mcp.Enable {
		var mcpHttpHandler http.Handler
		streamableHTTPOpts := &mcp.StreamableHTTPOptions{
			Stateless:      true,
			JSONResponse:   true,
			SessionTimeout: s.opts.mcp.SessionTimeout,
		}
		mcpStreamableHTTPHandler := mcp.NewStreamableHTTPHandler(s.getMcpServer, streamableHTTPOpts)
		if s.opts.mcp.Authorization != "" {
			mcpAuthMiddleware := auth.RequireBearerToken(s.verifyMcpToken, &auth.RequireBearerTokenOptions{
				Scopes: []string{"access"},
			})
			mcpHttpHandler = mcpAuthMiddleware(mcpStreamableHTTPHandler)
		} else {
			mcpHttpHandler = mcpStreamableHTTPHandler
		}
		s.engine.GET(s.opts.mcp.Path, gin.WrapH(mcpHttpHandler))
		s.engine.POST(s.opts.mcp.Path, gin.WrapH(mcpHttpHandler))
		s.engine.PUT(s.opts.mcp.Path, gin.WrapH(mcpHttpHandler))
	}
	if s.opts.enableHealth {
		s.engine.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "up"})
		})
		s.engine.GET("/ready", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ready"})
		})
	}
	if s.opts.enableMetrics {
		s.engine.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}

	if err = s.createListener(); err != nil {
		return
	}
	s.engine.NoRoute(s.notFoundHandle)
	s.Logger.Infof(ctx, "http server listen on: %s", s.uri.Host)
	if s.opts.certFile != "" && s.opts.keyFile != "" {
		s.uri.Scheme = "https"
		err = s.serve.ServeTLS(s.listener, s.opts.certFile, s.opts.keyFile)
	} else {
		err = s.serve.Serve(s.listener)
	}
	if !errs.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) (err error) {
	if s.pprofServe != nil {
		if shutdownErr := s.pprofServe.Shutdown(ctx); shutdownErr != nil {
			s.Logger.Warnf(ctx, "pprof server shutdown error: %v", shutdownErr)
		}
	}
	if s.serve == nil {
		return nil
	}
	err = s.serve.Shutdown(ctx)
	s.Logger.Infof(ctx, "http server stopped")
	return
}

// startPprof spins up the dedicated pprof listener. We refuse to mount
// pprof on a non-loopback address unless the caller explicitly asks
// for it: pprof.Profile pins a CPU for 30s on demand, pprof.Trace
// streams execution traces, and the heap endpoint leaks internal
// types — all of which are dangerous on a public listener. The loopback
// guard keeps "open the debug port and forget to close it" from
// becoming an exposure.
func (s *Server) startPprof(ctx context.Context) error {
	if s.opts.debugAddr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", s.opts.debugAddr)
	if err != nil {
		return fmt.Errorf("pprof listen: %w", err)
	}
	host, _, splitErr := net.SplitHostPort(ln.Addr().String())
	if splitErr == nil && host != "" && host != "127.0.0.1" && host != "::1" && host != "localhost" {
		_ = ln.Close()
		return fmt.Errorf("pprof listener must bind to a loopback address; got %q", ln.Addr().String())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	s.pprofListener = ln
	s.pprofServe = &http.Server{Handler: mux}
	s.Logger.Infof(ctx, "pprof server listen on: %s", ln.Addr().String())
	go func() {
		if err := s.pprofServe.Serve(ln); err != nil && !errs.Is(err, http.ErrServerClosed) {
			s.Logger.Warnf(ctx, "pprof server error: %v", err)
		}
	}()
	return nil
}

func New(cbs ...Option) *Server {
	svr := &Server{
		uri: &url.URL{Scheme: "http"},
		opts: &options{
			network: "tcp",
		},
	}
	portStr := os.Getenv("HTTP_PORT")
	port, err := strconv.Atoi(portStr)
	if err != nil && portStr != "" {
		panic(fmt.Sprintf("invalid HTTP_PORT %q: %v", portStr, err))
	}
	svr.opts.address = fmt.Sprintf(":%d", port)
	for _, cb := range cbs {
		cb(svr.opts)
	}
	if !svr.opts.debug {
		gin.SetMode(gin.ReleaseMode)
	}
	svr.engine = gin.New(svr.opts.ginOptions...)
	if svr.opts.enableCORS {
		svr.engine.Use(svr.CORSInterceptor())
	}
	if svr.opts.mcp.Enable {
		mcpServerOpts := &mcp.ServerOptions{}
		svr.mcpServer = mcp.NewServer(&mcp.Implementation{
			Name:    svr.opts.mcp.Name,
			Version: svr.opts.mcp.Version,
		}, mcpServerOpts)
	}
	svr.engine.Use(svr.requestInterceptor())
	return svr
}
