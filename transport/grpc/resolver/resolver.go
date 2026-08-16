package resolver

import (
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/sjlit/aeus/registry"
	"google.golang.org/grpc/resolver"
)

type Resolver struct {
	conn      resolver.ClientConn
	watcher   registry.Watcher
	closeChan chan struct{}
	closeFlag int32
}

func (r *Resolver) update(items []*registry.Service) {
	addrs := make([]resolver.Address, 0, 10)
	for _, item := range items {
		for _, endpoint := range item.Endpoints {
			if !strings.HasPrefix(endpoint, "grpc://") {
				continue
			}
			if uri, err := url.Parse(endpoint); err == nil {
				addrs = append(addrs, resolver.Address{
					Addr: uri.Host,
				})
			}
		}
	}
	r.conn.UpdateState(resolver.State{Addresses: addrs})
}

func (r *Resolver) eventLoop() {
	for {
		select {
		case <-r.closeChan:
			return
		default:
		}
		if items, err := r.watcher.Next(); err == nil {
			r.update(items)
		}
	}
}

func (r *Resolver) ResolveNow(opts resolver.ResolveNowOptions) {

}

// Close closes the resolver.
func (r *Resolver) Close() {
	if !atomic.CompareAndSwapInt32(&r.closeFlag, 0, 1) {
		return
	}
	close(r.closeChan)
	r.watcher.Stop()
}

func newResolver(cc resolver.ClientConn, watcher registry.Watcher) *Resolver {
	r := &Resolver{
		conn:      cc,
		watcher:   watcher,
		closeChan: make(chan struct{}),
	}
	go r.eventLoop()
	return r
}
