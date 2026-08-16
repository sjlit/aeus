package admin

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/admin/service"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Role{},
		&models.User{},
		&models.LoginLog{},
		&models.Menu{},
		&models.Permission{},
		&models.RolePermission{},
		&models.Tenant{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSeed_CreatesAdminRoleAndUser(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var role models.Role
	if err := db.Where("key = ?", "admin").First(&role).Error; err != nil {
		t.Fatalf("role not found: %v", err)
	}
	if role.Name != "系统管理员" {
		t.Errorf("role.Name = %q, want 系统管理员", role.Name)
	}
	if !role.Builtin {
		t.Error("role.Builtin should be true")
	}
	if role.TenantID == "" {
		t.Error("role.TenantID should be set")
	}

	var user models.User
	if err := db.Where("uid = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("user not found: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("user.Username = %q, want admin", user.Username)
	}
	if user.RoleKey != "admin" {
		t.Errorf("user.RoleKey = %q, want admin", user.RoleKey)
	}
	if user.TenantID != role.TenantID {
		t.Errorf("user.TenantID = %q, want %q", user.TenantID, role.TenantID)
	}
	if !user.ValidatePassword("admin123") {
		t.Error("user.ValidatePassword(\"admin123\") should be true")
	}
	if user.ValidatePassword("wrong") {
		t.Error("user.ValidatePassword(\"wrong\") should be false")
	}
}

func TestSeed_Idempotent(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("first Seed: %v", err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("second Seed should be no-op, got: %v", err)
	}

	// count rows — must still be 1 role / 1 user
	var roleCount, userCount int64
	db.Model(&models.Role{}).Count(&roleCount)
	db.Model(&models.User{}).Count(&userCount)
	if roleCount != 1 {
		t.Errorf("role count = %d, want 1", roleCount)
	}
	if userCount != 1 {
		t.Errorf("user count = %d, want 1", userCount)
	}
}

func TestSeed_RequiresDB(t *testing.T) {
	if err := Seed(nil, "admin", "admin"); err == nil {
		t.Error("Seed(nil, ...) should error")
	}
}

func TestSeed_RequiresNonEmptyArgs(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db, "", "admin"); err == nil {
		t.Error("Seed with empty user should error")
	}
	if err := Seed(db, "admin", ""); err == nil {
		t.Error("Seed with empty password should error")
	}
}

// seedGrantCount returns the number of sys_role_permissions rows for the
// given role key, split by type, so tests can assert exact grant state
// without reaching into the diff internals.
func seedGrantCount(t *testing.T, db *gorm.DB, roleKey string) (menus, perms int64) {
	t.Helper()
	if err := db.Model(&models.RolePermission{}).
		Where("role_key = ? AND type = ?", roleKey, models.RolePermissionTypeMenu).
		Count(&menus).Error; err != nil {
		t.Fatalf("count menu grants: %v", err)
	}
	if err := db.Model(&models.RolePermission{}).
		Where("role_key = ? AND type = ?", roleKey, models.RolePermissionTypePermission).
		Count(&perms).Error; err != nil {
		t.Fatalf("count permission grants: %v", err)
	}
	return
}

// TestSeed_GrantsFullCatalogToAdminRole pins the super-admin contract:
// a fresh Seed grants the seeded role every row of the global catalog —
// all menus regardless of hidden, and all sys_permissions rows
// regardless of type — as tenant-scoped junction rows.
func TestSeed_GrantsFullCatalogToAdminRole(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&models.Menu{Component: "SysUsers", Name: "用户管理", Uri: "/system/sys-users"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Menu{Component: "SysRoles", Name: "角色管理", Uri: "/system/sys-roles"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Permission{Type: "api", Data: "POST /system/sys_user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Permission{Type: "button", Data: "btn:user:export"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var role models.Role
	if err := db.Where("key = ?", "admin").First(&role).Error; err != nil {
		t.Fatalf("role not found: %v", err)
	}
	if !role.IsSuper {
		t.Error("seeded role.IsSuper should be true")
	}
	menus, perms := seedGrantCount(t, db, "admin")
	if menus != 2 || perms != 2 {
		t.Fatalf("grants = %d menus / %d perms, want 2/2", menus, perms)
	}
	// every junction row must carry the role's tenant and the catalog
	// row's data, including the non-api catalog type.
	var rp models.RolePermission
	if err := db.Where("role_key = ? AND type = ? AND data = ?", "admin", models.RolePermissionTypePermission, "btn:user:export").First(&rp).Error; err != nil {
		t.Fatalf("button-type catalog grant missing: %v", err)
	}
	if rp.TenantID != role.TenantID {
		t.Errorf("grant tenant = %q, want role tenant %q", rp.TenantID, role.TenantID)
	}
}

// TestSeed_TopUp_GrantsNewCatalogRows pins the converge contract: a
// re-run of Seed grants catalog rows that appeared after the previous
// run without duplicating existing grants.
func TestSeed_TopUp_GrantsNewCatalogRows(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&models.Menu{Component: "MenuA", Name: "A", Uri: "/a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("first Seed: %v", err)
	}
	menus, perms := seedGrantCount(t, db, "admin")
	if menus != 1 || perms != 0 {
		t.Fatalf("after first Seed: %d menus / %d perms, want 1/0", menus, perms)
	}

	// catalog grows: a new menu and a new permission appear (upgrade
	// registered a new model).
	if err := db.Create(&models.Menu{Component: "MenuB", Name: "B", Uri: "/b"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Permission{Type: "api", Data: "GET /system/menu_b"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("second Seed: %v", err)
	}

	menus, perms = seedGrantCount(t, db, "admin")
	if menus != 2 || perms != 1 {
		t.Fatalf("after second Seed: %d menus / %d perms, want 2/1", menus, perms)
	}
}

// TestSeed_TopUp_HealsRevokedGrants pins the flip side of converge: a
// grant deleted from the junction is restored by the next Seed run, so
// the super-admin role always owns the full catalog (manual edits are
// rejected at the API layer instead).
func TestSeed_TopUp_HealsRevokedGrants(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&models.Menu{Component: "MenuA", Name: "A", Uri: "/a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("first Seed: %v", err)
	}
	// simulate a revoked grant (e.g. an operator deleted it out-of-band)
	if err := db.Where("role_key = ? AND type = ?", "admin", models.RolePermissionTypeMenu).
		Delete(&models.RolePermission{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("second Seed: %v", err)
	}
	menus, _ := seedGrantCount(t, db, "admin")
	if menus != 1 {
		t.Fatalf("after heal: %d menu grants, want 1", menus)
	}
}

// TestSeed_UpgradesExistingBuiltinRoleToSuper covers the pre-existing
// bootstrap: an admin role created before IsSuper existed (Builtin,
// is_super false) is upgraded on the next Seed run and then receives
// the full catalog.
func TestSeed_UpgradesExistingBuiltinRoleToSuper(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&models.Role{
		Key: "admin", Name: "系统管理员", Status: "enabled", Builtin: true, DataScope: "all",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Menu{Component: "MenuA", Name: "A", Uri: "/a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var role models.Role
	if err := db.Where("key = ?", "admin").First(&role).Error; err != nil {
		t.Fatalf("role not found: %v", err)
	}
	if !role.IsSuper {
		t.Error("existing builtin admin role should be upgraded to IsSuper")
	}
	menus, _ := seedGrantCount(t, db, "admin")
	if menus != 1 {
		t.Fatalf("menu grants = %d, want 1 after upgrade", menus)
	}
}

// TestSeed_LeavesNonBuiltinAdminRoleAlone pins the converge boundary:
// a pre-existing Key="admin" role that is NOT builtin is the
// operator's own construct — Seed must not upgrade it to IsSuper (and
// therefore must not grant it the full catalog), only ensure the user.
func TestSeed_LeavesNonBuiltinAdminRoleAlone(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&models.Role{Key: "admin", Name: "自建管理员", Builtin: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Menu{Component: "MenuA", Name: "A", Uri: "/a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var role models.Role
	if err := db.Where("key = ?", "admin").First(&role).Error; err != nil {
		t.Fatalf("role not found: %v", err)
	}
	if role.IsSuper {
		t.Error("non-builtin admin role must not be upgraded to IsSuper")
	}
	menus, perms := seedGrantCount(t, db, "admin")
	if menus != 0 || perms != 0 {
		t.Fatalf("grants = %d menus / %d perms, want 0/0 for non-builtin role", menus, perms)
	}
}

// TestSeed_CreatesUserWhenRoleExists covers the converge contract on
// the user side: a database that already has the admin role (e.g. an
// externally bootstrapped one) but no admin user gets the user created
// on the next Seed run, bound to the role's tenant.
func TestSeed_CreatesUserWhenRoleExists(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&models.Role{Key: "admin", Name: "系统管理员", Builtin: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var role models.Role
	if err := db.Where("key = ?", "admin").First(&role).Error; err != nil {
		t.Fatalf("role not found: %v", err)
	}
	var user models.User
	if err := db.Where("uid = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("user not found: %v", err)
	}
	if user.RoleKey != "admin" || user.TenantID != role.TenantID {
		t.Errorf("user role/tenant = %q/%q, want admin/%q", user.RoleKey, user.TenantID, role.TenantID)
	}
}

// TestSeed_ThenLoginFlow verifies the happy-path the P0 integration
// spec promises: Seed creates the admin user, AuthService authenticates
// it against the same DB and returns a LoginResponse whose tenant_id
// echoes the seeded tenant.
func TestSeed_ThenLoginFlow(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	svc := service.NewAuthService(
		service.WithAuthServiceDB(db),
		service.WithAuthSecret("test-secret"),
	)
	res, err := svc.Login(t.Context(), &pb.LoginRequest{Username: "admin", Password: "admin123"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.AccessToken == "" {
		t.Error("Login returned empty access_token")
	}
	if res.TenantId == "" {
		t.Error("LoginResponse.TenantId should not be empty after Seed")
	}
	// tenant_id should match the seeded user's tenant
	var user models.User
	if err := db.Where("uid = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("user lookup: %v", err)
	}
	if res.TenantId != user.TenantID {
		t.Errorf("TenantId = %q, want %q", res.TenantId, user.TenantID)
	}
	if res.TenantName != "默认租户" {
		t.Errorf("TenantName = %q, want 默认租户 (Seed converges the sys_tenants row)", res.TenantName)
	}
}
