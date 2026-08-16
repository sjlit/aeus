package etcd

import (
	"context"
	"time"
)

const (
	Name = "etcd"
)

type (
	etcdContextKey struct{}
)

type (
	ClientOptions struct {
		Prefix      string
		Username    string
		Password    string
		DialTimeout time.Duration
	}
)

func FromContext(ctx context.Context) *ClientOptions {
	if v := ctx.Value(etcdContextKey{}); v != nil {
		if v, ok := v.(*ClientOptions); ok {
			return v
		}
	}
	return nil
}

func WithContext(ctx context.Context, opts *ClientOptions) context.Context {
	return context.WithValue(ctx, etcdContextKey{}, opts)
}
