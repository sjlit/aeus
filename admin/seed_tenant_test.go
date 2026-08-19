package admin

import (
	"testing"

	"github.com/sjlit/aeus/admin/models"
)

// TestSeed_CreatesTenantRow pins the tenant converge contract: after
// Seed the sys_tenants table has an entity row whose id matches the
// admin role/user's tenant_id, so resolveTenantName returns a real
// name instead of falling back to the uuid.
func TestSeed_CreatesTenantRow(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var role models.Role
	if err := db.Where("key = ?", "admin").First(&role).Error; err != nil {
		t.Fatalf("role not found: %v", err)
	}
	var tenant models.Tenant
	if err := db.Where("id = ?", role.TenantID).First(&tenant).Error; err != nil {
		t.Fatalf("tenant row not found for role tenant %q: %v", role.TenantID, err)
	}
	if tenant.Name != "默认租户" {
		t.Errorf("tenant.Name = %q, want 默认租户", tenant.Name)
	}
	if tenant.Status != "enabled" {
		t.Errorf("tenant.Status = %q, want enabled", tenant.Status)
	}

	var user models.User
	if err := db.Where("uid = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("user not found: %v", err)
	}
	if user.TenantID != tenant.ID {
		t.Errorf("user.TenantID = %q, want tenant id %q", user.TenantID, tenant.ID)
	}
}

// TestSeed_ConvergesLegacyOrphanTenant covers the pre-tenant-model
// upgrade path: a database whose admin role/user already live on an
// orphan uuid (no sys_tenants row) gets the entity row created for
// that same uuid on the next Seed run.
func TestSeed_ConvergesLegacyOrphanTenant(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "legacy-uuid"},
		Key:         "admin",
		Name:        "系统管理员",
		Status:      "enabled",
		Builtin:     true,
		IsSuper:     true,
		DataScope:   "all",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{
		TenantModel: models.TenantModel{TenantID: "legacy-uuid"},
		UID:         "admin",
		Username:    "admin",
		Password:    "admin123",
		RoleKey:     "admin",
		Status:      "normal",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var tenant models.Tenant
	if err := db.Where("id = ?", "legacy-uuid").First(&tenant).Error; err != nil {
		t.Fatalf("tenant row not created for legacy tenant id: %v", err)
	}

	// Snapshot the tenant count after the first Seed.  The default
	// tenant (00000000-...) is also materialized by Seed, so the
	// absolute count depends on whether the legacy uuid differs from
	// it — what the test really pins is that a second Seed leaves the
	// count untouched (Seed must converge, not duplicate).
	var firstCount int64
	if err := db.Model(&models.Tenant{}).Count(&firstCount).Error; err != nil {
		t.Fatal(err)
	}

	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("second Seed: %v", err)
	}
	var secondCount int64
	if err := db.Model(&models.Tenant{}).Count(&secondCount).Error; err != nil {
		t.Fatal(err)
	}
	if secondCount != firstCount {
		t.Errorf("tenant rows: first Seed = %d, second Seed = %d (Seed must converge, not duplicate)", firstCount, secondCount)
	}
}
