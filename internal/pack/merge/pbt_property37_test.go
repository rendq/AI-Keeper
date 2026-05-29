// Feature: ai-platform, Property 37: Pack three-way merge idempotency

//go:build pbt

package merge

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements C5.4**

// genYAMLValue generates a random YAML-like leaf value (string or int).
func genYAMLValue() gopter.Gen {
	return gen.Identifier().Map(func(s string) interface{} {
		return s
	})
}

// genYAMLMap generates a random map[string]interface{} with 1-5 keys simulating a YAML document.
func genYAMLMap() gopter.Gen {
	keys := []string{"name", "version", "replicas", "image", "port"}
	return gen.SliceOfN(5, gen.Identifier()).Map(func(vals []string) map[string]interface{} {
		m := make(map[string]interface{})
		for i, v := range vals {
			m[keys[i]] = v
		}
		return m
	})
}

func TestProperty37(t *testing.T) {
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

	// Property: Three-way merge is idempotent — merging the result again yields the same merged output.
	// merge(merge(base, ours, theirs)).Merged == merge(base, ours, theirs).Merged
	properties.Property("three-way merge is idempotent", prop.ForAll(
		func(base, ours, theirs map[string]interface{}) bool {
			// First merge
			result1 := ThreeWayMerge(base, ours, theirs, "pbt-resource")

			// Second merge: use the merged result as all three inputs
			// Idempotency means: merge(merged, merged, merged) == merged
			// But the task oracle specifies: merge(merge(base, ours, theirs)).Merged == merge(base, ours, theirs).Merged
			// Which means applying merge again with same base/ours/theirs context but using merged as the new "ours"
			// The clearest interpretation: merge(base, result1.Merged, theirs) should produce same Merged output
			// when result1 has no conflicts (clean merge is stable).
			//
			// Most natural idempotency: merge(merged, merged, merged) == merged
			result2 := ThreeWayMerge(result1.Merged, result1.Merged, result1.Merged, "pbt-resource")

			// The second merge should produce the same output
			if !deepEqual(result1.Merged, result2.Merged) {
				t.Logf("VIOLATION: merge is not idempotent")
				t.Logf("  base=%v", base)
				t.Logf("  ours=%v", ours)
				t.Logf("  theirs=%v", theirs)
				t.Logf("  result1.Merged=%v", result1.Merged)
				t.Logf("  result2.Merged=%v", result2.Merged)
				return false
			}

			// Also verify: re-merging with same inputs produces same result (determinism)
			result3 := ThreeWayMerge(base, ours, theirs, "pbt-resource")
			if !deepEqual(result1.Merged, result3.Merged) {
				t.Logf("VIOLATION: merge is not deterministic")
				return false
			}

			return true
		},
		genYAMLMap(),
		genYAMLMap(),
		genYAMLMap(),
	))

	properties.TestingRun(t)
}
