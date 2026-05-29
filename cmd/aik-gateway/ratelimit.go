package gateway

import (
	"context"
	"sync"
	"time"
)

// RateLimiter decides whether a request identified by key should be allowed.
type RateLimiter interface {
	Allow(ctx context.Context, key string) bool
}

// RateLimitConfig holds per-agent/channel rate limit settings.
type RateLimitConfig struct {
	// RequestsPerMinute is the token bucket refill rate (tokens per minute).
	RequestsPerMinute int
	// BurstSize is the maximum tokens in the bucket (allows bursts).
	BurstSize int
}

// TokenBucketLimiter implements a per-key token bucket rate limiter.
type TokenBucketLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	configs map[string]*RateLimitConfig
	// defaultConfig is used when no specific config exists for a key.
	defaultConfig *RateLimitConfig
}

type bucket struct {
	tokens    float64
	lastTime  time.Time
	maxTokens float64
	refillRate float64 // tokens per second
}

// NewTokenBucketLimiter creates a rate limiter with per-key configs.
// configs maps "agentName/channelName" to rate limit settings.
func NewTokenBucketLimiter(configs map[string]*RateLimitConfig, defaultConfig *RateLimitConfig) *TokenBucketLimiter {
	if defaultConfig == nil {
		defaultConfig = &RateLimitConfig{
			RequestsPerMinute: 60,
			BurstSize:         10,
		}
	}
	return &TokenBucketLimiter{
		buckets:       make(map[string]*bucket),
		configs:       configs,
		defaultConfig: defaultConfig,
	}
}

// Allow checks if a request for the given key should be allowed.
func (l *TokenBucketLimiter) Allow(_ context.Context, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		cfg := l.configFor(key)
		b = &bucket{
			tokens:    float64(cfg.BurstSize),
			lastTime:  time.Now(),
			maxTokens: float64(cfg.BurstSize),
			refillRate: float64(cfg.RequestsPerMinute) / 60.0,
		}
		l.buckets[key] = b
	}

	// Refill tokens based on elapsed time.
	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastTime = now

	// Try to consume one token.
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// configFor returns the rate limit config for a key.
func (l *TokenBucketLimiter) configFor(key string) *RateLimitConfig {
	if cfg, ok := l.configs[key]; ok {
		return cfg
	}
	return l.defaultConfig
}

// noopRateLimiter always allows (for testing).
type noopRateLimiter struct{}

func (n *noopRateLimiter) Allow(_ context.Context, _ string) bool {
	return true
}
