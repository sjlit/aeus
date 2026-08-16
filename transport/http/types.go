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

	mcpOption struct {
		Enable         bool
		Name           string
		Path           string
		Version        string
		Description    string
		SessionTimeout time.Duration
		Authorization  string
	}

	RouteOptions struct {
		McpResponseMarshal McpResponseMarshal
	}

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
		mcp           mcpOption
		enableCORS    bool
		enableHealth  bool
		enableMetrics bool
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

func WithCORS() Option {
	return func(o *options) {
		o.enableCORS = true
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
// silently skipped (Start logs a warning) so an over-eager AEUS_DEBUG
// environment variable does not expose the process. To opt back into
// the legacy "pprof on the main listener" behaviour, pass the empty
// string explicitly with WithDebugAddr("").
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

func WithMCP(name, version, uri, description string) Option {
	if name == "" || version == "" {
		return func(o *options) {}
	}
	if uri == "" {
		uri = "/mcp"
	}
	return func(o *options) {
		o.mcp = mcpOption{
			Enable:      true,
			Name:        name,
			Path:        uri,
			Version:     version,
			Description: description,
		}
	}
}

func WithMcpName(name string) Option {
	return func(o *options) {
		o.mcp.Name = name
	}
}

func WithMcpPath(path string) Option {
	return func(o *options) {
		o.mcp.Path = path
	}
}

func WithMcpVersion(version string) Option {
	return func(o *options) {
		o.mcp.Version = version
	}
}

func WithMcpDescription(description string) Option {
	return func(o *options) {
		o.mcp.Description = description
	}
}

func WithMcpSessionTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.mcp.SessionTimeout = timeout
	}
}

func WithMcpAuthorization(auth string) Option {
	return func(o *options) {
		o.mcp.Authorization = auth
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

func WithRouteMcpResponseMarshal(marshal McpResponseMarshal) RouteOption {
	return func(o *RouteOptions) {
		o.McpResponseMarshal = marshal
	}
}

func NewRouteOptions(opts ...RouteOption) *RouteOptions {
	s := &RouteOptions{
		McpResponseMarshal: DefaultMcpResponseMarshal,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
