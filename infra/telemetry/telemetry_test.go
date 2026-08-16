package telemetry

import (
	"context"
	"testing"
)

func TestNoopTracer_Start(t *testing.T) {
	tr := NoopTracer()
	ctx, span := tr.Start(context.Background(), "test-op")
	if ctx == nil {
		t.Fatal("ctx must not be nil")
	}
	if span == nil {
		t.Fatal("span must not be nil")
	}
	span.End()
}

func TestNoopSpan_SetError(t *testing.T) {
	span := noopSpan{}
	span.SetError(nil)
	span.SetAttributes()
	span.End()
}

func TestNoopTracer_Extract(t *testing.T) {
	tr := NoopTracer()
	ctx := context.Background()
	extracted := tr.Extract(ctx, nil)
	if extracted != ctx {
		t.Fatal("noop Extract should return the same context")
	}
}

func TestNoopTracer_Inject(t *testing.T) {
	tr := NoopTracer()
	// Should not panic even with nil carrier
	tr.Inject(context.Background(), nil)
}

func TestAttributeConstructors(t *testing.T) {
	a1 := String("key", "val")
	if a1.Key != "key" || a1.Value != "val" {
		t.Fatalf("unexpected attribute: %+v", a1)
	}
	a2 := Int("num", 42)
	if a2.Key != "num" || a2.Value != 42 {
		t.Fatalf("unexpected attribute: %+v", a2)
	}
}

func TestWithSpanKind(t *testing.T) {
	cfg := &SpanConfig{}
	WithSpanKind(SpanKindServer)(cfg)
	if cfg.Kind != SpanKindServer {
		t.Fatalf("expected SpanKindServer, got %d", cfg.Kind)
	}
}

func TestNoopSpanContext(t *testing.T) {
	var s Span = noopSpan{}
	sc := s.SpanContext()
	if sc == nil {
		t.Fatal("SpanContext must not be nil")
	}
	if sc.IsValid() {
		t.Fatal("noop span context should not be valid")
	}
	if sc.TraceID() != "" || sc.SpanID() != "" {
		t.Fatal("noop span context ids should be empty")
	}
}
