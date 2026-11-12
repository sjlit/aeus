package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/sjlit/aeus/infra/telemetry"
)

type captureTraceTracer struct {
	mu          sync.Mutex
	traceparent string
}

func (t *captureTraceTracer) Start(ctx context.Context, name string, opts ...telemetry.SpanOption) (context.Context, telemetry.Span) {
	return telemetry.NoopTracer().Start(ctx, name, opts...)
}

func (t *captureTraceTracer) Extract(ctx context.Context, carrier telemetry.TextMapReader) context.Context {
	t.mu.Lock()
	t.traceparent = carrier.Get("traceparent")
	t.mu.Unlock()
	return ctx
}

func (t *captureTraceTracer) Inject(context.Context, telemetry.TextMapWriter) {}

func (t *captureTraceTracer) captured() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.traceparent
}

func TestRequestInterceptorExtractsInboundTraceparent(t *testing.T) {
	const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	tracer := &captureTraceTracer{}
	svr := New(WithAddress(":0"))
	svr.SetTracer(tracer)
	svr.GET("/ping", func(ctx *Context) error {
		return ctx.Success("pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("traceparent", traceparent)
	rec := httptest.NewRecorder()
	svr.Engine().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := tracer.captured(); got != traceparent {
		t.Fatalf("Extract traceparent = %q, want %q", got, traceparent)
	}
}
