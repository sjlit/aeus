package dbcache

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDependDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func TestSqlDependency_InvalidTableName(t *testing.T) {
	dep := NewSqlDependency(WithTable("users; DROP TABLE users; --"), WithColumn("MAX(updated_at)"))
	_, err := dep.GetValue(context.Background(), setupDependDB(t))
	if err == nil || !strings.Contains(err.Error(), "invalid table name") {
		t.Fatalf("want 'invalid table name' error, got %v", err)
	}
}

func TestSqlDependency_InvalidColumn(t *testing.T) {
	dep := NewSqlDependency(WithTable("users"), WithColumn("MAX(updated_at); DROP TABLE users; --"))
	_, err := dep.GetValue(context.Background(), setupDependDB(t))
	if err == nil || !strings.Contains(err.Error(), "invalid column expression") {
		t.Fatalf("want 'invalid column expression' error, got %v", err)
	}
}

func TestSqlDependency_ColumnBypassAttemptsRejected(t *testing.T) {
	invalid := []string{
		"MAX(updated_at)||'injected'",
		"MAX(updated_at)<1",
		"MAX(updated_at)>1",
		"COUNT(*)=1",
		"MAX(updated_at) AND 1=1",
	}
	for _, expr := range invalid {
		dep := NewSqlDependency(WithTable("users"), WithColumn(expr))
		if _, err := dep.GetValue(context.Background(), setupDependDB(t)); err == nil || !strings.Contains(err.Error(), "invalid column expression") {
			t.Fatalf("want error for %q, got %v", expr, err)
		}
	}
}

func TestSqlDependency_RequiresTableOrModel(t *testing.T) {
	dep := NewSqlDependency(WithColumn("MAX(updated_at)"))
	_, err := dep.GetValue(context.Background(), setupDependDB(t))
	if err == nil {
		t.Fatal("want error when neither table nor model is set")
	}
}

func TestSqlDependency_EmptyTableMarkerIsEmpty(t *testing.T) {
	db := setupDependDB(t)
	if err := db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, updated_at INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	// Aggregates over an empty table return NULL; a fresh install has
	// empty permission tables, so the marker must come back as "" — not
	// a scan error.
	dep := NewSqlDependency(WithTable("test_table"), WithColumn("MAX(updated_at)"))
	val, err := dep.GetValue(context.Background(), db)
	if err != nil {
		t.Fatalf("empty table marker must not error: %v", err)
	}
	if val != "" {
		t.Fatalf("want empty marker, got %q", val)
	}
}

func TestSqlDependency_ValidWithTable(t *testing.T) {
	db := setupDependDB(t)
	if err := db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, updated_at INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO test_table (updated_at) VALUES (12345)").Error; err != nil {
		t.Fatal(err)
	}
	dep := NewSqlDependency(WithTable("test_table"), WithColumn("MAX(updated_at)"))
	val, err := dep.GetValue(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if val != "12345" {
		t.Fatalf("want 12345, got %s", val)
	}
}

func TestSqlDependency_ValidWithCondition(t *testing.T) {
	db := setupDependDB(t)
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, tenant TEXT, updated_at INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO users (tenant, updated_at) VALUES ('t1', 111), ('t2', 222)").Error; err != nil {
		t.Fatal(err)
	}
	dep := NewSqlDependency(WithTable("users"), WithColumn("MAX(updated_at)"), WithCondition("tenant = ?", "t1"))
	val, err := dep.GetValue(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if val != "111" {
		t.Fatalf("want 111 (t1 only), got %s", val)
	}
}

type depModel struct {
	ID        uint `gorm:"primaryKey"`
	UpdatedAt int64
}

func (depModel) TableName() string { return "dep_models" }

func TestSqlDependency_ValidWithModel(t *testing.T) {
	db := setupDependDB(t)
	if err := db.AutoMigrate(&depModel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&depModel{UpdatedAt: 999}).Error; err != nil {
		t.Fatal(err)
	}
	dep := NewSqlDependency(WithModel(&depModel{}), WithColumn("MAX(updated_at)"))
	val, err := dep.GetValue(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if val != "999" {
		t.Fatalf("want 999, got %s", val)
	}
}
