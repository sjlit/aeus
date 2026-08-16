package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/metadata"
)

type spyRecorder struct {
	protocol string
	path     string
	status   string
	duration time.Duration
	calls    int
}

func (s *spyRecorder) RecordRequestDuration(_ context.Context, protocol, path, status string, d time.Duration) {
	s.protocol = protocol
	s.path = path
	s.status = status
	s.duration = d
	s.calls++
}

func (s *spyRecorder) RecordRequestTotal(_ context.Context, protocol, path, status string) {
	s.protocol = protocol
	s.path = path
	s.status = status
	s.calls++
}

func (s *spyRecorder) RecordRegistryHeartbeat(_ context.Context, status string) {}

func TestRequestMetrics_Success(t *testing.T) {
	spy := &spyRecorder{}
	md := metadata.New()
	md.Set(metadata.RequestProtocol, "http")
	md.Set(metadata.RequestPath, "/test")
	ctx := metadata.NewContext(context.Background(), md)

	m := RequestMetrics(spy)
	handler := m(func(ctx context.Context) error {
		return nil
	})

	if err := handler(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spy.calls != 2 {
		t.Fatalf("expected 2 recorder calls, got %d", spy.calls)
	}
	if spy.status != "success" {
		t.Fatalf("expected status success, got %s", spy.status)
	}
	if spy.protocol != "http" {
		t.Fatalf("expected protocol http, got %s", spy.protocol)
	}
	if spy.path != "/test" {
		t.Fatalf("expected path /test, got %s", spy.path)
	}
}

func TestRequestMetrics_Error(t *testing.T) {
	spy := &spyRecorder{}
	md := metadata.New()
	md.Set(metadata.RequestProtocol, "grpc")
	md.Set(metadata.RequestPath, "/api/error")
	ctx := metadata.NewContext(context.Background(), md)

	m := RequestMetrics(spy)
	handler := m(func(ctx context.Context) error {
		return errors.New("boom")
	})

	if err := handler(ctx); err == nil {
		t.Fatal("expected error")
	}

	if spy.calls != 2 {
		t.Fatalf("expected 2 recorder calls, got %d", spy.calls)
	}
	if spy.status != "error" {
		t.Fatalf("expected status error, got %s", spy.status)
	}
}

func TestRequestMetrics_NilRecorder(t *testing.T) {
	md := metadata.New()
	ctx := metadata.NewContext(context.Background(), md)

	m := RequestMetrics(nil)
	handler := m(func(ctx context.Context) error {
		return nil
	})

	if err := handler(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestMetrics_NoOp(t *testing.T) {
	md := metadata.New()
	md.Set(metadata.RequestProtocol, "http")
	md.Set(metadata.RequestPath, "/test")
	ctx := metadata.NewContext(context.Background(), md)

	m := RequestMetrics(telemetry.NoopMetricsRecorder())
	handler := m(func(ctx context.Context) error {
		return nil
	})

	if err := handler(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAbort(t *testing.T) {
	original := errors.New("auth failed")
	aborted := Abort(original)

	if !IsAbort(aborted) {
		t.Error("IsAbort should be true for Abort-wrapped error")
	}
	if IsAbort(original) {
		t.Error("IsAbort should be false for plain error")
	}
}
