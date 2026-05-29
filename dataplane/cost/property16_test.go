// Feature: ai-platform, Property 16: Rate limiter atomicity

//go:build pbt

package cost

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements B1.3**

func TestProperty16(t *testing.T) {
	seed := time.Now().UnixNano()
	if s := os.Getenv("AIP_PBT_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Property 1: Total allowed requests without time advancement never exceeds burst
	properties.Property("burst cap: allowed requests <= burst without time advance", prop.ForAll(
		func(rpm int, burst int, numRequests int) bool {
			config := RateLimitConfig{
				RequestsPerMinute: rpm,
				BurstSize:         burst,
			}
			rl := NewRateLimiter(config)

			// Fix time so no refill happens
			fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			rl.now = func() time.Time { return fixedTime }

			allowed := 0
			for i := 0; i < numRequests; i++ {
				ok, _ := rl.Allow("test-key")
				if ok {
					allowed++
				}
			}

			return allowed <= burst
		},
		gen.IntRange(10, 600),  // RPM
		gen.IntRange(1, 20),   // burst
		gen.IntRange(1, 50),   // number of requests
	))

	// Property 2: After N seconds, total allowed <= burst + (N * RPM/60)
	properties.Property("refill cap: allowed <= burst + refilled tokens after N seconds", prop.ForAll(
		func(rpm int, burst int, numRequests int, advanceSecs int) bool {
			config := RateLimitConfig{
				RequestsPerMinute: rpm,
				BurstSize:         burst,
			}
			rl := NewRateLimiter(config)

			currentTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			rl.now = func() time.Time { return currentTime }

			// Drain some tokens first, then advance time
			drained := 0
			for i := 0; i < burst; i++ {
				ok, _ := rl.Allow("test-key")
				if ok {
					drained++
				}
			}

			// Advance time by N seconds
			currentTime = currentTime.Add(time.Duration(advanceSecs) * time.Second)

			// Now count allowed requests
			allowed := 0
			for i := 0; i < numRequests; i++ {
				ok, _ := rl.Allow("test-key")
				if ok {
					allowed++
				}
			}

			// After draining and advancing N seconds, refilled tokens = N * RPM/60
			// but capped at burst. So max new tokens = min(burst, N*RPM/60)
			maxRefilled := float64(advanceSecs) * float64(rpm) / 60.0
			if maxRefilled > float64(burst) {
				maxRefilled = float64(burst)
			}

			return allowed <= int(maxRefilled)+1 // +1 for floating point rounding
		},
		gen.IntRange(10, 600),  // RPM
		gen.IntRange(1, 20),   // burst
		gen.IntRange(1, 50),   // number of requests
		gen.IntRange(1, 30),   // seconds to advance
	))

	// Property 3: Once denied, no more allows without time advancing
	properties.Property("consistency: once denied, subsequent requests also denied without time advance", prop.ForAll(
		func(rpm int, burst int, numRequests int) bool {
			config := RateLimitConfig{
				RequestsPerMinute: rpm,
				BurstSize:         burst,
			}
			rl := NewRateLimiter(config)

			fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			rl.now = func() time.Time { return fixedTime }

			denied := false
			for i := 0; i < numRequests; i++ {
				ok, _ := rl.Allow("test-key")
				if !ok {
					denied = true
				} else if denied {
					// Got an allow after a deny without time advancing — violation
					return false
				}
			}
			return true
		},
		gen.IntRange(10, 600),  // RPM
		gen.IntRange(1, 20),   // burst
		gen.IntRange(1, 50),   // number of requests
	))

	// Property 4: AllowN with n > burst always returns false
	properties.Property("oversize: AllowN(n > burst) always denied", prop.ForAll(
		func(rpm int, burst int, extra int) bool {
			config := RateLimitConfig{
				RequestsPerMinute: rpm,
				BurstSize:         burst,
			}
			rl := NewRateLimiter(config)

			fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			rl.now = func() time.Time { return fixedTime }

			n := burst + extra // n > burst since extra >= 1
			ok, _ := rl.AllowN("test-key", n)
			return !ok
		},
		gen.IntRange(10, 600),  // RPM
		gen.IntRange(1, 20),   // burst
		gen.IntRange(1, 30),   // extra above burst
	))

	properties.TestingRun(t)
}
