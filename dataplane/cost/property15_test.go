// Feature: ai-platform, Property 15: Budget enforcement purity and determinism

//go:build pbt

package cost

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements F15, A8, B11**

// genAlertConfig generates a valid AlertConfig with threshold in [0.0, 1.0).
func genAlertConfig() gopter.Gen {
	return gen.Struct(reflect.TypeOf(AlertConfig{}), map[string]gopter.Gen{
		"Threshold": gen.Float64Range(0.01, 0.99),
		"Channel":   gen.OneConstOf("slack", "email", "webhook"),
		"Action":    gen.OneConstOf(ActionNotify, ActionThrottle, ActionBlock),
	})
}

// genAlertConfigs generates a slice of 0-5 AlertConfig values.
func genAlertConfigs() gopter.Gen {
	return gen.SliceOfN(5, genAlertConfig()).Map(func(alerts []AlertConfig) []AlertConfig {
		// Ensure thresholds are distinct and sorted for clearer semantics
		seen := make(map[float64]bool)
		result := make([]AlertConfig, 0, len(alerts))
		for _, a := range alerts {
			// Round to 2 decimals to reduce near-duplicates
			rounded := float64(int(a.Threshold*100)) / 100
			if !seen[rounded] {
				seen[rounded] = true
				a.Threshold = rounded
				result = append(result, a)
			}
		}
		return result
	})
}

// genMonotonicAlertConfigs generates alert configs where severity is monotonically
// non-decreasing with threshold (a well-formed configuration).
func genMonotonicAlertConfigs() gopter.Gen {
	return gen.IntRange(0, 4).FlatMap(func(v interface{}) gopter.Gen {
		n := v.(int)
		if n == 0 {
			return gen.Const([]AlertConfig{})
		}
		return gen.SliceOfN(n, gen.Float64Range(0.1, 0.95)).Map(func(thresholds []float64) []AlertConfig {
			// Sort thresholds ascending
			sorted := make([]float64, len(thresholds))
			copy(sorted, thresholds)
			for i := 0; i < len(sorted); i++ {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[j] < sorted[i] {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			// Assign actions with non-decreasing severity
			actions := []AlertAction{ActionNotify, ActionThrottle, ActionBlock}
			configs := make([]AlertConfig, len(sorted))
			for i, th := range sorted {
				// Severity index increases with position (capped at max)
				sevIdx := (i * len(actions)) / len(sorted)
				if sevIdx >= len(actions) {
					sevIdx = len(actions) - 1
				}
				configs[i] = AlertConfig{
					Threshold: th,
					Channel:   "slack",
					Action:    actions[sevIdx],
				}
			}
			return configs
		})
	}, reflect.TypeOf(int(0)))
}

func TestProperty15(t *testing.T) {
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

	// Property 1: Determinism — same inputs always produce same output
	properties.Property("deterministic: Check(usage, limit) is pure", prop.ForAll(
		func(usage float64, limit float64, alerts []AlertConfig) bool {
			enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)
			ctx := context.Background()

			d1 := enforcer.Check(ctx, "tenant-test", usage, limit)
			d2 := enforcer.Check(ctx, "tenant-test", usage, limit)

			return d1.Allowed == d2.Allowed &&
				d1.Action == d2.Action &&
				d1.Reason == d2.Reason &&
				d1.ThrottleDelay == d2.ThrottleDelay
		},
		gen.Float64Range(0, 200),
		gen.Float64Range(50, 150),
		genAlertConfigs(),
	))

	// Property 2: HardCap — usage >= limit always blocks
	properties.Property("hardCap: usage >= limit implies block", prop.ForAll(
		func(limit float64, extra float64, alerts []AlertConfig) bool {
			usage := limit + extra // usage >= limit guaranteed since extra >= 0
			enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)
			ctx := context.Background()

			d := enforcer.Check(ctx, "tenant-test", usage, limit)

			return d.Allowed == false && d.Action == "block"
		},
		gen.Float64Range(50, 150),
		gen.Float64Range(0, 100), // extra >= 0 ensures usage >= limit
		genAlertConfigs(),
	))

	// Property 3: Below all thresholds — usage/limit < lowest threshold implies allow
	properties.Property("below thresholds: low ratio implies allow", prop.ForAll(
		func(limit float64, alerts []AlertConfig) bool {
			if len(alerts) == 0 || limit <= 0 {
				// No alerts → always allowed (below hard cap)
				enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)
				ctx := context.Background()
				d := enforcer.Check(ctx, "tenant-test", 0, limit)
				return d.Allowed == true
			}

			// Find the lowest threshold
			lowestThreshold := alerts[0].Threshold
			for _, a := range alerts[1:] {
				if a.Threshold < lowestThreshold {
					lowestThreshold = a.Threshold
				}
			}

			// Set usage to be safely below the lowest threshold
			usage := limit * lowestThreshold * 0.5 // half of lowest threshold

			enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)
			ctx := context.Background()
			d := enforcer.Check(ctx, "tenant-test", usage, limit)

			return d.Allowed == true && d.Action == "allow"
		},
		gen.Float64Range(50, 150),
		genAlertConfigs(),
	))

	// Property 4: Monotonic severity — higher usage ratio leads to equal or more severe action
	// This property only holds for well-formed alert configs (higher thresholds → equal or more severe actions).
	properties.Property("monotonic severity: higher ratio → equal or more severe action", prop.ForAll(
		func(limit float64, ratioLow float64, ratioHigh float64, alerts []AlertConfig) bool {
			if limit <= 0 {
				return true
			}
			// Ensure ratioLow <= ratioHigh
			if ratioLow > ratioHigh {
				ratioLow, ratioHigh = ratioHigh, ratioLow
			}

			usageLow := limit * ratioLow
			usageHigh := limit * ratioHigh

			enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)
			ctx := context.Background()

			dLow := enforcer.Check(ctx, "tenant-test", usageLow, limit)
			dHigh := enforcer.Check(ctx, "tenant-test", usageHigh, limit)

			return severityScore(dHigh) >= severityScore(dLow)
		},
		gen.Float64Range(50, 150),
		gen.Float64Range(0, 1.5),  // ratio low
		gen.Float64Range(0, 1.5),  // ratio high
		genMonotonicAlertConfigs(),
	))

	properties.TestingRun(t)
}

// severityScore maps an action string to a numeric severity for ordering comparison.
func severityScore(d BudgetDecision) int {
	switch d.Action {
	case "allow":
		return 0
	case "notify":
		return 1
	case "throttle":
		return 2
	case "block":
		return 3
	default:
		return -1
	}
}
