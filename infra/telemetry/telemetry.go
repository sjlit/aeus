// Package telemetry provides abstract tracing interfaces for AEUS.
// It defines Tracer, Span, and carrier types that are implemented
// by backends such as OpenTelemetry.
package telemetry

import "context"

// SpanKind represents the role of a span in a trace.
type SpanKind int

const (
	SpanKindUnspecified SpanKind = iota
	SpanKindInternal
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// SpanContext contains identifying trace information about a span.
type SpanContext interface {
	TraceID() string
	SpanID() string
	IsValid() bool
}

// Tracer is the abstract tracing interface used throughout AEUS.
type Tracer interface {
	// Start creates a new span. Returns the updated context and the span.
	Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)

	// Extract reads trace context from a carrier into the returned context.
	Extract(ctx context.Context, carrier TextMapReader) context.Context

	// Inject writes trace context from ctx into a carrier.
	Inject(ctx context.Context, carrier TextMapWriter)
}

// Span represents a single operation within a trace.
type Span interface {
	End()
	SetError(err error)
	SetAttributes(attrs ...Attribute)
	SpanContext() SpanContext
}

// SpanConfig holds configuration for span creation.
type SpanConfig struct {
	Kind SpanKind
}

// SpanOption allows configuring a span at creation time.
type SpanOption func(*SpanConfig)

// WithSpanKind sets the SpanKind for a span.
func WithSpanKind(k SpanKind) SpanOption {
	return func(c *SpanConfig) {
		c.Kind = k
	}
}

// Attribute is a key-value pair attached to a span.
type Attribute struct {
	Key   string
	Value any
}

func String(k, v string) Attribute  { return Attribute{Key: k, Value: v} }
func Int(k string, v int) Attribute { return Attribute{Key: k, Value: v} }

// TextMapReader reads key-value pairs from a carrier.
type TextMapReader interface {
	Get(key string) string
}

// TextMapWriter writes key-value pairs into a carrier.
type TextMapWriter interface {
	Set(key, value string)
}
