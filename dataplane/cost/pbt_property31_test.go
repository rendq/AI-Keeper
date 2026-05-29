// Feature: ai-platform, Property 31: Cost computation purity

//go:build pbt

package cost

import (
	"math"
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

// genUsage generates a Usage with non-negative Input, Output, Cached values.
func genUsage() gopter.Gen {
	return gen.Struct(reflect.TypeOf(Usage{}), map[string]gopter.Gen{
		"Input":  gen.Int64Range(0, 10_000_000),
		"Output": gen.Int64Range(0, 10_000_000),
		"Cached": gen.Int64Range(0, 10_000_000),
	})
}

// genEndpointCost generates an EndpointCost with non-negative pricing.
func genEndpointCost() gopter.Gen {
	return gen.Struct(reflect.TypeOf(EndpointCost{}), map[string]gopter.Gen{
		"InputPerMillion":  gen.Float64Range(0, 100),
		"OutputPerMillion": gen.Float64Range(0, 100),
		"CachedPerMillion": gen.Float64Range(0, 100),
	})
}

func TestProperty31(t *testing.T) {
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

	// Property: ComputeCost matches the oracle formula, is pure, deterministic, and >= 0
	properties.Property("cost = oracle formula, pure, deterministic, >= 0", prop.ForAll(
		func(usage Usage, ec EndpointCost) bool {
			// Compute via function under test
			result := ComputeCost(usage, ec)

			// Oracle: (usage.in × cost.input + usage.out × cost.output + usage.cached × cost.cached) / 1e6
			oracle := (float64(usage.Input)*ec.InputPerMillion +
				float64(usage.Output)*ec.OutputPerMillion +
				float64(usage.Cached)*ec.CachedPerMillion) / 1e6
			if oracle < 0 {
				oracle = 0
			}

			// Check formula correctness (within floating point tolerance)
			if math.Abs(result-oracle) > 1e-15 {
				return false
			}

			// Check purity / determinism: calling twice yields same result
			result2 := ComputeCost(usage, ec)
			if result != result2 {
				return false
			}

			// Check non-negative
			if result < 0 {
				return false
			}

			return true
		},
		genUsage(),
		genEndpointCost(),
	))

	properties.TestingRun(t)
}
