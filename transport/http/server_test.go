package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sjlit/aeus"
	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/middleware"
	"github.com/sjlit/aeus/pkg/errs"
)

func TestServer_AbortResponse(t *testing.T) {
	svr := New(
		WithAddress(":0"),
		WithDebug(true),
	)
	svr.Use(func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context) error {
			return middleware.Abort(errs.New(errs.CodePermissionDenied, "no access"))
		}
	})
	svr.GET("/admin", func(ctx *Context) error {
		return ctx.Success("secret")
	})

	ctxBg, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = svr.Start(ctxBg)
	}()
	time.Sleep(100 * time.Millisecond)

	endpoint, err := svr.Endpoint(ctxBg)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}

	resp, err := http.Get(endpoint + "/admin")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestServer_SetTracer(t *testing.T) {
	tracer := telemetry.NoopTracer()
	svr := New(WithAddress(":0"))
	svr.SetTracer(tracer)
	svr.SetMetrics(telemetry.NoopMetricsRecorder())

	if svr.tracer != tracer {
		t.Fatal("tracer not set on http server")
	}
}

func TestServer_ErrorCodeMapping(t *testing.T) {
	svr := New(
		WithAddress(":0"),
		WithDebug(true),
	)
	svr.GET("/not-found", func(ctx *Context) error {
		return errs.New(errs.CodeNotFound, "resource missing")
	})
	svr.GET("/bad-request", func(ctx *Context) error {
		return errs.New(errs.CodeInvalid, "invalid input")
	})
	svr.GET("/forbidden", func(ctx *Context) error {
		return errs.New(errs.CodePermissionDenied, "no access")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = svr.Start(ctx)
	}()
	time.Sleep(100 * time.Millisecond)

	endpoint, err := svr.Endpoint(ctx)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}

	cases := []struct {
		path       string
		wantStatus int
	}{
		{"/not-found", http.StatusNotFound},
		{"/bad-request", http.StatusBadRequest},
		{"/forbidden", http.StatusForbidden},
	}

	for _, c := range cases {
		resp, err := http.Get(endpoint + c.path)
		if err != nil {
			t.Fatalf("request %s: %v", c.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s: got status %d, want %d", c.path, resp.StatusCode, c.wantStatus)
		}
	}
}

func TestHTTPServer_Integration(t *testing.T) {
	tracer := telemetry.NoopTracer()
	recorder := telemetry.NoopMetricsRecorder()

	svr := New(
		WithAddress(":0"),
		WithDebug(true),
		WithEnableHealth(true),
		WithEnableMetrics(true),
	)
	svr.GET("/ping", func(ctx *Context) error {
		return ctx.Success("pong")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := aeus.New(
		aeus.WithName("test-svc"),
		aeus.WithTracer(tracer),
		aeus.WithMetrics(recorder),
		aeus.WithServer(svr),
		aeus.WithStopTimeout(time.Second),
		aeus.WithContext(ctx),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run()
	}()

	time.Sleep(200 * time.Millisecond)

	endpoint, err := svr.Endpoint(ctx)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}

	resp, err := http.Get(endpoint + "/ping")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	resp2, err := http.Get(endpoint + "/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("health unexpected status: %d", resp2.StatusCode)
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("service did not stop in time")
	}
}
