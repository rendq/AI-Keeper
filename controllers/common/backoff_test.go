package common_test

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// TestBackoffDuration_Bounds asserts the documented invariant
//
//	delay = min(base * 2^attempt, max) * (1 ± jitter)
//
// holds for `attempts ∈ [0, 20]` regardless of jitter draw.
//
// Validates: Requirements F23.
func TestBackoffDuration_Bounds(t *testing.T) {
	t.Parallel()

	const trials = 256

	for attempt := 0; attempt <= 20; attempt++ {
		// Deterministic component, capped at BackoffMax.
		expected := time.Duration(int64(common.BackoffBase) << minShift(attempt, common.MaxBackoffShift))
		if expected <= 0 || expected > common.BackoffMax {
			expected = common.BackoffMax
		}

		jitter := float64(common.BackoffJitterPercent) / 100.0
		lower := time.Duration(math.Round(float64(expected) * (1.0 - jitter)))
		upper := time.Duration(math.Round(float64(expected) * (1.0 + jitter)))

		// Top-level invariant: must never exceed BackoffMax * (1 + jitter).
		hardCap := time.Duration(math.Round(float64(common.BackoffMax) * (1.0 + jitter)))

		// Repeat with multiple seeds to exercise both extremes of the
		// jitter distribution.
		for seed := int64(0); seed < trials; seed++ {
			r := rand.New(rand.NewSource(seed))
			d := common.BackoffDuration(attempt, r)
			if d < 0 {
				t.Fatalf("attempt=%d seed=%d: negative duration %s", attempt, seed, d)
			}
			if d < lower-time.Millisecond {
				t.Fatalf("attempt=%d seed=%d: %s below lower bound %s", attempt, seed, d, lower)
			}
			if d > upper+time.Millisecond {
				t.Fatalf("attempt=%d seed=%d: %s above upper bound %s", attempt, seed, d, upper)
			}
			if d > hardCap+time.Millisecond {
				t.Fatalf("attempt=%d seed=%d: %s above hard cap %s", attempt, seed, d, hardCap)
			}
		}
	}
}

func TestBackoffDuration_Deterministic(t *testing.T) {
	t.Parallel()

	for attempt := 0; attempt <= 12; attempt++ {
		r1 := rand.New(rand.NewSource(42))
		r2 := rand.New(rand.NewSource(42))
		d1 := common.BackoffDuration(attempt, r1)
		d2 := common.BackoffDuration(attempt, r2)
		if d1 != d2 {
			t.Fatalf("attempt=%d not deterministic with same seed: %s vs %s", attempt, d1, d2)
		}
	}
}

func TestBackoffDuration_NegativeClampsToZeroAttempts(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	got := common.BackoffDuration(-3, r)
	r2 := rand.New(rand.NewSource(1))
	want := common.BackoffDuration(0, r2)
	if got != want {
		t.Fatalf("negative attempts not clamped: got %s want %s", got, want)
	}
}

func TestBackoffDuration_ConvergesToMaxAtHighAttempts(t *testing.T) {
	t.Parallel()

	jitter := float64(common.BackoffJitterPercent) / 100.0
	lower := time.Duration(math.Round(float64(common.BackoffMax) * (1.0 - jitter)))
	upper := time.Duration(math.Round(float64(common.BackoffMax) * (1.0 + jitter)))
	for attempt := 10; attempt <= 64; attempt++ {
		r := rand.New(rand.NewSource(int64(attempt)))
		d := common.BackoffDuration(attempt, r)
		if d < lower || d > upper {
			t.Fatalf("attempt=%d: %s not within [%s, %s] of BackoffMax", attempt, d, lower, upper)
		}
	}
}

func TestRequeueWithBackoff_SmokeTest(t *testing.T) {
	t.Parallel()

	res := common.RequeueWithBackoff(0)
	if res.RequeueAfter <= 0 {
		t.Fatalf("RequeueAfter must be positive for first retry, got %s", res.RequeueAfter)
	}
	if res.RequeueAfter > common.BackoffMax+common.BackoffMax/4 {
		t.Fatalf("RequeueAfter unreasonably large: %s", res.RequeueAfter)
	}
}

// minShift caps shift exponent the same way [common.BackoffDuration] does.
func minShift(attempt, cap int) int {
	if attempt > cap {
		return cap
	}
	return attempt
}
