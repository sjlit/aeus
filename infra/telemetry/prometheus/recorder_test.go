package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRecorder_RecordRequestDuration(t *testing.T) {
	r := NewRecorder(prometheus.NewRegistry())
	ctx := context.Background()
	r.RecordRequestDuration(ctx, "http", "/api/test", "200", 10*time.Millisecond)
	// Prometheus client does not return errors on Observe; just verify no panic.
}

func TestRecorder_RecordRequestTotal(t *testing.T) {
	r := NewRecorder(prometheus.NewRegistry())
	ctx := context.Background()
	r.RecordRequestTotal(ctx, "http", "/api/test", "200")
}

func TestRecorder_RecordRegistryHeartbeat(t *testing.T) {
	r := NewRecorder(prometheus.NewRegistry())
	ctx := context.Background()
	r.RecordRegistryHeartbeat(ctx, "success")
}

func TestNewRecorder_WithRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	r := NewRecorder(reg)
	ctx := context.Background()
	r.RecordRequestDuration(ctx, "http", "/api/test", "200", 10*time.Millisecond)
	r.RecordRequestTotal(ctx, "http", "/api/test", "200")
	r.RecordRegistryHeartbeat(ctx, "success")

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(families) == 0 {
		t.Fatal("expected metrics to be registered in custom registry")
	}
}

func TestNewRecorder_NilRegistry(t *testing.T) {
	// Nil registry falls back to DefaultRegisterer.
	// This test primarily ensures no panic; do not assert on DefaultRegisterer state.
	r := NewRecorder(nil)
	ctx := context.Background()
	r.RecordRequestDuration(ctx, "http", "/api/test", "200", 10*time.Millisecond)
}
