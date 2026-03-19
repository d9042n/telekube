package middleware

import (
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

// RateLimit implements per-user sliding window rate limiting.
func RateLimit(msgsPerMin int) telebot.MiddlewareFunc {
	if msgsPerMin <= 0 {
		msgsPerMin = 30
	}

	limiter := newRateLimiter(msgsPerMin)

	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			sender := c.Sender()
			if sender == nil {
				return next(c)
			}

			if !limiter.allow(sender.ID) {
				return c.Send("🐢 Slow down! You're sending too many requests. Please wait a moment.")
			}

			return next(c)
		}
	}
}

type rateLimiter struct {
	mu          sync.Mutex
	requests    map[int64][]time.Time
	maxPerMin   int
}

func newRateLimiter(maxPerMin int) *rateLimiter {
	return &rateLimiter{
		requests:  make(map[int64][]time.Time),
		maxPerMin: maxPerMin,
	}
}

func (rl *rateLimiter) allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	// Get current window
	times := rl.requests[userID]

	// Remove expired entries
	validIdx := 0
	for _, t := range times {
		if t.After(cutoff) {
			times[validIdx] = t
			validIdx++
		}
	}
	times = times[:validIdx]

	if len(times) >= rl.maxPerMin {
		rl.requests[userID] = times
		return false
	}

	// Add current request
	rl.requests[userID] = append(times, now)
	return true
}
