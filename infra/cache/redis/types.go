package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type (
	options struct {
		context context.Context
		client  *redis.Client
		prefix  string
	}

	Option func(*options)
)

func WithClient(client *redis.Client) Option {
	return func(o *options) {
		o.client = client
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.context = ctx
	}
}

func WithPrefix(prefix string) Option {
	return func(o *options) {
		o.prefix = prefix
	}
}

// NewOptions returns a new options struct.
func newOptions(opts ...Option) *options {
	options := &options{
		prefix: "cache:",
	}
	for _, o := range opts {
		o(options)
	}
	return options
}
