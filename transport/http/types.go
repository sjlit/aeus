package http

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjlit/aeus/infra/logger"
)

const (
	Protocol = "http"
)

type (
	Option func(*options)

	// CORSConfig controls the CORS interceptor installed by WithCORS.
	//
	// AllowOrigins is required: an empty list is rejected at construction
	// time instead of silently disabling the interceptor. Origins are
	// exact scheme://host[:port] values; "*" allows any origin but is
	// rejected together with AllowCredentials (the Fetch spec forbids
	// wildcard origins on credentialed requests).
	CORSConfig struct {
		AllowOrigins     []string      // required; exact origins or "*"
		AllowMethods     []string      // default GET, HEAD, POST, PUT, PATCH, DELETE
		AllowHeaders     []string      // empty = reflect Access-Control-Request-Headers
		ExposeHeaders    []string      // exposed to browser JavaScript
		AllowCredentials bool          // send Access-Control-Allow-Credentials
		MaxAge           time.Duration // preflight cache lifetime; 0 = omit
	}

	RouteOptions struct{}

	RouteOption func(o *RouteOptions)

	options struct {
		network       string
		address       string
		certFile      string
		keyFile       string
		debug         bool
		debugAddr     string // loopback-only pprof listener; "" disables pprof even when debug is true
		handler       http.Handler
		logger        logger.Logger
		context       context.Context
		ginOptions    []gin.OptionFunc
		cors          *CORSConfig
		enableHealth  bool
		enableMetrics bool

		// Server timeouts. ReadHeaderTimeout is the Slowloris guard and
		// is always on by default; read/write timeouts stay 0 (disabled)
		// because MCP / streamable HTTP and file downloads may legitimately
		// run longer than a normal request. Callers can opt into stricter
		// values via WithReadTimeout / WithWriteTimeout.
		readHeaderTimeout time.Duration
		readTimeout       time.Duration
		writeTimeout      time.Duration
		idleTimeout       time.Duration
		maxHeaderBytes    int
	}

	HandleFunc func(ctx *Context) (err error)

	Middleware func(http.Handler) http.Handler

	OnRouteRegistered func(method string, parent string)

	httpMetadataReader struct {
		r http.Header
	}

	httpMetadataWriter struct {
		w http.ResponseWriter
	}
)

const (
	headerAcceptEncoding  = "Accept-Encoding"
	headerContentEncoding = "Content-Encoding"
	headerVary            = "Vary"
)

var (
	gzPool           sync.Pool
	assetsExtensions = []string{".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".otf"}
)

type gzipWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (g *gzipWriter) WriteString(s string) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write([]byte(s))
}

func (g *gzipWriter) Write(data []byte) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write(data)
}

func (g *gzipWriter) Flush() {
	_ = g.writer.Flush()
	g.ResponseWriter.Flush()
}

// Fix: https://github.com/mholt/caddy/issues/38
func (g *gzipWriter) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

var _ http.Hijacker = (*gzipWriter)(nil)

// Hijack allows the caller to take over the connection from the HTTP server.
// After a call to Hijack, the HTTP server library will not do anything else with the connection.
// It becomes the caller's responsibility to manage and close the connection.
//
// It returns the underlying net.Conn, a buffered reader/writer for the connection, and an error
// if the ResponseWriter does not support the Hijacker interface.
func (g *gzipWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := g.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the ResponseWriter doesn't support the Hijacker interface")
	}
	return hijacker.Hijack()
}

func newGzipWriter() (writer *gzip.Writer) {
	v := gzPool.Get()
	if v == nil {
		writer, _ = gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
	} else {
		if w, ok := v.(*gzip.Writer); ok {
			return w
		} else {
			writer, _ = gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		}
	}
	return
}

func putGzipWriter(writer *gzip.Writer) {
	gzPool.Put(writer)
}

func WithNetwork(network string) Option {
	return func(o *options) {
		o.network = network
	}
}

// WithCORS installs the CORS interceptor. The configuration is validated
// when the server is constructed; an empty AllowOrigins panics rather than
// silently allowing or blocking every cross-origin request.
func WithCORS(cfg CORSConfig) Option {
	return func(o *options) {
		c := cfg
		o.cors = &c
	}
}

func WithAddress(address string) Option {
	return func(o *options) {
		o.address = address
	}
}

func WithCertFile(certFile string) Option {
	return func(o *options) {
		o.certFile = certFile
	}
}

func WithKeyFile(keyFile string) Option {
	return func(o *options) {
		o.keyFile = keyFile
	}
}

func WithLogger(lg logger.Logger) Option {
	return func(o *options) {
		o.logger = lg
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.context = ctx
	}
}

func WithDebug(debug bool) Option {
	return func(o *options) {
		o.debug = debug
	}
}

// WithDebugAddr exposes the pprof endpoints on a separate listener bound
// to the given address (e.g. "127.0.0.1:6060"). pprof is intentionally
// never reachable on the main HTTP listener: a remote attacker can pin
// the CPU for 30s with /debug/pprof/profile, and the heap endpoint
// leaks internal types — both are dangerous to leave on a public port.
//
// When WithDebug(true) is set and WithDebugAddr is left empty, pprof is
// skipped (Start logs a warning) so an over-eager AEUS_DEBUG environment
// variable does not expose the process. pprof is never mounted on the
// main listener; provide a loopback address here to enable it.
func WithDebugAddr(addr string) Option {
	return func(o *options) {
		o.debugAddr = addr
	}
}

func WithHandler(h http.Handler) Option {
	return func(o *options) {
		o.handler = h
	}
}

func WithGinOptions(opts ...gin.OptionFunc) Option {
	return func(o *options) {
		o.ginOptions = opts
	}
}

func (m *httpMetadataReader) Get(key string) string {
	if m.r == nil {
		return ""
	}
	return m.r.Get(key)
}

func (m *httpMetadataWriter) Set(key string, value string) {
	if m.w == nil {
		return
	}
	m.w.Header().Set(key, value)
}

func WithEnableHealth(enable bool) Option {
	return func(o *options) {
		o.enableHealth = enable
	}
}

func WithEnableMetrics(enable bool) Option {
	return func(o *options) {
		o.enableMetrics = enable
	}
}

// WithReadHeaderTimeout sets the http.Server ReadHeaderTimeout. The default
// is 10s, which blocks Slowloris-style header trickling without affecting
// normal requests.
func WithReadHeaderTimeout(d time.Duration) Option {
	return func(o *options) {
		o.readHeaderTimeout = d
	}
}

// WithReadTimeout sets the http.Server ReadTimeout. The default is 0
// (disabled) so large uploads and streaming requests are not cut off.
func WithReadTimeout(d time.Duration) Option {
	return func(o *options) {
		o.readTimeout = d
	}
}

// WithWriteTimeout sets the http.Server WriteTimeout. The default is 0
// (disabled) so MCP streamable HTTP and file downloads are not cut off.
func WithWriteTimeout(d time.Duration) Option {
	return func(o *options) {
		o.writeTimeout = d
	}
}

// WithIdleTimeout sets the http.Server IdleTimeout. The default is 120s.
func WithIdleTimeout(d time.Duration) Option {
	return func(o *options) {
		o.idleTimeout = d
	}
}

// WithMaxHeaderBytes sets the http.Server MaxHeaderBytes. The default is
// 1 MiB.
func WithMaxHeaderBytes(n int) Option {
	return func(o *options) {
		o.maxHeaderBytes = n
	}
}

func NewRouteOptions(opts ...RouteOption) *RouteOptions {
	s := &RouteOptions{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
