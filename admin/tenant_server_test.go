package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

// newTenantServer runs Setup with a fixed tenant resolver so every
// request resolves to tenant "t1" without needing a JWT.  This is the
// harness for the registerModel TenantResolve guard: models without a
// tenant_id column must not receive rest/v3's tenant filter, models
// with one must.
func newTenantServer(t *testing.T) (*gorm.DB, *ghttp.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv),
		WithTenantResolver(func(context.Context) string { return "t1" }))
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("admin.Setup: %v", err)
	}
	return db, httpSrv
}

// searchEndpoint GETs a rest/v3 search route and decodes the envelope.
// It returns the business code and the total_count of the page.
func searchEndpoint(t *testing.T, srv *ghttp.Server, path string) (int, int64) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var env struct {
		Code int `json:"code"`
		Data struct {
			TotalCount int64 `json:"total_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal search envelope: %v body=%s", err, rec.Body.String())
	}
	return env.Code, env.Data.TotalCount
}

// TestSetup_TenantSearchIsGlobalAndUnfiltered is the acceptance test for
// the Tenant resource: /system/sys_tenants must be registered and its
// search must return rows from every tenant (no tenant_id column to
// filter on, and rest/v3 must not inject a filter that breaks the SQL).
func TestSetup_TenantSearchIsGlobalAndUnfiltered(t *testing.T) {
	db, srv := newTenantServer(t)
	for _, tn := range []*models.Tenant{
		{ID: "t1", Name: "租户一"},
		{ID: "t2", Name: "租户二"},
	} {
		if err := db.Create(tn).Error; err != nil {
			t.Fatal(err)
		}
	}

	code, total := searchEndpoint(t, srv, "/system/sys_tenants")
	if code != 0 {
		t.Fatalf("tenant search code = %d, want 0 (no SQL error from injected tenant filter)", code)
	}
	if total != 2 {
		t.Errorf("tenant search total = %d, want 2 (global, unfiltered)", total)
	}
}

// TestSetup_GlobalModelSearchStillWorks guards the §3.8 fix on the
// pre-existing global models: Menu carries no tenant_id column, so its
// search must not blow up on an injected tenant filter either — it
// must return every menu row (Setup auto-inserts one per MenuProvider
// model, plus the one we add here).
func TestSetup_GlobalModelSearchStillWorks(t *testing.T) {
	db, srv := newTenantServer(t)
	if err := db.Create(&models.Menu{Component: "tenant", Name: "租户管理"}).Error; err != nil {
		t.Fatal(err)
	}
	var want int64
	if err := db.Model(&models.Menu{}).Count(&want).Error; err != nil {
		t.Fatal(err)
	}

	// rest/v3's mechanical pluralization turns sys_menus into
	// sys_menuses — the search route is at the pluralized path.
	code, total := searchEndpoint(t, srv, "/system/sys_menuses")
	if code != 0 {
		t.Fatalf("menu search code = %d, want 0", code)
	}
	if total != want {
		t.Errorf("menu search total = %d, want %d (all global rows)", total, want)
	}
}

// TestSetup_TenantScopedModelSearchStillFiltered is the regression
// guard for the other side of the §3.8 change: models WITH a tenant_id
// column must keep receiving rest/v3's tenant filter.
func TestSetup_TenantScopedModelSearchStillFiltered(t *testing.T) {
	db, srv := newTenantServer(t)
	for _, r := range []*models.Role{
		{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "r1", Name: "t1 角色"},
		{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "r2", Name: "t2 角色"},
	} {
		if err := db.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}

	code, total := searchEndpoint(t, srv, "/system/sys_roles")
	if code != 0 {
		t.Fatalf("role search code = %d, want 0", code)
	}
	if total != 1 {
		t.Errorf("role search total = %d, want 1 (only t1 visible)", total)
	}
}
