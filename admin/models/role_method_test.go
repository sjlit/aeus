package models

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newRolePermDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Role{}, &RolePermission{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// newRolePermUserDB mirrors newRolePermDB but also migrates sys_users so
// tests can exercise the user-side cascade in BeforeUpdate.
func newRolePermUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Role{}, &RolePermission{}, &User{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRole_ReplacePermissions_Success(t *testing.T) {
	db := newRolePermDB(t)
	if err := db.Create(&Role{Key: "admin", Name: "Admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "menu", Data: "old"}).Error; err != nil {
		t.Fatal(err)
	}

	err := (&Role{}).ReplacePermissions(db, context.Background(), "admin",
		[]string{"user", "role"},
		[]string{"user:create"},
	)
	if err != nil {
		t.Fatal(err)
	}

	var rows []RolePermission
	if err := db.Where("role_key = ?", "admin").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d, want 3 RolePermission rows (2 menu + 1 permission)", len(rows))
	}
}

func TestRole_ReplacePermissions_Empty(t *testing.T) {
	db := newRolePermDB(t)
	if err := db.Create(&Role{Key: "admin", Name: "Admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "menu", Data: "old"}).Error; err != nil {
		t.Fatal(err)
	}

	err := (&Role{}).ReplacePermissions(db, context.Background(), "admin", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var n int64
	db.Model(&RolePermission{}).Where("role_key = ?", "admin").Count(&n)
	if n != 0 {
		t.Fatalf("got %d, want 0 perms after empty replace", n)
	}
}

// TestRole_BeforeUpdate_SoftDelete_PurgesPerms pins the bug that
// AfterDelete doesn't fire on GORM's soft-delete path (Update on
// deleted_at). Without this branch in BeforeUpdate, soft-deleting a role
// would leave orphan RolePermission rows that come back to life if the
// row is restored.
func TestRole_BeforeUpdate_SoftDelete_PurgesPerms(t *testing.T) {
	db := newRolePermDB(t)
	if err := db.Create(&Role{
		TenantModel: TenantModel{TenantID: "t1"},
		Key:         "admin",
		Name:        "Admin",
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, p := range []RolePermission{
		{TenantModel: TenantModel{TenantID: "t1"}, RoleKey: "admin", Type: "menu", Data: "user"},
		{TenantModel: TenantModel{TenantID: "t1"}, RoleKey: "admin", Type: "menu", Data: "role"},
	} {
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Load first so the hook sees a non-zero m.ID; this matches the
	// service-layer usage (fetch, mutate, Save/Update).
	var role Role
	if err := db.Where("`key` = ?", "admin").First(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&role).Update("deleted_at", gorm.DeletedAt{Time: time.Now(), Valid: true}).Error; err != nil {
		t.Fatal(err)
	}

	var n int64
	db.Model(&RolePermission{}).Where("role_key = ?", "admin").Count(&n)
	if n != 0 {
		t.Fatalf("soft-delete must purge RolePermission rows, got %d", n)
	}
}

// TestRole_BeforeUpdate_RenameSyncsUsers guards the historical bug that
// sys_users.role fell out of sync after a Role rename. The hook must
// propagate newKey into both sys_role_permissions and sys_users.
func TestRole_BeforeUpdate_RenameSyncsUsers(t *testing.T) {
	db := newRolePermUserDB(t)
	if err := db.Create(&Role{
		TenantModel: TenantModel{TenantID: "t1"},
		Key:         "admin",
		Name:        "Admin",
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, p := range []RolePermission{
		{TenantModel: TenantModel{TenantID: "t1"}, RoleKey: "admin", Type: "menu", Data: "user"},
		{TenantModel: TenantModel{TenantID: "t1"}, RoleKey: "admin", Type: "permission", Data: "x:read"},
	} {
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&User{
		TenantModel: TenantModel{TenantID: "t1"},
		UID:         "u1",
		Username:    "alice",
		RoleKey:     "admin",
	}).Error; err != nil {
		t.Fatal(err)
	}

	var role Role
	if err := db.Where("`key` = ?", "admin").First(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&role).Update("`key`", "ops").Error; err != nil {
		t.Fatal(err)
	}

	var n int64
	db.Model(&RolePermission{}).Where("role_key = ?", "ops").Count(&n)
	if n != 2 {
		t.Fatalf("got %d, want 2 perms under ops", n)
	}
	var u User
	if err := db.Where("uid = ?", "u1").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.RoleKey != "ops" {
		t.Fatalf("user role_key not synced, got %q", u.RoleKey)
	}
}
