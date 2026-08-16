package auth

import (
	"context"

	jwt "github.com/golang-jwt/jwt/v5"
	mwauth "github.com/sjlit/aeus/middleware/auth"
)

// Claims embeds jwt.RegisteredClaims so standard exp/nbf/iat handling,
// including Valid() and the GetXxx getters, comes from the canonical
// implementation rather than being re-derived by hand.
//
// Context storage and retrieval are intentionally NOT defined here: this
// package only owns the type. Caller code reads claims via the canonical
// middleware/auth helpers, like so:
//
//	hs.Use(mwauth.JWT(
//	    func(*jwt.Token) (any, error) { return secret, nil },
//	    mwauth.WithClaims(Claims{}),  // parse into *auth.Claims
//	    mwauth.WithAllow("/auth/login", "/auth/refresh-token"),
//	))
//
//	// inside handlers:
//	if claims, ok := auth.ClaimsFromContext(ctx); ok {
//	    _ = claims.UID
//	    _ = claims.Role
//	    _ = claims.TenantID
//	}
type Claims struct {
	jwt.RegisteredClaims
	UID      string `json:"uid,omitempty"`
	Role     string `json:"uro,omitempty"`
	TenantID string `json:"tid,omitempty"`
	// TokenType marks the JWT as an access ("access") or refresh
	// ("refresh") token so RefreshToken can reject an access token
	// presented as a refresh token. Empty on tokens minted before the
	// claim existed.
	TokenType string `json:"token_type,omitempty"`
}

// ClaimsFromContext returns the *Claims stored in ctx by the JWT middleware,
// with ok=false when ctx carries no JWT (or one of the wrong concrete type).
//
// This is the admin-side entry point; callers should not reach into
// middleware/auth directly because the canonical type assertion is
// shared by every consumer (GORM callbacks, UserService RPCs).
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	j, ok := mwauth.FromContext(ctx)
	if !ok || j == nil {
		return nil, false
	}
	c, ok := j.(*Claims)
	return c, ok
}

// TenantIDFromContext returns claims.TenantID or "" when no claims are in ctx.
// Equivalent to the default tenant resolver; exposed as a helper so non-resolver
// callers (e.g. service-layer hand-rolled reads) don't have to round-trip
// through middleware/tenant.
func TenantIDFromContext(ctx context.Context) string {
	if c, ok := ClaimsFromContext(ctx); ok {
		return c.TenantID
	}
	return ""
}
