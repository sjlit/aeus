package registry

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/sjlit/aeus/infra/logger"
)

type (
	Service struct {
		ID        string            `json:"id"`
		Name      string            `json:"name"`
		Version   string            `json:"version"`
		Metadata  map[string]string `json:"metadata"`
		Endpoints []string          `json:"endpoints"`
	}
)

type (
	Watcher interface {
		// Next returns services in the following two cases:
		// 1.the first time to watch and the service instance list is not empty.
		// 2.any service instance changes found.
		// if the above two conditions are not met, it will block until context deadline exceeded or canceled
		Next() ([]*Service, error)
		// Stop close the watcher.
		Stop() error
	}
)

type (
	RegistryOptions struct {
		// Other options for implementations of the interface
		// can be stored in a context
		Context   context.Context
		TLSConfig *tls.Config
		Addrs     []string
		Timeout   time.Duration
		Secure    bool
	}

	RegistryOption func(o *RegistryOptions)

	RegisterOptions struct {
		Context context.Context
		// register ttl
		TTL time.Duration
	}

	RegisterOption func(o *RegisterOptions)

	WatchOptions struct {
		// Other options for implementations of the interface
		// can be stored in a context
		Context context.Context
		// Specify a service to watch
		// If blank, the watch is for all services
		Service string
		// Specify a logger
		Logger logger.Logger
	}

	WatchOption func(o *WatchOptions)

	DeregisterOptions struct {
		// Other options for implementations of the interface
		// can be stored in a context
		Context context.Context
	}

	DeregisterOption func(o *DeregisterOptions)

	GetOptions struct {
		// Other options for implementations of the interface
		// can be stored in a context
		Context context.Context
	}

	GetOption func(o *GetOptions)
)

func WithRegistryContext(ctx context.Context) RegistryOption {
	return func(o *RegistryOptions) {
		o.Context = ctx
	}
}

func WithAddress(address string) RegistryOption {
	return func(o *RegistryOptions) {
		o.Addrs = append(o.Addrs, address)
	}
}

func WithTLS(tls *tls.Config) RegistryOption {
	return func(o *RegistryOptions) {
		o.TLSConfig = tls
	}
}

func WithTimeout(t time.Duration) RegistryOption {
	return func(o *RegistryOptions) {
		o.Timeout = t
	}
}

func NewRegistryOption(cbs ...RegistryOption) *RegistryOptions {
	opts := &RegistryOptions{
		Context: context.Background(),
	}
	for _, o := range cbs {
		o(opts)
	}
	return opts
}

func WithTTL(t time.Duration) RegisterOption {
	return func(o *RegisterOptions) {
		o.TTL = t
	}
}

// WithRegisterContext attaches a context.Context to the Register call.
//
// The context controls cancellation and deadlines for the underlying
// registry backend (etcd, consul, ...).
func WithRegisterContext(ctx context.Context) RegisterOption {
	return func(o *RegisterOptions) {
		o.Context = ctx
	}
}

func NewRegisterOption(cbs ...RegisterOption) *RegisterOptions {
	opts := &RegisterOptions{
		Context: context.Background(),
	}
	for _, o := range cbs {
		o(opts)
	}
	return opts
}

func WithDeregisterContext(ctx context.Context) DeregisterOption {
	return func(o *DeregisterOptions) {
		o.Context = ctx
	}
}

func NewDeregisterOption(cbs ...DeregisterOption) *DeregisterOptions {
	opts := &DeregisterOptions{
		Context: context.Background(),
	}
	for _, o := range cbs {
		o(opts)
	}
	return opts
}

func WithService(service string) WatchOption {
	return func(o *WatchOptions) {
		o.Service = service
	}
}

func WithLogger(logger logger.Logger) WatchOption {
	return func(o *WatchOptions) {
		o.Logger = logger
	}
}

func WithWatchContext(ctx context.Context) WatchOption {
	return func(o *WatchOptions) {
		o.Context = ctx
	}
}

func NewWatchOption(cbs ...WatchOption) *WatchOptions {
	opts := &WatchOptions{
		Context: context.Background(),
		Logger:  logger.Default(),
	}
	for _, o := range cbs {
		o(opts)
	}
	return opts
}

func WithGetContext(ctx context.Context) GetOption {
	return func(o *GetOptions) {
		o.Context = ctx
	}
}

func NewGetOption(cbs ...GetOption) *GetOptions {
	opts := &GetOptions{
		Context: context.Background(),
	}
	for _, o := range cbs {
		o(opts)
	}
	return opts
}
