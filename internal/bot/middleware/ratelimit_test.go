package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_Allow(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(5) // 5 per minute

	userID := int64(100)

	// First 5 should be allowed
	for i := 0; i < 5; i++ {
		assert.True(t, rl.allow(userID), "request %d should be allowed", i+1)
	}

	// 6th should be denied
	assert.False(t, rl.allow(userID), "6th request should be denied")
}

func TestRateLimiter_DifferentUsers(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(2)

	// User 1 uses up limit
	assert.True(t, rl.allow(100))
	assert.True(t, rl.allow(100))
	assert.False(t, rl.allow(100))

	// User 2 still has quota
	assert.True(t, rl.allow(200))
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(1)

	// Use up the limit
	assert.True(t, rl.allow(100))
	assert.False(t, rl.allow(100))

	// Manually expire the entries
	rl.mu.Lock()
	rl.requests[100] = []time.Time{time.Now().Add(-2 * time.Minute)}
	rl.mu.Unlock()

	// Should be allowed again
	assert.True(t, rl.allow(100))
}
