package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const rateLimitPrefix = "telekube:ratelimit:"

// RateLimiter provides Redis-backed rate limiting.
type RateLimiter struct {
	rdb *goredis.Client
}

// NewRateLimiter creates a rate limiter using the client's Redis connection.
func (c *Client) NewRateLimiter() *RateLimiter {
	return &RateLimiter{rdb: c.rdb}
}

// Allow checks if the request should be allowed.
// Uses a sliding window rate limiter.
func (rl *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	fullKey := rateLimitPrefix + key

	now := time.Now().UnixMilli()
	windowStart := now - window.Milliseconds()

	pipe := rl.rdb.Pipeline()

	// Remove old entries
	pipe.ZRemRangeByScore(ctx, fullKey, "0", fmt.Sprintf("%d", windowStart))

	// Add current request
	pipe.ZAdd(ctx, fullKey, goredis.Z{
		Score:  float64(now),
		Member: fmt.Sprintf("%d", now),
	})

	// Count requests in window
	countCmd := pipe.ZCard(ctx, fullKey)

	// Set TTL on the set
	pipe.Expire(ctx, fullKey, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("rate limit check: %w", err)
	}

	count := int(countCmd.Val())
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return count <= limit, remaining, nil
}
