package models

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newMenuDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Menu{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMenu_BuildTree_Flattens(t *testing.T) {
	menus := []Menu{
		{Component: "Layout", Name: "system", Uri: "/system", Parent: ""},
		{Component: "User", Name: "user", Uri: "/system/user", Parent: "Layout"},
		{Component: "Role", Name: "role", Uri: "/system/role", Parent: "Layout"},
	}
	tree := (&Menu{}).BuildTree(menus)
	if len(tree) != 1 {
		t.Fatalf("got %d, want 1 root", len(tree))
	}
	if tree[0].Component != "Layout" {
		t.Fatalf("root want Layout, got %q", tree[0].Component)
	}
	if len(tree[0].Children) != 2 {
		t.Fatalf("Layout should have 2 children, got %d", len(tree[0].Children))
	}
}

func TestMenu_BuildTree_OrphanBecomesRoot(t *testing.T) {
	menus := []Menu{
		{Component: "ghost-child", Name: "ghost-child", Parent: "nonexistent"},
		{Component: "real", Name: "real", Parent: ""},
	}
	tree := (&Menu{}).BuildTree(menus)
	if len(tree) != 2 {
		t.Fatalf("orphan should fall back to root, want 2 roots, got %d", len(tree))
	}
}

func TestMenu_BuildTree_Empty(t *testing.T) {
	tree := (&Menu{}).BuildTree(nil)
	if len(tree) != 0 {
		t.Fatalf("got %d, want 0", len(tree))
	}
}

func TestMenu_PathTo_Deep(t *testing.T) {
	db := newMenuDB(t)
	chain := []Menu{
		{Component: "a", Name: "a", Parent: ""},
		{Component: "b", Name: "b", Parent: "a"},
		{Component: "c", Name: "c", Parent: "b"},
	}
	for i := range chain {
		if err := db.Create(&chain[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := (&Menu{}).PathTo(db, context.Background(), chain[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestMenu_PathTo_NotFound(t *testing.T) {
	db := newMenuDB(t)
	_, err := (&Menu{}).PathTo(db, context.Background(), 9999)
	if err == nil {
		t.Fatal("want error for missing id")
	}
}

func TestMenu_PathTo_BrokenChainStops(t *testing.T) {
	db := newMenuDB(t)
	if err := db.Create(&Menu{Component: "orphan", Name: "orphan", Parent: "ghost"}).Error; err != nil {
		t.Fatal(err)
	}
	var first Menu
	if err := db.Where("component = ?", "orphan").First(&first).Error; err != nil {
		t.Fatal(err)
	}
	got, err := (&Menu{}).PathTo(db, context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "orphan" {
		t.Fatalf("got %v, want [orphan]", got)
	}
}
