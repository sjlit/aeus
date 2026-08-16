package memory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

var (
	ErrNotFound    = errors.New("cache: key not found")
	ErrWrongType   = errors.New("val must be a pointer")
	ErrAddressable = errors.New("cannot set value: val is not addressable")
)

type memCache struct {
	opts Options

	items map[string]Item
	sync.RWMutex
}

// Load gets a cached value by key.
func (c *memCache) Load(ctx context.Context, key string, val any) error {
	refValue := reflect.ValueOf(val)
	if refValue.Type().Kind() != reflect.Ptr {
		return ErrWrongType
	}
	refElem := refValue.Elem()
	if !refElem.CanSet() {
		return ErrAddressable
	}

	// Snapshot the cached value under the read lock, then release the
	// lock before the (potentially slow) reflection copy. Holding the
	// lock across refElem.Set would block every other reader for the
	// duration of any user-defined type conversion.
	c.RWMutex.RLock()
	item, found := c.items[key]
	if !found {
		c.RWMutex.RUnlock()
		return ErrNotFound
	}
	if item.Expired() {
		c.RWMutex.RUnlock()
		return ErrNotFound
	}
	itemValue := reflect.ValueOf(item.Value)
	c.RWMutex.RUnlock()

	if !itemValue.IsValid() {
		return ErrNotFound
	}
	targetValue := reflect.Indirect(itemValue)
	if targetValue.Type() != refElem.Type() {
		return fmt.Errorf("type mismatch: expected %v, got %v", refElem.Type(), targetValue.Type())
	}
	// Clone so the caller's destination does not alias the cached
	// entry's backing storage. Without this, mutating a slice/map
	// obtained from Load would corrupt later reads.
	refElem.Set(clone(targetValue))
	return nil
}

// Exists checks if a key exists in cache.
func (c *memCache) Exists(ctx context.Context, key string) (bool, error) {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	item, found := c.items[key]
	if !found {
		return false, nil
	}
	if item.Expired() {
		return false, nil
	}
	return true, nil
}

func (c *memCache) Store(ctx context.Context, key string, val any, d time.Duration) error {
	if d == DefaultExpiration {
		d = c.opts.Expiration
	}
	var e int64
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}

	// Snapshot the value the caller hands us so the cache owns its own
	// copy. Without this, mutating the caller's original would silently
	// change the cached entry — and vice versa.
	snapshot := clone(reflect.ValueOf(val))

	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()

	if snapshot.IsValid() {
		c.items[key] = Item{
			Value:      snapshot.Interface(),
			Expiration: e,
		}
	} else {
		c.items[key] = Item{
			Value:      val,
			Expiration: e,
		}
	}

	return nil
}

func (c *memCache) Delete(ctx context.Context, key string) error {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()

	delete(c.items, key)
	return nil
}

func (m *memCache) String() string {
	return "memory"
}

// NewCache returns a new cache.
func NewCache(opts ...Option) *memCache {
	options := NewOptions(opts...)
	items := make(map[string]Item)

	if len(options.Items) > 0 {
		items = options.Items
	}

	return &memCache{
		opts:  options,
		items: items,
	}
}
