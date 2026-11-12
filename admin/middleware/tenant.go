// Package middleware owns the Resolver type used by admin.Server to
// scope GORM operations to the current tenant.
//
// The package intentionally does NOT own an HTTP middleware that
// injects the tenant into ctx; the GORM callbacks installed in
// admin.Server.Setup invoke the resolver directly against the
// operation's ctx on every DB call, so no middleware in the chain
// is required to wire up tenant isolation. Tenant scoping itself is
// opt-in: enable it with admin.WithMultiTenant(true) (or
// admin.WithTenantResolver(...)) before relying on the callbacks.
// The caller then only has to provide a Resolver that knows how to
// derive the tenant id from a request context.
package middleware

import (
	"context"

	"github.com/sjlit/aeus/admin/auth"
)

// tenantBypassKey marks operations that intentionally span tenants:
// Seed/Setup bootstrapping, cross-tenant admin tools, Login (before any
// tenant is known), and test fixtures.
type tenantBypassKey struct{}

// WithoutTenant returns a context that disables tenant scoping for the
// DB operations using it. Use it only where cross-tenant access is the
// explicit intent; ordinary request paths must never carry it.
func WithoutTenant(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantBypassKey{}, true)
}

// TenantBypassed reports whether ctx carries the explicit WithoutTenant
// marker.
func TenantBypassed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(tenantBypassKey{}).(bool)
	return v
}

// Resolver derives the current tenant id from a request context.
//
// Returning a non-empty value scopes every TenantModel read/update/
// delete to that tenant, and forces the TenantID field on TenantModel
// creates. Returning "" is treated as a missing tenant context and the
// operation fails closed with ErrTenantRequired. Cross-tenant operations
// must opt in explicitly with WithoutTenant:
//
//   - super-admin / cross-tenant tooling,
//   - background tasks that legitimately span tenants,
//   - the AuthService.Login RPC, which must locate the user before
//     any tenant is known.
//
// Implementations MUST be cheap (called once per DB op on every
// hot path) and MUST NOT panic on a malformed/missing JWT — just
// return "". The GORM callbacks installed by admin.Server.Setup
// rely on this contract; see admin.WithTenantResolver for the full
// rationale.
type Resolver func(ctx context.Context) string

// FromClaimsResolver is the default resolver: it reads
// *auth.Claims.TenantID off the ctx written by middleware/auth.JWT.
// It returns "" if there is no JWT in ctx or the value is empty, which
// causes tenant-scoped DB operations to fail closed. Use WithoutTenant
// for explicit cross-tenant/system paths.
//
// The GORM callbacks in admin.Server.Setup invoke this directly,
// so no additional middleware is required to scope DB ops after
// the JWT middleware has run.
func FromClaimsResolver(ctx context.Context) string {
	return auth.TenantIDFromContext(ctx)
}
