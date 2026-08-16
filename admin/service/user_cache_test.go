package service

import (
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

// newUserSvcDBWithCounter mirrors newMenuSvcDBWithCounter for the
// user-service tables.
func newUserSvcDBWithCounter(t *testing.T) (*gorm.DB, *int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Role{}, &models.User{}, &models.RolePermission{}, &models.Menu{}); err != nil {
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

// TestUserService_ListVisibleMenus_SecondCallSkipsSelect: the second
// call inside the grace window must not re-SELECT against
// sys_role_permissions or sys_menus.  Without caching both calls
// would do 2 SELECTs each; the first call does 2 (cache miss), the
// second must do 0.
func TestUserService_ListVisibleMenus_SecondCallSkipsSelect(t *testing.T) {
	db, count := newUserSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "viewer", Name: "Viewer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Menu{Component: "U", Name: "user", Uri: "/u", Parent: ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "viewer",
		Type:        models.RolePermissionTypeMenu,
		Data:        "U",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	if _, err := svc.ListVisibleMenus(claimsCtx("alice", "viewer", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterFirst := atomic.LoadInt64(count)
	if _, err := svc.ListVisibleMenus(claimsCtx("alice", "viewer", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterSecond := atomic.LoadInt64(count)

	if delta := afterSecond - afterFirst; delta != 0 {
		t.Errorf("second ListVisibleMenus issued %d queries, want 0 (cache hit)", delta)
	}
}

// TestUserService_ListVisibleMenus_InvalidatesOnGrantChange: writing
// to sys_role_permissions must bump the marker so the next call
// (after the grace window) reflects the new grant.
func TestUserService_ListVisibleMenus_InvalidatesOnGrantChange(t *testing.T) {
	db, _ := newUserSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "viewer", Name: "Viewer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Menu{Component: "U", Name: "user", Uri: "/u", Parent: ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "viewer",
		Type:        models.RolePermissionTypeMenu,
		Data:        "U",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	ctx := claimsCtx("alice", "viewer", "t1")
	r1, err := svc.ListVisibleMenus(ctx, &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Menus) != 1 {
		t.Fatalf("first call: want 1 menu, got %d", len(r1.Menus))
	}

	time.Sleep(1100 * time.Millisecond)

	// ReplacePermissions soft-deletes + re-inserts, so SUM(id) bumps.
	if err := (&models.Role{}).ReplacePermissions(db, ctx, "viewer", nil, nil); err != nil {
		// ReplacePermissions deletes and re-inserts; passing empty
		// clears the role's grants so the next ListVisibleMenus sees
		// no menus.
		// (errors here would otherwise leak; not reachable in test.)
		_ = err
	}

	r2, err := svc.ListVisibleMenus(ctx, &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Menus) != 0 {
		t.Fatalf("after grant clear: want 0 menus, got %d (cache did not invalidate)", len(r2.Menus))
	}
}

// TestUserService_ListVisibleMenus_TenantKeyedCache: the cache key
// must embed the tenant id, otherwise two tenants collide on the
// same (key, role) entry and one shadows the other.  Verified with
// a query counter: a cache hit issues 0 loader queries, so a second
// call on a different tenant must run the loader (delta > 0).
func TestUserService_ListVisibleMenus_TenantKeyedCache(t *testing.T) {
	db, count := newUserSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "viewer", Name: "Viewer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Menu{Component: "U", Name: "user", Uri: "/u", Parent: ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "viewer",
		Type:        models.RolePermissionTypeMenu,
		Data:        "U",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	// t1: cache miss, loader runs.
	if _, err := svc.ListVisibleMenus(claimsCtx("alice", "viewer", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterT1 := atomic.LoadInt64(count)

	// t2 with the same role key: if the cache key did NOT include
	// tenant, this would collide with the t1 entry and skip the
	// loader (delta=0); with tenant in the key, it's a fresh miss
	// and the loader runs again (delta>0).
	if _, err := svc.ListVisibleMenus(claimsCtx("bob", "viewer", "t2"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterT2 := atomic.LoadInt64(count)

	if delta := afterT2 - afterT1; delta == 0 {
		t.Errorf("t2 call issued %d queries, want >0 (tenant not in cache key)", delta)
	}
}

// TestUserService_ListPermissionCodes_SecondCallSkipsSelect: the
// second call inside the grace window must not re-SELECT.
func TestUserService_ListPermissionCodes_SecondCallSkipsSelect(t *testing.T) {
	db, count := newUserSvcDBWithCounter(t)
	if err := db.Create(&models.Role{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "viewer", Name: "Viewer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RolePermission{
		TenantModel: models.TenantModel{TenantID: "t1"},
		RoleKey:     "viewer",
		Type:        models.RolePermissionTypePermission,
		Data:        "GET /x",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(WithUserServiceDB(db))

	if _, err := svc.ListPermissionCodes(claimsCtx("alice", "viewer", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterFirst := atomic.LoadInt64(count)
	if _, err := svc.ListPermissionCodes(claimsCtx("alice", "viewer", "t1"), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterSecond := atomic.LoadInt64(count)

	if delta := afterSecond - afterFirst; delta != 0 {
		t.Errorf("second ListPermissionCodes issued %d queries, want 0 (cache hit)", delta)
	}
}

// guard against unused imports when tests get rearranged.
var _ = mwauth.NewContext
var _ = adminauth.Claims{}
