package aeus

import (
	"context"
	"maps"
	"reflect"
	"time"

	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/registry"
)

type options struct {
	id               string
	name             string
	version          string
	metadata         map[string]string
	ctx              context.Context
	logger           logger.Logger
	servers          []Server
	endpoints        []string
	scope            Scope
	debug            bool
	registrarTimeout time.Duration
	registry         registry.Registrar
	serviceLoader    ServiceLoader
	stopTimeout      time.Duration
	injectVars       []reflect.Value
	tracer           telemetry.Tracer
	metrics          telemetry.MetricsRecorder
}

func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

func WithVersion(version string) Option {
	return func(o *options) {
		o.version = version
	}
}

func WithMetadata(metadata map[string]string) Option {
	return func(o *options) {
		if o.metadata == nil {
			o.metadata = make(map[string]string)
		}
		maps.Copy(o.metadata, metadata)
	}
}

func WithServer(servers ...Server) Option {
	return func(o *options) {
		o.servers = append(o.servers, servers...)
	}
}

func WithEndpoint(endpoints ...string) Option {
	return func(o *options) {
		o.endpoints = append(o.endpoints, endpoints...)
	}
}

func WithLogger(logger logger.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

func WithScope(scope Scope) Option {
	return func(o *options) {
		o.scope = scope
	}
}

func WithDebug(debug bool) Option {
	return func(o *options) {
		o.debug = debug
	}
}

func WithInjectVars(vars ...any) Option {
	return func(o *options) {
		for _, v := range vars {
			o.injectVars = append(o.injectVars, reflect.ValueOf(v))
		}
	}
}

func WithServiceLoader(loader ServiceLoader) Option {
	return func(o *options) {
		o.serviceLoader = loader
	}
}

func WithStopTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.stopTimeout = timeout
	}
}

// WithRegistrar sets the service discovery registrar used by the service.
//
// Deprecated: Use [WithRegistry] for backward compatibility. This alias preserves
// the previous name and will be removed in the next major version.
func WithRegistrar(registrar registry.Registrar) Option {
	return func(o *options) {
		o.registry = registrar
	}
}

// WithRegistry is an alias for [WithRegistrar].
func WithRegistry(registrar registry.Registrar) Option {
	return WithRegistrar(registrar)
}

func WithRegistrarTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.registrarTimeout = timeout
	}
}

func WithTracer(tracer telemetry.Tracer) Option {
	return func(o *options) {
		o.tracer = tracer
	}
}

func WithMetrics(recorder telemetry.MetricsRecorder) Option {
	return func(o *options) {
		o.metrics = recorder
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

func WithAppContext(ctx context.Context, app Application) context.Context {
	return context.WithValue(ctx, applicationKey{}, app)
}
