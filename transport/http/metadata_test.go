package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/middleware"
)

// TestRequestMetadata_NoRouteUsesRawPath pins the split between the
// matched route template and the raw request path. Static/NoRoute
// requests must expose the raw path to JWT allowlists while keeping
// RequestRoute empty so the permission checker treats them as
// infrastructure.
func TestRequestMetadata_NoRouteUsesRawPath(t *testing.T) {
	svr := New(WithAddress(":0"), WithDebug(true))
	var gotPath, gotRoute string
	svr.Use(func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context) error {
			md := metadata.FromContext(ctx)
			gotPath, _ = md.Get(metadata.RequestPath)
			gotRoute, _ = md.Get(metadata.RequestRoute)
			return next(ctx)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = svr.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	endpoint, err := svr.Endpoint(ctx)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	resp, err := http.Get(endpoint + "/missing-asset.js")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if gotPath != "/missing-asset.js" {
		t.Fatalf("RequestPath = %q, want /missing-asset.js", gotPath)
	}
	if gotRoute != "" {
		t.Fatalf("RequestRoute = %q, want empty for NoRoute", gotRoute)
	}
}
