package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	jwt "github.com/golang-jwt/jwt/v5"
	adminAuth "github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/middleware"
	"github.com/sjlit/aeus/admin/models"
	mwauth "github.com/sjlit/aeus/middleware/auth"
	ghttp "github.com/sjlit/aeus/transport/http"
	"github.com/sjlit/rest/v3"
	"gorm.io/gorm"
)

// auditToy is a minimal application model used to exercise the audit
// hooks end-to-end through the real REST routes. Its (module, table)
// pair is distinct from every built-in model so permission-row lookups
// and sys_audits assertions stay unambiguous.
type auditToy struct {
	rest.BaseModel
	Name string `json:"name" comment:"名称"`
}

func (*auditToy) TableName() string  { return "audit_toys" }
func (*auditToy) ModuleName() string { return "system" }

// newAuditServer boots a full admin Server over in-memory sqlite with
// the given extra options, registers the toy model, and returns the DB
// handle plus the HTTP engine. The exact REST URIs are read back from
// the sys_permissions rows that ensurePermissionRows seeded ("POST
// /system/audit_toy" ...) so the tests never hard-code rest/v3's
// inflector output.
func newAuditServer(t *testing.T, opts ...Option) (*gorm.DB, *ghttp.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	log, _ := captureLogger()
	httpSrv := ghttp.New()
	base := []Option{WithDB(db), WithRouter(httpSrv), WithLogger(log),
		WithUserResolve(func(context.Context, *http.Request) (string, error) {
			return "u0001", nil
		})}
	s := New(append(base, opts...)...)
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("admin.Setup: %v", err)
	}
	if err := s.RegisterModel(&auditToy{}); err != nil {
		t.Fatalf("RegisterModel(auditToy): %v", err)
	}
	return db, httpSrv
}

// auditRoute resolves the mounted URI for one HTTP method from the
// seeded sys_permissions catalog.
func auditRoute(t *testing.T, db *gorm.DB, method string) string {
	t.Helper()
	var row models.Permission
	if err := db.Where("data LIKE ?", method+" /system/audit%").First(&row).Error; err != nil {
		t.Fatalf("lookup %s permission for audit_toys: %v", method, err)
	}
	prefix := method + " "
	return row.Data[len(prefix):]
}

// withID substitutes the ":id" path template segment of a mounted
// route (e.g. "/system/audit_toy/:id") with an actual key.
func withID(route string, id int) string {
	return strings.Replace(route, ":id", strconv.Itoa(id), 1)
}

func doReq(t *testing.T, srv *ghttp.Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	rec := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec, httptest.NewRequest(method, path, rd))
	return rec
}

// countAudits returns how many sys_audits rows match action.
func countAudits(t *testing.T, db *gorm.DB, action string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&models.Audit{}).Where("action = ?", action).Count(&n).Error; err != nil {
		t.Fatalf("count sys_audits(%s): %v", action, err)
	}
	return n
}

func TestAudit_CreateUpdateDeleteRecordsRows(t *testing.T) {
	db, srv := newAuditServer(t, WithAudit(true))
	createURI := auditRoute(t, db, http.MethodPost)
	updateURI := auditRoute(t, db, http.MethodPut)
	deleteURI := auditRoute(t, db, http.MethodDelete)

	// create → one row, diff carries the full value set (previous=null).
	rec := doReq(t, srv, http.MethodPost, createURI, `{"name":"alice"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := countAudits(t, db, "create"); got != 1 {
		t.Fatalf("after create, create-audit rows = %d, want 1", got)
	}
	var row models.Audit
	if err := db.Where("action = ?", "create").First(&row).Error; err != nil {
		t.Fatalf("load create row: %v", err)
	}
	if row.UID != "u0001" || row.Module != "system" || row.Table != "audit_toys" {
		t.Errorf("create row attribution = (%q,%q,%q), want (u0001,system,audit_toys)",
			row.UID, row.Module, row.Table)
	}
	var diff []*rest.DiffAttr
	if err := json.Unmarshal([]byte(row.Data), &diff); err != nil {
		t.Fatalf("create Data not valid DiffAttr JSON: %v data=%s", err, row.Data)
	}
	foundName := false
	for _, d := range diff {
		if d.Column == "name" {
			foundName = true
			if d.Previous != nil || d.Current != "alice" {
				t.Errorf("create name diff = %+v, want previous=nil current=alice", d)
			}
		}
	}
	if !foundName {
		t.Errorf("create Data has no name column entry: %s", row.Data)
	}

	// update with an actual change → one row with previous/current.
	id := 1
	rec = doReq(t, srv, http.MethodPut, withID(updateURI, id), `{"name":"bob"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := countAudits(t, db, "update"); got != 1 {
		t.Fatalf("after update, update-audit rows = %d, want 1", got)
	}
	row = models.Audit{}
	if err := db.Where("action = ?", "update").First(&row).Error; err != nil {
		t.Fatalf("load update row: %v", err)
	}
	diff = nil
	if err := json.Unmarshal([]byte(row.Data), &diff); err != nil {
		t.Fatalf("update Data not valid DiffAttr JSON: %v data=%s", err, row.Data)
	}
	for _, d := range diff {
		if d.Column == "name" && (d.Previous != "alice" || d.Current != "bob") {
			t.Errorf("update name diff = %+v, want alice→bob", d)
		}
	}

	// update that changes nothing → no additional row.
	rec = doReq(t, srv, http.MethodPut, withID(updateURI, id), `{"name":"bob"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("no-op update status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := countAudits(t, db, "update"); got != 1 {
		t.Errorf("after no-op update, update-audit rows = %d, want still 1", got)
	}

	// delete → one row whose Data is {"id":<pk>}.
	rec = doReq(t, srv, http.MethodDelete, withID(deleteURI, id), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := countAudits(t, db, "delete"); got != 1 {
		t.Fatalf("after delete, delete-audit rows = %d, want 1", got)
	}
	row = models.Audit{}
	if err := db.Where("action = ?", "delete").First(&row).Error; err != nil {
		t.Fatalf("load delete row: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
		t.Fatalf("delete Data not valid JSON object: %v data=%s", err, row.Data)
	}
	if v, ok := payload["id"].(float64); !ok || int(v) != id {
		t.Errorf("delete Data = %s, want {\"id\":%d}", row.Data, id)
	}
}

// TestAudit_DisabledByDefault pins the zero-value contract: without
// WithAudit(true) no hook is registered and no sys_audits row appears.
func TestAudit_DisabledByDefault(t *testing.T) {
	db, srv := newAuditServer(t)
	createURI := auditRoute(t, db, http.MethodPost)
	rec := doReq(t, srv, http.MethodPost, createURI, `{"name":"x"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var n int64
	if err := db.Model(&models.Audit{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("audit disabled but sys_audits has %d rows, want 0", n)
	}
}

// TestAudit_BuiltinExcludes covers both halves of the built-in
// exclusion list:
//
//   - writes to sys_audits itself never produce further audit rows
//     (recursion guard);
//   - sys_login_logs is excluded because login auditing owns its own
//     WithLoginLogger pipeline.
func TestAudit_BuiltinExcludes(t *testing.T) {
	db, srv := newAuditServer(t, WithAudit(true))
	body := `{"uid":"u0001","action":"create","module":"m","table":"t","data":"payload"}`
	rec := doReq(t, srv, http.MethodPost, "/system/sys_audit", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("manual sys_audit create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var n int64
	if err := db.Model(&models.Audit{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("sys_audits rows = %d, want exactly the 1 manual row (hook must not re-audit)", n)
	}
}

// TestAudit_CustomExclude verifies WithAuditExcludes suppresses
// recording for a caller-supplied "<module>/<table>" pair while other
// models keep being audited.
func TestAudit_CustomExclude(t *testing.T) {
	db, srv := newAuditServer(t, WithAudit(true), WithAuditExcludes("system/audit_toys"))
	createURI := auditRoute(t, db, http.MethodPost)
	rec := doReq(t, srv, http.MethodPost, createURI, `{"name":"quiet"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var n int64
	if err := db.Model(&models.Audit{}).Where("`table` = ?", "audit_toys").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("excluded table produced %d audit rows, want 0", n)
	}
}

// TestFromClaimsUserResolve unit-checks the default UID resolver: it
// reads *auth.Claims off the JWT middleware's ctx key and yields ""
// when no claims are present.
func TestFromClaimsUserResolve(t *testing.T) {
	ctx := mwauth.NewContext(context.Background(),
		&adminAuth.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "sub"}, UID: "u9999"})
	uid, err := middleware.FromClaimsUserResolve(ctx, nil)
	if err != nil || uid != "u9999" {
		t.Errorf("uid = %q err = %v, want u9999/<nil>", uid, err)
	}
	uid, err = middleware.FromClaimsUserResolve(context.Background(), nil)
	if err != nil || uid != "" {
		t.Errorf("uid = %q err = %v, want empty on claims-less ctx", uid, err)
	}
}
