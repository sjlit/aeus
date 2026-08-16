package resolver

import (
	"context"

	"github.com/sjlit/aeus/registry"
)

type (
	options struct {
		context  context.Context
		registry registry.Registrar
	}

	Option func(*options)
)

func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.context = ctx
	}
}

func WithRegistrar(registrar registry.Registrar) Option {
	return func(o *options) {
		o.registry = registrar
	}
}

// WithRegistry is an alias for [WithRegistrar].
func WithRegistry(registrar registry.Registrar) Option {
	return WithRegistrar(registrar)
}

func newOptions(cbs ...Option) *options {
	opts := &options{
		context: context.Background(),
	}
	for _, cb := range cbs {
		cb(opts)
	}
	return opts
}
