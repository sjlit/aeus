package service

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

func newMenuSvcDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Menu{}, &models.RolePermission{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMenuService_MenuTree_BuildsHierarchy(t *testing.T) {
	db := newMenuSvcDB(t)
	db.Create(&models.Menu{Name: "system", Component: "Layout", Uri: "/sys", Parent: ""})
	db.Create(&models.Menu{Name: "user", Component: "User", Uri: "/sys/user", Parent: "Layout"})

	svc := NewMenuService(WithMenuServiceDB(db))
	res, err := svc.MenuTree(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "Layout" {
		t.Fatalf("got %+v, want single root Layout", res.Items)
	}
	if len(res.Items[0].Children) != 1 || res.Items[0].Children[0].Name != "User" {
		t.Fatalf("child missing")
	}
}

func TestMenuService_MenuOptions_NotEmpty(t *testing.T) {
	db := newMenuSvcDB(t)
	db.Create(&models.Menu{Name: "system", Component: "L", Uri: "/s", Parent: ""})
	db.Create(&models.Menu{Name: "user", Component: "U", Uri: "/u", Parent: "system"})

	svc := NewMenuService(WithMenuServiceDB(db))
	res, err := svc.MenuOptions(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Fatal("want at least 1 root tier")
	}
}

func TestMenuService_MenuBreadcrumb_Deep(t *testing.T) {
	db := newMenuSvcDB(t)
	a := models.Menu{Name: "a", Component: "a", Uri: "/a", Parent: ""}
	db.Create(&a)
	b := models.Menu{Name: "b", Component: "b", Uri: "/b", Parent: "a"}
	db.Create(&b)
	c := models.Menu{Name: "c", Component: "c", Uri: "/c", Parent: "b"}
	db.Create(&c)

	svc := NewMenuService(WithMenuServiceDB(db))
	res, err := svc.MenuBreadcrumb(context.Background(), &pb.MenuBreadcrumbRequest{Id: uint32(c.ID)})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if len(res.Path) != len(want) {
		t.Fatalf("got %v, want %v", res.Path, want)
	}
	for i := range want {
		if res.Path[i] != want[i] {
			t.Fatalf("idx %d: want %q got %q", i, want[i], res.Path[i])
		}
	}
}

func TestMenuService_MenuBreadcrumb_NotFound(t *testing.T) {
	db := newMenuSvcDB(t)
	svc := NewMenuService(WithMenuServiceDB(db))
	_, err := svc.MenuBreadcrumb(context.Background(), &pb.MenuBreadcrumbRequest{Id: 9999})
	if err == nil {
		t.Fatal("want error")
	}
	// MenuBreadcrumb maps gorm.ErrRecordNotFound to a fresh
	// *errs.Error with code 4004 so the HTTP wrapper surfaces
	// NotFound, not Unavailable.
	var pe *errs.Error
	if !errors.As(err, &pe) || pe.Code != errs.CodeNotFound {
		t.Fatalf("got %v, want *errs.Error code %d", err, errs.CodeNotFound)
	}
}
