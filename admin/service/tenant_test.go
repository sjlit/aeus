package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

func newTenantSvcDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Tenant{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTenantService_ListTenantOptions_AllSortedByName(t *testing.T) {
	db := newTenantSvcDB(t)
	for _, tn := range []*models.Tenant{
		{ID: "t-zhang", Name: "张氏集团", Status: "enabled"},
		{ID: "t-li", Name: "李氏科技", Status: "enabled"},
		{ID: "t-chen", Name: "陈记商贸", Status: "disabled"},
	} {
		if err := db.Create(tn).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewTenantService(WithTenantServiceDB(db))
	res, err := svc.ListTenantOptions(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	// Disabled tenants are still selectable options (status is not a
	// filter here); the dropdown is a global catalog.
	if len(res.Items) != 3 {
		t.Fatalf("got %d options, want 3", len(res.Items))
	}
	// Sorted by name: 张(zhang) > 李(li) > 陈(chen) in codepoint order.
	want := []struct{ id, name string }{
		{"t-zhang", "张氏集团"},
		{"t-li", "李氏科技"},
		{"t-chen", "陈记商贸"},
	}
	for i, w := range want {
		if res.Items[i].Id != w.id || res.Items[i].Name != w.name {
			t.Errorf("item[%d] = {%q, %q}, want {%q, %q}", i, res.Items[i].Id, res.Items[i].Name, w.id, w.name)
		}
	}
}

func TestTenantService_Tenant_Found(t *testing.T) {
	db := newTenantSvcDB(t)
	if err := db.Create(&models.Tenant{ID: "t1", Name: "租户一", Status: "enabled"}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewTenantService(WithTenantServiceDB(db))
	res, err := svc.Tenant(context.Background(), &pb.TenantRequest{Id: "t1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Id != "t1" || res.Name != "租户一" || res.Status != "enabled" {
		t.Fatalf("got %+v, want {t1 租户一 enabled}", res)
	}
	if res.CreatedAt == 0 {
		t.Error("CreatedAt should be set")
	}
}

func TestTenantService_Tenant_NotFound(t *testing.T) {
	db := newTenantSvcDB(t)

	svc := NewTenantService(WithTenantServiceDB(db))
	_, err := svc.Tenant(context.Background(), &pb.TenantRequest{Id: "missing"})
	wantPkError(t, err, errs.CodeNotFound)
}

func TestTenantService_Tenant_InvalidRequest(t *testing.T) {
	db := newTenantSvcDB(t)

	svc := NewTenantService(WithTenantServiceDB(db))
	_, err := svc.Tenant(context.Background(), &pb.TenantRequest{})
	if err == nil {
		t.Fatal("empty id should fail validation")
	}
}
