package middleware

import (
	"context"
	"net/http"

	"github.com/sjlit/aeus/admin/auth"
)

// FromClaimsUserResolve derives the caller's UID from the ctx written
// by middleware/auth.JWT. It is the default rest.ResolveUserFunc
// installed by admin.New (overridable via admin.WithUserResolve) and
// feeds both rest/v3's RuntimeScope.User and the uid column of every
// audit row.
//
// Like FromClaimsResolver, it MUST be cheap and MUST NOT panic on a
// missing or malformed JWT — it simply returns "" in that case.
func FromClaimsUserResolve(ctx context.Context, _ *http.Request) (string, error) {
	if c, ok := auth.ClaimsFromContext(ctx); ok {
		return c.UID, nil
	}
	return "", nil
}
