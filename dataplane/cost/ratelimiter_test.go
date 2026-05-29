package cost

import (
	"testing"
	"time"
)

func newTestRateLimiter(rpm, burst int) (*RateLimiter, *time.Time) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerMinute:  rpm,
		ConcurrentSessions: 10,
		BurstSize:          burst,
	})
	rl.now = func() time.Time { return now }
	return rl, &now
}

func TestRateLimiter_AllowWithinLimit(t *testing.T) {
	rl, _ := newTestRateLimiter(60, 10)

	// Should allow requests within burst size.
	for i := 0; i < 10; i++ {
		allowed, retryAfter := rl.Allow("agent:test")
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if retryAfter != 0 {
			t.Fatalf("retryAfter should be 0 when allowed, got %v", retryAfter)
		}
	}
}

func TestRateLimiter_DenyWhenExhausted(t *testing.T) {
	rl, _ := newTestRateLimiter(60, 5)

	// Exhaust all tokens.
	for i := 0; i < 5; i++ {
		allowed, _ := rl.Allow("tenant:abc")
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// Next request should be denied.
	allowed, retryAfter := rl.Allow("tenant:abc")
	if allowed {
		t.Fatal("request should be denied after exhaustion")
	}
	if retryAfter <= 0 {
		t.Fatal("retryAfter should be positive when denied")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl, now := newTestRateLimiter(60, 5) // 1 token per second

	// Exhaust all tokens.
	for i := 0; i < 5; i++ {
		rl.Allow("user:u1")
	}

	// Denied immediately.
	allowed, _ := rl.Allow("user:u1")
	if allowed {
		t.Fatal("should be denied when exhausted")
	}

	// Advance time by 2 seconds — should refill 2 tokens.
	*now = now.Add(2 * time.Second)
	rl.now = func() time.Time { return *now }

	allowed, _ = rl.Allow("user:u1")
	if !allowed {
		t.Fatal("should be allowed after refill")
	}

	// Second request should also succeed (2 tokens refilled, 1 consumed).
	allowed, _ = rl.Allow("user:u1")
	if !allowed {
		t.Fatal("should be allowed with remaining refilled token")
	}

	// Third request should be denied.
	allowed, _ = rl.Allow("user:u1")
	if allowed {
		t.Fatal("should be denied after consuming refilled tokens")
	}
}

func TestRateLimiter_DifferentKeysIndependent(t *testing.T) {
	rl, _ := newTestRateLimiter(60, 2)

	// Exhaust key1.
	rl.Allow("agent:a1")
	rl.Allow("agent:a1")
	allowed, _ := rl.Allow("agent:a1")
	if allowed {
		t.Fatal("agent:a1 should be exhausted")
	}

	// key2 should still be fine.
	allowed, _ = rl.Allow("agent:a2")
	if !allowed {
		t.Fatal("agent:a2 should be allowed independently")
	}
}

func TestRateLimiter_RetryAfter(t *testing.T) {
	rl, _ := newTestRateLimiter(60, 1) // 1 token/sec, burst 1

	// Use the one token.
	rl.Allow("user:x")

	// Next request denied — retryAfter should be ~1 second.
	allowed, retryAfter := rl.Allow("user:x")
	if allowed {
		t.Fatal("should be denied")
	}
	// With 1 token/sec rate, need 1 token, retryAfter ≈ 1s.
	if retryAfter < 900*time.Millisecond || retryAfter > 1100*time.Millisecond {
		t.Fatalf("expected retryAfter ~1s, got %v", retryAfter)
	}
}

func TestRateLimiter_BurstSize(t *testing.T) {
	rl, _ := newTestRateLimiter(120, 3) // 2 tokens/sec, burst 3

	// AllowN for the full burst.
	allowed, _ := rl.AllowN("tenant:t1", 3)
	if !allowed {
		t.Fatal("should allow up to burst size")
	}

	// AllowN for 1 more should fail.
	allowed, retryAfter := rl.AllowN("tenant:t1", 1)
	if allowed {
		t.Fatal("should deny when bucket is empty")
	}
	// 1 token needed at 2 tokens/sec = 0.5s.
	if retryAfter < 400*time.Millisecond || retryAfter > 600*time.Millisecond {
		t.Fatalf("expected retryAfter ~500ms, got %v", retryAfter)
	}

	// AllowN requesting more than burst should fail even on fresh key.
	allowed, _ = rl.AllowN("tenant:t2", 4)
	if allowed {
		t.Fatal("should deny when requesting more than burst size")
	}
}
