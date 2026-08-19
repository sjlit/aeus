package admin

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// testPermCode is one catalog permission row: the request "PUT
// /system/sys_user/:id" matches rest/v3's Update scenario for the
// User resource (see derive.go permissionCode).
const testPermCode = "PUT /system/sys_user/:id"

// permCtx builds a request ctx carrying the metadata the checker reads.
// In production the transport/http interceptor fills these keys; the
// checker contract is exercised directly here.
func permCtx(protocol, method, path string) context.Context {
	ctx := metadata.Set(context.Background(), metadata.RequestProtocol, protocol)
	ctx = metadata.Set(ctx, metadata.RequestMethod, method)
	return metadata.Set(ctx, metadata.RequestPath, path)
}

// seedPermCatalog inserts the api permission row and, when grantRole is
// non-empty, one junction row scoped to the given tenant.
func seedPermCatalog(t *testing.T, db *gorm.DB, grantTenant, grantRole string) {
	t.Helper()
	if err := db.Create(&models.Permission{Type: string(models.PermissionTypeAPI), Data: testPermCode}).Error; err != nil {
		t.Fatal(err)
	}
	if grantRole != "" {
		if err := db.Create(&models.RolePermission{
			TenantModel: models.TenantModel{TenantID: grantTenant},
			RoleKey:     grantRole,
			Type:        models.RolePermissionTypePermission,
			Data:        testPermCode,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPermissionChecker_GrantedRolePasses(t *testing.T) {
	db := newTestDB(t)
	seedPermCatalog(t, db, "t1", "admin")

	checker := NewPermissionChecker(db)
	err := checker(permCtx("http", "PUT", "/system/sys_user/:id"), &auth.Claims{Role: "admin", TenantID: "t1"})
	if err != nil {
		t.Fatalf("granted role denied: %v", err)
	}
}

func TestPermissionChecker_UngrantedRoleDenied(t *testing.T) {
	db := newTestDB(t)
	seedPermCatalog(t, db, "t1", "admin")

	checker := NewPermissionChecker(db)
	err := checker(permCtx("http", "PUT", "/system/sys_user/:id"), &auth.Claims{Role: "guest", TenantID: "t1"})
	if !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("ungranted role: want code %d, got %v", errs.CodePermissionDenied, err)
	}
}

func TestPermissionChecker_UncataloguedRoutePassesByDefault(t *testing.T) {
	db := newTestDB(t)

	// /user/menus is a business RPC never registered in sys_permissions.
	// The default policy is fail-open: without an allowlist match the
	// request is allowed through. The deny path is reserved for routes
	// that the catalog explicitly knows about.
	checker := NewPermissionChecker(db)
	if err := checker(permCtx("http", "GET", "/user/menus"), &auth.Claims{Role: "guest", TenantID: "t1"}); err != nil {
		t.Fatalf("uncatalogued route denied under fail-open: %v", err)
	}
}

func TestPermissionChecker_UncataloguedRoutePassesWithAllowlist(t *testing.T) {
	db := newTestDB(t)
	// /user/menus is a business RPC that doesn't fit the catalog format.
	// The caller has explicitly listed it on the allowlist, so the
	// catalog check is bypassed.
	checker := NewPermissionChecker(db, WithCheckerAllowlist("GET /user/menus"))
	if err := checker(permCtx("http", "GET", "/user/menus"), &auth.Claims{Role: "guest", TenantID: "t1"}); err != nil {
		t.Fatalf("allowlisted route denied: %v", err)
	}
}

func TestPermissionChecker_AllowlistWildcardAnchored(t *testing.T) {
	db := newTestDB(t)
	// Allowlist a whole segment tree. The "*" must anchor at a segment
	// boundary, so "/internal-rogue/foo" does NOT match the allowlist.
	//
	// Under fail-open, an uncatalogued route is allowed by default —
	// so to observe the allowlist's anchoring behavior we put the
	// would-be leak path INTO the catalog (no grant). The deny path
	// then proves the wildcard didn't accidentally match it.
	if err := db.Create(&models.Permission{Type: string(models.PermissionTypeAPI), Data: "GET /internal-rogue/foo"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Permission{Type: string(models.PermissionTypeAPI), Data: "GET /internal/health"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Permission{Type: string(models.PermissionTypeAPI), Data: "GET /internal/v1/ping"}).Error; err != nil {
		t.Fatal(err)
	}
	// The allowed paths are also grant-covered; the rogue path is not.
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "guest",
		Type:        models.RolePermissionTypePermission,
		Data:        "GET /internal/health",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "guest",
		Type:        models.RolePermissionTypePermission,
		Data:        "GET /internal/v1/ping",
	}).Error; err != nil {
		t.Fatal(err)
	}

	checker := NewPermissionChecker(db, WithCheckerAllowlist("* /internal/*"))
	cases := []struct {
		path   string
		passes bool
	}{
		{"/internal/health", true},
		{"/internal/v1/ping", true},
		// Catalogued (so fail-open no longer saves it) but not in the
		// allowlist and not granted — the wildcard anchoring must NOT
		// have matched "/internal-rogue/foo".
		{"/internal-rogue/foo", false},
	}
	for _, tc := range cases {
		err := checker(permCtx("http", "GET", tc.path), &auth.Claims{Role: "guest", TenantID: "t1"})
		if tc.passes && err != nil {
			t.Fatalf("%s unexpectedly denied: %v", tc.path, err)
		}
		if !tc.passes && !errs.IsCode(err, errs.CodePermissionDenied) {
			t.Fatalf("%s: want code %d, got %v", tc.path, errs.CodePermissionDenied, err)
		}
	}
}

func TestPermissionChecker_UncataloguedRoutePassesWithCatalog(t *testing.T) {
	db := newTestDB(t)
	// The catalog is populated with a DIFFERENT route; the requested
	// route is still not catalogued. Under fail-open, the catalog's
	// presence does not affect gates on routes that aren't in it —
	// only routes whose code IS in the catalog must satisfy the
	// grant check.
	if err := db.Create(&models.Permission{Type: string(models.PermissionTypeAPI), Data: "GET /system/sys_tenant"}).Error; err != nil {
		t.Fatal(err)
	}

	checker := NewPermissionChecker(db)
	if err := checker(permCtx("http", "PUT", "/system/sys_user/:id"), &auth.Claims{Role: "guest", TenantID: "t1"}); err != nil {
		t.Fatalf("uncatalogued route denied despite fail-open: %v", err)
	}
}

// graceWindowSleep is how long a test must wait for the dbcache 1s
// grace window to expire before a cached entry revalidates against its
// dependency marker. Kept as a named constant so the intent is clear;
// it must stay > graceWindow in package dbcache.
const graceWindowSleep = 1200 * time.Millisecond

func TestPermissionChecker_GrantRevocationTakesEffect(t *testing.T) {
	db := newTestDB(t)
	seedPermCatalog(t, db, "t1", "admin")

	checker := NewPermissionChecker(db)
	claims := &auth.Claims{Role: "admin", TenantID: "t1"}
	req := permCtx("http", "PUT", "/system/sys_user/:id")
	if err := checker(req, claims); err != nil {
		t.Fatalf("granted role denied: %v", err)
	}

	// Revoke the grant (soft delete), then wait out the cache grace
	// window: the next check must revalidate against the version marker
	// and reload the now-empty grant set.
	if err := db.Where("role_key = ? AND tenant_id = ?", "admin", "t1").Delete(&models.RolePermission{}).Error; err != nil {
		t.Fatal(err)
	}
	time.Sleep(graceWindowSleep)

	if err := checker(req, claims); !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("revoked role still granted: want code %d, got %v", errs.CodePermissionDenied, err)
	}
}

func TestPermissionChecker_NonHTTPProtocolPasses(t *testing.T) {
	db := newTestDB(t)
	seedPermCatalog(t, db, "t1", "admin")

	// A non-HTTP transport (e.g. CLI) skips the HTTP permission check.
	checker := NewPermissionChecker(db)
	if err := checker(permCtx("cli", "PUT", "/system/sys_user/:id"), &auth.Claims{Role: "guest", TenantID: "t1"}); err != nil {
		t.Fatalf("non-http request denied: %v", err)
	}
}

func TestPermissionChecker_CrossTenantIsolation(t *testing.T) {
	db := newTestDB(t)
	// t1's "admin" role is granted; t2's "admin" role (same key) is not.
	seedPermCatalog(t, db, "t1", "admin")

	checker := NewPermissionChecker(db)
	err := checker(permCtx("http", "PUT", "/system/sys_user/:id"), &auth.Claims{Role: "admin", TenantID: "t2"})
	if !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("t2 leaked t1's grant: want code %d, got %v", errs.CodePermissionDenied, err)
	}
}

func TestPermissionChecker_NonAdminClaimsDenied(t *testing.T) {
	db := newTestDB(t)
	seedPermCatalog(t, db, "t1", "admin")

	// The checker is only meaningful with *auth.Claims; a wiring error
	// (middleware set up with a different claims type) must fail closed.
	checker := NewPermissionChecker(db)
	err := checker(permCtx("http", "PUT", "/system/sys_user/:id"), jwt.MapClaims{"uid": "admin"})
	if !errs.IsCode(err, errs.CodeAccessDenied) {
		t.Fatalf("wrong claims type: want code %d, got %v", errs.CodeAccessDenied, err)
	}
}
