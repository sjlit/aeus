package admin

import (
	"context"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/dbcache"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/infra/cache"
	"github.com/sjlit/aeus/metadata"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// httpProtocol is the transport/http.Protocol value ("http"). admin does
// not import transport/http (it would drag gin into the module), so the
// literal is mirrored here — keep it in sync with
// github.com/sjlit/aeus/transport/http.Protocol.
const httpProtocol = "http"

// catalogMarker is the version marker for sys_permissions. Catalog
// rows change at deploy time (FirstOrCreate inserts) and via admin CRUD
// edits (in-place updates); MAX(updated_at) catches both across
// seconds. A same-second edit is a residual blind spot bounded by the
// cache TTL, which is acceptable for a code-derived catalog.
const catalogMarker = "MAX(updated_at)"

// grantsMarker is the version marker for sys_role_permissions. Grant
// rows are never updated in place — assignment rewrites them wholesale
// (soft delete + re-insert) — so SUM(id) over live rows catches every
// realistic pattern with a compact value: soft deletes drop the id,
// inserts raise it, and a same-second revoke+grant still changes the
// sum because a fresh auto-increment id is always larger than any it
// replaces. BaseModel's updated_at (Unix seconds) alone cannot detect
// same-second writes, which is why the sum is preferred here.
const grantsMarker = "SUM(id)"

// NewPermissionChecker returns a mwauth.PermissionCheckerFunc that
// enforces the role's api permissions on HTTP requests. Wire it into
// the JWT middleware:
//
//	mw.JWT(keyFunc,
//	    mw.WithClaims(adminAuth.Claims{}),
//	    mw.WithPermissionChecker(admin.NewPermissionChecker(db)),
//	    mw.WithAllow("/auth/login", "/auth/refresh-token"),
//	)
//
// The checker runs after token verification but BEFORE claims are
// stored in ctx (see middleware/auth.JWT), so it takes the parsed
// claims as an argument and must NOT rely on auth.ClaimsFromContext.
//
// # Matching
//
// The request is identified by `RequestMethod + " " + RequestRoute`.
// RequestRoute is the registered route pattern (gin FullPath, e.g.
// "PUT /system/sys_user/:id"), which matches the "<METHOD> <URI>"
// format derive.go writes into sys_permissions. Embedders that only
// populate RequestPath fall back to it; NoRoute/static requests carry
// an empty RequestRoute and are treated as infrastructure.
//
// # Default policy: fail-closed
//
// A request is allowed when at least one of the following holds:
//
//   - the request is an infrastructure route (NoRoute/static, probes,
//     /metrics, /mcp, or generated openapi.json) — JWT still
//     authenticates it, but it has no catalog entry;
//   - the request path+method is on the checker's public allowlist
//     (self-service routes such as /user/profile and /user/menus);
//   - the request code appears in the sys_permissions catalog AND the
//     caller's role holds a matching grant on the caller's tenant.
//
// A request whose route is NOT in the catalog is denied by default.
// That is the safe default for an admin module: a newly added route
// cannot silently become available to every authenticated user just
// because nobody remembered to catalog it. Applications that still need
// the historical fail-open behavior can opt in with
// WithCheckerFailOpen(true), but this should be treated as a temporary
// migration escape hatch.
//
// # Caching
//
// Instead of a point query per request, the checker caches two sets
// through dbcache: the api catalog (one key) and each (tenant, role)'s
// grant set (one key per pair). Both revalidate against a version
// marker of the source table (catalogMarker / grantsMarker), so grant
// changes propagate to the next request after dbcache's 1s grace
// window; the grants cache adds a 1-minute TTL as a hard bound for the
// markers' residual blind spots. Both markers are table-global (a
// fixed dependency cannot be per-key), so a write anywhere invalidates
// every affected key lazily — over-invalidation is safe,
// under-invalidation would leak permissions.
//
// # Isolation
//
// The grant loader filters tenant_id explicitly: the GORM tenant
// callbacks cannot backfill a scope here because claims are not yet in
// ctx when the checker runs.
func NewPermissionChecker(db *gorm.DB, opts ...CheckerOption) mwauth.PermissionCheckerFunc {
	if db == nil {
		panic("admin: NewPermissionChecker requires a non-nil *gorm.DB")
	}
	cfg := &checkerConfig{}
	// Built-in public routes: every authenticated user needs these to
	// render their own session (profile, menu tree, permission codes).
	// Keeping them in the allowlist means a normal user does not need an
	// explicit grant for self-service, while every admin route remains
	// catalogued and fail-closed.
	for _, entry := range defaultCheckerAllowlist {
		method, pattern := splitAllowEntry(entry)
		cfg.allowlist = append(cfg.allowlist, allowEntry{method: method, pattern: pattern})
	}
	for _, o := range opts {
		o(cfg)
	}
	// buildCacher is the shared constructor for the two read-through
	// caches below; it threads the optional shared cache.Cache
	// backend through dbcache.WithCache when one is supplied.
	buildCacher := func(dep *dbcache.SqlDependency, ttl time.Duration) *dbcache.Cacher {
		opts := []dbcache.Option{dbcache.WithDependency(dep)}
		if ttl > 0 {
			opts = append(opts, dbcache.WithCacheDuration(ttl))
		}
		if cfg.cache != nil {
			opts = append(opts, dbcache.WithCache(cfg.cache))
		}
		return dbcache.New(db, opts...)
	}
	// Both loaders read only live rows; the soft-delete scope is added
	// by gorm via Model(), and the markers are computed over live rows
	// too via the explicit deleted_at condition.
	apiCatalog := buildCacher(dbcache.NewSqlDependency(
		dbcache.WithTable((&models.Permission{}).TableName()),
		dbcache.WithColumn(catalogMarker),
		dbcache.WithCondition("deleted_at IS NULL"),
	), 0)
	grants := buildCacher(dbcache.NewSqlDependency(
		dbcache.WithTable((&models.RolePermission{}).TableName()),
		dbcache.WithColumn(grantsMarker),
		dbcache.WithCondition("deleted_at IS NULL"),
		// Grant changes must land quickly: the marker catches them on
		// the next revalidation, and the TTL bounds the residual
		// same-second blind spots.
	), time.Minute)
	allowlist := cfg.allowlist
	return func(ctx context.Context, claims jwt.Claims) error {
		// Non-HTTP transports (CLI, gRPC, ...) skip the HTTP permission
		// check entirely.
		if proto, _ := metadata.Get(ctx, metadata.RequestProtocol); proto != httpProtocol {
			return nil
		}
		ac, ok := claims.(*auth.Claims)
		if !ok || ac == nil {
			// The middleware was wired with a claims type admin does not
			// understand; failing closed surfaces the misconfiguration.
			return errs.ErrAccessDenied
		}
		method, _ := metadata.Get(ctx, metadata.RequestMethod)
		path, _ := metadata.Get(ctx, metadata.RequestPath)
		route, routeSet := metadata.Get(ctx, metadata.RequestRoute)
		if !routeSet {
			// Tests and embedders that only populate RequestPath predate
			// RequestRoute; treat the path as the route template.
			route = path
		}

		// Infrastructure routes are not business APIs and have no
		// catalog entries. Let them through: JWT still authenticates
		// them, but a permission row cannot be granted for routes such
		// as the SPA's static files (NoRoute has an empty FullPath),
		// /health, /metrics, /mcp, or the generated openapi.json docs.
		// Without this bypass the fail-closed default turns a 404 or a
		// static asset into a misleading 403.
		if isInfrastructurePath(route) {
			return nil
		}

		// Allowlist check runs first: an allowlisted path is bypassed
		// before the catalog lookup, so business RPCs that don't fit
		// the catalog format don't pay the cache miss on the hot path.
		if allowlist.match(method, path) {
			return nil
		}

		code := method + " " + route

		// Catalog set first: an empty catalog means every non-public
		// request is denied by the fail-closed default.
		catalogSet, err := dbcache.Try(apiCatalog, ctx, "perm:catalog", func(tx *gorm.DB) (map[string]struct{}, error) {
			var datas []string
			if err := tx.Model(&models.Permission{}).
				Where("type = ?", string(models.PermissionTypeAPI)).
				Pluck("data", &datas).Error; err != nil {
				return nil, err
			}
			set := make(map[string]struct{}, len(datas))
			for _, d := range datas {
				set[d] = struct{}{}
			}
			return set, nil
		})
		if err != nil {
			return err
		}
		if _, ok := catalogSet[code]; !ok {
			// Uncatalogued route: deny by default so a newly added route
			// cannot silently become public. The explicit opt-out is
			// WithCheckerFailOpen(true) for applications migrating from
			// the historical behavior.
			if cfg.failOpen {
				return nil
			}
			return errs.ErrPermissionDenied
		}

		// Catalogued: the role must hold the matching grant on the
		// caller's tenant.
		grantKey := "perm:grant:" + ac.TenantID + ":" + ac.Role
		grantSet, err := dbcache.Try(grants, ctx, grantKey, func(tx *gorm.DB) (map[string]struct{}, error) {
			var datas []string
			if err := tx.Model(&models.RolePermission{}).
				Where("role_key = ? AND tenant_id = ? AND type = ?",
					ac.Role, ac.TenantID, models.RolePermissionTypePermission).
				Pluck("data", &datas).Error; err != nil {
				return nil, err
			}
			set := make(map[string]struct{}, len(datas))
			for _, d := range datas {
				set[d] = struct{}{}
			}
			return set, nil
		})
		if err != nil {
			return err
		}
		if _, ok := grantSet[code]; !ok {
			return errs.ErrPermissionDenied
		}
		return nil
	}
}

// CheckerOption configures NewPermissionChecker.
type CheckerOption func(*checkerConfig)

// checkerConfig is the per-NewPermissionChecker configuration. It is
// unexported because nothing outside the package should depend on its
// layout; callers mutate it through the With* helpers.
type checkerConfig struct {
	cache     cache.Cache
	allowlist pathMatcher
	failOpen  bool
}

// defaultCheckerAllowlist contains the self-service routes every signed-in
// user must be able to call: own profile, own menu tree, own permission
// codes, password/avatar changes, and the navigation breadcrumb. These
// routes do not grant any administrative capability.
var defaultCheckerAllowlist = []string{
	"GET /user/profile",
	"PATCH /user/profile",
	"POST /user/change-password",
	"POST /user/set-avatar",
	"GET /user/menus",
	"GET /user/permissions",
	"GET /menu/tree",
	"GET /menu/breadcrumb",
	// Dashboard is the landing page for every signed-in role. The handler
	// returns only personal data unless the caller's role is super, so it
	// can bypass the catalog grant check safely.
	"GET /dashboard/overview",
}

// WithCheckerFailOpen restores the historical fail-open behavior for
// uncatalogued routes. Prefer cataloging the route (or adding it to the
// explicit allowlist) instead; this option exists for staged migrations.
func WithCheckerFailOpen(failOpen bool) CheckerOption {
	return func(c *checkerConfig) {
		c.failOpen = failOpen
	}
}

// WithCheckerAllowlist registers routes that BYPASS the catalog-based
// permission check. The historical name is kept for compatibility; the
// semantics are a public/bypass list, not an extra enforcement layer.
// Each entry is "<METHOD> <path-pattern>", e.g.
//
//	WithCheckerAllowlist(
//	    "*    /internal/*", // protected by an upstream gateway / handler
//	)
//
// The path-pattern syntax mirrors middleware/auth.jwt's isAllowed:
//
//   - an exact match passes;
//   - "*" alone matches every path;
//   - a trailing "*" matches any path with the given prefix, but only
//     at segment boundaries (so "/internal/*" does not match
//     "/internal-rogue/foo").
//
// Use it sparingly — every entry is a route whose authorization is
// enforced by some other mechanism (JWT allowlist, an upstream
// gateway, an application-level guard) or is genuinely public. Each
// entry makes a bypass visible in code review. The self-service routes
// (profile/menu/password) are already public by default; this option is
// for additional application-specific routes.
func WithCheckerAllowlist(entries ...string) CheckerOption {
	return func(c *checkerConfig) {
		for _, e := range entries {
			method, pattern := splitAllowEntry(e)
			c.allowlist = append(c.allowlist, allowEntry{method: method, pattern: pattern})
		}
	}
}

func WithCheckerCache(cache cache.Cache) CheckerOption {
	return func(c *checkerConfig) {
		c.cache = cache
	}
}

// allowEntry is one WithAllowlist record after splitting into the
// "<METHOD> <pattern>" pair.
type allowEntry struct {
	method  string // "" = any method
	pattern string
}

// pathMatcher is the parsed allowlist with a per-request match method.
type pathMatcher []allowEntry

// match reports whether (method, path) hits any allowlist entry. The
// check is order-preserving: an exact method+path match short-circuits
// any wildcard entries.
func (m pathMatcher) match(method, path string) bool {
	wildcardHit := false
	for _, e := range m {
		switch e.method {
		case "":
			if matchPattern(e.pattern, path) {
				wildcardHit = true
			}
		case method:
			if matchPattern(e.pattern, path) {
				return true
			}
		}
	}
	return wildcardHit
}

// isInfrastructurePath reports routes that are not part of the RBAC
// catalog: the router's NoRoute/static handler (empty FullPath), probes,
// infrastructure endpoints, and generated OpenAPI JSON. These are still
// protected by the JWT middleware; the permission checker only decides
// whether a role may call a business API.
func isInfrastructurePath(path string) bool {
	if path == "" {
		return true
	}
	switch path {
	case "/health", "/ready", "/metrics", "/mcp":
		return true
	}
	if strings.HasPrefix(path, "/mcp/") {
		return true
	}
	return strings.HasSuffix(path, "/openapi.json")
}

// splitAllowEntry parses "<METHOD> <pattern>"; METHOD is "*" or empty
// for a method-agnostic match.
func splitAllowEntry(s string) (method, pattern string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	before, after, ok := strings.Cut(s, " ")
	if !ok {
		// No method prefix: treat the whole string as a path pattern
		// that matches any method. Useful for entries like "/internal/*".
		return "", s
	}
	method = strings.TrimSpace(before)
	if method == "*" {
		method = ""
	}
	return method, strings.TrimSpace(after)
}

// matchPattern mirrors middleware/auth.jwt.isAllowed's segment-aware
// semantics: exact match, "*" alone, or "<prefix>*" anchored at a
// segment boundary.
func matchPattern(pattern, path string) bool {
	n := len(pattern)
	if pattern == path {
		return true
	}
	if pattern == "*" {
		return true
	}
	if n > 1 && pattern[n-1] == '*' {
		prefix := pattern[:n-1]
		if !strings.HasPrefix(path, prefix) {
			return false
		}
		// Anchor at a segment boundary: either the prefix ends at a
		// slash already, or the matched portion ends right at a slash.
		if prefix[len(prefix)-1] == '/' {
			return true
		}
		return len(path) > len(prefix) && path[len(prefix)] == '/'
	}
	return false
}
