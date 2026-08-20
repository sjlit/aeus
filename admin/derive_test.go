package admin

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/admin/models"
	ghttp "github.com/sjlit/aeus/transport/http"
	"github.com/sjlit/rest/v3"
	"github.com/sjlit/rest/v3/formats"
	"gorm.io/gorm"
)

// newResourceForBackfill builds a *rest.Resource for fakeGroupBackfill
// without mounting it on a router.  ensurePermissionRows only needs
// the resource handle (it reads ModuleName/TableName/Scenarios off it
// to derive the data strings); skipping Register/Handle lets us call
// ensurePermissionRows twice for the same model without rest/v3's
// gin panic on duplicate route mounts.
func newResourceForBackfill(t *testing.T, db *gorm.DB, model fakeGroupBackfill) *rest.Resource {
	t.Helper()
	r, err := rest.NewResourceWithOptions(model,
		rest.ResourceConfig{
			Router:    nil, // unused — we never call Register
			Formatter: formats.DefaultFormatter(),
		},
		rest.WithDB(db),
	)
	if err != nil {
		t.Fatalf("NewResourceWithOptions: %v", err)
	}
	return r
}

// fakeGroupModel is a stand-in resource registered only for testing
// derive.go's Group wiring.  MenuEntry returns "测试分组" so a derived
// Group should equal that exact string — no MenuProvider wiring shared
// with production models, keeping this test independent of them.
//
// TableName "fake_groups" is plural; rest/v3's BuildUri singularizes
// the table to "fake_group" when emitting /system/fake_group/*, which
// is why the test fixtures below all use the singular form.
type fakeGroupModel struct {
	models.BaseModel
	Name string `gorm:"size:64"`
}

func (fakeGroupModel) TableName() string { return "fake_groups" }
func (fakeGroupModel) ModuleName() string { return "system" }
func (fakeGroupModel) MenuEntry() models.MenuSpec {
	return models.MenuSpec{Name: "测试分组", Sort: 1}
}

// fakeGroupBackfill is the back-fill test's twin — same MenuEntry but
// a different TableName, so the second RegisterModel mounts a distinct
// gin route set and avoids rest/v3's duplicate-URI panic.
type fakeGroupBackfill struct {
	models.BaseModel
	Name string `gorm:"size:64"`
}

func (fakeGroupBackfill) TableName() string { return "fake_group_fills" }
func (fakeGroupBackfill) ModuleName() string { return "system" }
func (fakeGroupBackfill) MenuEntry() models.MenuSpec {
	return models.MenuSpec{Name: "测试分组", Sort: 1}
}

// allRows is the helper used by the two tests below to dump every
// permission row in the catalog — purely diagnostic, so failures
// surface the actual data instead of "len == 0" with no clue why.
func dumpPermRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	var rows []models.Permission
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("Find permissions: %v", err)
	}
	for _, r := range rows {
		t.Logf("catalog row: data=%q group=%q", r.Data, r.Group)
	}
}

// fakeGroupRows returns the subset of catalog rows for the model under
// test, identified by the singular URI prefix rest/v3 emits.  Filtering
// by the known canonical-6 scenario paths avoids cross-talk with
// built-in models Setup() pre-registers and bypasses the LIKE "_"
// wildcard that singularized table names would mis-match.
func fakeGroupRows(t *testing.T, db *gorm.DB, singularURI string) []models.Permission {
	t.Helper()
	const where = `data IN (?, ?, ?, ?, ?, ?)`
	singular := singularURI
	plural := singularURI + "s" // rest/v3's "search" scenario emits <singular>s
	args := []any{
		"POST " + singular,
		"PUT " + singular + "/:id",
		"DELETE " + singular + "/:id",
		"GET " + plural,
		"GET " + singular + "/detail/:id",
		"GET " + singular + "/export",
	}
	var rows []models.Permission
	if err := db.Where(where, args...).Find(&rows).Error; err != nil {
		t.Fatalf("Find permissions: %v", err)
	}
	return rows
}

// blankFakeGroups zeros the Group column for the given singular URI's
// catalog rows.  Used by the back-fill test to simulate pre-migration
// data without disturbing the built-in catalog rows Setup() seeded.
func blankFakeGroups(t *testing.T, db *gorm.DB, singularURI string) {
	t.Helper()
	singular := singularURI
	plural := singularURI + "s"
	if err := db.Model(&models.Permission{}).
		Where(`data IN (?, ?, ?, ?, ?, ?)`,
			"POST "+singular,
			"PUT "+singular+"/:id",
			"DELETE "+singular+"/:id",
			"GET "+plural,
			"GET "+singular+"/detail/:id",
			"GET "+singular+"/export",
		).
		Update("group", "").Error; err != nil {
		t.Fatalf("blank Group: %v", err)
	}
}

// operatorCustomGroup updates the lowest-id row for the given URI to
// `custom`.  Mirrors how an operator would edit the catalog by hand:
// only the row they touched changes, the rest stay empty so the
// back-fill pass can be observed.
func operatorCustomGroup(t *testing.T, db *gorm.DB, singularURI, custom string) {
	t.Helper()
	singular := singularURI
	plural := singularURI + "s"
	if err := db.Exec(
		`UPDATE sys_permissions SET "group" = ? WHERE id = (
			SELECT MIN(id) FROM sys_permissions
			WHERE data IN (?, ?, ?, ?, ?, ?)
		)`,
		custom,
		"POST "+singular,
		"PUT "+singular+"/:id",
		"DELETE "+singular+"/:id",
		"GET "+plural,
		"GET "+singular+"/detail/:id",
		"GET "+singular+"/export",
	).Error; err != nil {
		t.Fatalf("set custom Group: %v", err)
	}
}

// TestEnsurePermissionRows_GroupAutoSet pins the happy-path: a fresh
// Setup auto-registers one permission row whose Group is populated
// from MenuEntry().Name, so the UI's grouping has authoritative data
// without operator intervention.
func TestEnsurePermissionRows_GroupAutoSet(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	httpSrv := ghttp.New()
	s := New(WithDB(db), WithRouter(httpSrv))
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := s.RegisterModel(fakeGroupModel{}); err != nil {
		t.Fatalf("RegisterModel: %v", err)
	}

	rows := fakeGroupRows(t, db, "/system/fake_group")
	if len(rows) == 0 {
		dumpPermRows(t, db)
		t.Fatal("RegisterModel should have auto-created at least one permission row for fakeGroupModel")
	}
	for _, r := range rows {
		if r.Group != "测试分组" {
			t.Errorf("permission %q has Group = %q, want %q", r.Data, r.Group, "测试分组")
		}
	}
}

// TestEnsurePermissionRows_BackfillsEmptyGroup pins the migration
// contract: a row whose Group is empty (legacy data from before the
// column existed, or rows an operator wiped) gets back-filled on the
// next ensurePermissionRows pass — but operator-set Group values are
// NEVER overwritten, even when re-seeding with the same model.
//
// We construct the *rest.Resource directly (instead of going through
// Server.RegisterModel) so we can call ensurePermissionRows twice for
// the same model without rest/v3's gin panic on duplicate route
// mounts.  The catalog side is idempotent; only the gin route mount
// isn't — that's exactly what we're bypassing here.
func TestEnsurePermissionRows_BackfillsEmptyGroup(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	s := New(WithDB(db), WithRouter(ghttp.New()))
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	resource := newResourceForBackfill(t, db, fakeGroupBackfill{})

	// First pass: seeds the catalog with Group populated.
	if err := s.ensurePermissionRows(db, resource, nil); err != nil {
		t.Fatalf("ensurePermissionRows (first pass): %v", err)
	}

	const uri = "/system/fake_group_fill"
	if rows := fakeGroupRows(t, db, uri); len(rows) == 0 {
		dumpPermRows(t, db)
		t.Fatal("precondition: fake rows must exist before back-fill test")
	}

	// Pretend the rows pre-date the Group column: blank it.
	blankFakeGroups(t, db, uri)

	// And pretend an operator edited one fake row to a custom value.
	const customGroup = "运营改的分组"
	operatorCustomGroup(t, db, uri, customGroup)

	// Second pass: back-fills empty Groups, leaves the operator-set
	// value alone.
	if err := s.ensurePermissionRows(db, resource, nil); err != nil {
		t.Fatalf("ensurePermissionRows (second pass): %v", err)
	}

	rows := fakeGroupRows(t, db, uri)
	customSeen := false
	for _, r := range rows {
		switch r.Group {
		case "测试分组":
			// back-filled — good
		case customGroup:
			customSeen = true
		case "":
			t.Errorf("permission %q left Group empty after back-fill pass", r.Data)
		default:
			t.Errorf("permission %q has unexpected Group = %q", r.Data, r.Group)
		}
	}
	if !customSeen {
		t.Errorf("operator-set Group %q should NOT have been overwritten", customGroup)
	}
}