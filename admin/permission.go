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
// The request is identified by `RequestMethod + " " + RequestPath`.
// RequestPath is the registered route pattern (gin FullPath, e.g.
// "PUT /system/sys_user/:id"), which matches the "<METHOD> <URI>"
// format derive.go writes into sys_permissions.
//
// # Default policy: fail-closed
//
// A request is allowed when at least one of the following holds:
//
//   - the request code appears in the sys_permissions catalog AND the
//     caller's role holds a matching grant on the caller's tenant;
//   - the request path appears in the checker allowlist (see
//     WithAllowlist) — used for business RPCs whose auth posture is
//     enforced elsewhere (e.g. an application-level guard, or the JWT
//     middleware's own allowlist).
//
// Anything else is denied. The previous fail-open default (an
// uncatalogued route was always allowed) was a footgun: a developer
// who forgot to register a permission row for a new admin route would
// silently expose it without auth. Allowlisting is now explicit so
// every bypass is searchable in code review.
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

		// Allowlist check runs first: an allowlisted path is bypassed
		// before the catalog lookup, so business RPCs that don't fit
		// the catalog format don't pay the cache miss on the hot path.
		if allowlist.match(method, path) {
			return nil
		}

		code := method + " " + path

		// Catalog set first: an empty catalog means every catalogued
		// request is denied, and uncatalogued routes are also denied
		// (no allowlist match above).
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
			// Uncatalogued and not on the allowlist: deny. The fail-closed
			// default (documented on NewPermissionChecker) means a route
			// that has no catalog entry is never silently exposed — a
			// developer who forgets to register a permission row for a
			// new route must get a visible denial, not a quiet pass.
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
}

// WithAllowlist registers routes that bypass the catalog-based
// permission check. Each entry is "<METHOD> <path-pattern>", e.g.
//
//	WithAllowlist(
//	    "GET  /user/menus",
//	    "GET  /role/options",
//	    "*    /internal/*",
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
// gateway, an application-level guard). The whole point of the
// fail-closed default is that bypassing the catalog must be visible
// in code review.
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
