package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	adminAuth "github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

// newDashboardTestServer boots a multi-tenant admin server so the
// dashboard endpoint exercises the same tenant-scoped counts production
// uses. The resolver reads claims off the request context, matching the
// default JWT middleware wiring.
func newDashboardTestServer(t *testing.T) (*gorm.DB, *ghttp.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	srv := ghttp.New()
	s := New(
		WithDB(db),
		WithRouter(srv),
		WithMultiTenant(true),
		WithTenantResolver(func(ctx context.Context) string {
			return adminAuth.TenantIDFromContext(ctx)
		}),
	)
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("admin.Setup: %v", err)
	}
	return db, srv
}

func callDashboard(t *testing.T, srv *ghttp.Server, claims *adminAuth.Claims) (int, DashboardOverview, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, DashboardEndpointPath, nil)
	if claims != nil {
		req = req.WithContext(mwauth.NewContext(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec, req)

	var env struct {
		Code int               `json:"code"`
		Data DashboardOverview `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal dashboard envelope: %v body=%s", err, rec.Body.String())
	}
	return env.Code, env.Data, rec.Body.String()
}

func TestDashboardEndpoint_AdminOverview(t *testing.T) {
	db, srv := newDashboardTestServer(t)
	now := time.Now()
	today := now.Unix()
	yesterday := now.AddDate(0, 0, -1).Unix()

	for _, tenant := range []*models.Tenant{
		{ID: "t1", Name: "租户一"},
		{ID: "t2", Name: "租户二"},
	} {
		if err := db.Create(tenant).Error; err != nil {
			t.Fatalf("create tenant: %v", err)
		}
	}
	for _, role := range []*models.Role{
		{TenantModel: models.TenantModel{TenantID: "t1"}, Key: "super", Name: "超级管理员", IsSuper: true},
		{TenantModel: models.TenantModel{TenantID: "t2"}, Key: "other", Name: "其他租户角色"},
	} {
		if err := db.Create(role).Error; err != nil {
			t.Fatalf("create role: %v", err)
		}
	}
	for _, user := range []*models.User{
		{TenantModel: models.TenantModel{TenantID: "t1"}, UID: "u0001", Username: "admin", RoleKey: "super"},
		{TenantModel: models.TenantModel{TenantID: "t1"}, UID: "u0002", Username: "operator", RoleKey: "super"},
		{TenantModel: models.TenantModel{TenantID: "t2"}, UID: "u0003", Username: "other", RoleKey: "other"},
	} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	if err := db.Create(&models.LoginLog{
		TenantModel: models.TenantModel{TenantID: "t1", BaseModel: models.BaseModel{CreatedAt: today}},
		UID:         "u0001",
	}).Error; err != nil {
		t.Fatalf("create today login: %v", err)
	}
	if err := db.Create(&models.LoginLog{
		TenantModel: models.TenantModel{TenantID: "t1", BaseModel: models.BaseModel{CreatedAt: yesterday}},
		UID:         "u0001",
	}).Error; err != nil {
		t.Fatalf("create yesterday login: %v", err)
	}
	if err := db.Create(&models.LoginLog{
		TenantModel: models.TenantModel{TenantID: "t2", BaseModel: models.BaseModel{CreatedAt: today}},
		UID:         "u0003",
	}).Error; err != nil {
		t.Fatalf("create other-tenant login: %v", err)
	}
	if err := db.Create(&models.Audit{
		TenantModel: models.TenantModel{TenantID: "t1", BaseModel: models.BaseModel{CreatedAt: today}},
		UID:         "u0001", Action: "update", Module: "system", Table: "sys_users",
	}).Error; err != nil {
		t.Fatalf("create audit: %v", err)
	}
	if err := db.Create(&models.Audit{
		TenantModel: models.TenantModel{TenantID: "t2", BaseModel: models.BaseModel{CreatedAt: today}},
		UID:         "u0003", Action: "delete", Module: "system", Table: "sys_roles",
	}).Error; err != nil {
		t.Fatalf("create other-tenant audit: %v", err)
	}

	code, data, body := callDashboard(t, srv, &adminAuth.Claims{
		UID: "u0001", Role: "super", TenantID: "t1",
	})
	if code != 0 {
		t.Fatalf("code = %d, body=%s", code, body)
	}
	if data.Scope != "admin" {
		t.Fatalf("scope = %q, want admin", data.Scope)
	}
	if data.Counts == nil {
		t.Fatal("counts is nil")
	}
	if data.Counts.Users != 2 {
		t.Errorf("users = %d, want 2 (tenant t1 only)", data.Counts.Users)
	}
	if data.Counts.Roles != 1 {
		t.Errorf("roles = %d, want 1 (tenant t1 only)", data.Counts.Roles)
	}
	if data.Counts.Tenants != 2 {
		t.Errorf("tenants = %d, want 2", data.Counts.Tenants)
	}
	if data.Counts.Menus == 0 || data.Counts.Permissions == 0 {
		t.Errorf("menus/permissions must be seeded, got %d/%d", data.Counts.Menus, data.Counts.Permissions)
	}

	if data.Logins == nil {
		t.Fatal("logins is nil")
	}
	if data.Logins.Today != 1 {
		t.Errorf("today logins = %d, want 1", data.Logins.Today)
	}
	if data.Logins.Yesterday != 1 {
		t.Errorf("yesterday logins = %d, want 1", data.Logins.Yesterday)
	}
	if len(data.Logins.Trend) != 7 {
		t.Fatalf("trend length = %d, want 7", len(data.Logins.Trend))
	}
	if got, want := data.Logins.Trend[6].Date, time.Now().Format("2006-01-02"); got != want {
		t.Errorf("trend last date = %q, want %q", got, want)
	}
	if data.Logins.Trend[6].Count != 1 {
		t.Errorf("trend today count = %d, want 1", data.Logins.Trend[6].Count)
	}

	if data.Audits == nil {
		t.Fatal("audits is nil")
	}
	if data.Audits.Today != 1 {
		t.Errorf("today audits = %d, want 1", data.Audits.Today)
	}
	if len(data.Audits.Recent) != 1 {
		t.Fatalf("recent audits = %d, want 1", len(data.Audits.Recent))
	}
	if data.Audits.Recent[0].Table != "sys_users" {
		t.Errorf("recent audit table = %q, want sys_users", data.Audits.Recent[0].Table)
	}
}

func TestDashboardEndpoint_PersonalScope(t *testing.T) {
	db, srv := newDashboardTestServer(t)
	if err := db.Create(&models.Role{
		TenantModel: models.TenantModel{TenantID: "t1"},
		Key:         "viewer", Name: "只读用户",
	}).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}

	code, data, body := callDashboard(t, srv, &adminAuth.Claims{
		UID: "u0002", Role: "viewer", TenantID: "t1",
	})
	if code != 0 {
		t.Fatalf("code = %d, body=%s", code, body)
	}
	if data.Scope != "personal" {
		t.Fatalf("scope = %q, want personal", data.Scope)
	}
	if data.Counts != nil || data.Logins != nil || data.Audits != nil {
		t.Fatalf("personal scope must omit system sections: %+v", data)
	}
	if data.GeneratedAt == 0 {
		t.Error("generated_at must be set")
	}
}

func TestDashboardEndpoint_RequiresClaims(t *testing.T) {
	_, srv := newDashboardTestServer(t)
	code, _, body := callDashboard(t, srv, nil)
	if code != 4001 {
		t.Fatalf("code = %d, want 4001; body=%s", code, body)
	}
}

func TestSetup_CatalogsDashboardPermission(t *testing.T) {
	db, _ := newDashboardTestServer(t)
	var count int64
	if err := db.Model(&models.Permission{}).
		Where("type = ? AND data = ?", models.PermissionTypeAPI, "GET /dashboard/overview").
		Count(&count).Error; err != nil {
		t.Fatalf("count permission: %v", err)
	}
	if count != 1 {
		t.Fatalf("dashboard permission rows = %d, want 1", count)
	}
}
