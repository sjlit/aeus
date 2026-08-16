package http

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestHealthEndpoints(t *testing.T) {
	svr := New(WithAddress(":0"), WithEnableHealth(true), WithEnableMetrics(true))
	go func() {
		_ = svr.Start(context.Background())
	}()
	time.Sleep(100 * time.Millisecond)

	endpoint, _ := svr.Endpoint(context.Background())

	resp, err := http.Get(endpoint + "/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp2, err := http.Get(endpoint + "/metrics")
	if err != nil {
		t.Fatalf("metrics request: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("metrics status: %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	_ = svr.Stop(context.Background())
}
