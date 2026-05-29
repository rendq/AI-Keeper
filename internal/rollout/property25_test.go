//go:build pbt

package rollout

import (
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements A4.5**

// genAnalysisMetrics generates random AnalysisMetrics within task-specified ranges:
// error rate [0, 0.5], latency [0, 2000ms], guardrail triggers [0, 50].
func genAnalysisMetrics() gopter.Gen {
	return gen.Struct(reflect.TypeOf(AnalysisMetrics{}), map[string]gopter.Gen{
		"ErrorRate":         gen.Float64Range(0.0, 0.5),
		"LatencyP95":        gen.Int64Range(0, 2000).Map(func(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }),
		"GuardrailTriggers": gen.IntRange(0, 50),
	})
}

// genAnalysisConfig generates random AnalysisConfig with reasonable threshold ranges.
func genAnalysisConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.Float64Range(0.01, 0.3),         // ErrorRateThreshold
		gen.Int64Range(10, 1000),             // LatencyP95Threshold (ms)
		gen.IntRange(1, 20),                  // GuardrailTriggerThreshold
	).Map(func(values []interface{}) AnalysisConfig {
		return AnalysisConfig{
			ErrorRateThreshold:        values[0].(float64),
			LatencyP95Threshold:       time.Duration(values[1].(int64)) * time.Millisecond,
			GuardrailTriggerThreshold: values[2].(int),
		}
	})
}

func TestProperty25(t *testing.T) {
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

	analyzer := NewAnalyzer()

	// Sub-property 1: Determinism — same inputs always produce same output.
	properties.Property("deterministic: same inputs produce same output", prop.ForAll(
		func(canary, stable AnalysisMetrics, config AnalysisConfig) bool {
			r1 := analyzer.Analyze(canary, stable, config)
			r2 := analyzer.Analyze(canary, stable, config)
			return r1.Passed == r2.Passed &&
				r1.Recommendation == r2.Recommendation &&
				slicesEqual(r1.FailedMetrics, r2.FailedMetrics)
		},
		genAnalysisMetrics(),
		genAnalysisMetrics(),
		genAnalysisConfig(),
	))

	// Sub-property 2: Degraded past threshold → always recommends Rollback.
	// We generate metrics where canary is strictly worse than stable beyond thresholds.
	properties.Property("degraded past threshold implies Rollback", prop.ForAll(
		func(stable AnalysisMetrics, config AnalysisConfig) bool {
			// Construct canary that exceeds ALL thresholds relative to stable.
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			canary := AnalysisMetrics{
				ErrorRate:         stable.ErrorRate + config.ErrorRateThreshold + 0.001 + rng.Float64()*0.1,
				LatencyP95:        stable.LatencyP95 + config.LatencyP95Threshold + time.Millisecond + time.Duration(rng.Intn(100))*time.Millisecond,
				GuardrailTriggers: stable.GuardrailTriggers + config.GuardrailTriggerThreshold + 1 + rng.Intn(5),
			}
			result := analyzer.Analyze(canary, stable, config)
			return !result.Passed && result.Recommendation == RecommendRollback
		},
		genAnalysisMetrics(),
		genAnalysisConfig(),
	))

	// Sub-property 3: Equal metrics → Continue (no degradation means pass).
	properties.Property("equal metrics implies Continue", prop.ForAll(
		func(metrics AnalysisMetrics, config AnalysisConfig) bool {
			// canary == stable → difference is 0 for all metrics, which is ≤ threshold (thresholds > 0)
			result := analyzer.Analyze(metrics, metrics, config)
			return result.Passed && result.Recommendation == RecommendContinue && len(result.FailedMetrics) == 0
		},
		genAnalysisMetrics(),
		genAnalysisConfig(),
	))

	properties.TestingRun(t)
}

// slicesEqual checks if two string slices are equal.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
