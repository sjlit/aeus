package etcd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"path"
	"sync"

	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/aeus/registry"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdRegistry struct {
	prefix  string
	client  *clientv3.Client
	options *registry.RegistryOptions
	leaseID clientv3.LeaseID
	sync.RWMutex
}

func (e *etcdRegistry) prefixKey(name string) string {
	return path.Join(e.prefix, name) + "/"
}

func (e *etcdRegistry) buildKey(s *registry.Service) string {
	return path.Join(e.prefix, s.Name, s.ID)
}

func (e *etcdRegistry) marshalService(s *registry.Service) string {
	buf, err := json.Marshal(s)
	if err == nil {
		return string(buf)
	}
	return ""
}

func (e *etcdRegistry) unmarshalService(b []byte) (*registry.Service, error) {
	var service registry.Service
	err := json.Unmarshal(b, &service)
	if err == nil {
		return &service, nil
	}
	return nil, err
}

func (e *etcdRegistry) Name() string {
	return Name
}

func (e *etcdRegistry) Init(opts ...registry.RegistryOption) (err error) {
	for _, cb := range opts {
		cb(e.options)
	}
	cfg := clientv3.Config{}
	if e.options.Context != nil {
		clientOpts := FromContext(e.options.Context)
		if clientOpts != nil {
			cfg.Username = clientOpts.Username
			cfg.Password = clientOpts.Password
			cfg.DialTimeout = clientOpts.DialTimeout
			if clientOpts.Prefix != "" {
				e.prefix = clientOpts.Prefix
			}
		}
	}
	if e.options.TLSConfig != nil {
		cfg.TLS = e.options.TLSConfig
	} else if e.options.Secure {
		cfg.TLS = &tls.Config{
			InsecureSkipVerify: true, // TODO: 生产环境应配置有效 TLS 证书
		}
	}
	if e.prefix == "" {
		e.prefix = "/micro/service/"
	}
	cfg.Endpoints = append(cfg.Endpoints, e.options.Addrs...)
	e.client, err = clientv3.New(cfg)
	return err
}

func (e *etcdRegistry) Register(s *registry.Service, cbs ...registry.RegisterOption) (err error) {
	var lgr *clientv3.LeaseGrantResponse
	opts := registry.NewRegisterOption(cbs...)
	if opts.TTL.Seconds() > 0 {
		if e.leaseID > 0 {
			if _, err = e.client.KeepAliveOnce(opts.Context, e.leaseID); err == nil {
				return
			} else {
				if err != rpctypes.ErrLeaseNotFound {
					return err
				}
			}
		}
		lgr, err = e.client.Grant(opts.Context, int64(opts.TTL.Seconds()))
		if err != nil {
			return err
		}
	}
	if lgr != nil {
		if _, err = e.client.Put(opts.Context, e.buildKey(s), e.marshalService(s), clientv3.WithLease(lgr.ID)); err == nil {
			e.leaseID = lgr.ID
		}
	} else {
		_, err = e.client.Put(opts.Context, e.buildKey(s), e.marshalService(s))
	}
	if err != nil {
		return err
	}
	return nil
}

func (e *etcdRegistry) Deregister(s *registry.Service, cbs ...registry.DeregisterOption) error {
	opts := registry.NewDeregisterOption(cbs...)
	e.client.Delete(opts.Context, e.buildKey(s))
	return nil
}

func (e *etcdRegistry) GetService(name string, cbs ...registry.GetOption) ([]*registry.Service, error) {
	opts := registry.NewGetOption(cbs...)
	rsp, err := e.client.Get(opts.Context, e.prefixKey(name), clientv3.WithPrefix(), clientv3.WithSerializable())
	if err != nil {
		return nil, err
	}
	if len(rsp.Kvs) == 0 {
		return nil, errs.ErrNotFound
	}
	ss := make([]*registry.Service, 0, len(rsp.Kvs))
	for _, n := range rsp.Kvs {
		if s, err := e.unmarshalService(n.Value); err == nil {
			ss = append(ss, s)
		}
	}
	return ss, nil
}

func (e *etcdRegistry) Watch(opts ...registry.WatchOption) (registry.Watcher, error) {
	return newWatcher(e, opts...)
}

// Status reports whether the etcd cluster is reachable.
// It implements the optional registry.HealthChecker capability interface.
//
// Health semantics: all configured endpoints must be reachable for the
// cluster to be considered healthy (a downed node in a multi-node cluster
// therefore reports HealthUnavailable). Callers should pass a context with
// a deadline, as Status performs live network probes.
func (e *etcdRegistry) Status(ctx context.Context) (registry.HealthStatus, error) {
	if e.client == nil {
		return registry.HealthUnavailable, errs.New(errs.CodeUnavailable, "etcd registry not initialized")
	}
	for _, endpoint := range e.client.Endpoints() {
		if _, err := e.client.Status(ctx, endpoint); err != nil {
			return registry.HealthUnavailable, err
		}
	}
	return registry.HealthOK, nil
}

func New() registry.Registrar {
	e := &etcdRegistry{
		options: &registry.RegistryOptions{},
	}
	return e
}
