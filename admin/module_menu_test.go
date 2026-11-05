package admin

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/policy"
	"gorm.io/gorm"
)

func TestEnsureModuleMenuAndEndpointPermissions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Menu{}, &models.Permission{}); err != nil {
		t.Fatal(err)
	}

	spec := models.ModuleSpec{Name: "shop", Title: "商城管理", Icon: "Shop", Sort: 100}
	for i := 0; i < 2; i++ {
		if err := EnsureModuleMenu(db, spec); err != nil {
			t.Fatal(err)
		}
	}
	var menus []models.Menu
	if err := db.Where("component = ?", "ShopCenter").Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	if len(menus) != 1 || menus[0].Name != "商城管理" {
		t.Fatalf("menus = %+v", menus)
	}

	policies := []policy.Policy{
		{Method: "GET", Path: "/shop/orders/:id", Button: "shop:order:detail", Group: "订单", Description: "详情"},
		{Method: "GET", Path: "/public/ping", Public: true},
	}
	for i := 0; i < 2; i++ {
		if err := EnsureEndpointPermissions(db, policies...); err != nil {
			t.Fatal(err)
		}
	}
	var apiRows []models.Permission
	if err := db.Where("type = ?", string(models.PermissionTypeAPI)).Find(&apiRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(apiRows) != 1 || apiRows[0].Data != "GET /shop/orders/:id" {
		t.Fatalf("api permissions = %+v", apiRows)
	}
	var buttonRows []models.Permission
	if err := db.Where("type = ?", string(models.PermissionTypeButton)).Find(&buttonRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(buttonRows) != 1 || buttonRows[0].Data != "shop:order:detail" {
		t.Fatalf("button permissions = %+v", buttonRows)
	}
}
