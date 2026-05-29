//go:build pbt

package lint

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements A9.2**

// allKinds enumerates the 12 resource kinds recognized by AIP.
var allKinds = []string{
	"Skill",
	"Agent",
	"Policy",
	"Tool",
	"ModelEndpoint",
	"ModelRouter",
	"KnowledgeBase",
	"DataSource",
	"Tenant",
	"ServiceAccount",
	"Budget",
	"Quota",
}

// genResource generates a random Resource with a valid Kind from 12 types and random Spec maps.
func genResource() gopter.Gen {
	return gen.IntRange(0, len(allKinds)-1).FlatMap(func(v interface{}) gopter.Gen {
		kind := allKinds[v.(int)]
		return gen.IntRange(0, 5).Map(func(n int) *Resource {
			spec := make(map[string]interface{})
			// Add some random keys to the spec
			keys := []string{"stability", "version", "evaluation", "reliability", "cost", "governance", "acl", "runtime", "skills", "priority", "compliance", "privacy"}
			for i := 0; i < n && i < len(keys); i++ {
				spec[keys[i]] = fmt.Sprintf("val-%d", i)
			}
			return &Resource{
				Kind: kind,
				Name: fmt.Sprintf("test-%s", kind),
				Spec: spec,
			}
		})
	}, nil)
}

// genRandomResource generates a more diverse Resource with nested maps and various value types.
func genRandomResource() gopter.Gen {
	return gen.IntRange(0, len(allKinds)-1).FlatMap(func(v interface{}) gopter.Gen {
		kind := allKinds[v.(int)]
		return gen.IntRange(0, 7).FlatMap(func(v2 interface{}) gopter.Gen {
			variant := v2.(int)
			return gen.Const(&Resource{
				Kind: kind,
				Name: fmt.Sprintf("res-%s-%d", kind, variant),
				Spec: buildVariantSpec(kind, variant),
			})
		}, nil)
	}, nil)
}

// buildVariantSpec creates different spec variants to exercise various code paths in the rules.
func buildVariantSpec(kind string, variant int) map[string]interface{} {
	spec := make(map[string]interface{})
	switch variant {
	case 0:
		// Empty spec
	case 1:
		// Minimal string fields
		spec["stability"] = "experimental"
	case 2:
		// Nested map
		spec["governance"] = map[string]interface{}{"classification": "public"}
	case 3:
		// Deep nesting
		spec["runtime"] = map[string]interface{}{
			"pattern": "simple",
			"sandbox": map[string]interface{}{"enabled": false},
		}
		spec["skills"] = []interface{}{map[string]interface{}{"ref": "skill://test"}}
	case 4:
		// Integer fields
		spec["priority"] = 500
		spec["effectiveWindow"] = map[string]interface{}{
			"notAfter": time.Now().AddDate(1, 0, 0).Format(time.RFC3339),
		}
	case 5:
		// Boolean fields
		spec["_lintSpecChanged"] = false
		spec["_hasCodeTool"] = true
	case 6:
		// Slice fields
		spec["compliance"] = []interface{}{"SOC2", "ISO27001"}
		spec["privacy"] = map[string]interface{}{"dpaSigned": true}
	case 7:
		// Nil/empty nested values
		spec["acl"] = map[string]interface{}{"mode": "rbac", "enforcement": "pre_filter"}
		spec["governance"] = map[string]interface{}{"classification": "internal"}
	}
	return spec
}

// TestProperty33 validates P33: lint rule completeness.
// 1. RunAll never panics on any random valid resource.
// 2. RunAll always returns []LintViolation (possibly empty).
// 3. For known-bad inputs (stable Skill without evalSet), error-level violations always fire.
func TestProperty33(t *testing.T) {
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

	// Property 1: RunAll never panics on any random resource and always returns a valid slice.
	properties.Property("P33: RunAll never panics on random resources and returns []LintViolation", prop.ForAll(
		func(res *Resource) bool {
			// If this panics, the property fails automatically.
			violations := RunAll(res)
			// violations must be a valid slice (nil or non-nil)
			_ = len(violations)
			// Every violation must have non-empty Rule and Level
			for _, v := range violations {
				if v.Rule == "" || v.Level == "" {
					return false
				}
				if v.Level != LevelError && v.Level != LevelWarn {
					return false
				}
			}
			return true
		},
		genRandomResource(),
	))

	// Property 2: For known-bad inputs (stable Skill without evalSet), error-level violations always fire.
	properties.Property("P33: stable Skill without evalSet always triggers error-level violation", prop.ForAll(
		func(name string) bool {
			res := &Resource{
				Kind: "Skill",
				Name: name,
				Spec: map[string]interface{}{
					"stability": "stable",
					"version":   "1.0.0",
					// No evaluation.evalSet — this is the known-bad pattern
				},
			}
			violations := RunAll(res)
			for _, v := range violations {
				if v.Rule == "skill/has-eval-set" && v.Level == LevelError {
					return true
				}
			}
			return false
		},
		gen.RegexMatch("[a-z][a-z0-9-]{0,20}"),
	))

	// Property 3: For known-bad input (destructive Tool without approval), error-level violation fires.
	properties.Property("P33: destructive Tool without approval always triggers error-level violation", prop.ForAll(
		func(name string) bool {
			res := &Resource{
				Kind: "Tool",
				Name: name,
				Spec: map[string]interface{}{
					"governance": map[string]interface{}{
						"sideEffects": "destructive",
						// No requiresApproval
					},
				},
			}
			violations := RunAll(res)
			for _, v := range violations {
				if v.Rule == "tool/destructive-needs-approval" && v.Level == LevelError {
					return true
				}
			}
			return false
		},
		gen.RegexMatch("[a-z][a-z0-9-]{0,20}"),
	))

	// Property 4: For known-bad input (Agent with no skills), error-level violation fires.
	properties.Property("P33: Agent with empty skills always triggers error-level violation", prop.ForAll(
		func(name string) bool {
			res := &Resource{
				Kind: "Agent",
				Name: name,
				Spec: map[string]interface{}{
					"skills": []interface{}{},
				},
			}
			violations := RunAll(res)
			for _, v := range violations {
				if v.Rule == "agent/skills-resolved" && v.Level == LevelError {
					return true
				}
			}
			return false
		},
		gen.RegexMatch("[a-z][a-z0-9-]{0,20}"),
	))

	properties.TestingRun(t)
}
