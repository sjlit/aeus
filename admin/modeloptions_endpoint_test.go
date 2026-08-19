package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/pkg/errs"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

// ---------- pure-unit: parseModuleTablePath ----------

func TestParseModuleTablePath(t *testing.T) {
	cases := []struct {
		path        string
		wantSegment string
		module      string
		table       string
		wantOK      bool
	}{
		// model-types happy path
		{"/rest/model-types/system/sys_users", "model-types", "system", "sys_users", true},
		{"/rest/model-types/system/sys_menus", "model-types", "system", "sys_menus", true},
		// model-tiers happy path
		{"/rest/model-tiers/system/sys_menus", "model-tiers", "system", "sys_menus", true},
		// wrong segment for a given handler
		{"/rest/model-types/system/sys_menus", "model-tiers", "", "", false},
		{"/rest/model-tiers/system/sys_users", "model-types", "", "", false},
		// missing one segment
		{"/rest/model-types/system", "model-types", "", "", false},
		{"/rest/model-types", "model-types", "", "", false},
		// wrong prefix (4 segments, but first isn't "rest")
		{"/wrong/model-types/system/sys_users", "model-types", "", "", false},
		// empty segment values
		{"/rest/model-types//sys_users", "model-types", "", "", false},
		{"/rest/model-types/system/", "model-types", "", "", false},
		// trailing slash is OK
		{"/rest/model-types/system/sys_users/", "model-types", "system", "sys_users", true},
	}
	for _, c := range cases {
		m, tab, ok := parseModuleTablePath(c.path, c.wantSegment)
		if ok != c.wantOK {
			t.Errorf("parseModuleTablePath(%q, %q) ok=%v want %v", c.path, c.wantSegment, ok, c.wantOK)
			continue
		}
		if ok && (m != c.module || tab != c.table) {
			t.Errorf("parseModuleTablePath(%q, %q) = (%q,%q) want (%q,%q)",
				c.path, c.wantSegment, m, tab, c.module, c.table)
		}
	}
}

// ---------- pre-condition failures ----------

func TestRegisterModelTypesEndpoint_NilServer(t *testing.T) {
	_, err := RegisterModelTypesEndpoint(nil)
	if err != ErrRouterRequired {
		t.Errorf("err = %v, want ErrRouterRequired", err)
	}
}

func TestRegisterModelTypesEndpoint_NilRouter(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	s := &Server{opts: &Options{DB: db}}
	_, err := RegisterModelTypesEndpoint(s)
	if err != ErrRouterRequired {
		t.Errorf("err = %v, want ErrRouterRequired", err)
	}
}

func TestRegisterModelTypesEndpoint_NilDB(t *testing.T) {
	s := &Server{opts: &Options{Router: stubRouter{}}}
	_, err := RegisterModelTypesEndpoint(s)
	if err != ErrDBRequired {
		t.Errorf("err = %v, want ErrDBRequired", err)
	}
}

func TestRegisterModelTiersEndpoint_NilServer(t *testing.T) {
	_, err := RegisterModelTiersEndpoint(nil)
	if err != ErrRouterRequired {
		t.Errorf("err = %v, want ErrRouterRequired", err)
	}
}

func TestRegisterModelTiersEndpoint_NilRouter(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	s := &Server{opts: &Options{DB: db}}
	_, err := RegisterModelTiersEndpoint(s)
	if err != ErrRouterRequired {
		t.Errorf("err = %v, want ErrRouterRequired", err)
	}
}

func TestRegisterModelTiersEndpoint_NilDB(t *testing.T) {
	s := &Server{opts: &Options{Router: stubRouter{}}}
	_, err := RegisterModelTiersEndpoint(s)
	if err != ErrDBRequired {
		t.Errorf("err = %v, want ErrDBRequired", err)
	}
}

// ---------- end-to-end: real HTTP path ----------

// setupOptionDB is the same fixture used by TestSchemaEndpoint_* tests:
// in-memory SQLite, full Setup, which now also mounts the model-types
// and model-tiers endpoints as a side effect.  Seed rows are inserted
// explicitly per-test (the built-in getModels() loop does NOT seed
// data; it only registers tables / auto-migrates).
func setupOptionDB(t *testing.T) (*gorm.DB, *ghttp.Server) {
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

// callOptionEndpoint drives a real HTTP request through the gin
// engine and returns the recorded response.  qs (if non-empty) is
// appended as a URL-encoded query string.  Same shape as
// callEndpoint, but separated so the test's intent (it's hitting the
// option endpoints, not /rest/schema) is visible at the call site.
func callOptionEndpoint(t *testing.T, httpSrv *ghttp.Server, path, qs string) *httptest.ResponseRecorder {
	t.Helper()
	full := path
	if qs != "" {
		full += "?" + qs
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, full, nil)
	httpSrv.Engine().ServeHTTP(rec, req)
	return rec
}

// typeValueEnvelope mirrors the {code, message, data} shape admin's
// writeEnvelope emits.  Data is decoded as raw JSON so the test can
// assert structural shape (label/value presence) without binding to
// rest.TypeValue's generic parameter T.
type typeValueEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    []typeValueItem `json:"data"`
}

type typeValueItem struct {
	Label string          `json:"label"`
	Value json.RawMessage `json:"value"`
}

// tierValueEnvelope mirrors the {code, message, data} shape for
// ModelTiers.  Data is decoded recursively so the test can walk
// children when asserting a tier tree.
type tierValueEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    []tierValueItem `json:"data"`
}

type tierValueItem struct {
	Label    string          `json:"label"`
	Value    json.RawMessage `json:"value"`
	Children []tierValueItem `json:"children"`
}

func TestModelTypesEndpoint_HappyPath_StringValue(t *testing.T) {
	db, httpSrv := setupOptionDB(t)
	// Seed two menu rows; sys_menus has no tenant_id, so a global
	// (tenant="") query is the right shape for this model.
	if err := db.Create(&models.Menu{Component: "OptionSeedUsers", Name: "用户管理-选项测试", Parent: "SystemUserCenter"}).Error; err != nil {
		t.Fatalf("seed menu 1: %v", err)
	}
	if err := db.Create(&models.Menu{Component: "OptionSeedRoles", Name: "角色管理-选项测试", Parent: "SystemUserCenter"}).Error; err != nil {
		t.Fatalf("seed menu 2: %v", err)
	}

	qs := url.Values{}
	qs.Set("label", "name")
	qs.Set("value", "component")
	rec := callOptionEndpoint(t, httpSrv, "/rest/model-types/system/sys_menus", qs.Encode())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var env typeValueEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("env.Code = %d, want 0; msg=%q", env.Code, env.Message)
	}
	if len(env.Data) < 2 {
		t.Fatalf("env.Data length = %d, want >= 2; body=%s", len(env.Data), rec.Body.String())
	}
	// Build a label->value map so the test isn't order-sensitive
	// (rest.ModelTypes has no ORDER BY guarantee).  Setup's
	// auto-menu loop already inserts rows for every built-in model,
	// so we only assert that our two seeds are present — not that
	// they are the only rows.
	got := make(map[string]string, len(env.Data))
	for _, item := range env.Data {
		var v string
		if err := json.Unmarshal(item.Value, &v); err != nil {
			t.Fatalf("value unmarshal: %v", err)
		}
		got[item.Label] = v
	}
	if got["用户管理-选项测试"] != "OptionSeedUsers" {
		t.Errorf("got[用户管理-选项测试] = %q, want OptionSeedUsers", got["用户管理-选项测试"])
	}
	if got["角色管理-选项测试"] != "OptionSeedRoles" {
		t.Errorf("got[角色管理-选项测试] = %q, want OptionSeedRoles", got["角色管理-选项测试"])
	}
}

func TestModelTypesEndpoint_MissingLabel(t *testing.T) {
	_, httpSrv := setupOptionDB(t)
	qs := url.Values{}
	qs.Set("value", "name")
	rec := callOptionEndpoint(t, httpSrv, "/rest/model-types/system/sys_menus", qs.Encode())

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
	if env.Code != int(errs.CodeInvalid) {
		t.Errorf("env.Code = %d, want %d (Invalid)", env.Code, errs.CodeInvalid)
	}
	if !strings.Contains(env.Message, "label") {
		t.Errorf("env.Message = %q, want it to mention 'label'", env.Message)
	}
}

func TestModelTypesEndpoint_UnknownValueType(t *testing.T) {
	_, httpSrv := setupOptionDB(t)
	qs := url.Values{}
	qs.Set("label", "name")
	qs.Set("value", "component")
	qs.Set("valueType", "float64")
	rec := callOptionEndpoint(t, httpSrv, "/rest/model-types/system/sys_menus", qs.Encode())

	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != int(errs.CodeInvalid) {
		t.Errorf("env.Code = %d, want %d (Invalid); msg=%q", env.Code, errs.CodeInvalid, env.Message)
	}
	if !strings.Contains(env.Message, "float64") {
		t.Errorf("env.Message = %q, want it to mention 'float64'", env.Message)
	}
}

func TestModelTypesEndpoint_UnknownModuleTable(t *testing.T) {
	_, httpSrv := setupOptionDB(t)
	qs := url.Values{}
	qs.Set("label", "name")
	qs.Set("value", "component")
	rec := callOptionEndpoint(t, httpSrv, "/rest/model-types/system/nonexistent_table", qs.Encode())

	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != int(errs.CodeNotFound) {
		t.Errorf("env.Code = %d, want %d (NotFound); msg=%q", env.Code, errs.CodeNotFound, env.Message)
	}
	if !strings.Contains(env.Message, "nonexistent_table") {
		t.Errorf("env.Message = %q, want it to mention the table name", env.Message)
	}
}

func TestModelTypesEndpoint_RouteRegistered(t *testing.T) {
	_, httpSrv := setupOptionDB(t)
	routes := httpSrv.Engine().Routes()
	var seen bool
	for _, r := range routes {
		if r.Method == http.MethodGet && r.Path == "/rest/model-types/:module/:table" {
			seen = true
			break
		}
	}
	if !seen {
		t.Errorf("route GET /rest/model-types/:module/:table not registered; routes=%v", routes)
	}
}

func TestModelTiersEndpoint_HappyPath_StringValue(t *testing.T) {
	db, httpSrv := setupOptionDB(t)
	// Build a small tree with unique component names so the test is
	// stable against Setup's auto-menu inserts (which already add
	// rows like SystemSysMenus → SystemSettings).  Our pair:
	//   "" → "TierTestRoot" (our root)
	//   "TierTestRoot" → "TierTestChild" (our child)
	if err := db.Create(&models.Menu{Component: "TierTestRoot", Name: "tier test root", Parent: ""}).Error; err != nil {
		t.Fatalf("seed root: %v", err)
	}
	if err := db.Create(&models.Menu{Component: "TierTestChild", Name: "tier test child", Parent: "TierTestRoot"}).Error; err != nil {
		t.Fatalf("seed child: %v", err)
	}

	qs := url.Values{}
	qs.Set("parent", "parent")
	qs.Set("label", "name")
	qs.Set("value", "component")
	rec := callOptionEndpoint(t, httpSrv, "/rest/model-tiers/system/sys_menus", qs.Encode())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var env tierValueEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("env.Code = %d, want 0; msg=%q", env.Code, env.Message)
	}
	if len(env.Data) == 0 {
		t.Fatalf("env.Data is empty; want at least our seeded root; body=%s", rec.Body.String())
	}
	// Walk the top level until we find our seeded root, then assert
	// its child link.  Setup's own tree (SystemSettings → SystemSys*)
	// is also present; we don't assert on it.
	rootIdx := findTierByValue(t, env.Data, "TierTestRoot")
	if rootIdx < 0 {
		t.Fatalf("seeded root TierTestRoot not found in response; body=%s", rec.Body.String())
	}
	root := env.Data[rootIdx]
	var rootVal string
	if err := json.Unmarshal(root.Value, &rootVal); err != nil {
		t.Fatalf("root value unmarshal: %v", err)
	}
	if rootVal != "TierTestRoot" {
		t.Errorf("root value = %q, want TierTestRoot", rootVal)
	}
	childIdx := findTierByValue(t, root.Children, "TierTestChild")
	if childIdx < 0 {
		t.Fatalf("seeded child TierTestChild not nested under TierTestRoot; body=%s", rec.Body.String())
	}
	if err := json.Unmarshal(root.Children[childIdx].Value, &rootVal); err != nil {
		t.Fatalf("child value unmarshal: %v", err)
	}
}

// findTierByValue walks nodes looking for a node whose Value (as a
// JSON string) equals want.  Returns -1 when no node matches.  The
// search is flat: it does not descend into Children — the caller is
// responsible for choosing the right level to search.
func findTierByValue(t *testing.T, nodes []tierValueItem, want string) int {
	t.Helper()
	for i := range nodes {
		var v string
		if err := json.Unmarshal(nodes[i].Value, &v); err != nil {
			continue
		}
		if v == want {
			return i
		}
	}
	return -1
}

func TestModelTiersEndpoint_MissingParent(t *testing.T) {
	_, httpSrv := setupOptionDB(t)
	qs := url.Values{}
	qs.Set("label", "name")
	qs.Set("value", "component")
	rec := callOptionEndpoint(t, httpSrv, "/rest/model-tiers/system/sys_menus", qs.Encode())

	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != int(errs.CodeInvalid) {
		t.Errorf("env.Code = %d, want %d (Invalid); msg=%q", env.Code, errs.CodeInvalid, env.Message)
	}
	if !strings.Contains(env.Message, "parent") {
		t.Errorf("env.Message = %q, want it to mention 'parent'", env.Message)
	}
}

func TestModelTiersEndpoint_RouteRegistered(t *testing.T) {
	_, httpSrv := setupOptionDB(t)
	routes := httpSrv.Engine().Routes()
	var seen bool
	for _, r := range routes {
		if r.Method == http.MethodGet && r.Path == "/rest/model-tiers/:module/:table" {
			seen = true
			break
		}
	}
	if !seen {
		t.Errorf("route GET /rest/model-tiers/:module/:table not registered; routes=%v", routes)
	}
}

// ---------- valueType dispatch coverage ----------
//
// queryModelTypes / queryModelTiers are pure dispatchers — the
// reachable branches (string) are exercised end-to-end above.  These
// unit-level checks confirm the helpers compile and run for the
// common string path against a fresh, single-model fixture (no
// auto-menu noise from Setup).

func TestQueryModelTypes_StringBranch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Menu{}); err != nil {
		t.Fatalf("migrate menus: %v", err)
	}
	if err := db.Create(&models.Menu{Component: "Foo", Name: "foo", Parent: ""}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	items, err := queryModelTypes(context.Background(), db, &models.Menu{}, "", "name", "component", "string")
	if err != nil {
		t.Fatalf("queryModelTypes string: %v", err)
	}
	b, _ := json.Marshal(items)
	if !strings.Contains(string(b), `"value":"Foo"`) {
		t.Errorf("body=%s; want substring %q", string(b), `"value":"Foo"`)
	}
}

func TestQueryModelTiers_StringBranch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Menu{}); err != nil {
		t.Fatalf("migrate menus: %v", err)
	}
	if err := db.Create(&models.Menu{Component: "Root", Name: "root", Parent: ""}).Error; err != nil {
		t.Fatalf("seed root: %v", err)
	}
	if err := db.Create(&models.Menu{Component: "Child", Name: "child", Parent: "Root"}).Error; err != nil {
		t.Fatalf("seed child: %v", err)
	}
	items, err := queryModelTiers(context.Background(), db, &models.Menu{}, "", "parent", "name", "component", "string")
	if err != nil {
		t.Fatalf("queryModelTiers string: %v", err)
	}
	b, _ := json.Marshal(items)
	if !strings.Contains(string(b), `"value":"Child"`) {
		t.Errorf("body=%s; want substring %q", string(b), `"value":"Child"`)
	}
}
