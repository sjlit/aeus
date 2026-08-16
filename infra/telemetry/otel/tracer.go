package otel

import (
	"context"
	"fmt"

	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/metadata"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type tracer struct {
	provider *sdktrace.TracerProvider
	tracer   oteltrace.Tracer
	prop     propagation.TextMapPropagator
}

type span struct {
	otelSpan oteltrace.Span
}

type otelSpanContext struct {
	sc oteltrace.SpanContext
}

func (c otelSpanContext) TraceID() string { return c.sc.TraceID().String() }
func (c otelSpanContext) SpanID() string  { return c.sc.SpanID().String() }
func (c otelSpanContext) IsValid() bool   { return c.sc.IsValid() }

// NewProvider creates an OpenTelemetry TracerProvider.
// The returned shutdown function should be called before application exit.
func NewProvider(serviceName string, opts ...Option) (*sdktrace.TracerProvider, func(), error) {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	res, err := resource.Merge(
		resource.Environment(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("resource merge: %w", err)
	}

	var exp sdktrace.SpanExporter
	if cfg.endpoint != "" {
		exp, err = otlptracegrpc.New(context.Background(), otlptracegrpc.WithEndpoint(cfg.endpoint), otlptracegrpc.WithInsecure())
		if err != nil {
			return nil, nil, fmt.Errorf("otlp exporter: %w", err)
		}
	}

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}
	if cfg.sampler != nil {
		tpOpts = append(tpOpts, sdktrace.WithSampler(cfg.sampler))
	} else {
		tpOpts = append(tpOpts, sdktrace.WithSampler(sdktrace.AlwaysSample()))
	}
	if exp != nil {
		tpOpts = append(tpOpts, sdktrace.WithBatcher(exp))
	}

	provider := sdktrace.NewTracerProvider(tpOpts...)

	shutdown := func() {
		_ = provider.Shutdown(context.Background())
	}
	return provider, shutdown, nil
}

// NewTracer creates a telemetry.Tracer backed by the given TracerProvider.
func NewTracer(serviceName string, provider *sdktrace.TracerProvider) (telemetry.Tracer, func(), error) {
	if provider == nil {
		return nil, nil, fmt.Errorf("provider is nil")
	}
	t := &tracer{
		provider: provider,
		tracer:   provider.Tracer(serviceName),
		prop:     propagation.TraceContext{},
	}
	return t, func() {}, nil
}

func (t *tracer) Start(ctx context.Context, name string, opts ...telemetry.SpanOption) (context.Context, telemetry.Span) {
	cfg := &telemetry.SpanConfig{}
	for _, o := range opts {
		o(cfg)
	}

	var otelOpts []oteltrace.SpanStartOption
	switch cfg.Kind {
	case telemetry.SpanKindInternal:
		otelOpts = append(otelOpts, oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	case telemetry.SpanKindServer:
		otelOpts = append(otelOpts, oteltrace.WithSpanKind(oteltrace.SpanKindServer))
	case telemetry.SpanKindClient:
		otelOpts = append(otelOpts, oteltrace.WithSpanKind(oteltrace.SpanKindClient))
	case telemetry.SpanKindProducer:
		otelOpts = append(otelOpts, oteltrace.WithSpanKind(oteltrace.SpanKindProducer))
	case telemetry.SpanKindConsumer:
		otelOpts = append(otelOpts, oteltrace.WithSpanKind(oteltrace.SpanKindConsumer))
	}

	ctx, s := t.tracer.Start(ctx, name, otelOpts...)
	sc := s.SpanContext()
	if sc.IsValid() {
		md := metadata.FromContext(ctx)
		md.Set("trace_id", sc.TraceID().String())
		md.Set("span_id", sc.SpanID().String())
		ctx = metadata.NewContext(ctx, md)
	}
	return ctx, &span{otelSpan: s}
}

func (t *tracer) Extract(ctx context.Context, carrier telemetry.TextMapReader) context.Context {
	return t.prop.Extract(ctx, textMapAdapter{carrier})
}

func (t *tracer) Inject(ctx context.Context, carrier telemetry.TextMapWriter) {
	t.prop.Inject(ctx, injectAdapter{carrier})
}

func (s *span) End() { s.otelSpan.End() }

func (s *span) SetError(err error) {
	if err != nil {
		s.otelSpan.RecordError(err)
	}
}

func (s *span) SetAttributes(attrs ...telemetry.Attribute) {
	var otelAttrs []attribute.KeyValue
	for _, a := range attrs {
		switch v := a.Value.(type) {
		case string:
			otelAttrs = append(otelAttrs, attribute.String(a.Key, v))
		case int:
			otelAttrs = append(otelAttrs, attribute.Int(a.Key, v))
		}
	}
	s.otelSpan.SetAttributes(otelAttrs...)
}

func (s *span) SpanContext() telemetry.SpanContext {
	sc := s.otelSpan.SpanContext()
	return otelSpanContext{sc: sc}
}

// textMapAdapter bridges telemetry.TextMapReader/Writer to propagation.TextMapCarrier.
type textMapAdapter struct {
	inner telemetry.TextMapReader
}

func (a textMapAdapter) Get(key string) string {
	if w, ok := a.inner.(interface{ Get(string) string }); ok {
		return w.Get(key)
	}
	return ""
}

func (a textMapAdapter) Set(key, value string) {
	if w, ok := a.inner.(interface{ Set(string, string) }); ok {
		w.Set(key, value)
	}
}

func (a textMapAdapter) Keys() []string {
	return nil // propagation.TraceContext only uses Get/Set
}

// injectAdapter bridges telemetry.TextMapWriter to propagation.TextMapCarrier.
type injectAdapter struct {
	inner telemetry.TextMapWriter
}

func (a injectAdapter) Get(key string) string { return "" }
func (a injectAdapter) Set(key, value string) { a.inner.Set(key, value) }
func (a injectAdapter) Keys() []string        { return nil }
