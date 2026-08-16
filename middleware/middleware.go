package middleware

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sjlit/aeus/infra/telemetry"
	"github.com/sjlit/aeus/metadata"
)

type Handler func(ctx context.Context) error

type Middleware func(Handler) Handler

func Chain(m ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(m) - 1; i >= 0; i-- {
			next = m[i](next)
		}
		return next
	}
}

// RequestMetrics returns a middleware that records request metrics.
//
// NOTE: HTTP and gRPC servers already record metrics automatically in their
// interceptors. Only use this middleware for CLI transports or custom handler
// chains where automatic metric recording is not enabled.
func RequestMetrics(recorder telemetry.MetricsRecorder) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context) error {
			if recorder == nil {
				return next(ctx)
			}
			start := time.Now()
			md := metadata.FromContext(ctx)
			protocol, _ := md.Get(metadata.RequestProtocol)
			path, _ := md.Get(metadata.RequestPath)

			err := next(ctx)

			status := "success"
			if err != nil {
				status = "error"
			}
			recorder.RecordRequestDuration(ctx, protocol, path, status, time.Since(start))
			recorder.RecordRequestTotal(ctx, protocol, path, status)
			return err
		}
	}
}

var ErrAbort = errors.New("middleware abort")

// Abort wraps an error to indicate intentional chain short-circuit.
func Abort(err error) error {
	return fmt.Errorf("%w: %w", ErrAbort, err)
}

// IsAbort reports whether err was created by Abort.
func IsAbort(err error) bool {
	return errors.Is(err, ErrAbort)
}
