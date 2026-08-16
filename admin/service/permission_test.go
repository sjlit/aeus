package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"gorm.io/gorm"
)

func newPermSvcDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Permission{}, &models.RolePermission{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPermissionService_ListCatalog_All(t *testing.T) {
	db := newPermSvcDB(t)
	db.Create(&models.Permission{Type: "api", Data: "x:y"})
	db.Create(&models.Permission{Type: "button", Data: "submit"})
	svc := NewPermissionService(WithPermissionServiceDB(db))

	res, err := svc.ListCatalog(context.Background(), &pb.ListCatalogRequest{
		Type: pb.PermissionType_PERMISSION_TYPE_UNSPECIFIED,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("got %v, want 2 distinct", res.Items)
	}
}

func TestPermissionService_ListCatalog_TypeAPI(t *testing.T) {
	db := newPermSvcDB(t)
	db.Create(&models.Permission{Type: "api", Data: "x:y"})
	db.Create(&models.Permission{Type: "button", Data: "submit"})
	svc := NewPermissionService(WithPermissionServiceDB(db))

	res, err := svc.ListCatalog(context.Background(), &pb.ListCatalogRequest{
		Type: pb.PermissionType_PERMISSION_TYPE_API,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0] != "x:y" {
		t.Fatalf("got %v, want [x:y]", res.Items)
	}
}
