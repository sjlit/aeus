package memory

import (
	"time"

	"github.com/sjlit/aeus/infra/logger"
)

var (
	DefaultExpiration time.Duration = 0
	NoExpiration      time.Duration = -1
)

// Options represents the options for the cache.
type Options struct {
	// Logger is the be used logger
	Logger     logger.Logger
	Items      map[string]Item
	Expiration time.Duration
}

// Option manipulates the Options passed.
type Option func(o *Options)

// Expiration sets the duration for items stored in the cache to expire.
func Expiration(d time.Duration) Option {
	return func(o *Options) {
		o.Expiration = d
	}
}

// Items initializes the cache with preconfigured items.
func Items(i map[string]Item) Option {
	return func(o *Options) {
		o.Items = i
	}
}

// WithLogger sets underline logger.
func WithLogger(l logger.Logger) Option {
	return func(o *Options) {
		o.Logger = l
	}
}

// NewOptions returns a new options struct.
func NewOptions(opts ...Option) Options {
	options := Options{
		Expiration: DefaultExpiration,
		Items:      make(map[string]Item),
		Logger:     logger.Default(),
	}

	for _, o := range opts {
		o(&options)
	}

	return options
}
