package auth_test

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin/auth"
	mwauth "github.com/sjlit/aeus/middleware/auth"
)

// TestClaims_ImplementsJWTClaims is a compile-time guard ensuring that
// *auth.Claims satisfies the jwt.Claims interface so that
// mwauth.WithClaims(Claims{}) works at middleware setup time.
func TestClaims_ImplementsJWTClaims(t *testing.T) {
	var _ jwt.Claims = (*auth.Claims)(nil)
}

// TestClaimsFromContext_RoundTripViaMiddleware demonstrates the documented
// usage: validation middleware stores claims via mwauth.NewContext, callers
// retrieve them through auth.ClaimsFromContext and read fields directly.
func TestClaimsFromContext_RoundTripViaMiddleware(t *testing.T) {
	original := &auth.Claims{Role: "admin", UID: "u0001", TenantID: "t1"}

	ctx := mwauth.NewContext(context.Background(), original)
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("auth.ClaimsFromContext returned ok=false after mwauth.NewContext")
	}
	if claims != original {
		t.Errorf("ClaimsFromContext returned %p, want %p", claims, original)
	}
	if claims.UID != "u0001" || claims.Role != "admin" || claims.TenantID != "t1" {
		t.Errorf("claims round-trip lost fields: %+v", claims)
	}
	if got := auth.TenantIDFromContext(ctx); got != "t1" {
		t.Errorf("TenantIDFromContext = %q, want t1", got)
	}
}

// TestClaimsFromContext_Empty ensures callers can handle an unauthenticated
// request without panicking.
func TestClaimsFromContext_Empty(t *testing.T) {
	got, ok := auth.ClaimsFromContext(context.Background())
	if ok || got != nil {
		t.Fatalf("ClaimsFromContext on empty ctx = (%v, %v); want (nil, false)", got, ok)
	}
	if id := auth.TenantIDFromContext(context.Background()); id != "" {
		t.Errorf("TenantIDFromContext on empty ctx = %q, want empty", id)
	}
}
