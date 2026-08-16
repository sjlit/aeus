package dbcache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/sjlit/aeus/infra/cache/memory"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// setupDB opens an in-memory sqlite DB for a test.
func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// storeEntry seeds the cache with a raw cacheEntry, bypassing Try so
// tests can control CompareValue and CreatedAt directly.
func storeEntry[T any](t *testing.T, c *Cacher, key string, e *cacheEntry[T]) {
	t.Helper()
	if err := c.cache.Store(context.Background(), key, e, time.Hour); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
}

func TestTry_KeyTooLong(t *testing.T) {
	c := New(setupDB(t))
	_, err := Try(c, context.Background(), strings.Repeat("a", maxCacheKeyLen+1), func(tx *gorm.DB) (string, error) {
		return "value", nil
	})
	if !errs.IsCode(err, errs.CodeInvalid) {
		t.Fatalf("want code %d, got %v", errs.CodeInvalid, err)
	}
}

// emptyDependency always reports a fresh, empty version marker.
type emptyDependency struct{}

func (emptyDependency) GetValue(ctx context.Context, tx *gorm.DB) (string, error) {
	return "", nil
}

func TestTry_StaleEntryReloadsOnDependencyChange(t *testing.T) {
	c := New(setupDB(t), WithCache(memory.NewCache()), WithDependency(emptyDependency{}))
	key := "dep-change"
	storeEntry(t, c, key, &cacheEntry[string]{
		Value:        "cached-value",
		CompareValue: "old-value",
		CreatedAt:    time.Now().Add(-2 * time.Second).UnixMilli(),
	})

	called := false
	result, err := Try(c, context.Background(), key, func(tx *gorm.DB) (string, error) {
		called = true
		return "db-value", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("dependency change must invalidate the cache and reload")
	}
	if result != "db-value" {
		t.Fatalf("want db-value, got %s", result)
	}
}

func TestTry_StaleEntryHitWhenDependencyUnchanged(t *testing.T) {
	c := New(setupDB(t), WithCache(memory.NewCache()), WithDependency(emptyDependency{}))
	key := "dep-match"
	storeEntry(t, c, key, &cacheEntry[string]{
		Value:        "cached-value",
		CompareValue: "",
		CreatedAt:    time.Now().Add(-2 * time.Second).UnixMilli(),
	})

	called := false
	result, err := Try(c, context.Background(), key, func(tx *gorm.DB) (string, error) {
		called = true
		return "db-value", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("matching dependency must serve the cached value")
	}
	if result != "cached-value" {
		t.Fatalf("want cached-value, got %s", result)
	}
}

// errorDependency always fails.
type errorDependency struct{}

func (errorDependency) GetValue(ctx context.Context, tx *gorm.DB) (string, error) {
	return "", errors.New("dependency error")
}

func TestTry_DependencyErrorAborts(t *testing.T) {
	c := New(setupDB(t), WithDependency(errorDependency{}))
	called := false
	_, err := Try(c, context.Background(), "dep-error", func(tx *gorm.DB) (string, error) {
		called = true
		return "db-value", nil
	})
	if err == nil {
		t.Fatal("want error when the dependency query fails")
	}
	if called {
		t.Fatal("loader must not run when the dependency query fails")
	}
}

func TestTry_ClockRewindReloads(t *testing.T) {
	c := New(setupDB(t), WithCache(memory.NewCache()))
	key := "clock-rewind"
	storeEntry(t, c, key, &cacheEntry[string]{
		Value:        "cached-value",
		CompareValue: "v1",
		CreatedAt:    time.Now().Add(time.Hour).UnixMilli(), // in the future
	})

	called := false
	result, err := Try(c, context.Background(), key, func(tx *gorm.DB) (string, error) {
		called = true
		return "db-value", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("a future CreatedAt must not serve stale cache forever")
	}
	if result != "db-value" {
		t.Fatalf("want db-value, got %s", result)
	}
}

func TestTry_FreshHitSkipsLoader(t *testing.T) {
	c := New(setupDB(t))
	key := "fresh-hit"

	first, err := Try(c, context.Background(), key, func(tx *gorm.DB) (string, error) {
		return "value", nil
	})
	if err != nil || first != "value" {
		t.Fatalf("first load: %v %q", err, first)
	}

	called := false
	second, err := Try(c, context.Background(), key, func(tx *gorm.DB) (string, error) {
		called = true
		return "value2", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("a fresh entry must be served from cache")
	}
	if second != "value" {
		t.Fatalf("want cached value, got %s", second)
	}
}

func TestTry_GraceWindowSkipsDependency(t *testing.T) {
	c := New(setupDB(t), WithCache(memory.NewCache()), WithDependency(emptyDependency{}))
	key := "grace-window"
	// CompareValue differs from the dependency value, but the entry is
	// younger than the 1s grace window: it must be served without the
	// dependency query or the loader.
	storeEntry(t, c, key, &cacheEntry[string]{
		Value:        "cached-value",
		CompareValue: "old-value",
		CreatedAt:    time.Now().UnixMilli(),
	})

	called := false
	result, err := Try(c, context.Background(), key, func(tx *gorm.DB) (string, error) {
		called = true
		return "db-value", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called || result != "cached-value" {
		t.Fatalf("grace window must serve cache without loader, got called=%v result=%q", called, result)
	}
}

type reqCtxKey struct{}

// recordingDependency records the context carried by the tx it receives.
type recordingDependency struct {
	gotCtx context.Context
}

func (d *recordingDependency) GetValue(ctx context.Context, tx *gorm.DB) (string, error) {
	d.gotCtx = tx.Statement.Context
	return "v", nil
}

func TestTry_DependencyGetsRequestContext(t *testing.T) {
	c := New(setupDB(t), WithCache(memory.NewCache()), WithDependency(&recordingDependency{}))
	key := "ctx-propagation"
	storeEntry(t, c, key, &cacheEntry[string]{
		Value:        "cached-value",
		CompareValue: "stale",
		CreatedAt:    time.Now().Add(-2 * time.Second).UnixMilli(),
	})

	rctx := context.WithValue(context.Background(), reqCtxKey{}, "marker")
	if _, err := Try(c, rctx, key, func(tx *gorm.DB) (string, error) {
		return "db-value", nil
	}); err != nil {
		t.Fatal(err)
	}
	dep := c.dependency.(*recordingDependency)
	if dep.gotCtx == nil || dep.gotCtx.Value(reqCtxKey{}) != "marker" {
		t.Fatalf("dependency must run with the request context, got %v", dep.gotCtx)
	}
}
