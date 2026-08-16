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

// wantPkError asserts err is a fresh *errs.Error carrying the given
// business code. ReplaceRolePermissions / GetMenuBreadcrumb now return
// errs.Format results (no cause chain), so errors.Is against the
// sentinel no longer matches; the HTTP wrapper's direct *errs.Error
// assertion is what consumers rely on, and Code is the contract.
func wantPkError(t *testing.T, err error, code errs.Code) {
	t.Helper()
	var pe *errs.Error
	if !errors.As(err, &pe) || pe.Code != code {
		t.Fatalf("got %v, want *errs.Error code %d", err, code)
	}
}

func newRoleSvcDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Role{}, &models.RolePermission{}, &models.Permission{}, &models.Menu{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRoleService_ReplaceRolePermissions_Success(t *testing.T) {
	db := newRoleSvcDB(t)
	db.Create(&models.Role{Key: "admin", Name: "Admin"})
	db.Create(&models.Menu{Component: "user", Name: "user"})
	db.Create(&models.Menu{Component: "role", Name: "role"})
	db.Create(&models.Permission{Type: "api", Data: "x:read"})
	svc := NewRoleService(WithRoleServiceDB(db))

	res, err := svc.ReplaceRolePermissions(context.Background(), &pb.ReplaceRolePermissionsRequest{
		Role:  "admin",
		Menus: []string{"user", "role"},
		Apis:  []string{"x:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Affected != 3 {
		t.Fatalf("got %d, want affected=3", res.Affected)
	}
	var n int64
	db.Model(&models.RolePermission{}).Where("role_key = ?", "admin").Count(&n)
	if n != 3 {
		t.Fatalf("got %d, want 3 perm rows", n)
	}
}

func TestRoleService_ReplaceRolePermissions_RejectsSuperRole(t *testing.T) {
	db := newRoleSvcDB(t)
	db.Create(&models.Role{Key: "admin", Name: "Admin", IsSuper: true})
	db.Create(&models.Menu{Component: "user", Name: "user"})
	svc := NewRoleService(WithRoleServiceDB(db))

	_, err := svc.ReplaceRolePermissions(context.Background(), &pb.ReplaceRolePermissionsRequest{
		Role:  "admin",
		Menus: []string{"user"},
	})
	if err == nil {
		t.Fatal("want error")
	}
	wantPkError(t, err, errs.CodePermissionDenied)
	// super role grants are auto-managed: the request must not write
	var n int64
	db.Model(&models.RolePermission{}).Where("role_key = ?", "admin").Count(&n)
	if n != 0 {
		t.Fatalf("super role grants must not be written, got %d", n)
	}
}

func TestRoleService_ReplaceRolePermissions_RoleNotFound(t *testing.T) {
	db := newRoleSvcDB(t)
	svc := NewRoleService(WithRoleServiceDB(db))
	_, err := svc.ReplaceRolePermissions(context.Background(), &pb.ReplaceRolePermissionsRequest{
		Role:  "ghost",
		Menus: nil,
	})
	if err == nil {
		t.Fatal("want error")
	}
	wantPkError(t, err, errs.CodeNotFound)
}

func TestRoleService_ReplaceRolePermissions_MenuNotFound(t *testing.T) {
	db := newRoleSvcDB(t)
	db.Create(&models.Role{Key: "admin", Name: "Admin"})
	svc := NewRoleService(WithRoleServiceDB(db))
	_, err := svc.ReplaceRolePermissions(context.Background(), &pb.ReplaceRolePermissionsRequest{
		Role:  "admin",
		Menus: []string{"ghost"},
	})
	if err == nil {
		t.Fatal("want error")
	}
	wantPkError(t, err, errs.CodeInvalid)
	// ensure no side-effect: no perms for admin
	var n int64
	db.Model(&models.RolePermission{}).Where("role_key = ?", "admin").Count(&n)
	if n != 0 {
		t.Fatalf("transaction should have rolled back, got %d perms", n)
	}
}

func TestRoleService_ReplaceRolePermissions_Empty(t *testing.T) {
	db := newRoleSvcDB(t)
	db.Create(&models.Role{Key: "admin", Name: "Admin"})
	db.Create(&models.RolePermission{RoleKey: "admin", Type: "menu", Data: "old"})
	svc := NewRoleService(WithRoleServiceDB(db))

	_, err := svc.ReplaceRolePermissions(context.Background(), &pb.ReplaceRolePermissionsRequest{
		Role: "admin", Menus: nil, Apis: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&models.RolePermission{}).Where("role_key = ?", "admin").Count(&n)
	if n != 0 {
		t.Fatalf("got %d, want 0 after empty replace", n)
	}
}

func TestRoleService_RolePermissions_Empty(t *testing.T) {
	db := newRoleSvcDB(t)
	svc := NewRoleService(WithRoleServiceDB(db))
	res, err := svc.RolePermissions(context.Background(), &pb.RolePermissionsRequest{Role: "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Menus) != 0 || len(res.Apis) != 0 {
		t.Fatalf("got %+v, want empty", res)
	}
}

// TestRoleService_RolePermissions_TypeFilter covers the merged
// ?type= semantics: MENU returns menus only, API returns apis only,
// button/data_scope have no role-level junction query (empty), and
// UNSPECIFIED (the default) returns the combined read.
func TestRoleService_RolePermissions_TypeFilter(t *testing.T) {
	db := newRoleSvcDB(t)
	db.Create(&models.RolePermission{RoleKey: "admin", Type: "menu", Data: "user"})
	db.Create(&models.RolePermission{RoleKey: "admin", Type: "menu", Data: "role"})
	db.Create(&models.RolePermission{RoleKey: "admin", Type: "permission", Data: "x:read"})
	svc := NewRoleService(WithRoleServiceDB(db))

	res, err := svc.RolePermissions(context.Background(), &pb.RolePermissionsRequest{
		Role: "admin",
		Type: pb.PermissionType_PERMISSION_TYPE_MENU,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Menus) != 2 || len(res.Apis) != 0 {
		t.Fatalf("MENU: want menus only, got %+v", res)
	}

	res, err = svc.RolePermissions(context.Background(), &pb.RolePermissionsRequest{
		Role: "admin",
		Type: pb.PermissionType_PERMISSION_TYPE_API,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Menus) != 0 || len(res.Apis) != 1 || res.Apis[0] != "x:read" {
		t.Fatalf("API: want apis only, got %+v", res)
	}

	res, err = svc.RolePermissions(context.Background(), &pb.RolePermissionsRequest{
		Role: "admin",
		Type: pb.PermissionType_PERMISSION_TYPE_BUTTON,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Menus) != 0 || len(res.Apis) != 0 {
		t.Fatalf("BUTTON: want empty, got %+v", res)
	}

	res, err = svc.RolePermissions(context.Background(), &pb.RolePermissionsRequest{Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Menus) != 2 || len(res.Apis) != 1 {
		t.Fatalf("UNSPECIFIED: want combined, got %+v", res)
	}
}

func TestRoleService_RoleMenus_NoMenus(t *testing.T) {
	db := newRoleSvcDB(t)
	svc := NewRoleService(WithRoleServiceDB(db))
	res, err := svc.RoleMenus(context.Background(), &pb.RoleMenusRequest{Role: "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Menus) != 0 {
		t.Fatalf("got %+v, want empty", res)
	}
}

func TestRoleService_RoleMenus_RoundTrip(t *testing.T) {
	db := newRoleSvcDB(t)
	db.Create(&models.Menu{Name: "user", Component: "U", Uri: "/u", Parent: ""})
	db.Create(&models.RolePermission{RoleKey: "admin", Type: "menu", Data: "U"})

	svc := NewRoleService(WithRoleServiceDB(db))
	res, err := svc.RoleMenus(context.Background(), &pb.RoleMenusRequest{Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Menus) != 1 || res.Menus[0].Component != "U" {
		t.Fatalf("got %+v, want [user]", res.Menus)
	}
}
