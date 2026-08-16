package memory

import (
	"context"
	"testing"
	"time"
)

func TestDeleteNotExists(t *testing.T) {
	t.Parallel()
	c := NewCache()
	ctx := context.Background()

	err := c.Delete(ctx, "no-such-key")
	if err != nil {
		t.Fatalf("Delete non-existent key should return nil, got %v", err)
	}
}

func TestLoadNilValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("internal nil item", func(t *testing.T) {
		c := NewCache()
		c.items["nil-key"] = Item{Value: nil, Expiration: 0}

		var dst string
		err := c.Load(ctx, "nil-key", &dst)
		if err != ErrNotFound {
			t.Fatalf("Load nil value = %v, want ErrNotFound", err)
		}
	})

	t.Run("stored nil value", func(t *testing.T) {
		c := NewCache()
		c.Store(ctx, "nil-key", nil, DefaultExpiration)

		var dst string
		err := c.Load(ctx, "nil-key", &dst)
		if err != ErrNotFound {
			t.Fatalf("Load stored nil value = %v, want ErrNotFound", err)
		}
	})
}

func TestExpirationSemantics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("NoExpiration means never expire", func(t *testing.T) {
		c := NewCache(Expiration(time.Hour))
		c.Store(ctx, "k1", "v1", NoExpiration)

		var v string
		if err := c.Load(ctx, "k1", &v); err != nil {
			t.Fatalf("NoExpiration load failed: %v", err)
		}
		if v != "v1" {
			t.Errorf("value = %q, want %q", v, "v1")
		}
	})

	t.Run("DefaultExpiration uses opts.Expiration", func(t *testing.T) {
		c := NewCache(Expiration(50 * time.Millisecond))
		c.Store(ctx, "k2", "v2", DefaultExpiration)

		time.Sleep(100 * time.Millisecond)
		var v string
		err := c.Load(ctx, "k2", &v)
		if err != ErrNotFound {
			t.Fatalf("DefaultExpiration expired load = %v, want ErrNotFound", err)
		}
	})

	t.Run("explicit duration", func(t *testing.T) {
		c := NewCache()
		c.Store(ctx, "k3", "v3", 50*time.Millisecond)

		time.Sleep(100 * time.Millisecond)
		var v string
		err := c.Load(ctx, "k3", &v)
		if err != ErrNotFound {
			t.Fatalf("explicit duration expired load = %v, want ErrNotFound", err)
		}
	})
}
