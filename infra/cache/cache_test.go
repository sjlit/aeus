package cache

import (
	"context"
	"testing"

	"github.com/sjlit/aeus/infra/cache/memory"
)

func TestConstants(t *testing.T) {
	if DefaultExpiration != 0 {
		t.Errorf("DefaultExpiration should be 0, got %v", DefaultExpiration)
	}
	if NoExpiration != -1 {
		t.Errorf("NoExpiration should be -1, got %v", NoExpiration)
	}
	if ErrNotFound == nil {
		t.Error("ErrNotFound should not be nil")
	}
}

func TestTypedCache(t *testing.T) {
	mem := memory.NewCache()
	c := NewTypedCache[string](mem)

	// Store
	if err := c.Store(context.Background(), "k1", "v1", NoExpiration); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Load
	val, err := c.Load(context.Background(), "k1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if val != "v1" {
		t.Errorf("Load value = %q, want %q", val, "v1")
	}

	// Exists
	ok, err := c.Exists(context.Background(), "k1")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !ok {
		t.Error("Exists should return true")
	}

	// Delete
	if err := c.Delete(context.Background(), "k1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Load after delete
	_, err = c.Load(context.Background(), "k1")
	if err != ErrNotFound {
		t.Errorf("Load after delete = %v, want ErrNotFound", err)
	}
}
