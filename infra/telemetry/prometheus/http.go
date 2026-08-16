package prometheus

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler returns an http.Handler that exposes Prometheus metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
