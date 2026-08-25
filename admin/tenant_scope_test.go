package admin

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/middleware"
	"github.com/sjlit/aeus/admin/models"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	"gorm.io/gorm"
)

// withTenantDB builds an in-memory sqlite, migrates the supplied
// models, installs the tenant scope callback with a test resolver
// that pulls the tenant id from a context.Value("test_tenant"), and
// returns the gorm handle plus a helper that wraps ctx with that key.
//
// Using a dedicated test resolver (rather than the production
// FromClaimsResolver) keeps the legacy "ctx with a tenant id" style
// expressive without leaking test wiring into admin's runtime path.
func withTenantDB(t *testing.T, ms ...any) (*gorm.DB, func(string) context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) > 0 {
		if err := db.AutoMigrate(ms...); err != nil {
			t.Fatal(err)
		}
	}
	resolver := func(ctx context.Context) string {
		if ctx == nil {
			return ""
		}
		if v, ok := ctx.Value(testTenantKey{}).(string); ok {
			return v
		}
		return ""
	}
	installTenantScope(db, resolver)
	return db, func(tid string) context.Context {
		if tid == "" {
			return context.Background()
		}
		return context.WithValue(context.Background(), testTenantKey{}, tid)
	}
}

type testTenantKey struct{}

func TestTenantScope_Read_Isolated(t *testing.T) {
	db, ctx := withTenantDB(t, &models.Role{})
	// Seed: each tenant owns a role with code "shared".
	if err := db.WithContext(ctx("t1")).Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "shared", Name: "t1-role"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx("t2")).Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "shared", Name: "t2-role"}).Error; err != nil {
		t.Fatal(err)
	}

	var got models.Role
	// t1 reads its own row.
	if err := db.WithContext(ctx("t1")).Where("`key` = ?", "shared").First(&got).Error; err != nil {
		t.Fatalf("t1 first own: %v", err)
	}
	if got.TenantID != "t1" || got.Name != "t1-role" {
		t.Fatalf("t1 sees %+v, want t1/t1-role", got)
	}

	// t1 must not be able to read t2's row, even by querying the exact id.
	var miss models.Role
	err := db.WithContext(ctx("t1")).Where("`key` = ? AND tenant_id = ?", "shared", "t2").First(&miss).Error
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("t1 reading t2 row: want ErrRecordNotFound, got %v (row=%+v)", err, miss)
	}
}

func TestTenantScope_Update_DoesNotLeak(t *testing.T) {
	db, ctx := withTenantDB(t, &models.Role{})
	if err := db.WithContext(ctx("t1")).Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "shared", Name: "before"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx("t2")).Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "shared", Name: "before"}).Error; err != nil {
		t.Fatal(err)
	}

	// t1 tries to rename the shared code; with the scope in place the
	// UPDATE only fires against rows owned by t1.
	res := db.WithContext(ctx("t1")).Model(&models.Role{}).
		Where("`key` = ?", "shared").
		Update("name", "after-t1")
	if res.Error != nil {
		t.Fatalf("t1 update error: %v", res.Error)
	}
	if res.RowsAffected != 1 {
		t.Fatalf("t1 update rows = %d, want 1", res.RowsAffected)
	}

	var t1, t2 models.Role
	if err := db.Where("`key` = ? AND tenant_id = ?", "shared", "t1").First(&t1).Error; err != nil {
		t.Fatal(err)
	}
	if t1.Name != "after-t1" {
		t.Fatalf("t1 name after update = %q", t1.Name)
	}
	if err := db.Where("`key` = ? AND tenant_id = ?", "shared", "t2").First(&t2).Error; err != nil {
		t.Fatal(err)
	}
	if t2.Name != "before" {
		t.Fatalf("t2 name leaked into update = %q, want before", t2.Name)
	}
}

func TestTenantScope_Delete_DoesNotLeak(t *testing.T) {
	// Role.AfterDelete cascades into sys_role_permissions and detaches
	// sys_users.role_key, so both tables must exist for the hook to fire
	// cleanly even though this test doesn't seed any rows in them.
	db, ctx := withTenantDB(t, &models.Role{}, &models.RolePermission{}, &models.User{})
	if err := db.WithContext(ctx("t1")).Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "shared", Name: "r"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx("t2")).Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "shared", Name: "r"}).Error; err != nil {
		t.Fatal(err)
	}

	// t1 loads its row by id and deletes; the callback must prevent the
	// delete from also hitting t2's row (both share code "shared").
	var t1 models.Role
	if err := db.WithContext(ctx("t1")).Where("`key` = ? AND tenant_id = ?", "shared", "t1").First(&t1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx("t1")).Delete(&t1).Error; err != nil {
		t.Fatalf("t1 delete error: %v", err)
	}

	var count int64
	if err := db.Model(&models.Role{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows after t1 delete = %d, want 1 (t2's row must survive)", count)
	}
	var survivor models.Role
	if err := db.First(&survivor).Error; err != nil {
		t.Fatal(err)
	}
	if survivor.TenantID != "t2" {
		t.Fatalf("survivor tenant = %q, want t2", survivor.TenantID)
	}
}

func TestTenantScope_Create_BackfillsBlankTenantID(t *testing.T) {
	db, ctx := withTenantDB(t, &models.Role{})
	// TenantID left blank on the model — callback should fill it from ctx.
	if err := db.WithContext(ctx("acme")).Create(&models.Role{Key: "owner", Name: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	var got models.Role
	if err := db.Where("`key` = ?", "owner").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.TenantID != "acme" {
		t.Fatalf("backfilled tenant = %q, want acme", got.TenantID)
	}
}

func TestTenantScope_Create_PreservesExplicitTenantID(t *testing.T) {
	db, ctx := withTenantDB(t, &models.Role{})
	// Caller sets TenantID explicitly; callback must NOT overwrite it.
	if err := db.WithContext(ctx("acme")).Create(&models.Role{TenantModel: models.TenantModel{TenantID: "manual"}, Key: "owner", Name: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	var got models.Role
	if err := db.Where("`key` = ?", "owner").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.TenantID != "manual" {
		t.Fatalf("explicit tenant overwritten = %q, want manual", got.TenantID)
	}
}

func TestTenantScope_NoResolver_NoScope(t *testing.T) {
	// installTenantScope with a nil resolver is a no-op — used to
	// confirm opt-out paths (e.g. super-admin tooling) still work.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Role{}); err != nil {
		t.Fatal(err)
	}
	installTenantScope(db, nil)

	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "x", Name: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "x", Name: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.Role{}).Where("`key` = ?", "x").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rows without resolver = %d, want 2 (no scope)", count)
	}
}

func TestTenantScope_LoginPath_NoTenantInCtx_BypassesScope(t *testing.T) {
	db, ctx := withTenantDB(t, &models.User{})
	// Two users in two tenants sharing the same uid — exactly the
	// case AuthService.Login has to resolve before any tenant is known.
	if err := db.Create(&models.User{TenantModel: models.TenantModel{TenantID: "t1"}, UID: "u0001", Username: "alice", RoleKey: "admin", DeptID: 1, Password: "pass1234"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{TenantModel: models.TenantModel{TenantID: "t2"}, UID: "u0001", Username: "alice", RoleKey: "admin", DeptID: 1, Password: "pass1234"}).Error; err != nil {
		t.Fatal(err)
	}

	// No tenant on ctx — simulating the Login RPC, which JWT-middleware
	// lets through without populating claims.
	var u models.User
	if err := db.WithContext(ctx("")).Where("username = ?", "alice").First(&u).Error; err != nil {
		t.Fatalf("login lookup failed: %v", err)
	}
	if u.Username != "alice" {
		t.Fatalf("login saw %+v", u)
	}
}

func TestTenantScope_MenuWithoutTenantID_Unaffected(t *testing.T) {
	db, ctx := withTenantDB(t, &models.Menu{})
	if err := db.WithContext(ctx("t1")).Create(&models.Menu{Name: "dashboard", Component: "Dashboard", Uri: "/dashboard"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx("t2")).Create(&models.Menu{Name: "settings", Component: "Settings", Uri: "/settings"}).Error; err != nil {
		t.Fatal(err)
	}

	// t2 reading must see BOTH menus — Menu embeds BaseModel, not
	// TenantModel, so the callback bails and no scope is added.
	var count int64
	if err := db.WithContext(ctx("t2")).Model(&models.Menu{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("menus visible to t2 = %d, want 2 (Menu is non-tenant)", count)
	}
}

func TestTenantScope_CreateSlice_BackfillsAll(t *testing.T) {
	db, ctx := withTenantDB(t, &models.Role{})
	roles := []models.Role{
		{Key: "a", Name: "A"},
		{Key: "b", Name: "B"},
	}
	if err := db.WithContext(ctx("acme")).Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.TenantID != "acme" {
			t.Fatalf("row %+v tenant = %q, want acme", r, r.TenantID)
		}
	}
	var count int64
	if err := db.Model(&models.Role{}).Where("tenant_id = ?", "acme").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rows in acme = %d, want 2", count)
	}
}

// withClaimsDB is the production-shaped wiring: installTenantScope
// uses the default FromClaimsResolver, and tests put *auth.Claims
// onto ctx via mwauth.NewContext — i.e. exactly what the JWT
// middleware does at runtime. This catches regressions in
// FromClaimsResolver that withTenantDB's StoreOnContext path misses.
func withClaimsDB(t *testing.T, ms ...any) (*gorm.DB, func(*auth.Claims) context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) > 0 {
		if err := db.AutoMigrate(ms...); err != nil {
			t.Fatal(err)
		}
	}
	installTenantScope(db, middleware.FromClaimsResolver)
	return db, func(c *auth.Claims) context.Context {
		return mwauth.NewContext(context.Background(), c)
	}
}

func TestTenantScope_ClaimsResolver_ScopesRead(t *testing.T) {
	db, ctx := withClaimsDB(t, &models.Role{})
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "shared", Name: "t1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "shared", Name: "t2"}).Error; err != nil {
		t.Fatal(err)
	}

	// t1's request — claims carries TenantID="t1".
	t1Claims := &auth.Claims{TenantID: "t1"}
	var got models.Role
	if err := db.WithContext(ctx(t1Claims)).Where("`key` = ?", "shared").First(&got).Error; err != nil {
		t.Fatalf("t1 first: %v", err)
	}
	if got.TenantID != "t1" || got.Name != "t1" {
		t.Fatalf("t1 saw %+v, want t1/t1", got)
	}

	// t2's request — same code, different tenant.
	t2Claims := &auth.Claims{TenantID: "t2"}
	var got2 models.Role
	if err := db.WithContext(ctx(t2Claims)).Where("`key` = ?", "shared").First(&got2).Error; err != nil {
		t.Fatalf("t2 first: %v", err)
	}
	if got2.TenantID != "t2" || got2.Name != "t2" {
		t.Fatalf("t2 saw %+v, want t2/t2", got2)
	}
}

func TestTenantScope_ClaimsResolver_NoClaims_BypassesScope(t *testing.T) {
	// Simulates AuthService.Login: the request reaches the handler
	// without JWT claims (login path is allowlisted in the middleware).
	// FromClaimsResolver returns "" and the callback bails.
	db, _ := withClaimsDB(t, &models.Role{})
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "shared", Name: "t1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "shared", Name: "t2"}).Error; err != nil {
		t.Fatal(err)
	}

	// No claims — background ctx, like Login.
	var got models.Role
	if err := db.Where("`key` = ?", "shared").First(&got).Error; err != nil {
		t.Fatalf("login-shaped lookup failed: %v", err)
	}
	// First row wins; the point is that the lookup did NOT add a
	// tenant filter, which is the contract for the login path.
	if got.Key != "shared" {
		t.Fatalf("got %+v", got)
	}
}

func TestTenantScope_ClaimsResolver_BackfillsCreate(t *testing.T) {
	// Confirms the Create backfill reads FromClaimsResolver, not
	// the StoreOnContext path. Without an explicit TenantID on the
	// model, the row must land in the claims' tenant.
	db, ctx := withClaimsDB(t, &models.Role{})
	if err := db.WithContext(ctx(&auth.Claims{TenantID: "acme"})).Create(&models.Role{Key: "owner", Name: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	var got models.Role
	if err := db.Where("`key` = ?", "owner").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.TenantID != "acme" {
		t.Fatalf("backfilled tenant = %q, want acme", got.TenantID)
	}
}
