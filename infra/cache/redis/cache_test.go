package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/sjlit/aeus/infra/cache"
)

func TestLoadNotFound(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	defer client.Close()

	c := NewCache(WithClient(client))
	ctx := context.Background()

	var dst string
	err := c.Load(ctx, "missing-key", &dst)
	if err != cache.ErrNotFound {
		t.Fatalf("Load missing key = %v, want cache.ErrNotFound", err)
	}
}

func TestLoadFound(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	defer client.Close()

	c := NewCache(WithClient(client))
	ctx := context.Background()

	c.Store(ctx, "existing-key", "hello", 0)

	var dst string
	err := c.Load(ctx, "existing-key", &dst)
	if err != nil {
		t.Fatalf("Load existing key failed: %v", err)
	}
	if dst != "hello" {
		t.Errorf("Load value = %q, want %q", dst, "hello")
	}
}
