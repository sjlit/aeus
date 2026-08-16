package etcd

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/sjlit/aeus/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdWatcher struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	watchChan  clientv3.WatchChan
	watcher    clientv3.Watcher
	registry   *etcdRegistry
	opts       *registry.WatchOptions
	setupFlag  int32
}

func newWatcher(r *etcdRegistry, cbs ...registry.WatchOption) (watcher registry.Watcher, err error) {
	w := &etcdWatcher{
		registry: r,
	}
	w.opts = registry.NewWatchOption(cbs...)
	w.ctx, w.cancelFunc = context.WithCancel(w.opts.Context)
	w.watcher = clientv3.NewWatcher(r.client)
	w.watchChan = w.watcher.Watch(w.ctx, r.prefixKey(w.opts.Service), clientv3.WithPrefix(), clientv3.WithRev(0), clientv3.WithKeysOnly())
	if err = w.watcher.RequestProgress(w.ctx); err != nil {
		w.opts.Logger.Warn(w.ctx, "etcd request progress error: %v", err)
	}
	return w, err
}

func (w *etcdWatcher) getServices() ([]*registry.Service, error) {
	return w.registry.GetService(w.opts.Service)
}

func (w *etcdWatcher) Next() ([]*registry.Service, error) {
	if atomic.CompareAndSwapInt32(&w.setupFlag, 0, 1) {
		return w.getServices()
	}
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case res, ok := <-w.watchChan:
		if ok {
			if res.Err() == nil {
				return w.getServices()
			} else {
				w.opts.Logger.Warn(w.ctx, "etcd watcher read error: %v", res.Err())
				return nil, res.Err()
			}
		}
	}
	return nil, io.ErrClosedPipe
}

func (w *etcdWatcher) Stop() (err error) {
	if err = w.watcher.Close(); err != nil {
		w.opts.Logger.Warn(w.ctx, "etcd watcher close error: %v", err)
	}
	w.cancelFunc()
	return err
}
