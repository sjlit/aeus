package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	ghttp "github.com/sjlit/aeus/transport/http"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

func openConvergeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	return db
}

func assertRoleKeyCreateRequired(t *testing.T, db *gorm.DB) {
	t.Helper()
	var row schema.Schema
	if err := db.Where(
		"module_name = ? AND table_name = ? AND column = ?",
		"system", "sys_users", "role_key",
	).First(&row).Error; err != nil {
		t.Fatalf("load role_key schema: %v", err)
	}
	if !slices.Contains(row.Rules.Required, schema.ScenarioCreate) {
		t.Fatalf("role_key required scenarios = %v, want create", row.Rules.Required)
	}
}

// TestSetup_SingleTenantOmitsTenantManagement pins the default
// deployment shape: no tenant menu, no /system/sys_tenants REST route.
// The sys_tenants table still exists because Seed writes the default
// tenant row used by login.
func TestSetup_SingleTenantOmitsTenantManagement(t *testing.T) {
	db := openConvergeDB(t)
	httpSrv := ghttp.New()
	if err := New(WithDB(db), WithRouter(httpSrv)).Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var menu models.Menu
	err := db.Where("component = ?", "SystemSysTenants").First(&menu).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("single-tenant tenant menu lookup = %v, want record not found", err)
	}
	if !db.Migrator().HasTable("sys_tenants") {
		t.Fatal("sys_tenants table must exist even in single-tenant mode")
	}
	var permissions int64
	if err := db.Model(&models.Permission{}).
		Where("data IN ?", tenantResourcePermissionData).
		Count(&permissions).Error; err != nil {
		t.Fatalf("count tenant permissions: %v", err)
	}
	if permissions != 0 {
		t.Fatalf("single-tenant tenant permissions = %d, want 0", permissions)
	}

	rec := httptest.NewRecorder()
	httpSrv.Engine().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/system/sys_tenants", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /system/sys_tenants status = %d, want 404", rec.Code)
	}
}

// TestSetup_MultiTenantRegistersTenantManagement is the counterpart:
// enabling tenant scoping brings the menu and REST route back.
func TestSetup_MultiTenantRegistersTenantManagement(t *testing.T) {
	db, httpSrv := newTenantServer(t)

	var menu models.Menu
	if err := db.Where("component = ?", "SystemSysTenants").First(&menu).Error; err != nil {
		t.Fatalf("tenant menu missing in multi-tenant mode: %v", err)
	}
	if menu.Parent != "SystemSettings" {
		t.Errorf("tenant menu parent = %q, want SystemSettings", menu.Parent)
	}
	code, _ := searchEndpoint(t, httpSrv, "/system/sys_tenants")
	if code != 0 {
		t.Fatalf("tenant search code = %d, want 0", code)
	}
}

// TestSetup_SingleTenantPrunesLegacyTenantMenu covers the upgrade path:
// a database bootstrapped in an earlier version still has the tenant
// menu row, and switching to single-tenant mode must remove it. Turning
// multi-tenant mode back on recreates it.
func TestSetup_SingleTenantPrunesLegacyTenantMenu(t *testing.T) {
	db := openConvergeDB(t)

	multi := New(WithDB(db), WithRouter(ghttp.New()), WithMultiTenant(true))
	if err := multi.Setup(context.Background()); err != nil {
		t.Fatalf("multi Setup: %v", err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var menu models.Menu
	if err := db.Where("component = ?", "SystemSysTenants").First(&menu).Error; err != nil {
		t.Fatalf("tenant menu missing after multi Setup: %v", err)
	}
	var grants int64
	if err := db.Model(&models.RolePermission{}).
		Where("type = ? AND data = ?", models.RolePermissionTypeMenu, "SystemSysTenants").
		Count(&grants).Error; err != nil {
		t.Fatalf("count tenant menu grants: %v", err)
	}
	if grants == 0 {
		t.Fatal("expected Seed to grant the tenant menu to the super role")
	}
	var permissions int64
	if err := db.Model(&models.Permission{}).
		Where("data IN ?", tenantResourcePermissionData).
		Count(&permissions).Error; err != nil {
		t.Fatalf("count tenant permissions: %v", err)
	}
	if permissions == 0 {
		t.Fatal("expected multi-tenant Setup to catalog tenant CRUD permissions")
	}

	single := New(WithDB(db), WithRouter(ghttp.New()))
	if err := single.Setup(context.Background()); err != nil {
		t.Fatalf("single Setup: %v", err)
	}
	menu = models.Menu{}
	err := db.Unscoped().Where("component = ?", "SystemSysTenants").First(&menu).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("tenant menu after single Setup = %v (deleted_at=%v), want hard delete", err, menu.DeletedAt)
	}
	grants = 0
	if err := db.Model(&models.RolePermission{}).
		Where("type = ? AND data = ?", models.RolePermissionTypeMenu, "SystemSysTenants").
		Count(&grants).Error; err != nil {
		t.Fatalf("count tenant menu grants after cleanup: %v", err)
	}
	if grants != 0 {
		t.Fatalf("tenant menu grants after cleanup = %d, want 0", grants)
	}
	permissions = 0
	if err := db.Model(&models.Permission{}).
		Where("data IN ?", tenantResourcePermissionData).
		Count(&permissions).Error; err != nil {
		t.Fatalf("count tenant permissions after cleanup: %v", err)
	}
	if permissions != 0 {
		t.Fatalf("tenant permissions after cleanup = %d, want 0", permissions)
	}

	multiAgain := New(WithDB(db), WithRouter(ghttp.New()), WithMultiTenant(true))
	if err := multiAgain.Setup(context.Background()); err != nil {
		t.Fatalf("second multi Setup: %v", err)
	}
	menu = models.Menu{}
	if err := db.Where("component = ?", "SystemSysTenants").First(&menu).Error; err != nil {
		t.Fatalf("tenant menu not recreated after enabling multi-tenant: %v", err)
	}
	permissions = 0
	if err := db.Model(&models.Permission{}).
		Where("data IN ?", tenantResourcePermissionData).
		Count(&permissions).Error; err != nil {
		t.Fatalf("count tenant permissions after re-enabling multi-tenant: %v", err)
	}
	if permissions != int64(len(tenantResourcePermissionData)) {
		t.Fatalf("tenant permissions after re-enabling multi-tenant = %d, want %d",
			permissions, len(tenantResourcePermissionData))
	}
}

// TestSetup_SingleTenantKeepsCustomizedTenantMenu guards the cleanup's
// narrow match: an operator who repurposed the component with a custom
// name/path keeps that row.
func TestSetup_SingleTenantKeepsCustomizedTenantMenu(t *testing.T) {
	db := openConvergeDB(t)
	if err := db.AutoMigrate(&models.Menu{}); err != nil {
		t.Fatalf("migrate menu: %v", err)
	}
	custom := models.Menu{
		Component: "SystemSysTenants",
		Name:      "自定义租户页",
		Uri:       "/custom/tenant",
		ViewPath:  "@/views/custom/tenant/Index.vue",
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatalf("create custom tenant menu: %v", err)
	}
	if err := New(WithDB(db), WithRouter(ghttp.New())).Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	var got models.Menu
	if err := db.Where("component = ?", "SystemSysTenants").First(&got).Error; err != nil {
		t.Fatalf("custom tenant menu was removed: %v", err)
	}
	if got.Name != custom.Name {
		t.Fatalf("custom tenant menu name = %q, want %q", got.Name, custom.Name)
	}
}

// TestSetup_SyncsLegacyRoleKeyRule pins the schema metadata backfill.
// rest/v3 only inserts new schema rows, so a database created before
// User.RoleKey gained rule:"required:create" keeps the old empty Rules.
// Setup must patch every matching row, including tenant clones.
func TestSetup_SyncsLegacyRoleKeyRule(t *testing.T) {
	db := openConvergeDB(t)
	if err := New(WithDB(db), WithRouter(ghttp.New())).Setup(context.Background()); err != nil {
		t.Fatalf("first Setup: %v", err)
	}
	assertRoleKeyCreateRequired(t, db)

	// Simulate a database initialized before the rule tag existed.
	if err := db.Model(&schema.Schema{}).
		Where("module_name = ? AND table_name = ? AND column = ?", "system", "sys_users", "role_key").
		UpdateColumn("rules", schema.Rule{}).Error; err != nil {
		t.Fatalf("clear role_key rules: %v", err)
	}

	if err := New(WithDB(db), WithRouter(ghttp.New())).Setup(context.Background()); err != nil {
		t.Fatalf("second Setup: %v", err)
	}
	assertRoleKeyCreateRequired(t, db)
}
