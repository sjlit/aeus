package telemetry

import (
	"context"
	"time"
)

// MetricsRecorder is the abstract metrics interface used throughout AEUS.
type MetricsRecorder interface {
	RecordRequestDuration(ctx context.Context, protocol, path, status string, d time.Duration)
	RecordRequestTotal(ctx context.Context, protocol, path, status string)
	RecordRegistryHeartbeat(ctx context.Context, status string)
}

// NoopMetricsRecorder returns a recorder that does nothing.
func NoopMetricsRecorder() MetricsRecorder { return noopMetrics{} }

type noopMetrics struct{}

func (noopMetrics) RecordRequestDuration(context.Context, string, string, string, time.Duration) {}
func (noopMetrics) RecordRequestTotal(context.Context, string, string, string)                   {}
func (noopMetrics) RecordRegistryHeartbeat(context.Context, string)                              {}
