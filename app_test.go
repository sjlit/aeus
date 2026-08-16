package aeus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/aeus/infra/telemetry"
)

// mockServer implements Server and Traceable for testing.
type mockServer struct {
	started bool
	stopped bool
	tracer  telemetry.Tracer
	metrics telemetry.MetricsRecorder
}

func (m *mockServer) Start(ctx context.Context) error        { m.started = true; return nil }
func (m *mockServer) Stop(ctx context.Context) error         { m.stopped = true; return nil }
func (m *mockServer) SetTracer(t telemetry.Tracer)           { m.tracer = t }
func (m *mockServer) SetMetrics(r telemetry.MetricsRecorder) { m.metrics = r }

// mockScope for injection testing.
type mockScope struct {
	Logger logger.Logger
}

func (m *mockScope) Init(ctx context.Context) error { return nil }

func TestService_UnusedInjectVar(t *testing.T) {
	// Create an inject var that nothing accepts.
	unused := "I am not injectable"

	svc := New(
		WithName("test-di"),
		WithServer(&mockServer{}),
		WithScope(&mockScope{}),
		WithInjectVars(unused),
		WithStopTimeout(time.Second),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := svc.preStart(ctx)
	if err == nil {
		t.Fatal("expected error for unused inject var")
	}
}

func TestService_InjectLoggerSuccess(t *testing.T) {
	scope := &mockScope{}

	svc := New(
		WithName("test-di-ok"),
		WithServer(&mockServer{}),
		WithScope(scope),
		WithStopTimeout(time.Second),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := svc.preStart(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.Logger == nil {
		t.Fatal("expected Logger to be injected into scope")
	}
}

func TestService_WithTracerInjection(t *testing.T) {
	tracer := telemetry.NoopTracer()
	recorder := telemetry.NoopMetricsRecorder()
	ms := &mockServer{}

	svc := New(
		WithName("test-svc"),
		WithTracer(tracer),
		WithMetrics(recorder),
		WithServer(ms),
		WithStopTimeout(time.Second),
	)

	if svc.opts.tracer != tracer {
		t.Fatal("tracer not set in options")
	}
	if svc.opts.metrics != recorder {
		t.Fatal("metrics not set in options")
	}
}

// failingServer returns a fixed error from Start so tests can exercise the
// error path of Run: graceful shutdown must still run and Run must return
// instead of hanging.
type failingServer struct {
	startErr error
	stopped  bool
}

func (f *failingServer) Start(ctx context.Context) error { return f.startErr }
func (f *failingServer) Stop(ctx context.Context) error  { f.stopped = true; return nil }

func TestService_RunReturnsOnServerError(t *testing.T) {
	f := &failingServer{startErr: errors.New("boom")}
	svc := New(
		WithName("test-run-error"),
		WithServer(f),
		WithStopTimeout(time.Second),
	)

	done := make(chan error, 1)
	go func() { done <- svc.Run() }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected Run to return the server error")
		}
		if !f.stopped {
			t.Fatal("expected Stop to be called on the server error path")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run hung: did not return after a server start error")
	}
}
