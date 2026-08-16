package aeus

import (
	"context"

	"github.com/sjlit/aeus/infra/telemetry"
)

type (
	applicationKey struct{}

	// application context value.
	Application interface {
		ID() string
		Name() string
		Version() string
		Debug() bool
		Metadata() map[string]string
		Endpoint() []string
	}

	// application scope interface.
	Scope interface {
		Init(ctx context.Context) (err error)
	}

	// service loader interface.
	ServiceLoader interface {
		Init(ctx context.Context) (err error)
		Run(ctx context.Context) (err error)
	}

	// endpoint interface.
	Endpointer interface {
		Endpoint(ctx context.Context) (string, error)
	}

	// option callback
	Option func(*options)

	// server interface.
	Server interface {
		Start(ctx context.Context) (err error)
		Stop(ctx context.Context) (err error)
	}

	// Traceable is a capability interface that allows telemetry injection.
	Traceable interface {
		SetTracer(tracer telemetry.Tracer)
		SetMetrics(recorder telemetry.MetricsRecorder)
	}
)

func FromContext(ctx context.Context) Application {
	app, ok := ctx.Value(applicationKey{}).(Application)
	if !ok {
		return nil
	}
	return app
}
