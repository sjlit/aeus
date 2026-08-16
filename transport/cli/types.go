package cli

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"github.com/sjlit/aeus/infra/logger"
)

var (
	// Feature is the protocol feature identifier sent at handshake:
	// the ASCII bytes "CLI". Kept as []byte to match the wire format.
	Feature = []byte("CLI")
	OK      = []byte("OK")
	Bye     = "Bye Bye"
)

const (
	Protocol = "cli"
)

var (
	ctxPool sync.Pool
)

type Param struct {
	Key   string
	Value string
}

type Params []Param

type HandleFunc func(ctx *Context) (err error)

type (
	Option func(*options)

	options struct {
		network string
		address string
		logger  logger.Logger
		context context.Context
	}

	clientOptions struct {
		tls *tls.Config
	}

	ClientOption func(*clientOptions)
)

type (
	encoder interface {
		Marshal() ([]byte, error)
	}

	responsePayload struct {
		Type    uint8  `json:"-"`
		Code    int    `json:"code"`
		Message string `json:"message,omitempty"`
		Data    any    `json:"data,omitempty"`
	}
)

type (
	Command struct {
		Path        string
		Handle      HandleFunc
		Description string
	}

	commander struct {
		Name        string
		Path        string
		Description string
	}

	handshake struct {
		ID         int64     `json:"id"`
		OS         string    `json:"os"`
		Name       string    `json:"name"`
		Version    string    `json:"version"`
		ServerTime time.Time `json:"server_time"`
		RemoteAddr string    `json:"remote_addr"`
	}

	cliMetadataReader struct {
		ctx *Context
	}
	cliMetadataWriter struct {
		ctx *Context
	}
)

func WithAddress(addr string) Option {
	return func(o *options) {
		o.address = addr
	}
}

func WithNetwork(network string) Option {
	return func(o *options) {
		o.network = network
	}
}

func WithLogger(logger logger.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.context = ctx
	}
}

func (r *cliMetadataReader) Get(key string) string {
	return r.ctx.Param(key)
}

func (r *cliMetadataWriter) Set(key string, value string) {
	r.ctx.SetValue(key, value)
}
