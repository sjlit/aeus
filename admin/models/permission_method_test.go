package models

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPermDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&RolePermission{}, &Permission{}, &Menu{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRolePermission_ValidateMenuData_AllExist(t *testing.T) {
	db := newPermDB(t)
	if err := db.Create(&Menu{Component: "a", Name: "alpha"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Menu{Component: "b", Name: "beta"}).Error; err != nil {
		t.Fatal(err)
	}
	missing, err := (&RolePermission{}).ValidateMenuData(db, context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("got %v, want no missing", missing)
	}
}

func TestRolePermission_ValidateMenuData_SomeMissing(t *testing.T) {
	db := newPermDB(t)
	if err := db.Create(&Menu{Component: "a", Name: "alpha"}).Error; err != nil {
		t.Fatal(err)
	}
	missing, err := (&RolePermission{}).ValidateMenuData(db, context.Background(), []string{"a", "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "ghost" {
		t.Fatalf("got %v, want [ghost]", missing)
	}
}

func TestRolePermission_ValidateMenuData_Empty(t *testing.T) {
	db := newPermDB(t)
	missing, err := (&RolePermission{}).ValidateMenuData(db, context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Fatalf("got %v, want nil", missing)
	}
}

func TestRolePermission_ValidatePermissionData_AllExist(t *testing.T) {
	db := newPermDB(t)
	if err := db.Create(&Permission{Type: "api", Data: "user:list"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Permission{Type: "api", Data: "user:create"}).Error; err != nil {
		t.Fatal(err)
	}
	missing, err := (&RolePermission{}).ValidatePermissionData(db, context.Background(), []string{"user:list", "user:create"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("got %v, want no missing", missing)
	}
}

func TestPermission_ListByType_API(t *testing.T) {
	db := newPermDB(t)
	if err := db.Create(&Permission{Type: "api", Data: "x:y"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Permission{Type: "api", Data: "x:z"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Permission{Type: "button", Data: "submit"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := (&Permission{}).ListByType(db, context.Background(), PermissionTypeAPI)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 api items", got)
	}
}

func TestPermission_ListByType_All(t *testing.T) {
	db := newPermDB(t)
	if err := db.Create(&Permission{Type: "api", Data: "x:y"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Permission{Type: "button", Data: "submit"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := (&Permission{}).ListByType(db, context.Background(), PermissionTypeUnspec)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 distinct", got)
	}
}

func TestRolePermission_MenuPermissionDatas(t *testing.T) {
	db := newPermDB(t)
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "menu", Data: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "menu", Data: "role"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "permission", Data: "x:y"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := (&RolePermission{}).MenuPermissionDatas(db, context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 menu components", got)
	}
}

func TestRolePermission_APIPermissionDatas(t *testing.T) {
	db := newPermDB(t)
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "permission", Data: "x:y"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "permission", Data: "x:z"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RolePermission{RoleKey: "admin", Type: "menu", Data: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := (&RolePermission{}).APIPermissionDatas(db, context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 api datas", got)
	}
}
