package otel

import sdktrace "go.opentelemetry.io/otel/sdk/trace"

// Option configures the OpenTelemetry tracer provider.
type Option func(*config)

type config struct {
	endpoint string
	sampler  sdktrace.Sampler
}

func WithEndpoint(endpoint string) Option {
	return func(c *config) {
		c.endpoint = endpoint
	}
}

// WithSampler sets the sampler for the TracerProvider.
// If not provided, the provider defaults to AlwaysSample.
func WithSampler(sampler sdktrace.Sampler) Option {
	return func(c *config) {
		c.sampler = sampler
	}
}
