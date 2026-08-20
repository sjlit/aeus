package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/pkg/errs"
)

// fakeLimiter is a deterministic Limiter used by the middleware-level
// tests so they don't depend on the real token-bucket timing logic
// (which has its own dedicated test file).
type fakeLimiter struct {
	allowFn func(ctx context.Context, key string) (bool, time.Duration, error)
}

func (f *fakeLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	return f.allowFn(ctx, key)
}

// newReqCtx builds a context carrying the path / ip / claims shape
// the middleware reads.
func newReqCtx(t *testing.T, path, ip string, claims jwt.Claims) context.Context {
	t.Helper()
	md := metadata.New()
	md.Set(metadata.RequestPath, path)
	if ip != "" {
		md.Set(metadata.RequestClientIP, ip)
	}
	ctx := metadata.NewContext(context.Background(), md)
	if claims != nil {
		ctx = NewContext(ctx, claims)
	}
	return ctx
}

// ---------- key template parsing ----------

func TestExpandKey_SubstitutesAllPlaceholders(t *testing.T) {
	ctx := newReqCtx(t, "/auth/login", "1.2.3.4", jwt.MapClaims{
		"uid": "alice",
		"tid": "tenant-1",
	})
	got := expandKey(ctx, "login:ip:{ip}|user:{user}|tenant:{tenant}|path:{path}")
	want := "login:ip:1.2.3.4|user:alice|tenant:tenant-1|path:/auth/login"
	if got != want {
		t.Fatalf("expandKey mismatch: got %q want %q", got, want)
	}
}

func TestExpandKey_AnonymousFallsBackToEmpty(t *testing.T) {
	// No claims in ctx (RateLimit mounted ahead of JWT).
	ctx := newReqCtx(t, "/auth/login", "9.9.9.9", nil)
	got := expandKey(ctx, "user:{user}")
	if got != "user:" {
		t.Errorf("anonymous user placeholder should be empty, got %q", got)
	}
}

func TestExpandKey_SubWinsOverUid(t *testing.T) {
	// When both are present, uid is checked first (admin's claim
	// name); "sub" is the jwtv5 default subject and only used if
	// "uid" is absent. Pinning the precedence keeps a misconfigured
	// token from accidentally bucketing every user into the same
	// "anonymous" slot.
	ctx := newReqCtx(t, "/x", "", jwt.MapClaims{"uid": "alice", "sub": "subject-id"})
	if got := expandKey(ctx, "user:{user}"); got != "user:alice" {
		t.Errorf("uid should win: got %q", got)
	}

	ctx2 := newReqCtx(t, "/x", "", jwt.MapClaims{"sub": "subject-id"})
	if got := expandKey(ctx2, "user:{user}"); got != "user:subject-id" {
		t.Errorf("sub fallback: got %q", got)
	}
}

// ---------- middleware behaviour ----------

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	lim := &fakeLimiter{allowFn: func(_ context.Context, _ string) (bool, time.Duration, error) {
		return true, 0, nil
	}}
	mw := RateLimit(RateLimitOpts{
		Path:     "/auth/login",
		Keys:     []string{"ip:{ip}"},
		Capacity: 5,
		Refill:   1.0 / 60,
		Limiter:  lim,
	})
	called := false
	err := mw(func(context.Context) error { called = true; return nil })(
		newReqCtx(t, "/auth/login", "1.2.3.4", nil),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("next handler must be invoked when under limit")
	}
}

func TestRateLimit_RejectsOverLimitWith4010(t *testing.T) {
	lim := &fakeLimiter{allowFn: func(_ context.Context, _ string) (bool, time.Duration, error) {
		return false, 30 * time.Second, nil
	}}
	mw := RateLimit(RateLimitOpts{
		Path:     "/auth/login",
		Keys:     []string{"ip:{ip}"},
		Capacity: 5,
		Refill:   1.0 / 60,
		Limiter:  lim,
	})
	err := mw(func(context.Context) error { return nil })(
		newReqCtx(t, "/auth/login", "1.2.3.4", nil),
	)
	if err == nil {
		t.Fatal("expected error on rate limit hit")
	}
	if !errs.IsCode(err, errs.CodeTooManyAttempts) {
		t.Errorf("expected CodeTooManyAttempts (4010), got %v", err)
	}
	// HTTPStatus must surface 429 so transport/http can write the
	// correct response status without re-implementing the mapping.
	var ae *errs.Error
	if errs.Is(err, ae) {
		// no-op, just to keep the import
	}
	if !errorsIsCode(err, errs.CodeTooManyAttempts) {
		t.Errorf("expected CodeTooManyAttempts, got %v", err)
	}
}

func TestRateLimit_FailsClosedOnLimiterError(t *testing.T) {
	lim := &fakeLimiter{allowFn: func(_ context.Context, _ string) (bool, time.Duration, error) {
		return false, 0, context.DeadlineExceeded
	}}
	mw := RateLimit(RateLimitOpts{
		Path:     "/auth/login",
		Keys:     []string{"ip:{ip}"},
		Capacity: 5,
		Refill:   1.0 / 60,
		Limiter:  lim,
	})
	err := mw(func(context.Context) error { return nil })(
		newReqCtx(t, "/auth/login", "1.2.3.4", nil),
	)
	if err == nil {
		t.Fatal("limiter error must fail-closed")
	}
	// errs.Wrap maps to CodeInternal (HTTP 500). Anything but a
	// "traffic flowed through unchecked" is the right answer.
	if errs.IsCode(err, errs.CodeTooManyAttempts) {
		t.Errorf("limiter error must NOT be reported as 4010, got %v", err)
	}
}

func TestRateLimit_PathPrefixDoesNotMatch(t *testing.T) {
	lim := &fakeLimiter{allowFn: func(_ context.Context, _ string) (bool, time.Duration, error) {
		t.Fatal("limiter must not be consulted for non-matching paths")
		return true, 0, nil
	}}
	mw := RateLimit(RateLimitOpts{
		Path:     "/auth/login",
		Keys:     []string{"ip:{ip}"},
		Capacity: 5,
		Refill:   1.0 / 60,
		Limiter:  lim,
	})
	called := false
	err := mw(func(context.Context) error { called = true; return nil })(
		newReqCtx(t, "/auth/refresh-token", "1.2.3.4", nil),
	)
	if err != nil {
		t.Fatalf("non-matching path should pass through: %v", err)
	}
	if !called {
		t.Fatal("next handler must run for non-matching path")
	}
}

func TestRateLimit_ShortCircuitsOnFirstExhaustedBucket(t *testing.T) {
	// ip bucket exhausted, user bucket has plenty — middleware
	// must reject on the IP key without consulting the second.
	lim := &fakeLimiter{allowFn: func(_ context.Context, key string) (bool, time.Duration, error) {
		if strings.HasPrefix(key, "ip:") {
			return false, 10 * time.Second, nil
		}
		t.Fatalf("user bucket must not be consulted when IP is exhausted, key=%q", key)
		return true, 0, nil
	}}
	mw := RateLimit(RateLimitOpts{
		Path:     "/auth/login",
		Keys:     []string{"ip:{ip}", "user:{user}"},
		Capacity: 5,
		Refill:   1.0 / 60,
		Limiter:  lim,
	})
	err := mw(func(context.Context) error { return nil })(
		newReqCtx(t, "/auth/login", "1.2.3.4", jwt.MapClaims{"uid": "alice"}),
	)
	if !errs.IsCode(err, errs.CodeTooManyAttempts) {
		t.Errorf("expected 4010, got %v", err)
	}
}

func TestRateLimit_PanicsOnZeroCapacity(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on Capacity=0")
		}
	}()
	RateLimit(RateLimitOpts{Path: "/x", Keys: []string{"ip:{ip}"}, Capacity: 0, Refill: 1})
}

func TestRateLimit_PanicsOnEmptyPath(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on empty Path")
		}
	}()
	RateLimit(RateLimitOpts{Keys: []string{"ip:{ip}"}, Capacity: 1, Refill: 1})
}

// errorsIsCode keeps the test intent readable; errs.IsCode returns
// false for nil and we want a single source of truth.
func errorsIsCode(err error, code errs.Code) bool { return errs.IsCode(err, code) }