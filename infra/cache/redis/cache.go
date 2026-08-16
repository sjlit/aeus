package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sjlit/aeus"
	"github.com/sjlit/aeus/infra/cache"
)

type redisCache struct {
	opts *options
}

func (c *redisCache) buildKey(key string) string {
	return c.opts.prefix + key
}

func (c *redisCache) Load(ctx context.Context, key string, val any) error {
	buf, err := c.opts.client.Get(ctx, c.buildKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return cache.ErrNotFound
		}
		return err
	}
	return json.Unmarshal(buf, val)
}

func (c *redisCache) Exists(ctx context.Context, key string) (ok bool, err error) {
	var n int64
	n, err = c.opts.client.Exists(ctx, c.buildKey(key)).Result()
	if n > 0 {
		ok = true
	}
	return
}

// Put stores a key-value pair into cache.
func (c *redisCache) Store(ctx context.Context, key string, val any, d time.Duration) error {
	var (
		err error
		buf []byte
	)
	if buf, err = json.Marshal(val); err == nil {
		err = c.opts.client.Set(ctx, c.buildKey(key), buf, d).Err()
	}
	return err
}

// Delete removes a key from cache.
func (c *redisCache) Delete(ctx context.Context, key string) error {
	return c.opts.client.Del(ctx, c.buildKey(key)).Err()
}

// String returns the name of the implementation.
func (c *redisCache) String() string {
	return "redis"
}

func NewCache(opts ...Option) *redisCache {
	cache := &redisCache{
		opts: newOptions(opts...),
	}
	if cache.opts.context != nil {
		app := aeus.FromContext(cache.opts.context)
		if app != nil {
			cache.opts.prefix = app.Name() + ":" + cache.opts.prefix
		}
	}
	return cache
}
