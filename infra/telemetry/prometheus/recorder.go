package prometheus

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sjlit/aeus/infra/telemetry"
)

type recorder struct {
	requestDuration *prometheus.HistogramVec
	requestTotal    *prometheus.CounterVec
	heartbeatTotal  *prometheus.CounterVec
}

// NewRecorder creates a Prometheus-backed MetricsRecorder.
// If registry is nil, prometheus.DefaultRegisterer is used.
//
// WARNING: The "path" label uses the route path as-is. For REST APIs with
// path parameters (e.g. /api/users/123), this creates unbounded cardinality.
// HTTP server integration uses ginCtx.FullPath() to normalize to route templates
// like /api/users/:id. Non-HTTP transports should sanitize paths before recording.
func NewRecorder(registry prometheus.Registerer) telemetry.MetricsRecorder {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}
	r := &recorder{
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aeus_request_duration_seconds",
			Help:    "Request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"protocol", "path", "status"}),
		requestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aeus_request_total",
			Help: "Total number of requests.",
		}, []string{"protocol", "path", "status"}),
		heartbeatTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aeus_registry_heartbeat_total",
			Help: "Total number of registry heartbeats.",
		}, []string{"status"}),
	}
	registry.MustRegister(
		r.requestDuration,
		r.requestTotal,
		r.heartbeatTotal,
	)
	return r
}

func (r *recorder) RecordRequestDuration(_ context.Context, protocol, path, status string, d time.Duration) {
	r.requestDuration.WithLabelValues(protocol, path, status).Observe(d.Seconds())
}

func (r *recorder) RecordRequestTotal(_ context.Context, protocol, path, status string) {
	r.requestTotal.WithLabelValues(protocol, path, status).Inc()
}

func (r *recorder) RecordRegistryHeartbeat(_ context.Context, status string) {
	r.heartbeatTotal.WithLabelValues(status).Inc()
}
