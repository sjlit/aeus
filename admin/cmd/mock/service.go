package main

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin"
	adminAuth "github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/admin/service"
	memcache "github.com/sjlit/aeus/infra/cache/memory"
	mw "github.com/sjlit/aeus/middleware/auth"
)

// ServiceLoader wires admin's resources onto the HTTP server and
// seeds the initial admin user. Implements aeus.ServiceLoader.
type ServiceLoader struct {
	scope *ScopeContext
}

// Init runs the steps below in this order — each depends on the prior:
//
//  1. admin.Server.Setup: migrates sys_* schema, registers the 5
//     writable REST resources and the 4 read-only security/audit ones.
//  2. AuthService: hand-wired onto the same router so /auth/{login,
//     refresh-token, logout} exist. The HMAC secret comes from
//     JWTSecret() (env, dev default).
//  3. JWT middleware: validates Bearer tokens on every route not on
//     the allowlist. Without this the REST resources and business
//     RPCs are open to anyone.
//  4. Business RPCs: UserService / RoleService /
//     PermissionService / MenuService.
//  5. Seed: run after Setup so the permission catalog exists, and
//     before the HTTP server starts so the fail-closed checker already
//     has the super-role grants on the first request.
func (s *ServiceLoader) Init(ctx context.Context) (err error) {
	secret := []byte(JWTSecret())
	http := s.scope.HTTP()

	if err = s.scope.Admin().Setup(ctx); err != nil {
		return
	}

	// AuthService is registered manually (admin no longer auto-wires
	// it from a secret option); same router as the REST resources so
	// the JWT allowlist can reference /auth/login uniformly.
	pb.RegisterAuthServiceRouter(http, service.NewAuthService(
		service.WithAuthServiceDB(s.scope.DB()),
		service.WithAuthSecret(JWTSecret()),
	))

	db := s.scope.DB()
	// sharedCache backs every read-through Cacher the admin services
	// build (permission catalog + role grants via NewPermissionChecker,
	// menus / visible menus / permission codes / role options via the
	// business services).  In-memory keeps the mock self-contained;
	// swap memcache.NewCache() for a Redis-backed implementation to
	// share across processes.
	sharedCache := memcache.NewCache()
	// RateLimit sits BEFORE JWT so an attacker spamming /auth/login
	// can't burn bcrypt cycles (the most expensive thing on the
	// login path). Only the client IP is available before JWT, so the
	// login bucket keys on IP; per-account limiting belongs in the
	// login handler (or after JWT on authenticated routes), because
	// {user} would otherwise always be empty and every account would
	// share one global bucket.
	//
	// 5 burst / 60s refill is the dev default — production should
	// tighten (e.g. Capacity=3, Refill=1/30s) per the threat model.
	http.Use(mw.RateLimit(mw.RateLimitOpts{
		Path:     "/auth/login",
		Keys:     []string{"login:ip:{ip}"},
		Capacity: 5,
		Refill:   1.0 / 60,
	}))
	http.Use(mw.JWT(
		func(*jwt.Token) (any, error) { return secret, nil },
		// Parse into admin's Claims so GORM tenant callbacks and
		// UserService see UID/Role/TenantID via ClaimsFromContext.
		mw.WithClaims(adminAuth.Claims{}),
		// Enforce the role's api permissions on catalogued HTTP routes.
		// Uncatalogued routes are denied by default; self-service
		// routes are built into the checker's public allowlist.
		mw.WithPermissionChecker(admin.NewPermissionChecker(db, admin.WithCheckerCache(sharedCache))),
		mw.WithAllow(
			"/auth/login",
			"/auth/refresh-token",
			// Logout must be reachable with an expired access token:
			// the handler verifies the tokens in the body itself, and
			// otherwise an expired session could never revoke its
			// refresh token.
			"/auth/logout",
			"/health",
			"/ready",
		),
	))
	pb.RegisterUserServiceRouter(http, service.NewUserService(
		service.WithUserServiceDB(db),
		service.WithUserServiceCache(sharedCache),
	))
	pb.RegisterRoleServiceRouter(http, service.NewRoleService(
		service.WithRoleServiceDB(db),
		service.WithRoleServiceCache(sharedCache),
	))
	pb.RegisterPermissionServiceRouter(http, service.NewPermissionService(
		service.WithPermissionServiceDB(db),
	))
	pb.RegisterMenuServiceRouter(http, service.NewMenuService(
		service.WithMenuServiceDB(db),
		service.WithMenuServiceCache(sharedCache),
	))
	pb.RegisterTenantServiceRouter(http, service.NewTenantService(
		service.WithTenantServiceDB(db),
	))

	// Seed after Setup so the catalog exists, but before the servers
	// start: with the checker's fail-closed default, a super role must
	// have its grants before the first request arrives. Seed is
	// idempotent, so restarts are safe.
	if err = admin.Seed(db, AdminUser(), AdminPassword()); err != nil {
		return
	}
	return
}

// Run is intentionally a no-op: bootstrap seeding now happens in Init,
// before the HTTP server starts. Keeping the ServiceLoader contract
// (Init + Run) avoids special-casing the framework lifecycle.
func (s *ServiceLoader) Run(ctx context.Context) error {
	return nil
}

// NewServiceLoader constructs a ServiceLoader with a back-reference
// to the scope's singleton resources.
func NewServiceLoader(scope *ScopeContext) *ServiceLoader {
	return &ServiceLoader{scope: scope}
}
