package middleware

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/telebot.v3"
)

// ─── Edge Cases for RateLimit middleware ─────────────────────────────────────

func TestRateLimiter_BurstAttack_OnlyFirstNAllowed(t *testing.T) {
	t.Parallel()

	const limit = 10
	rl := newRateLimiter(limit)

	var allowed, denied int64

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.allow(42) {
				atomic.AddInt64(&allowed, 1)
			} else {
				atomic.AddInt64(&denied, 1)
			}
		}()
	}
	wg.Wait()

	// Exactly `limit` requests should have passed within a 1-minute window.
	assert.Equal(t, int64(limit), allowed, "exactly %d requests should be allowed in burst", limit)
	assert.Equal(t, int64(100-limit), denied, "remaining requests must be denied")
}

func TestRateLimiter_ManyUsers_Concurrent_NoRace(t *testing.T) {
	t.Parallel()

	const usersCount = 1000
	const msgsPerUser = 5
	rl := newRateLimiter(msgsPerUser)

	var wg sync.WaitGroup
	for userID := int64(1); userID <= usersCount; userID++ {
		userID := userID
		for req := 0; req < msgsPerUser+2; req++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rl.allow(userID)
			}()
		}
	}
	wg.Wait()
	// No panic, no race → test passed via -race flag.
}

func TestRateLimiter_NilSender_PassesThrough(t *testing.T) {
	t.Parallel()

	calledNext := false
	handler := func(c telebot.Context) error {
		calledNext = true
		return nil
	}

	mw := RateLimit(5)
	// Create a context where Sender() returns nil.
	ctx := &fakeTelebotContext{
		sender:   nil,
		ctxStore: make(map[string]interface{}),
	}
	if ctx.chat == nil {
		ctx.chat = &telebot.Chat{ID: 1}
	}

	_ = mw(handler)(ctx)
	assert.True(t, calledNext, "nil sender must pass through rate limiter")
}

func TestRateLimiter_DefaultLimit_UsedWhenZeroOrNegative(t *testing.T) {
	t.Parallel()

	// The default limit normalization happens in RateLimit(), not in newRateLimiter().
	// When msgsPerMin <= 0, RateLimit() sets it to 30 before calling newRateLimiter.
	// We verify the middleware behavior: with limit=0 (→ default 30), the 31st message is rejected.
	mw := RateLimit(0)

	sender := &telebot.User{ID: 777, FirstName: "TestUser"}
	handler := func(_ telebot.Context) error { return nil }

	allowed := 0
	for i := 0; i < 35; i++ {
		ctx := &fakeTelebotContext{
			sender:   sender,
			chat:     &telebot.Chat{ID: 777},
			ctxStore: make(map[string]interface{}),
		}
		_ = mw(handler)(ctx)
		if len(ctx.sent) == 0 {
			allowed++
		}
	}

	assert.Equal(t, 30, allowed, "with default limit 30, exactly 30 requests must be allowed")
}

func TestRateLimiter_WindowSlide_Correct(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(3)
	userID := int64(101)

	// Use 3 requests.
	for i := 0; i < 3; i++ {
		assert.True(t, rl.allow(userID))
	}
	// 4th is denied.
	assert.False(t, rl.allow(userID))

	// Slide window: move existing timestamps 2 minutes into the past → they expire.
	rl.mu.Lock()
	for i := range rl.requests[userID] {
		rl.requests[userID][i] = rl.requests[userID][i].Add(-2 * 60 * 1_000_000_000)
	}
	rl.mu.Unlock()

	// Now 3 new requests should pass.
	for i := 0; i < 3; i++ {
		assert.True(t, rl.allow(userID), "request %d after window slide should pass", i+1)
	}
}

func TestRateLimiter_IndependentUsers_NotAffectedByEachOther(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(2)

	// User A uses up their quota.
	assert.True(t, rl.allow(1))
	assert.True(t, rl.allow(1))
	assert.False(t, rl.allow(1))

	// Users B and C should be unaffected.
	assert.True(t, rl.allow(2))
	assert.True(t, rl.allow(2))
	assert.True(t, rl.allow(3))
}

func TestRateLimit_Middleware_SendsSlowDownMessage(t *testing.T) {
	t.Parallel()

	mw := RateLimit(1)

	sender := &telebot.User{ID: 999, FirstName: "Spammer"}
	handler := func(_ telebot.Context) error { return nil }

	// First message passes.
	ctx1 := &fakeTelebotContext{
		sender:   sender,
		chat:     &telebot.Chat{ID: 999},
		ctxStore: make(map[string]interface{}),
	}
	_ = mw(handler)(ctx1)
	assert.Empty(t, ctx1.sent, "first message should not trigger slow-down")

	// Second message triggers rate limit.
	ctx2 := &fakeTelebotContext{
		sender:   sender,
		chat:     &telebot.Chat{ID: 999},
		ctxStore: make(map[string]interface{}),
	}
	_ = mw(handler)(ctx2)
	assert.NotEmpty(t, ctx2.sent, "rate-limited message must receive a response")
	assert.Contains(t, ctx2.sent[0], "Slow down", "response must contain slow-down message")
}
