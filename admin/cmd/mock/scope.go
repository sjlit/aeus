package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

// dev-only defaults; env vars override at startup.
const (
	defaultJWTSecret = "change-me" // dev-only, never use in production
	defaultAdminUser = "admin"
	defaultAdminPass = "Admin123"
	defaultDBPath    = "./mock.db"
)

// JWTSecret returns the HMAC secret used to sign login tokens.
// Reads JWT_SECRET from env, falls back to a dev-only default.
func JWTSecret() string {
	if v := os.Getenv("JWT_SECRET"); v != "" {
		return v
	}
	return defaultJWTSecret
}

// AdminUser returns the username Seed will bootstrap.
// Reads ADMIN_USER from env, falls back to a dev-only default.
func AdminUser() string {
	if v := os.Getenv("ADMIN_USER"); v != "" {
		return v
	}
	return defaultAdminUser
}

// AdminPassword returns the password Seed will bootstrap.
// Reads ADMIN_PASSWORD from env, falls back to a dev-only default.
func AdminPassword() string {
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		return v
	}
	return defaultAdminPass
}

// ScopeContext holds singleton resources initialized at boot.
// Http is injected by the aeus framework via reflect (injectVars);
// Init bridges it into admin.Server so Setup can register routes.
type ScopeContext struct {
	db    *gorm.DB
	Http  *ghttp.Server
	admin *admin.Server
}

// DB returns the *gorm.DB owned by this scope.
func (s *ScopeContext) DB() *gorm.DB { return s.db }

// Admin returns the *admin.Server owned by this scope.
func (s *ScopeContext) Admin() *admin.Server { return s.admin }

// HTTP returns the *http.Server injected by the aeus framework.
func (s *ScopeContext) HTTP() *ghttp.Server { return s.Http }

// Init opens the SQLite database, constructs admin.Server with the
// mock integration options, and bridges the framework-injected
// *http.Server into admin.Server. It does NOT call admin.Server.Setup,
// mount middleware, register RPC services, or seed data — those belong
// to ServiceLoader.
func (s *ScopeContext) Init(ctx context.Context) (err error) {
	if s.db, err = gorm.Open(sqlite.Open(dbPath()), &gorm.Config{}); err != nil {
		return
	}
	s.db = s.db.Debug()
	s.admin = admin.New(
		admin.WithDB(s.db),
		admin.WithOpenAPI(true),
		admin.WithRouter(s.Http),
		admin.WithAudit(true),
		// The mock intentionally exercises the multi-tenant path. Real
		// single-tenant projects can omit this option.
		admin.WithMultiTenant(true),
		admin.WithVueOutputDir(vueOutputDir()),
	)
	// Wire the framework-injected *http.Server into admin so Setup
	// can register REST + RPC routes onto the same gin engine that
	// the JWT middleware will guard in ServiceLoader.Init.
	return
}

// dbPath resolves the SQLite file. The default is relative to the mock
// directory (the documented `go run .` working directory); AEUS_DB_PATH
// lets callers keep the file elsewhere, e.g. when running from the repo
// root with `go run ./admin/cmd/mock`.
func dbPath() string {
	if v := os.Getenv("AEUS_DB_PATH"); v != "" {
		return v
	}
	return defaultDBPath
}

// vueOutputDir resolves the optional Vue view-generation directory.
// The default assumes `go run .` from admin/cmd/mock; override with
// AEUS_VUE_OUTPUT_DIR when running from a different working directory.
func vueOutputDir() string {
	if v := os.Getenv("AEUS_VUE_OUTPUT_DIR"); v != "" {
		return v
	}
	return filepath.Join("..", "..", "web", "src", "views")
}

// NewScopeContext constructs an empty ScopeContext; the aeus framework
// injects the *http.Server field, then calls Init.
func NewScopeContext() *ScopeContext {
	return &ScopeContext{}
}
