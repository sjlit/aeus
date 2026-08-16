package telemetry

import (
	"net/http"
	"testing"

	"github.com/sjlit/aeus/metadata"
)

func TestMetadataCarrier(t *testing.T) {
	md := metadata.New()
	carrier := MetadataCarrier{MD: md}

	carrier.Set("traceparent", "00-abc123-def456-01")
	if v := carrier.Get("traceparent"); v != "00-abc123-def456-01" {
		t.Fatalf("expected traceparent value, got %q", v)
	}
}

func TestHTTPHeaderCarrier(t *testing.T) {
	h := make(http.Header)
	carrier := HTTPHeaderCarrier{Header: h}
	carrier.Set("X-Trace-Id", "trace-123")
	if v := carrier.Get("X-Trace-Id"); v != "trace-123" {
		t.Fatalf("unexpected value %q", v)
	}
}
