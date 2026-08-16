package cache

import (
	"context"
	"time"

	"github.com/sjlit/aeus/infra/cache/memory"
)

var ErrNotFound = memory.ErrNotFound

const (
	DefaultExpiration time.Duration = 0
	NoExpiration      time.Duration = -1
)

var (
	std = memory.NewCache()
)

type Cache interface {
	// Get gets a cached value by key.
	Load(ctx context.Context, key string, val any) error
	// Exists checks if a key exists in cache.
	Exists(ctx context.Context, key string) (bool, error)
	// Put stores a key-value pair into cache.
	Store(ctx context.Context, key string, val any, d time.Duration) error
	// Delete removes a key from cache.
	Delete(ctx context.Context, key string) error
	// String returns the name of the implementation.
	String() string
}

// TypedCache is the generic version of Cache providing type-safe access.
type TypedCache[T any] interface {
	Load(ctx context.Context, key string) (T, error)
	Exists(ctx context.Context, key string) (bool, error)
	Store(ctx context.Context, key string, val T, d time.Duration) error
	Delete(ctx context.Context, key string) error
	String() string
}

type typedCache[T any] struct {
	cache Cache
}

func (c *typedCache[T]) Load(ctx context.Context, key string) (T, error) {
	var result T
	if err := c.cache.Load(ctx, key, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *typedCache[T]) Exists(ctx context.Context, key string) (bool, error) {
	return c.cache.Exists(ctx, key)
}

func (c *typedCache[T]) Store(ctx context.Context, key string, val T, d time.Duration) error {
	return c.cache.Store(ctx, key, val, d)
}

func (c *typedCache[T]) Delete(ctx context.Context, key string) error {
	return c.cache.Delete(ctx, key)
}

func (c *typedCache[T]) String() string {
	return c.cache.String()
}

// NewTypedCache wraps a Cache into a TypedCache[T].
func NewTypedCache[T any](c Cache) TypedCache[T] {
	return &typedCache[T]{cache: c}
}

func Default() Cache {
	return std
}

// Get gets a cached value by key.
func Load(ctx context.Context, key string, val any) error {
	return std.Load(ctx, key, val)
}

// Put stores a key-value pair into cache.
func Store(ctx context.Context, key string, val any, d time.Duration) error {
	return std.Store(ctx, key, val, d)
}

// Delete removes a key from cache.
func Delete(ctx context.Context, key string) error {
	return std.Delete(ctx, key)
}
