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
	if err := db.AutoMigrate(&models.Tenant{}, &models.Role{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// newTenantSvc creates the role named by roleKey and returns a service
// wired to the same DB. Tenant RPCs are super-only, so every test must
// explicitly choose the caller's role instead of relying on the old
// unauthenticated behavior.
func newTenantSvc(t *testing.T, roleKey string, isSuper bool) (*gorm.DB, *TenantService) {
	t.Helper()
	db := newTenantSvcDB(t)
	if err := db.Create(&models.Role{
		Key:     roleKey,
		Name:    roleKey,
		Status:  "enabled",
		IsSuper: isSuper,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return db, NewTenantService(WithTenantServiceDB(db))
}

func TestTenantService_ListTenantOptions_AllSortedByName(t *testing.T) {
	db, svc := newTenantSvc(t, "root", true)
	for _, tn := range []*models.Tenant{
		{ID: "t-zhang", Name: "张氏集团", Status: "enabled"},
		{ID: "t-li", Name: "李氏科技", Status: "enabled"},
		{ID: "t-chen", Name: "陈记商贸", Status: "disabled"},
	} {
		if err := db.Create(tn).Error; err != nil {
			t.Fatal(err)
		}
	}

	res, err := svc.ListTenantOptions(claimsCtx("alice", "root", "t1"), &pb.Empty{})
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
	db, svc := newTenantSvc(t, "root", true)
	if err := db.Create(&models.Tenant{ID: "t1", Name: "租户一", Status: "enabled"}).Error; err != nil {
		t.Fatal(err)
	}

	res, err := svc.Tenant(claimsCtx("alice", "root", "t1"), &pb.TenantRequest{Id: "t1"})
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
	_, svc := newTenantSvc(t, "root", true)

	_, err := svc.Tenant(claimsCtx("alice", "root", "t1"), &pb.TenantRequest{Id: "missing"})
	wantPkError(t, err, errs.CodeNotFound)
}

func TestTenantService_Tenant_InvalidRequest(t *testing.T) {
	_, svc := newTenantSvc(t, "root", true)

	_, err := svc.Tenant(claimsCtx("alice", "root", "t1"), &pb.TenantRequest{})
	if err == nil {
		t.Fatal("empty id should fail validation")
	}
}

func TestTenantService_NonSuperDenied(t *testing.T) {
	db, svc := newTenantSvc(t, "ops", false)
	if err := db.Create(&models.Tenant{ID: "t1", Name: "租户一", Status: "enabled"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := claimsCtx("alice", "ops", "t1")

	if _, err := svc.ListTenantOptions(ctx, &pb.Empty{}); !errs.IsCode(err, errs.CodeAccessDenied) {
		t.Fatalf("ListTenantOptions: got %v, want AccessDenied", err)
	}
	if _, err := svc.Tenant(ctx, &pb.TenantRequest{Id: "t1"}); !errs.IsCode(err, errs.CodeAccessDenied) {
		t.Fatalf("Tenant: got %v, want AccessDenied", err)
	}
}

func TestTenantService_NoClaimsDenied(t *testing.T) {
	_, svc := newTenantSvc(t, "root", true)
	if _, err := svc.ListTenantOptions(context.Background(), &pb.Empty{}); !errs.IsCode(err, errs.CodeAccessDenied) {
		t.Fatalf("no claims: got %v, want AccessDenied", err)
	}
}
