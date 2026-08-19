package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/pkg/errs"
	ghttp "github.com/sjlit/aeus/transport/http"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

// setupSchemaDB opens an in-memory SQLite, migrates the schema meta-table,
// and runs admin.Server.Setup which auto-registers all admin built-in
// models (writing one sys_schemas row per column) and mounts the schema
// endpoint at GET /schema/:module/:table. Returns the *gorm.DB and the
// HTTP server (already wired as opts.Router).
func setupSchemaDB(t *testing.T) (*gorm.DB, *ghttp.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv))
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("admin.Setup: %v", err)
	}
	return db, httpSrv
}

// callEndpoint drives a real HTTP request through the gin engine — the
// same path production traffic uses — and returns the recorded response.
func callEndpoint(t *testing.T, httpSrv *ghttp.Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	httpSrv.Engine().ServeHTTP(rec, req)
	return rec
}

// ---------- pure-unit: parseSchemaPath ----------

func TestParseSchemaPath(t *testing.T) {
	cases := []struct {
		path   string
		module string
		table  string
		wantOK bool
	}{
		{"/schema/system/sys_users", "system", "sys_users", true},
		{"/schema/system/sys_roles", "system", "sys_roles", true},
		// missing one segment
		{"/schema/system", "", "", false},
		{"/schema", "", "", false},
		// wrong prefix (3 segments, but first isn't "schema")
		{"/wrong/system/sys_users", "", "", false},
		// empty segment values
		{"/schema//sys_users", "", "", false},
		{"/schema/system/", "", "", false},
		// trailing slash is OK
		{"/schema/system/sys_users/", "system", "sys_users", true},
	}
	for _, c := range cases {
		m, tab, ok := parseSchemaPath(c.path)
		if ok != c.wantOK {
			t.Errorf("parseSchemaPath(%q) ok=%v want %v", c.path, ok, c.wantOK)
			continue
		}
		if ok && (m != c.module || tab != c.table) {
			t.Errorf("parseSchemaPath(%q) = (%q,%q) want (%q,%q)",
				c.path, m, tab, c.module, c.table)
		}
	}
}

// ---------- pre-condition failures (no full server required) ----------

func TestRegisterSchemaEndpoint_NilRouter(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_, err := RegisterSchemaEndpoint(&Options{DB: db})
	if err == nil {
		t.Fatal("expected ErrRouterRequired, got nil")
	}
	if err != ErrRouterRequired {
		t.Errorf("err = %v, want ErrRouterRequired", err)
	}
}

func TestRegisterSchemaEndpoint_NilDB(t *testing.T) {
	_, err := RegisterSchemaEndpoint(&Options{Router: stubRouter{}})
	if err == nil {
		t.Fatal("expected ErrDBRequired, got nil")
	}
	if err != ErrDBRequired {
		t.Errorf("err = %v, want ErrDBRequired", err)
	}
}

func TestRegisterSchemaEndpoint_NilOpts(t *testing.T) {
	_, err := RegisterSchemaEndpoint(nil)
	if err != ErrRouterRequired {
		t.Errorf("err = %v, want ErrRouterRequired", err)
	}
}

// ---------- end-to-end: real HTTP path ----------

func TestSchemaEndpoint_HappyPath(t *testing.T) {
	_, httpSrv := setupSchemaDB(t)
	rec := callEndpoint(t, httpSrv, "/schema/system/sys_users")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    []schema.Schema `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("env.Code = %d, want 0; msg=%q", env.Code, env.Message)
	}
	if len(env.Data) == 0 {
		t.Fatal("env.Data is empty; expected at least one schema row")
	}
	// sys_users must have an "id" primary key column.
	var hasID bool
	for _, row := range env.Data {
		if row.Column == "id" && row.PrimaryKey == 1 {
			hasID = true
			break
		}
	}
	if !hasID {
		t.Errorf("schema data missing id primary key row; got %d rows", len(env.Data))
	}
}

func TestSchemaEndpoint_UnknownModuleTable(t *testing.T) {
	_, httpSrv := setupSchemaDB(t)
	rec := callEndpoint(t, httpSrv, "/schema/system/nonexistent_table")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (envelope carries code); body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != int(errs.CodeNotFound) {
		t.Errorf("env.Code = %d, want %d (NotFound)", env.Code, errs.CodeNotFound)
	}
	if !strings.Contains(env.Message, "nonexistent_table") {
		t.Errorf("env.Message = %q, want it to mention the table name", env.Message)
	}
}

// Note: 4001 (Invalid) for malformed /schema/ paths is unreachable
// through the gin engine — `/schema/system` doesn't match the
// `/schema/:module/:table` route pattern, so gin returns 404 before
// our handler runs. The same parseSchemaPath logic is unit-tested by
// TestParseSchemaPath which asserts `/schema/system` → ok=false, which
// the handler maps to 4001.

func TestSchemaEndpoint_RouteRegistered(t *testing.T) {
	_, httpSrv := setupSchemaDB(t)
	routes := httpSrv.Engine().Routes()
	var seen bool
	for _, r := range routes {
		if r.Method == http.MethodGet && r.Path == "/schema/:module/:table" {
			seen = true
			break
		}
	}
	if !seen {
		t.Errorf("route GET /schema/:module/:table not registered; routes=%v", routes)
	}
}

// ---------- direct-handler coverage for paths the gin engine would
//            normalize/redirect before they reach the handler ----------
//
// Empty / wrong-prefix / missing-segment URL variants are exercised by
// TestParseSchemaPath at the pure-function layer; gin would 404 them
// before the handler sees them in production, so an end-to-end variant
// would only test gin's router, not our code.

// ---------- helpers ----------

// stubRouter is a no-op rest.Router used to test pre-conditions without
// requiring a full *ghttp.Server.
type stubRouter struct{}

func (stubRouter) Handle(string, string, http.HandlerFunc) {}
