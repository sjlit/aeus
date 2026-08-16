package grpc

import (
	"context"
	"crypto/tls"

	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/aeus/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	Protocol = "grpc"
)

type (
	Option func(*options)

	options struct {
		network  string
		address  string
		grpcOpts []grpc.ServerOption
		logger   logger.Logger
		context  context.Context
	}

	clientOptions struct {
		tls         *tls.Config
		registry    registry.Registrar
		dialOptions []grpc.DialOption
	}

	ClientOption func(*clientOptions)

	grpcMetadataReader struct {
		md metadata.MD
	}

	grpcMetadataWriter struct {
		md metadata.MD
	}

	requestValueContextKey struct{}
)

func WithNetwork(network string) Option {
	return func(o *options) {
		o.network = network
	}
}

func WithAddress(address string) Option {
	return func(o *options) {
		o.address = address
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.context = ctx
	}
}

func WithLogger(lg logger.Logger) Option {
	return func(o *options) {
		o.logger = lg
	}
}

func WithTLS(tls *tls.Config) ClientOption {
	return func(o *clientOptions) {
		o.tls = tls
	}
}

// WithRegistrar sets the service discovery registrar used by the gRPC client.
//
// Deprecated: Use [WithRegistry] for backward compatibility. This alias preserves
// the previous name and will be removed in the next major version.
func WithRegistrar(reg registry.Registrar) ClientOption {
	return func(o *clientOptions) {
		o.registry = reg
	}
}

// WithRegistry is an alias for [WithRegistrar].
func WithRegistry(reg registry.Registrar) ClientOption {
	return WithRegistrar(reg)
}

func WithGrpcDialOptions(opts ...grpc.DialOption) ClientOption {
	return func(o *clientOptions) {
		o.dialOptions = opts
	}
}

func GetRequestValueFromContext(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(requestValueContextKey{})
}

func (m *grpcMetadataReader) Get(key string) string {
	if m.md == nil {
		return ""
	}
	vs := m.md.Get(key)
	if len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func (m *grpcMetadataWriter) Set(key string, value string) {
	if m.md == nil {
		return
	}
	m.md.Set(key, value)
}
