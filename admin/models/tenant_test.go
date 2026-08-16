package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newTenantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Tenant{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTenant_BeforeCreate_GeneratesID(t *testing.T) {
	db := newTenantDB(t)

	tnt := &Tenant{Name: "测试租户", Status: "enabled"}
	if err := db.Create(tnt).Error; err != nil {
		t.Fatal(err)
	}
	if tnt.ID == "" {
		t.Fatal("Tenant.ID should be backfilled by BeforeCreate")
	}
	if _, err := uuid.Parse(tnt.ID); err != nil {
		t.Fatalf("Tenant.ID = %q is not a valid uuid: %v", tnt.ID, err)
	}
}

func TestTenant_BeforeCreate_KeepsExplicitID(t *testing.T) {
	db := newTenantDB(t)

	tnt := &Tenant{ID: "fixed-tenant-id", Name: "显式租户", Status: "enabled"}
	if err := db.Create(tnt).Error; err != nil {
		t.Fatal(err)
	}
	if tnt.ID != "fixed-tenant-id" {
		t.Fatalf("Tenant.ID = %q, want explicit value preserved", tnt.ID)
	}
}

func TestTenant_DefaultStatus(t *testing.T) {
	db := newTenantDB(t)

	tnt := &Tenant{Name: "无状态租户"}
	if err := db.Create(tnt).Error; err != nil {
		t.Fatal(err)
	}
	if tnt.Status != "enabled" {
		t.Fatalf("Tenant.Status = %q, want default enabled", tnt.Status)
	}
}

func TestTenant_NamingAndMenu(t *testing.T) {
	m := &Tenant{}
	if got := m.TableName(); got != "sys_tenants" {
		t.Errorf("TableName() = %q, want sys_tenants", got)
	}
	if got := m.ModuleName(); got != "system" {
		t.Errorf("ModuleName() = %q, want system", got)
	}
	if got := m.MenuEntry().Name; got != "租户管理" {
		t.Errorf("MenuEntry().Name = %q, want 租户管理", got)
	}
}
