package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	adminauth "github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	"gorm.io/gorm"
)

// newRoleSvcDBWithCounter mirrors the menu/user helpers for the
// role-service tables (sys_roles, sys_role_permissions).
func newRoleSvcDBWithCounter(t *testing.T) (*gorm.DB, *int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Role{}, &models.RolePermission{}, &models.Permission{}, &models.Menu{}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Callback().Query().Before("gorm:query").Register(
		"test_query_count", func(d *gorm.DB) {
			atomic.AddInt64(&count, 1)
		}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test_query_count")
	})
	return db, &count
}

// roleCtx wraps ctx with JWT claims carrying a fixed tenant.
func roleCtx(uid, role, tenant string) context.Context {
	return mwauth.NewContext(context.Background(), &adminauth.Claims{
		UID:      uid,
		Role:     role,
		TenantID: tenant,
	})
}

// TestRoleService_RolePermissions_SecondCallSkipsSelect: the second
// UNSPECIFIED call inside the grace window must not re-SELECT.
func TestRoleService_RolePermissions_SecondCallSkipsSelect(t *testing.T) {
	db, count := newRoleSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "ops", Name: "Ops"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "ops",
		Type:        models.RolePermissionTypeMenu,
		Data:        "U",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "ops",
		Type:        models.RolePermissionTypePermission,
		Data:        "GET /x",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewRoleService(WithRoleServiceDB(db))

	if _, err := svc.RolePermissions(roleCtx("alice", "ops", "t1"), &pb.RolePermissionsRequest{Role: "ops"}); err != nil {
		t.Fatal(err)
	}
	afterFirst := atomic.LoadInt64(count)
	if _, err := svc.RolePermissions(roleCtx("alice", "ops", "t1"), &pb.RolePermissionsRequest{Role: "ops"}); err != nil {
		t.Fatal(err)
	}
	afterSecond := atomic.LoadInt64(count)

	if delta := afterSecond - afterFirst; delta != 0 {
		t.Errorf("second RolePermissions issued %d queries, want 0", delta)
	}
}

// TestRoleService_RolePermissions_InvalidatesOnGrantChange: rewriting
// a role's grants must bump the SUM(id) marker so the next call
// (after the grace window) returns the new state.
func TestRoleService_RolePermissions_InvalidatesOnGrantChange(t *testing.T) {
	db, _ := newRoleSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "ops", Name: "Ops"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "ops",
		Type:        models.RolePermissionTypeMenu,
		Data:        "OLD",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewRoleService(WithRoleServiceDB(db))

	r1, err := svc.RolePermissions(roleCtx("alice", "ops", "t1"), &pb.RolePermissionsRequest{Role: "ops"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Menus) != 1 || r1.Menus[0] != "OLD" {
		t.Fatalf("first call: want [OLD], got %v", r1.Menus)
	}

	time.Sleep(1100 * time.Millisecond)

	// ReplacePermissions soft-deletes + re-inserts NEW.
	if err := (&models.Role{}).ReplacePermissions(db, context.Background(), "ops", []string{"NEW"}, nil); err != nil {
		t.Fatal(err)
	}
	r2, err := svc.RolePermissions(roleCtx("alice", "ops", "t1"), &pb.RolePermissionsRequest{Role: "ops"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Menus) != 1 || r2.Menus[0] != "NEW" {
		t.Fatalf("after rewrite: want [NEW], got %v (cache did not invalidate)", r2.Menus)
	}
}

// TestRoleService_ListRoleOptions_SecondCallSkipsSelect: tenant-scoped
// role options cache.
func TestRoleService_ListRoleOptions_SecondCallSkipsSelect(t *testing.T) {
	db, count := newRoleSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "a", Name: "A"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "b", Name: "B"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewRoleService(WithRoleServiceDB(db))

	if _, err := svc.ListRoleOptions(roleCtx("alice", "a", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterFirst := atomic.LoadInt64(count)
	if _, err := svc.ListRoleOptions(roleCtx("alice", "a", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterSecond := atomic.LoadInt64(count)

	if delta := afterSecond - afterFirst; delta != 0 {
		t.Errorf("second ListRoleOptions issued %d queries, want 0", delta)
	}
}

// TestRoleService_ListRoleOptions_TenantKeyedCache: the cache key
// must embed the tenant.  Same call, different tenant ids, must not
// collide.  This is checked by hitting the cache twice and verifying
// that a re-query on a different tenant gets a fresh loader run
// (visible via a query counter that resets per-tenant call).
func TestRoleService_ListRoleOptions_TenantKeyedCache(t *testing.T) {
	db, count := newRoleSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "a", Name: "A"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "b", Name: "B"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewRoleService(WithRoleServiceDB(db))

	if _, err := svc.ListRoleOptions(roleCtx("alice", "a", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterT1 := atomic.LoadInt64(count)

	// Different tenant = different key.  Without the tenant in the key,
	// this would be a cache hit and issue 0 queries; with it, it's a
	// miss and issues the loader's full SELECT count.
	if _, err := svc.ListRoleOptions(roleCtx("bob", "b", "t2"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterT2 := atomic.LoadInt64(count)

	if delta := afterT2 - afterT1; delta == 0 {
		t.Errorf("second tenant call issued %d queries, want >0 (tenant not in key)", delta)
	}
}
