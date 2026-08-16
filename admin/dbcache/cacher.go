// Package dbcache provides read-through caching of database queries,
// with optional dependency-validated invalidation.
//
// A Cacher wraps a *gorm.DB and a cache.Cache backend. Try(c, ...)
// loads the value for key from the cache; on a miss it runs the loader
// f(tx) against the DB (deduplicated via singleflight) and stores the
// result.
//
// # Dependency-validated invalidation
//
// With a CacheDependency configured, a cached entry older than the 1s
// grace window is only served when the dependency's version marker
// still matches the marker captured when the entry was stored. The
// typical dependency is SqlDependency over MAX(updated_at) of the
// table the loader reads: writes bump the marker and the stale entry
// is reloaded on the next Try, without the caller placing explicit
// invalidation calls. Without a dependency the entry is served until
// its TTL expires.
//
// # Value aliasing
//
// The in-memory cache backend (infra/cache/memory) copies values by
// reference: the value returned from Try may share its backing
// slice/map with the cached entry. Callers must not mutate the
// returned value. The Redis backend round-trips through JSON and does
// not alias.
package dbcache

import (
	"context"
	"time"

	"github.com/sjlit/aeus/infra/cache"
	"github.com/sjlit/aeus/infra/cache/memory"
	"github.com/sjlit/aeus/pkg/errs"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// maxCacheKeyLen bounds the cache key, matching the backend's key
// limits (e.g. Redis). Longer keys are a caller bug and fail fast.
const maxCacheKeyLen = 256

// defaultCacheDuration is the TTL used when WithCacheDuration is not
// given. It is the hard expiry bound; with a dependency configured the
// entry is normally revalidated long before it expires.
const defaultCacheDuration = 10 * time.Minute

// graceWindow is the freshness window during which a just-stored entry
// is served without running the dependency query. It absorbs the burst
// of reads that follows a reload (or a write) without paying an extra
// query per request.
const graceWindow = time.Second

// Cacher is a read-through cache over a *gorm.DB.
type Cacher struct {
	db         *gorm.DB
	cache      cache.Cache
	duration   time.Duration
	dependency CacheDependency
	sf         singleflight.Group
}

// Option configures a Cacher.
type Option func(*Cacher)

// WithCache sets the cache backend. Defaults to a fresh in-memory
// cache owned by this Cacher; pass a Redis-backed cache to share
// across processes.
func WithCache(c cache.Cache) Option {
	return func(o *Cacher) {
		o.cache = c
	}
}

// WithCacheDuration sets the entry TTL. Defaults to 10 minutes.
func WithCacheDuration(d time.Duration) Option {
	return func(o *Cacher) {
		if d > 0 {
			o.duration = d
		}
	}
}

// WithDependency sets the version-marker dependency used to revalidate
// cached entries (see the package doc). Without one the cache is
// purely TTL-based.
func WithDependency(d CacheDependency) Option {
	return func(o *Cacher) {
		o.dependency = d
	}
}

// New returns a Cacher bound to db. db must be non-nil; a nil db is a
// wiring error and panics rather than failing every Try call.
func New(db *gorm.DB, opts ...Option) *Cacher {
	if db == nil {
		panic("dbcache: New requires a non-nil *gorm.DB")
	}
	c := &Cacher{
		db:       db,
		cache:    memory.NewCache(),
		duration: defaultCacheDuration,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// cacheEntry is the on-disk/on-wire shape of one cached value. The JSON
// tags keep the wire format compact for JSON-marshaling backends
// (Redis); the memory backend copies the struct directly.
type cacheEntry[T any] struct {
	Value        T      `json:"v"`
	CompareValue string `json:"d"`
	CreatedAt    int64  `json:"t"` // UnixMilli
}

// Try returns the cached value for key, loading it through f on a miss
// or when the entry no longer revalidates against the dependency. T is
// inferred from the loader's return type.
//
// The loader receives a tx carrying ctx; it must not be reused across
// calls. Errors from f are returned as-is and never cached. See the
// package doc for the aliasing contract of the returned value.
//
// Try is a free function rather than a method: Go does not allow type
// parameters on methods, and a free function lets one Cacher serve
// values of different types.
func Try[T any](c *Cacher, ctx context.Context, key string, f func(tx *gorm.DB) (T, error)) (T, error) {
	var none T
	if len(key) > maxCacheKeyLen {
		return none, errs.Newf(errs.CodeInvalid, "cache key too long")
	}

	// Read path: serve from the cache when the entry is fresh, or when
	// the dependency still matches the captured marker. Any Load error
	// is a miss (the backend may be down: fall back to the DB).
	entry := &cacheEntry[T]{}
	if err := c.cache.Load(ctx, key, entry); err == nil {
		created := time.UnixMilli(entry.CreatedAt)
		now := time.Now()
		if !now.Before(created) { // clock rewound: treat as stale
			if now.Sub(created) <= graceWindow {
				return entry.Value, nil
			}
			if c.dependency == nil {
				return entry.Value, nil
			}
			if dependValue, err := c.dependency.GetValue(ctx, c.db.WithContext(ctx)); err == nil {
				if entry.CompareValue == dependValue {
					return entry.Value, nil
				}
				// Marker moved: drop the stale entry. A failed delete is
				// harmless — the singleflight reload below replaces it.
				_ = c.cache.Delete(ctx, key)
			}
		}
	}

	// Reload path: one loader run per key at a time, all concurrent
	// callers sharing the result.
	tx := c.db.WithContext(ctx)
	val, err, _ := c.sf.Do(key, func() (any, error) {
		var dependValue string
		if c.dependency != nil {
			var err error
			if dependValue, err = c.dependency.GetValue(ctx, tx); err != nil {
				return none, err
			}
		}
		result, err := f(tx)
		if err != nil {
			return none, err
		}
		// A failed Store is not fatal: the entry is missing, and the
		// next Try simply reloads.
		_ = c.cache.Store(ctx, key, &cacheEntry[T]{
			Value:        result,
			CompareValue: dependValue,
			CreatedAt:    time.Now().UnixMilli(),
		}, c.duration)
		return result, nil
	})
	if err != nil {
		return none, err
	}
	result, ok := val.(T)
	if !ok {
		return none, errs.Newf(errs.CodeIncompatible, "cache value type mismatch")
	}
	return result, nil
}
