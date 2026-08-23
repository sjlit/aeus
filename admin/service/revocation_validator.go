package service

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin/auth"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	"github.com/sjlit/aeus/pkg/errs"
)

// RevocationValidator closes the loop between AuthService's TokenStore
// and the JWT middleware: middleware/auth.JWT runs its Validate hook on
// every non-allowlisted request, and this adapter turns a revoked jti
// (logout) or a refresh token (wrong token_type) into an immediate
// rejection — without it, logout only deletes a map entry nobody reads.
//
// Wire it alongside the permission checker:
//
//	revoker := service.NewRevocationValidator(store, secret)
//	httpSrv.Use(mwauth.JWT(keyFunc,
//	    mwauth.WithClaims(auth.Claims{}),
//	    mwauth.WithValidate(revoker), // revocation + token_type gate
//	    mwauth.WithPermissionChecker(admin.NewPermissionChecker(db)),
//	))
type RevocationValidator struct {
	store TokenStore
	key   []byte
}

var _ mwauth.Validate = (*RevocationValidator)(nil)

// NewRevocationValidator builds the adapter.  secret must be the same
// signing key AuthService uses, so the adapter can verify signatures
// before consulting the denylist.
func NewRevocationValidator(store TokenStore, secret string) *RevocationValidator {
	return &RevocationValidator{store: store, key: []byte(secret)}
}

// Validate implements middleware/auth.Validate.  It receives the raw
// bearer token and enforces exactly two semantics; every other failure
// mode is left to the middleware's own parse so error mapping
// (AccessDenied vs TokenExpired vs PermissionDenied) stays in one place:
//
//   - TokenType == "refresh" → AccessDenied: refresh tokens must never
//     authenticate API routes, only POST /auth/refresh-token;
//   - jti present and revoked → AccessDenied: the session was logged
//     out.
//
// Tokens that cannot be parsed here (bad signature, expired) pass
// through untouched — the middleware re-parses with full validation
// and reports the precise reason.
func (v *RevocationValidator) Validate(ctx context.Context, raw string) error {
	if v == nil || v.store == nil || raw == "" {
		return nil
	}
	claims := &auth.Claims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithoutClaimsValidation(), // expiry etc. is the middleware's call
	)
	token, err := parser.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return v.key, nil
	})
	if err != nil || token == nil || !token.Valid {
		return nil
	}
	if claims.TokenType == tokenTypeRefresh {
		return errs.ErrAccessDenied
	}
	if claims.ID != "" {
		revoked, rerr := v.store.Revoked(ctx, claims.ID)
		if rerr != nil {
			// Fail closed: a denylist outage should not silently
			// resurrect logged-out sessions.
			return errs.ErrAccessDenied
		}
		if revoked {
			return errs.ErrAccessDenied
		}
	}
	return nil
}
