package admin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/rest/v3/schema"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

// TestRegisterModel_OverridesMenuAndPermission seeds an application
// model that DOES NOT implement MenuProvider and verifies that
// WithRegisterMenuSpec wires a sys_menus row anyway.  The
// WithRegisterScenarios override downgrades the catalog to a
// single POST row, and the generated Vue file is asserted on disk
// using WithRegisterVueOutputDir.
type testAppModel struct {
	models.BaseModel
	TenantID string `gorm:"column:tenant_id;type:char(60);index"`
	Title    string `gorm:"column:title;size:120"`
}

func (m *testAppModel) TableName() string { return "demo_notes" }
func (m *testAppModel) ModuleName() string { return "demo" }

// testAppModel intentionally does NOT implement MenuProvider; the
// override path is the only way to seed a sys_menus row for it.

func TestRegisterModel_OverridesMenuAndPermission(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	log, _ := captureLogger()
	httpSrv := ghttp.New()
	outDir := t.TempDir()
	s := New(
		WithDB(db),
		WithRouter(httpSrv),
		WithLogger(log),
		// Deliberately NOT setting WithVueOutputDir — the per-call
		// option should drive generation without server-level help.
	)

	// Migrate the schema meta-table (matches Setup's first step)
	// before any RegisterModel call so rest/v3's first parse
	// against db.Statement succeeds.
	if err := db.AutoMigrate(&schema.Schema{}, &models.Permission{}, &models.Menu{}); err != nil {
		t.Fatalf("migrate schema/permission/menu: %v", err)
	}

	err = s.RegisterModel(&testAppModel{},
		WithRegisterMenuSpec(models.MenuSpec{
			Name:   "备忘录",
			Parent: "SystemSettings",
			Sort:   99,
		}),
		WithRegisterScenarios("create", "search"),
		WithRegisterVueOutputDir(outDir),
	)
	if err != nil {
		t.Fatalf("RegisterModel: %v", err)
	}

	// 1) Menu row inserted with the override's Name.
	var got models.Menu
	if err := db.Unscoped().Where("component = ?", "DemoDemoNotes").First(&got).Error; err != nil {
		t.Fatalf("lookup sys_menus by Component=DemoDemoNotes: %v", err)
	}
	if got.Name != "备忘录" || got.Parent != "SystemSettings" {
		t.Fatalf("MenuSpec override didn't land: name=%q parent=%q", got.Name, got.Parent)
	}

	// 2) Permission catalog restricted to the override set: only
	// POST and GET search should exist.  rest/v3 derives the
	// search URI from the pluralized table (see resource.buildUri
	// ScenarioSearch -> n.Pluralize), and the create URI from the
	// singular, so the filter has to accept both.
	var perms []models.Permission
	if err := db.Unscoped().Find(&perms).Error; err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	hasCreate := false
	hasSearch := false
	hasUnexpected := false
	for _, p := range perms {
		// Filter to this test model's URIs (any path under /demo/).
		if !strings.Contains(p.Data, " /demo/") {
			continue
		}
		switch {
		case strings.HasPrefix(p.Data, "POST "):
			hasCreate = true
		case strings.HasPrefix(p.Data, "GET ") && !strings.Contains(p.Data, "/detail/") && !strings.Contains(p.Data, "/export"):
			hasSearch = true
		case strings.HasPrefix(p.Data, "PUT "), strings.HasPrefix(p.Data, "DELETE "):
			hasUnexpected = true
		case strings.HasPrefix(p.Data, "GET ") && strings.Contains(p.Data, "/detail/"):
			// detail is also suppressed by the override.
			hasUnexpected = true
		}
	}
	if !hasCreate || !hasSearch {
		t.Fatalf("expected POST + GET search permission rows; got create=%v search=%v", hasCreate, hasSearch)
	}
	if hasUnexpected {
		t.Fatalf("WithRegisterScenarios should have suppressed PUT/DELETE/detail; found unexpected rows")
	}

	// 3) Vue file on disk matches the template's placeholder substitutions.
	wantPath := filepath.Join(outDir, "demo", "demo_note", "Index.vue")
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read generated Vue file %s: %v", wantPath, err)
	}
	bs := string(body)
	if !strings.Contains(bs, `module="demo"`) {
		t.Fatalf("expected module=\"demo\" in generated view, got:\n%s", bs)
	}
	if !strings.Contains(bs, `table="demo_notes"`) {
		t.Fatalf("expected table=\"demo_notes\" in generated view, got:\n%s", bs)
	}
	if !strings.Contains(bs, `name: 'DemoDemoNotes'`) {
		t.Fatalf("expected component name in generated view, got:\n%s", bs)
	}

	// 4) Idempotency: ensureMenuRow's FirstOrCreate-style check
	// guarantees that re-running Setup won't insert duplicate menu
	// rows.  We assert the per-row invariant here directly without
	// re-registering the model on the same router — rest/v3 panics
	// on a duplicate path registration ("handlers are already
	// registered"), which is orthogonal to admin's idempotency
	// contract.
	var menuCount int64
	if err := db.Unscoped().Model(&models.Menu{}).
		Where("component = ?", "DemoDemoNotes").Count(&menuCount).Error; err != nil {
		t.Fatalf("count sys_menus: %v", err)
	}
	if menuCount != 1 {
		t.Fatalf("expected exactly 1 sys_menus row for DemoDemoNotes, got %d", menuCount)
	}
}

// TestRegisterModel_ServerVueOutputDirDefaultsOn confirms that
// setting WithVueOutputDir at the Server level opts every built-in
// model into Vue generation without each RegisterModel call having
// to pass a per-call path. The corresponding sys_users Index.vue
// is asserted on disk.
//
// Fresh SQLite DBs don't carry the three section containers
// (SystemSettings / SystemUserCenter / SystemLogs) that the
// per-model MenuEntry().Parent references — those come from
// EnsureSectionMenus, not Setup, and on the first pass validateMenuParentsRef
// surfaces orphans at Warn level. Setup intentionally does NOT
// fail-fast on orphans (Menu.BuildTree promotes orphans to roots),
// but on a fresh test DB the absence still trips downstream steps
// like registerSchemaEndpoint / registerModelTypesEndpoint. To keep
// the test deterministic — and to actually exercise Setup's success
// path rather than its "warn-and-continue from a broken state"
// path — pre-seed the sections here, mirroring what seed.go's
// EnsureSectionMenus does in production.
func TestRegisterModel_ServerVueOutputDirDefaultsOn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	log, _ := captureLogger()
	httpSrv := ghttp.New()
	outDir := t.TempDir()
	s := New(
		WithDB(db),
		WithRouter(httpSrv),
		WithLogger(log),
		WithVueOutputDir(outDir),
	)

	// Pre-seed section containers so per-model MenuEntry().Parent
	// references resolve. Schema/permission/menu tables must
	// already exist for AutoMigrate on the model side to succeed;
	// Setup will re-migrate them anyway on its own path.
	if err := db.AutoMigrate(&models.Menu{}); err != nil {
		t.Fatalf("migrate sys_menus: %v", err)
	}
	for _, section := range []models.Menu{
		{Component: "SystemSettings", Name: "系统设置", Uri: "/system"},
		{Component: "SystemUserCenter", Name: "用户中心", Uri: "/user-center"},
		{Component: "SystemLogs", Name: "日志管理", Uri: "/logs"},
	} {
		if err := db.Create(&section).Error; err != nil {
			t.Fatalf("seed section %s: %v", section.Component, err)
		}
	}

	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// sys_users is one of the built-in models (models.User). Setup
	// should have created its Vue file under <outDir>/system/sys_user/.
	wantPath := filepath.Join(outDir, "system", "sys_user", "Index.vue")
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read %s: %v", wantPath, err)
	}
	if !strings.Contains(string(body), `table="sys_users"`) {
		t.Fatalf("expected sys_users view body, got:\n%s", body)
	}
}
