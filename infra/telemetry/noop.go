package telemetry

import "context"

// NoopTracer returns a tracer that does nothing. Used as default.
func NoopTracer() Tracer { return noopTracer{} }

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, _ string, _ ...SpanOption) (context.Context, Span) {
	return ctx, noopSpan{}
}

func (noopTracer) Extract(ctx context.Context, _ TextMapReader) context.Context { return ctx }
func (noopTracer) Inject(_ context.Context, _ TextMapWriter)                    {}

type noopSpan struct{}

func (noopSpan) End()                       {}
func (noopSpan) SetError(error)             {}
func (noopSpan) SetAttributes(...Attribute) {}
func (noopSpan) SpanContext() SpanContext   { return noopSpanContext{} }

type noopSpanContext struct{}

func (noopSpanContext) TraceID() string { return "" }
func (noopSpanContext) SpanID() string  { return "" }
func (noopSpanContext) IsValid() bool   { return false }
