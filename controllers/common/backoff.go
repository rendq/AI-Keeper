package common

import (
	"math"
	"math/rand"
	"sync"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

// Backoff configuration constants (design.md §14.2 / Requirement F23):
//
//	attempt N: delay = min(base * 2^N, max) * (1 ± jitter)
//	  base   = 2s
//	  max    = 5m
//	  jitter = ±20%
const (
	// BackoffBase is the first-attempt delay (2 seconds).
	BackoffBase = 2 * time.Second

	// BackoffMax caps any single requeue delay (5 minutes).
	BackoffMax = 5 * time.Minute

	// BackoffJitterPercent is the symmetric jitter window applied around
	// the deterministic delay (±20 %).
	BackoffJitterPercent = 20

	// MaxBackoffShift bounds the exponential shift to keep
	// `BackoffBase << attempts` from overflowing int64. With the constants
	// above the cap is reached at attempt = 8 (2s * 256 = 512s ≫ 5m), so
	// the bound is comfortably loose.
	MaxBackoffShift = 32
)

// pkgRand is the package-private RNG used by [BackoffDuration] when the
// caller does not supply one. It is seeded once at process start with a
// time-based seed because the Go 1.20+ default math/rand is already
// auto-seeded but the explicit allocation keeps this tunable for tests.
var (
	pkgRandMu sync.Mutex
	pkgRand   = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// BackoffDuration returns the requeue delay for the given retry
// `attempts` count (0 for the first retry, 1 for the second, ...). The
// computation is the deterministic exponential
// `min(BackoffBase * 2^attempts, BackoffMax)` perturbed by a uniformly
// distributed factor in `[1 - jitter, 1 + jitter]` where `jitter = 0.20`.
//
// Negative `attempts` are clamped to zero so callers can pass naturally
// monotonic counters without guarding the boundary.
//
// The optional `r` argument lets tests inject a seeded RNG for
// reproducible jitter; when nil, a process-global RNG is used under a
// mutex. The function is safe to call concurrently from multiple
// reconcile goroutines.
//
// Validates: Requirements F23 (retry bound).
func BackoffDuration(attempts int, r *rand.Rand) time.Duration {
	if attempts < 0 {
		attempts = 0
	}

	// Deterministic exponential backoff with overflow protection.
	shift := attempts
	if shift > MaxBackoffShift {
		shift = MaxBackoffShift
	}
	exp := time.Duration(int64(BackoffBase) << shift)
	if exp <= 0 || exp > BackoffMax {
		exp = BackoffMax
	}

	// Symmetric multiplicative jitter in [1 - p, 1 + p].
	p := float64(BackoffJitterPercent) / 100.0
	var f float64
	if r == nil {
		pkgRandMu.Lock()
		f = pkgRand.Float64()
		pkgRandMu.Unlock()
	} else {
		f = r.Float64()
	}
	factor := 1.0 + (2.0*f-1.0)*p

	d := time.Duration(math.Round(float64(exp) * factor))
	if d < 0 {
		d = 0
	}
	if d > BackoffMax {
		// The +20 % jitter can overshoot BackoffMax slightly; clamp so the
		// invariant `BackoffDuration <= BackoffMax * (1 + jitter)` we
		// document in the test suite holds without surprises elsewhere.
		// We allow up to +jitter past the nominal cap because the spec
		// describes the cap as the *deterministic* component. Anything
		// larger is a bug.
		max := time.Duration(math.Round(float64(BackoffMax) * (1.0 + p)))
		if d > max {
			d = max
		}
	}
	return d
}

// RequeueWithBackoff is the convenience wrapper that returns a
// [ctrl.Result] suitable for direct use from a `Reconcile` method:
//
//	return common.RequeueWithBackoff(attempts), nil
//
// The jitter source is the package-global RNG; tests that need
// determinism should use [BackoffDuration] directly.
func RequeueWithBackoff(attempts int) ctrl.Result {
	return ctrl.Result{RequeueAfter: BackoffDuration(attempts, nil)}
}
