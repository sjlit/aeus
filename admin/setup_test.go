package admin

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/infra/logger"
	ghttp "github.com/sjlit/aeus/transport/http"
	"gorm.io/gorm"
)

// captureLogger returns a logger.Logger whose Warn / Warnf / Error /
// Errorf etc. are routed to the returned buffer via slog's text handler.
// Test asserts use buf.String() against substrings — the structured
// key-value args show up in the text handler output as "key=value".
func captureLogger() (logger.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return logger.New(slog.New(h)), &buf
}

// TestSetup_OrphanParentWarnsAndContinues pins the warn-and-continue
// contract added to Setup's post-loop validation step.  A sys_menus
// row whose Parent references a non-existent Component must NOT abort
// Setup; instead the missing reference is logged at Warn level and
// Setup continues to mount the schema endpoint.  This guards against
// silently re-introducing a fail-fast that would block boot whenever a
// child MenuEntry().Parent points at a section menu that hasn't been
// inserted yet (the natural state on the first Setup pass when section
// menus are inserted only by Seed).
func TestSetup_OrphanParentWarnsAndContinues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Menu{}); err != nil {
		t.Fatalf("migrate sys_menus: %v", err)
	}
	// Pre-seed a row whose parent references a missing component.
	// Insert it before Setup runs so validateMenuParentsRef sees it
	// during its scan at the end of Setup.
	if err := db.Create(&models.Menu{
		Component: "SystemSysOrphan",
		Parent:    "SystemNonexistent",
		Name:      "孤儿菜单",
		Uri:       "/system/sys-orphan",
	}).Error; err != nil {
		t.Fatalf("seed orphan row: %v", err)
	}

	log, buf := captureLogger()
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv), WithLogger(log))

	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup should NOT abort on orphan Parent, got: %v", err)
	}

	out := buf.String()
	// the structured warn line should name both the failure mode and
	// the orphan component so operators can grep for it
	wantMsg := "menu parent references failed validation"
	if !strings.Contains(out, wantMsg) {
		t.Errorf("expected warn log mentioning %q, got:\n%s", wantMsg, out)
	}
	if !strings.Contains(out, "SystemNonexistent") {
		t.Errorf("expected warn log to identify the missing parent Component %q, got:\n%s",
			"SystemNonexistent", out)
	}
	// level=WARN is part of slog's text format
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("expected WARN level log, got:\n%s", out)
	}
}

// TestSetup_NoOrphans_NoWarn is the negative half of the contract: when
// every Menu.Parent resolves, validateMenuParentsRef returns nil and
// no warn-level entry is emitted.  Without this pin a future "always
// log" change would silently start spamming the log on every healthy
// boot.
//
// The 9 built-in model menus declare Parent references on section
// menus (inserted by Seed).  To isolate this test's "no extra orphan"
// intent from the expected setup-time orphans, we pre-seed the three
// section containers so Setup's auto-inserted model menus find valid
// parents on the very first pass.
func TestSetup_NoOrphans_NoWarn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Menu{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Pre-seed the three section containers so Setup's auto-inserted
	// model menus have valid parents on the first pass.
	for _, sec := range []models.Menu{
		{Component: "SystemUserCenter", Name: "用户中心"},
		{Component: "SystemLogs", Name: "日志记录"},
		{Component: "SystemSettings", Name: "系统设置"},
	} {
		if err := db.Create(&sec).Error; err != nil {
			t.Fatalf("seed section %q: %v", sec.Component, err)
		}
	}
	// closed parent chain: Parent -> some component that exists
	if err := db.Create(&models.Menu{
		Component: "ParentMenu",
		Name:      "父菜单",
	}).Error; err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if err := db.Create(&models.Menu{
		Component: "ChildMenu",
		Parent:    "ParentMenu",
		Name:      "子菜单",
		Uri:       "/system/child",
	}).Error; err != nil {
		t.Fatalf("seed child: %v", err)
	}

	log, buf := captureLogger()
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv), WithLogger(log))

	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "menu parent references failed validation") {
		t.Errorf("did not expect a warn log on healthy menu tree, got:\n%s", out)
	}
}

// TestSetup_Seed_LinksChildrenToSections pins the structural contract
// between Setup's auto-inserted model menus (which carry Parent
// references declared in each MenuEntry()) and Seed's section
// containers (inserted by EnsureSectionMenus).  After a full Setup +
// Seed on a fresh database, every built-in model's auto-derived
// Component must reference the section Component assigned in its
// MenuEntry(), and the section rows themselves must stay parent-less
// (they are top-level grouping containers).
func TestSetup_Seed_LinksChildrenToSections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	log, _ := captureLogger()
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv), WithLogger(log))
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	wantParent := map[string]string{
		"SystemSysUsers":            "SystemUserCenter",
		"SystemSysRoles":            "SystemUserCenter",
		"SystemSysRolePermissions":  "SystemUserCenter",
		"SystemSysDepartments":      "SystemUserCenter",
		"SystemSysAudits":           "SystemLogs",
		"SystemSysLoginLogs":        "SystemLogs",
		"SystemSysTenants":          "SystemSettings",
		"SystemSysMenus":            "SystemSettings",
		"SystemSysPermissions":      "SystemSettings",
	}
	for child, parent := range wantParent {
		var row models.Menu
		if err := db.Where("component = ?", child).First(&row).Error; err != nil {
			t.Errorf("child menu %q missing: %v", child, err)
			continue
		}
		if row.Parent != parent {
			t.Errorf("child %q Parent = %q, want %q", child, row.Parent, parent)
		}
	}

	// section containers themselves must stay top-level (no parent)
	for _, sec := range []string{"SystemUserCenter", "SystemLogs", "SystemSettings"} {
		var row models.Menu
		if err := db.Where("component = ?", sec).First(&row).Error; err != nil {
			t.Errorf("section menu %q missing: %v", sec, err)
			continue
		}
		if row.Parent != "" {
			t.Errorf("section %q should have empty Parent, got %q", sec, row.Parent)
		}
	}
}

// TestSetup_Seed_SortWithinSections pins the visual ordering declared
// in each MenuEntry().Sort — within a section, rows must sort
// ascending by Sort so the sidebar renders them in the documented
// order (User → Role → RolePermission → Department inside 用户中心,
// Audit → LoginLog inside 日志记录, Tenant → Menu → Permission inside
// 系统设置).
func TestSetup_Seed_SortWithinSections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	log, _ := captureLogger()
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv), WithLogger(log))
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := Seed(db, "admin", "admin123"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	wantSort := map[string]int64{
		"SystemUserCenter":  10,
		"SystemLogs":        20,
		"SystemSettings":    30,
		"SystemSysUsers":            10,
		"SystemSysRoles":            20,
		"SystemSysRolePermissions":  30,
		"SystemSysDepartments":      40,
		"SystemSysAudits":           10,
		"SystemSysLoginLogs":        20,
		"SystemSysTenants":          10,
		"SystemSysMenus":            20,
		"SystemSysPermissions":      30,
	}
	for component, want := range wantSort {
		var row models.Menu
		if err := db.Where("component = ?", component).First(&row).Error; err != nil {
			t.Errorf("menu %q missing: %v", component, err)
			continue
		}
		if row.Sort != want {
			t.Errorf("menu %q Sort = %d, want %d", component, row.Sort, want)
		}
	}
}