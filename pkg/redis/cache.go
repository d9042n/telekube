package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const cachePrefix = "telekube:cache:"

// CacheSet stores a value with TTL.
func (c *Client) CacheSet(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal cache value: %w", err)
	}
	return c.rdb.Set(ctx, cachePrefix+key, data, ttl).Err()
}

// CacheGet retrieves a cached value.
func (c *Client) CacheGet(ctx context.Context, key string, dest interface{}) (bool, error) {
	data, err := c.rdb.Get(ctx, cachePrefix+key).Bytes()
	if err != nil {
		if err.Error() == "redis: nil" {
			return false, nil
		}
		return false, fmt.Errorf("get cache: %w", err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("unmarshal cache value: %w", err)
	}
	return true, nil
}

// CacheDel removes a cached value.
func (c *Client) CacheDel(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, cachePrefix+key).Err()
}

// CacheDelByPattern removes all keys matching a pattern.
func (c *Client) CacheDelByPattern(ctx context.Context, pattern string) error {
	iter := c.rdb.Scan(ctx, 0, cachePrefix+pattern, 100).Iterator()
	for iter.Next(ctx) {
		_ = c.rdb.Del(ctx, iter.Val()).Err()
	}
	return iter.Err()
}
