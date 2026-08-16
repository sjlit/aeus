package otel

import (
	"context"
	"testing"

	"github.com/sjlit/aeus/infra/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTracer_Start(t *testing.T) {
	provider, shutdownProvider, err := NewProvider("test-service")
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	defer shutdownProvider()

	tr, shutdownTracer, err := NewTracer("test-service", provider)
	if err != nil {
		t.Fatalf("new tracer: %v", err)
	}
	defer shutdownTracer()

	ctx, span := tr.Start(context.Background(), "test-operation")
	if ctx == nil {
		t.Fatal("ctx is nil")
	}
	if span == nil {
		t.Fatal("span is nil")
	}
	span.End()
}

func TestTracer_InjectExtract(t *testing.T) {
	provider, shutdownProvider, err := NewProvider("test-service")
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	defer shutdownProvider()

	tr, shutdownTracer, err := NewTracer("test-service", provider)
	if err != nil {
		t.Fatalf("new tracer: %v", err)
	}
	defer shutdownTracer()

	ctx, span := tr.Start(context.Background(), "parent")
	defer span.End()

	carrier := &mapCarrier{data: make(map[string]string)}
	tr.Inject(ctx, carrier)

	if carrier.data["traceparent"] == "" {
		t.Fatal("traceparent not injected")
	}

	extractedCtx := tr.Extract(context.Background(), carrier)
	_, childSpan := tr.Start(extractedCtx, "child")
	childSpan.End()
}

func TestTracer_SpanKind(t *testing.T) {
	provider, shutdownProvider, err := NewProvider("test-service")
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	defer shutdownProvider()

	tr, shutdownTracer, err := NewTracer("test-service", provider)
	if err != nil {
		t.Fatalf("new tracer: %v", err)
	}
	defer shutdownTracer()

	_, span := tr.Start(context.Background(), "server-op", telemetry.WithSpanKind(telemetry.SpanKindServer))
	sc := span.SpanContext()
	if !sc.IsValid() {
		t.Fatal("span should be valid")
	}
	span.End()
}

func TestTracer_SpanContext(t *testing.T) {
	provider, shutdownProvider, err := NewProvider("test-service")
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	defer shutdownProvider()

	tr, shutdownTracer, err := NewTracer("test-service", provider)
	if err != nil {
		t.Fatalf("new tracer: %v", err)
	}
	defer shutdownTracer()

	_, span := tr.Start(context.Background(), "ctx-op")
	sc := span.SpanContext()
	if !sc.IsValid() {
		t.Fatal("span context should be valid")
	}
	if sc.TraceID() == "" {
		t.Fatal("trace_id should not be empty")
	}
	if sc.SpanID() == "" {
		t.Fatal("span_id should not be empty")
	}
	span.End()
}

func TestWithSampler(t *testing.T) {
	provider, shutdownProvider, err := NewProvider("test-svc", WithSampler(sdktrace.NeverSample()))
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	defer shutdownProvider()

	tr, shutdownTracer, err := NewTracer("test-svc", provider)
	if err != nil {
		t.Fatalf("new tracer: %v", err)
	}
	defer shutdownTracer()

	_, s := tr.Start(context.Background(), "should-not-sample")
	defer s.End()

	sc := s.SpanContext()
	if !sc.IsValid() {
		t.Fatal("span context should still be valid even when not sampled")
	}
	// Access the underlying OTel SpanContext to verify IsSampled == false.
	otelSpan := s.(*span).otelSpan
	if otelSpan.SpanContext().IsSampled() {
		t.Fatal("expected span to be not sampled with NeverSample")
	}
}

type mapCarrier struct {
	data map[string]string
}

func (m *mapCarrier) Get(key string) string { return m.data[key] }
func (m *mapCarrier) Set(key, value string) { m.data[key] = value }
