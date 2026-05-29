package cost

import (
	"sync"
	"time"
)

// RateLimitConfig defines the configuration for a token bucket rate limiter.
type RateLimitConfig struct {
	RequestsPerMinute  int // refill rate: tokens added per minute
	ConcurrentSessions int // max concurrent sessions (informational, not enforced here)
	BurstSize          int // max tokens in the bucket
}

// bucket holds the state for a single token bucket.
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// RateLimiter implements a per-key token bucket rate limiter.
// Keys are scoped like "agent:<name>", "tenant:<id>", "user:<id>".
type RateLimiter struct {
	mu      sync.Mutex
	config  RateLimitConfig
	buckets map[string]*bucket
	now     func() time.Time // for testing
}

// NewRateLimiter creates a new RateLimiter with the given configuration.
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		config:  config,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// refillRate returns the number of tokens added per second.
func (rl *RateLimiter) refillRate() float64 {
	return float64(rl.config.RequestsPerMinute) / 60.0
}

// getBucket returns the bucket for the given key, creating one if it doesn't exist.
// Must be called with rl.mu held.
func (rl *RateLimiter) getBucket(key string) *bucket {
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{
			tokens:     float64(rl.config.BurstSize),
			lastRefill: rl.now(),
		}
		rl.buckets[key] = b
	}
	return b
}

// refill adds tokens to the bucket based on elapsed time.
// Must be called with rl.mu held.
func (rl *RateLimiter) refill(b *bucket) {
	now := rl.now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	b.tokens += elapsed * rl.refillRate()
	max := float64(rl.config.BurstSize)
	if b.tokens > max {
		b.tokens = max
	}
	b.lastRefill = now
}

// Allow checks if a single request is allowed for the given key.
// Returns whether the request is allowed and, if denied, how long to wait before retrying.
func (rl *RateLimiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	return rl.AllowN(key, 1)
}

// AllowN checks if n tokens can be consumed for the given key.
// Returns whether the request is allowed and, if denied, how long to wait before retrying.
func (rl *RateLimiter) AllowN(key string, n int) (allowed bool, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b := rl.getBucket(key)
	rl.refill(b)

	needed := float64(n)
	if b.tokens >= needed {
		b.tokens -= needed
		return true, 0
	}

	// Calculate how long until enough tokens are available.
	deficit := needed - b.tokens
	rate := rl.refillRate()
	if rate <= 0 {
		return false, time.Duration(0)
	}
	wait := time.Duration(deficit / rate * float64(time.Second))
	return false, wait
}
