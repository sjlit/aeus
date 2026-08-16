package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"gorm.io/gorm"
)

// newMenuSvcDBWithCounter returns a fresh in-memory DB plus a
// monotonically-increasing counter that fires on every SELECT against
// it. The callback is installed before AutoMigrate so the test's own
// setup queries count too — assertions compare deltas, not absolutes.
func newMenuSvcDBWithCounter(t *testing.T) (*gorm.DB, *int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Menu{}, &models.RolePermission{}); err != nil {
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

// TestMenuService_MenuTree_SecondCallSkipsMenuSelect: the second
// MenuTree call inside the 1s grace window must not issue a SELECT
// against sys_menus.  Without caching both calls would SELECT; the
// first call always SELECTs (cache miss), the second must not.
func TestMenuService_MenuTree_SecondCallSkipsMenuSelect(t *testing.T) {
	db, count := newMenuSvcDBWithCounter(t)
	if err := db.Create(&models.Menu{Name: "x", Component: "X", Uri: "/x", Parent: ""}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMenuService(WithMenuServiceDB(db))

	if _, err := svc.MenuTree(context.Background(), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterFirst := atomic.LoadInt64(count)
	if _, err := svc.MenuTree(context.Background(), &pb.Empty{}); err != nil {
		t.Fatal(err)
	}
	afterSecond := atomic.LoadInt64(count)

	if delta := afterSecond - afterFirst; delta != 0 {
		t.Errorf("second MenuTree call issued %d queries, want 0 (cache hit)", delta)
	}
}

// TestMenuService_MenuTree_InvalidatesOnMenuChange: writing to
// sys_menus must bump the marker so the next MenuTree call — once
// dbcache's 1s grace window has elapsed — returns the new row.
// Without invalidation, the cached entry would shadow the new row
// until the 1m TTL.
func TestMenuService_MenuTree_InvalidatesOnMenuChange(t *testing.T) {
	db, _ := newMenuSvcDBWithCounter(t)
	if err := db.Create(&models.Menu{Name: "old", Component: "O", Uri: "/o", Parent: ""}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMenuService(WithMenuServiceDB(db))

	r1, err := svc.MenuTree(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Items) != 1 {
		t.Fatalf("first call: want 1 root, got %d", len(r1.Items))
	}

	// Wait past the 1s grace window so the next call revalidates
	// against the marker instead of returning the cached entry.
	time.Sleep(1100 * time.Millisecond)

	// New row bumps SUM(id); the marker query on the next call
	// returns a different value and the loader re-runs.
	if err := db.Create(&models.Menu{Name: "new", Component: "N", Uri: "/n", Parent: ""}).Error; err != nil {
		t.Fatal(err)
	}
	r2, err := svc.MenuTree(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Items) != 2 {
		t.Fatalf("after insert: want 2 roots, got %d (cache did not invalidate)", len(r2.Items))
	}
}

// TestMenuService_MenuBreadcrumb_SecondCallSkipsSelect: breadcrumb
// cache mirrors MenuTree — second call for the same id within the
// grace window does not re-SELECT.
func TestMenuService_MenuBreadcrumb_SecondCallSkipsSelect(t *testing.T) {
	db, count := newMenuSvcDBWithCounter(t)
	a := models.Menu{Name: "a", Component: "a", Uri: "/a", Parent: ""}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	b := models.Menu{Name: "b", Component: "b", Uri: "/b", Parent: "a"}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMenuService(WithMenuServiceDB(db))

	first, err := svc.MenuBreadcrumb(context.Background(), &pb.MenuBreadcrumbRequest{Id: uint32(b.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Path) != 2 {
		t.Fatalf("first breadcrumb: want 2 entries, got %v", first.Path)
	}
	afterFirst := atomic.LoadInt64(count)

	second, err := svc.MenuBreadcrumb(context.Background(), &pb.MenuBreadcrumbRequest{Id: uint32(b.ID)})
	if err != nil {
		t.Fatal(err)
	}
	afterSecond := atomic.LoadInt64(count)

	if delta := afterSecond - afterFirst; delta != 0 {
		t.Errorf("second breadcrumb call issued %d queries, want 0", delta)
	}
	if len(second.Path) != 2 || second.Path[0] != "a" || second.Path[1] != "b" {
		t.Errorf("second breadcrumb wrong: %v", second.Path)
	}
}

// TestMenuService_MenuTree_NilDB_NoCrash: NewMenuService without a DB
// must still construct, and the cached methods must return an error
// (not panic) — preserves backward compatibility for callers that
// build a MenuService ahead of wiring.
func TestMenuService_MenuTree_NilDB(t *testing.T) {
	svc := NewMenuService()
	if _, err := svc.MenuTree(context.Background(), &pb.Empty{}); err == nil {
		t.Fatal("want error from nil-db service, got nil")
	}
	if _, err := svc.MenuBreadcrumb(context.Background(), &pb.MenuBreadcrumbRequest{Id: 1}); err == nil {
		t.Fatal("want error from nil-db service, got nil")
	}
}
