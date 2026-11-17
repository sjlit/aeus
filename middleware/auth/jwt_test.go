package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/pkg/errs"
)

func TestIsAllowed_ExactMatch(t *testing.T) {
	if !isAllowed("/api/user", []string{"/api/user"}) {
		t.Error("exact match should be allowed")
	}
}

func TestIsAllowed_Wildcard(t *testing.T) {
	if !isAllowed("/anything", []string{"*"}) {
		t.Error("wildcard should allow everything")
	}
}

func TestIsAllowed_PrefixMatch(t *testing.T) {
	if !isAllowed("/api/user/list", []string{"/api/user/*"}) {
		t.Error("prefix match should be allowed")
	}
}

func TestIsAllowed_PrefixNoMatch(t *testing.T) {
	if isAllowed("/api/order", []string{"/api/user/*"}) {
		t.Error("non-matching prefix should not be allowed")
	}
}

func TestIsAllowed_EmptyAllows(t *testing.T) {
	if isAllowed("/api/user", []string{}) {
		t.Error("empty allows should deny everything")
	}
}

func TestIsAllowed_EmptyPath(t *testing.T) {
	if isAllowed("", []string{"/api/user"}) {
		t.Error("empty path should not match")
	}
}

func TestIsAllowed_MultiplePatterns(t *testing.T) {
	if !isAllowed("/health", []string{"/api/*", "/health"}) {
		t.Error("second pattern should match")
	}
}

func TestIsAllowed_SingleCharPrefix(t *testing.T) {
	if !isAllowed("/", []string{"/*"}) {
		t.Error("pattern /* should match /")
	}
}

func mockKeyfunc(*jwt.Token) (interface{}, error) {
	return []byte("secret"), nil
}

func mockKeyfuncFail(*jwt.Token) (interface{}, error) {
	return nil, errs.New(-1, "bad key")
}

func newTestContextWithPath(path string) context.Context {
	md := metadata.New()
	md.Set(metadata.RequestPath, path)
	return metadata.NewContext(context.Background(), md)
}

func TestJWT_AllowPathSkip(t *testing.T) {
	ctx := newTestContextWithPath("/health")
	called := false
	next := func(ctx context.Context) error {
		called = true
		return nil
	}
	m := JWT(mockKeyfunc, WithAllow("/health"))
	if err := m(next)(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next should have been called")
	}
}

func TestJWT_MissingToken(t *testing.T) {
	ctx := newTestContextWithPath("/api/user")
	m := JWT(mockKeyfunc)
	err := m(func(ctx context.Context) error { return nil })(ctx)
	if err != errs.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

func TestJWT_MalformedToken(t *testing.T) {
	ctx := newTestContextWithPath("/api/user")
	md := metadata.FromContext(ctx)
	md.Set(authorizationKey, "Basic abc")
	ctx = metadata.NewContext(context.Background(), md)
	m := JWT(mockKeyfunc)
	err := m(func(ctx context.Context) error { return nil })(ctx)
	if err != errs.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

func TestJWT_ParseMalformed(t *testing.T) {
	ctx := newTestContextWithPath("/api/user")
	md := metadata.FromContext(ctx)
	md.Set(authorizationKey, "Bearer bad.token.here")
	ctx = metadata.NewContext(context.Background(), md)
	m := JWT(mockKeyfunc)
	err := m(func(ctx context.Context) error { return nil })(ctx)
	if err != errs.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

func TestJWT_TokenExpired(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	})
	tokenStr, _ := token.SignedString([]byte("secret"))

	ctx := newTestContextWithPath("/api/user")
	md := metadata.FromContext(ctx)
	md.Set(authorizationKey, "Bearer "+tokenStr)
	ctx = metadata.NewContext(context.Background(), md)

	m := JWT(mockKeyfunc)
	err := m(func(ctx context.Context) error { return nil })(ctx)
	if err != errs.ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

// TestJWT_PermissionCheckerSeesClaims locks in the ordering contract:
// claims are stored in ctx before the permission checker runs, and remain
// there for the handler. This is what lets custom checkers query
// tenant-scoped models through the GORM tenant callbacks.
func TestJWT_PermissionCheckerSeesClaims(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user123",
	})
	tokenStr, _ := token.SignedString([]byte("secret"))

	ctx := newTestContextWithPath("/api/user")
	md := metadata.FromContext(ctx)
	md.Set(authorizationKey, "Bearer "+tokenStr)
	ctx = metadata.NewContext(context.Background(), md)

	checkerCalled := false
	nextCalled := false
	m := JWT(mockKeyfunc, WithPermissionChecker(func(checkerCtx context.Context, claims jwt.Claims) error {
		checkerCalled = true
		if _, ok := FromContext(checkerCtx); !ok {
			t.Error("claims should be in ctx when the permission checker runs")
		}
		if claims == nil {
			t.Error("claims argument should not be nil")
		}
		return nil
	}))
	err := m(func(nextCtx context.Context) error {
		nextCalled = true
		if _, ok := FromContext(nextCtx); !ok {
			t.Error("claims should remain in ctx for the handler")
		}
		return nil
	})(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !checkerCalled {
		t.Error("permission checker should have been called")
	}
	if !nextCalled {
		t.Error("next should have been called")
	}
}

func TestJWT_PermissionCheckerDeniesBeforeNext(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user123",
	})
	tokenStr, _ := token.SignedString([]byte("secret"))

	ctx := newTestContextWithPath("/api/user")
	md := metadata.FromContext(ctx)
	md.Set(authorizationKey, "Bearer "+tokenStr)
	ctx = metadata.NewContext(context.Background(), md)

	nextCalled := false
	m := JWT(mockKeyfunc, WithPermissionChecker(func(context.Context, jwt.Claims) error {
		return errs.ErrPermissionDenied
	}))
	err := m(func(context.Context) error {
		nextCalled = true
		return nil
	})(ctx)
	if !errors.Is(err, errs.ErrPermissionDenied) {
		t.Fatalf("error = %v, want ErrPermissionDenied", err)
	}
	if nextCalled {
		t.Error("next must not run after the permission checker denies")
	}
}

func TestJWT_SuccessWithClaims(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user123",
	})
	tokenStr, _ := token.SignedString([]byte("secret"))

	ctx := newTestContextWithPath("/api/user")
	md := metadata.FromContext(ctx)
	md.Set(authorizationKey, "Bearer "+tokenStr)
	ctx = metadata.NewContext(context.Background(), md)

	called := false
	m := JWT(mockKeyfunc)
	err := m(func(ctx context.Context) error {
		called = true
		claims, ok := FromContext(ctx)
		if !ok {
			t.Error("claims should be in context")
		}
		if claims == nil {
			t.Error("claims should not be nil")
		}
		return nil
	})(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next should have been called")
	}
}
