package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// newLoginDB migrates sys_users + sys_tenants + sys_roles and seeds one
// user on tenant t1 with a policy-compliant password and a matching
// enabled role (Login checks both statuses now).
func newLoginDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Tenant{}, &models.Role{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Key:         "admin",
		Name:        "admin",
		Status:      "enabled",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: "t1"},
		UID:         "u0001",
		Username:    "admin",
		Password:    "admin123", // BeforeCreate bcrypts it
		RoleKey:     "admin",
		Status:      "normal",
	}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func loginAsAdmin(t *testing.T, db *gorm.DB) *AuthService {
	t.Helper()
	return NewAuthService(WithAuthServiceDB(db), WithAuthSecret("test-secret"))
}

func TestLogin_RejectsDisabledTenant(t *testing.T) {
	db := newLoginDB(t)
	if err := db.Create(&models.Tenant{ID: "t1", Name: "停用租户", Status: "disabled"}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := loginAsAdmin(t, db).Login(context.Background(),
		&pb.LoginRequest{Username: "admin", Password: "admin123"})
	wantPkError(t, err, errs.CodePermissionDenied)
}

func TestLogin_AllowsEnabledTenant(t *testing.T) {
	db := newLoginDB(t)
	if err := db.Create(&models.Tenant{ID: "t1", Name: "正常租户", Status: "enabled"}).Error; err != nil {
		t.Fatal(err)
	}

	res, err := loginAsAdmin(t, db).Login(context.Background(),
		&pb.LoginRequest{Username: "admin", Password: "admin123"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TenantId != "t1" {
		t.Errorf("TenantId = %q, want t1", res.TenantId)
	}
	if res.TenantName != "正常租户" {
		t.Errorf("TenantName = %q, want 正常租户", res.TenantName)
	}
}

func TestLogin_AllowsMissingTenantRow(t *testing.T) {
	db := newLoginDB(t)
	// sys_tenants exists but has no row for t1 (legacy orphan uuid).

	res, err := loginAsAdmin(t, db).Login(context.Background(),
		&pb.LoginRequest{Username: "admin", Password: "admin123"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TenantName != "t1" {
		t.Errorf("TenantName = %q, want fallback to tenant id t1", res.TenantName)
	}
}

func TestLogin_AllowsMissingTenantTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Only sys_users + sys_roles are migrated: a database from before the
	// tenant model existed. Login must keep working (backward compat).
	if err := db.AutoMigrate(&models.User{}, &models.Role{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Key:         "admin",
		Name:        "admin",
		Status:      "enabled",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: "t1"},
		UID:         "u0001",
		Username:    "admin",
		Password:    "admin123",
		RoleKey:     "admin",
		Status:      "normal",
	}).Error; err != nil {
		t.Fatal(err)
	}

	res, err := loginAsAdmin(t, db).Login(context.Background(),
		&pb.LoginRequest{Username: "admin", Password: "admin123"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TenantName != "t1" {
		t.Errorf("TenantName = %q, want fallback to tenant id t1", res.TenantName)
	}
}
