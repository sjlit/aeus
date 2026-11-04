package admin

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	ghttp "github.com/sjlit/aeus/transport/http"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"

	"github.com/sjlit/aeus/admin/models"
)

// TestSetup_AutoMigrateDisabledSkipsBusinessDDL pins the production
// escape hatch: Setup must not create business tables when the caller
// has already run a migration step. The schema metadata is still
// populated because resource rendering depends on it.
func TestSetup_AutoMigrateDisabledSkipsBusinessDDL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	// The migration step would create these in a real deployment.
	if err := db.AutoMigrate(&schema.Schema{}, &models.Menu{}, &models.Permission{}); err != nil {
		t.Fatal(err)
	}

	s := New(
		WithDB(db),
		WithRouter(ghttp.New()),
		WithAutoMigrate(false),
	)
	if err := s.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if db.Migrator().HasTable(&models.User{}) {
		t.Fatal("Setup created sys_users despite WithAutoMigrate(false)")
	}

	var count int64
	if err := db.Model(&schema.Schema{}).
		Where("module_name = ? AND table_name = ?", "system", "sys_users").
		Count(&count).Error; err != nil {
		t.Fatalf("count schema metadata: %v", err)
	}
	if count == 0 {
		t.Fatal("Setup skipped sys_schemas metadata")
	}
}

// TestSetup_AutoMigrateDisabledDoesNotCreatePermissionTable guards the
// remaining defensive AutoMigrate in derive.go: without sys_permissions
// the catalog write must fail, not silently create the table.
func TestSetup_AutoMigrateDisabledDoesNotCreatePermissionTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&schema.Schema{}, &models.Menu{}); err != nil {
		t.Fatal(err)
	}

	s := New(
		WithDB(db),
		WithRouter(ghttp.New()),
		WithAutoMigrate(false),
	)
	if err := s.Setup(context.Background()); err == nil {
		t.Fatal("Setup unexpectedly succeeded without sys_permissions")
	}
	if db.Migrator().HasTable(&models.Permission{}) {
		t.Fatal("Setup created sys_permissions despite WithAutoMigrate(false)")
	}
}
