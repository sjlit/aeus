package grpc

import (
	"context"
	"testing"

	"github.com/sjlit/aeus/middleware"
	"github.com/sjlit/aeus/pkg/errs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryServerInterceptor_ErrorMapping(t *testing.T) {
	svr := New(WithAddress(":0"))
	svr.Use(func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context) error {
			return errs.New(errs.CodeNotFound, "resource missing")
		}
	})

	interceptor := svr.unaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Test/Method"}
	_, err := interceptor(context.Background(), nil, info, func(ctx context.Context, req any) (any, error) {
		return nil, nil
	})

	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected grpc status error")
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("want code %v, got %v", codes.NotFound, st.Code())
	}
}

func TestUnaryServerInterceptor_AbortMapping(t *testing.T) {
	svr := New(WithAddress(":0"))
	svr.Use(func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context) error {
			return middleware.Abort(errs.New(errs.CodePermissionDenied, "no access"))
		}
	})

	interceptor := svr.unaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Test/Method"}
	_, err := interceptor(context.Background(), nil, info, func(ctx context.Context, req any) (any, error) {
		return nil, nil
	})

	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected grpc status error")
	}
	if st.Code() != codes.PermissionDenied {
		t.Fatalf("want code %v, got %v", codes.PermissionDenied, st.Code())
	}
}
