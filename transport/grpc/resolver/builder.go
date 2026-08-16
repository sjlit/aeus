package resolver

import (
	"github.com/sjlit/aeus/registry"
	"google.golang.org/grpc/resolver"
)

type Builder struct {
	opts *options
}

func (b *Builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var (
		err     error
		watcher registry.Watcher
	)
	if watcher, err = b.opts.registry.Watch(func(o *registry.WatchOptions) {
		o.Context = b.opts.context
		o.Service = target.URL.Host
	}); err != nil {
		return nil, err
	}
	return newResolver(cc, watcher), nil
}

func (b *Builder) Scheme() string {
	return "service"
}

func New(cbs ...Option) resolver.Builder {
	return &Builder{
		opts: newOptions(cbs...),
	}
}
