package aeus

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/aeus/registry"
	"golang.org/x/sync/errgroup"
)

type Service struct {
	ctx       context.Context
	opts      *options
	errGroup  *errgroup.Group
	refValues []reflect.Value
	service   *registry.Service
	exitFlag  int32
}

func (s *Service) ID() string {
	return s.opts.id
}

func (s *Service) Name() string {
	return s.opts.name
}

func (s *Service) Debug() bool {
	if s.opts != nil {
		return s.opts.debug
	}
	return false
}

func (s *Service) Version() string {
	if s.opts != nil {
		return s.opts.version
	}
	return ""
}

func (s *Service) Metadata() map[string]string {
	if s.service == nil {
		return nil
	}
	return s.service.Metadata
}

func (s *Service) Endpoint() []string {
	if s.service == nil {
		return nil
	}
	return s.service.Endpoints
}

func (s *Service) Logger() logger.Logger {
	if s.opts.logger == nil {
		return logger.Default()
	}
	return s.opts.logger
}

func (s *Service) build(ctx context.Context) (*registry.Service, error) {
	svr := &registry.Service{
		ID:        s.ID(),
		Name:      s.Name(),
		Version:   s.Version(),
		Metadata:  s.opts.metadata,
		Endpoints: make([]string, 0, 4),
	}
	if svr.Metadata == nil {
		svr.Metadata = make(map[string]string)
	}
	svr.Metadata["os"] = runtime.GOOS
	svr.Metadata["go"] = runtime.Version()
	if hostname, herr := os.Hostname(); herr != nil {
		s.Logger().Warnf(ctx, "hostname lookup failed: %v", herr)
	} else {
		svr.Metadata["hostname"] = hostname
	}
	svr.Metadata["uptime"] = time.Now().Format(time.DateTime)
	if s.opts.endpoints != nil {
		svr.Endpoints = append(svr.Endpoints, s.opts.endpoints...)
	}
	for _, ptr := range s.opts.servers {
		if e, ok := ptr.(Endpointer); ok {
			uri, err := e.Endpoint(ctx)
			if err != nil {
				return nil, err
			}
			svr.Endpoints = append(svr.Endpoints, uri)
		}
	}
	return svr, nil
}

func (s *Service) injectVars(v any, used []bool) error {
	refValue := reflect.Indirect(reflect.ValueOf(v))
	refType := refValue.Type()
	for i := range refValue.NumField() {
		fieldValue := refValue.Field(i)
		if !fieldValue.CanSet() {
			continue
		}
		fieldType := refType.Field(i)
		if fieldType.Type.Kind() != reflect.Pointer && fieldType.Type.Kind() != reflect.Interface {
			continue
		}
		for idx, rv := range s.refValues {
			if fieldType.Type.Kind() == reflect.Interface && rv.Type().Implements(fieldType.Type) {
				refValue.Field(i).Set(rv)
				used[idx] = true
				break
			}
			if fieldType.Type == rv.Type() {
				refValue.Field(i).Set(rv)
				used[idx] = true
				break
			}
		}
	}
	return nil
}

func (s *Service) preStart(ctx context.Context) (err error) {
	if s.errGroup == nil {
		// errgroup.WithContext never returns a non-nil error and the
		// returned ctx is wired into the group internally — we only
		// need the *Group here. Run() will set up the real group
		// against the signal-aware ctx; this branch lets unit tests
		// call preStart() without going through Run().
		s.errGroup, _ = errgroup.WithContext(ctx)
	}
	s.Logger().Info(ctx, "starting")
	s.refValues = append(s.refValues, s.opts.injectVars...)
	injectVarsCount := len(s.opts.injectVars)
	s.refValues = append(s.refValues, reflect.ValueOf(s.Logger()))

	used := make([]bool, len(s.refValues))

	for _, ptr := range s.opts.servers {
		if err = s.injectVars(ptr, used); err != nil {
			return
		}
		// inject tracer and metrics into traceable servers
		if t, ok := ptr.(Traceable); ok {
			if s.opts.tracer != nil {
				t.SetTracer(s.opts.tracer)
			}
			if s.opts.metrics != nil {
				t.SetMetrics(s.opts.metrics)
			}
		}
		s.refValues = append(s.refValues, reflect.ValueOf(ptr))
		used = append(used, false)
	}
	if s.opts.registry != nil {
		s.refValues = append(s.refValues, reflect.ValueOf(s.opts.registry))
		used = append(used, false)
	}
	if s.opts.scope != nil {
		if err = s.injectVars(s.opts.scope, used); err != nil {
			return
		}
		if err = s.opts.scope.Init(ctx); err != nil {
			return
		}
		s.refValues = append(s.refValues, reflect.ValueOf(s.opts.scope))
		used = append(used, false)
	}
	if s.opts.serviceLoader != nil {
		if err = s.injectVars(s.opts.serviceLoader, used); err != nil {
			return
		}
		if err = s.opts.serviceLoader.Init(ctx); err != nil {
			return
		}
		s.refValues = append(s.refValues, reflect.ValueOf(s.opts.serviceLoader))
		used = append(used, false)
	}

	for i := range injectVarsCount {
		if !used[i] {
			return fmt.Errorf("unused inject var of type %s", s.opts.injectVars[i].Type())
		}
	}

	for _, srv := range s.opts.servers {
		svr := srv
		s.errGroup.Go(func() error {
			return svr.Start(ctx)
		})
	}
	if s.opts.serviceLoader != nil {
		s.errGroup.Go(func() error {
			return s.opts.serviceLoader.Run(ctx)
		})
	}
	if s.opts.registry != nil {
		s.errGroup.Go(func() error {
			childCtx, cancel := context.WithTimeout(ctx, s.opts.registrarTimeout)
			defer cancel()
			opts := func(o *registry.RegisterOptions) {
				o.Context = childCtx
			}
			if err = s.opts.registry.Register(s.service, opts); err != nil {
				return err
			}
			s.Logger().Info(ctx, "service registered")
			// Heartbeat cadence: cap at 1m, otherwise tick just before
			// the registration expires so a slow callback still renews
			// in time.
			duration := s.opts.registrarTimeout
			switch {
			case duration > time.Minute:
				duration = time.Minute
			case duration > time.Second*10:
				duration -= time.Second * 5
			}
			ticker := time.NewTicker(duration)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					s.Logger().Info(ctx, "service register stopped")
					return nil
				case <-ticker.C:
					if err = s.opts.registry.Register(s.service, func(o *registry.RegisterOptions) {
						o.Context = ctx
						o.TTL = s.opts.registrarTimeout
					}); err != nil {
						s.Logger().Warnf(ctx, "service register error: %v", err)
						if s.opts.metrics != nil {
							s.opts.metrics.RecordRegistryHeartbeat(ctx, "error")
						}
					} else {
						if s.opts.metrics != nil {
							s.opts.metrics.RecordRegistryHeartbeat(ctx, "success")
						}
					}
				}
			}
		})
	}
	s.Logger().Info(s.ctx, "started")
	return
}

func (s *Service) preStop() (err error) {
	if !atomic.CompareAndSwapInt32(&s.exitFlag, 0, 1) {
		return
	}
	s.Logger().Info(s.ctx, "stopping")
	ctx, cancelFunc := context.WithTimeoutCause(s.ctx, s.opts.stopTimeout, errs.ErrTimeout)
	defer func() {
		cancelFunc()
	}()
	for _, srv := range s.opts.servers {
		if err = srv.Stop(ctx); err != nil {
			s.Logger().Warnf(ctx, "server stop error: %v", err)
		}
	}
	if s.opts.registry != nil {
		if err = s.opts.registry.Deregister(s.service, func(o *registry.DeregisterOptions) {
			o.Context = ctx
		}); err != nil {
			s.Logger().Warnf(ctx, "server deregister error: %v", err)
		}
	}
	s.Logger().Info(ctx, "stopped")
	return
}

func (s *Service) Run() (err error) {
	var (
		ctx        context.Context
		errCtx     context.Context
		cancelFunc context.CancelFunc
	)
	s.ctx = WithAppContext(s.opts.ctx, s)
	if s.service, err = s.build(s.ctx); err != nil {
		return
	}
	signals := []os.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT}
	ctx, cancelFunc = signal.NotifyContext(s.ctx, signals...)
	defer cancelFunc()
	s.errGroup, errCtx = errgroup.WithContext(ctx)
	if err = s.preStart(errCtx); err != nil {
		return
	}
	s.errGroup.Go(func() error {
		// Unblock on an OS signal, the parent context being canceled, or
		// another errgroup goroutine returning an error (errCtx). The last
		// case matters: without it a failing server would leave this
		// goroutine blocked forever and Run would hang instead of returning
		// the error.
		select {
		case <-ctx.Done():
		case <-s.ctx.Done():
		case <-errCtx.Done():
		}
		return s.preStop()
	})
	err = s.errGroup.Wait()
	return
}

func New(cbs ...Option) *Service {
	s := &Service{
		refValues: make([]reflect.Value, 0, 20),
		opts: &options{
			id:               uuid.New().String(),
			ctx:              context.Background(),
			logger:           logger.Default(),
			stopTimeout:      time.Second * 10,
			registrarTimeout: time.Second * 30,
		},
	}
	// Unparseable AEUS_DEBUG falls back to false (the zero value).
	s.opts.debug, _ = strconv.ParseBool(os.Getenv("AEUS_DEBUG"))
	s.opts.metadata = make(map[string]string)
	for _, cb := range cbs {
		cb(s.opts)
	}
	return s
}
