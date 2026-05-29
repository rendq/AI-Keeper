//go:build pbt

// Feature: ai-platform, Property 24: 版本约束求解唯一性 / 单调性
//
// Generator: 随机 versionConstraint + 可用版本集合
// Oracle: resolve(c, V) 确定（同输入同输出）+ 选最高满足且 stability ≥ beta
// **Validates: Requirements F12, A3.3, A4.2, C7.3**

package resolver

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// genStableSemVer generates a valid semver string with no pre-release
// (stable release versions only).
func genStableSemVer() gopter.Gen {
	return gen.UInt8Range(0, 20).FlatMap(func(major interface{}) gopter.Gen {
		return gen.UInt8Range(0, 20).FlatMap(func(minor interface{}) gopter.Gen {
			return gen.UInt8Range(0, 20).Map(func(patch uint8) shared.SemVer {
				return shared.SemVer(fmt.Sprintf("%d.%d.%d", major.(uint8), minor.(uint8), patch))
			})
		}, reflect.TypeOf(shared.SemVer("")))
	}, reflect.TypeOf(shared.SemVer("")))
}

// genVersionSet generates a non-empty slice of unique stable semver versions (1–20 items).
func genVersionSet() gopter.Gen {
	return gen.IntRange(1, 20).FlatMap(func(n interface{}) gopter.Gen {
		count := n.(int)
		return gen.SliceOfN(count, genStableSemVer()).Map(func(vs []shared.SemVer) []shared.SemVer {
			// Deduplicate
			seen := map[shared.SemVer]struct{}{}
			out := make([]shared.SemVer, 0, len(vs))
			for _, v := range vs {
				if _, ok := seen[v]; !ok {
					seen[v] = struct{}{}
					out = append(out, v)
				}
			}
			if len(out) == 0 {
				out = append(out, "1.0.0")
			}
			return out
		})
	}, reflect.TypeOf([]shared.SemVer{}))
}

// constraintInput holds a generated constraint string and available version set.
type constraintInput struct {
	ConstraintStr string
	Versions      []shared.SemVer
}

// genConstraint generates a valid npm-style version constraint that is
// guaranteed to parse. We build constraints from known patterns using
// random version numbers.
func genConstraint() gopter.Gen {
	return gen.UInt8Range(0, 9).FlatMap(func(major interface{}) gopter.Gen {
		return gen.UInt8Range(0, 9).FlatMap(func(minor interface{}) gopter.Gen {
			return gen.UInt8Range(0, 9).FlatMap(func(patch interface{}) gopter.Gen {
				return gen.IntRange(0, 6).Map(func(style int) string {
					maj := major.(uint8)
					min := minor.(uint8)
					pat := patch.(uint8)
					v := fmt.Sprintf("%d.%d.%d", maj, min, pat)
					switch style {
					case 0:
						return fmt.Sprintf("^%s", v)
					case 1:
						return fmt.Sprintf("~%s", v)
					case 2:
						return fmt.Sprintf(">=%s", v)
					case 3:
						return fmt.Sprintf("<=%s", v)
					case 4:
						return fmt.Sprintf(">=%d.%d.0 <%d.0.0", maj, min, maj+1)
					case 5:
						return "*"
					default:
						return v // exact
					}
				})
			}, reflect.TypeOf(""))
		}, reflect.TypeOf(""))
	}, reflect.TypeOf(""))
}

// genConstraintInput generates a (constraint, versions) pair.
func genConstraintInput() gopter.Gen {
	return gen.Struct(reflect.TypeOf(constraintInput{}), map[string]gopter.Gen{
		"ConstraintStr": genConstraint(),
		"Versions":      genVersionSet(),
	})
}

// genStability generates a non-experimental stability stage.
func genStability() gopter.Gen {
	return gen.OneConstOf(
		shared.StageBeta,
		shared.StageStable,
		shared.StageDeprecated,
	)
}

// candidateInput holds generated data for the full pickHighestSatisfying test.
type candidateInput struct {
	ConstraintStr string
	Candidates    []candidateSkill
}

type candidateSkill struct {
	Version   shared.SemVer
	Stability shared.Stage
}

func genCandidateSkill() gopter.Gen {
	return gen.Struct(reflect.TypeOf(candidateSkill{}), map[string]gopter.Gen{
		"Version":   genStableSemVer(),
		"Stability": gen.OneConstOf(shared.StageExperimental, shared.StageBeta, shared.StageStable, shared.StageDeprecated),
	})
}

func genCandidateInput() gopter.Gen {
	return gen.Struct(reflect.TypeOf(candidateInput{}), map[string]gopter.Gen{
		"ConstraintStr": genConstraint(),
		"Candidates":    gen.SliceOfN(20, genCandidateSkill()),
	})
}

// ---------------------------------------------------------------------------
// Test
// ---------------------------------------------------------------------------

func TestProperty24(t *testing.T) {
	seed := time.Now().UnixNano()
	if envSeed := os.Getenv("AIP_PBT_SEED"); envSeed != "" {
		if s, err := strconv.ParseInt(envSeed, 10, 64); err == nil {
			seed = s
		}
	}
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// -----------------------------------------------------------------------
	// Sub-property 1: Determinism — same constraint + same versions → same result
	// -----------------------------------------------------------------------
	properties.Property("deterministic: same input always yields same output", prop.ForAll(
		func(input constraintInput) bool {
			c, err := ParseConstraint(input.ConstraintStr)
			if err != nil {
				// Invalid constraint → skip (should not happen with our generator)
				return true
			}
			r1 := c.MaxSatisfying(input.Versions)
			for i := 0; i < 10; i++ {
				// Shuffle the slice order to ensure ordering doesn't matter
				shuffled := make([]shared.SemVer, len(input.Versions))
				copy(shuffled, input.Versions)
				rand.Shuffle(len(shuffled), func(a, b int) {
					shuffled[a], shuffled[b] = shuffled[b], shuffled[a]
				})
				r2 := c.MaxSatisfying(shuffled)
				if r1 != r2 {
					return false
				}
			}
			return true
		},
		genConstraintInput(),
	))

	// -----------------------------------------------------------------------
	// Sub-property 2: Selects highest satisfying version with stability ≥ beta
	// -----------------------------------------------------------------------
	properties.Property("selects highest satisfying with stability >= beta", prop.ForAll(
		func(input candidateInput) bool {
			c, err := ParseConstraint(input.ConstraintStr)
			if err != nil {
				return true
			}

			// Build skillv1alpha1.Skill slice to use pickHighestSatisfying
			skills := make([]skillv1alpha1.Skill, 0, len(input.Candidates))
			for _, cand := range input.Candidates {
				sk := skillv1alpha1.Skill{}
				sk.Spec.Version = cand.Version
				sk.Spec.Stability = cand.Stability
				skills = append(skills, sk)
			}

			result, perr := pickHighestSatisfying(skills, input.ConstraintStr)
			if perr != nil {
				return true // constraint parse error, skip
			}

			// Oracle: manually compute expected result
			var expected *skillv1alpha1.Skill
			for i := range skills {
				sk := &skills[i]
				// Must not be experimental
				if sk.Spec.Stability == shared.StageExperimental {
					continue
				}
				// Must satisfy constraint
				if !c.Match(sk.Spec.Version) {
					continue
				}
				if expected == nil || sk.Spec.Version.Compare(expected.Spec.Version) > 0 {
					expected = sk
				}
			}

			if expected == nil {
				return result == nil
			}
			if result == nil {
				return false
			}
			return result.Spec.Version == expected.Spec.Version
		},
		genCandidateInput(),
	))

	// -----------------------------------------------------------------------
	// Sub-property 3: Monotonicity — adding versions never degrades result
	// -----------------------------------------------------------------------
	properties.Property("monotonic: adding versions never degrades selected version", prop.ForAll(
		func(input constraintInput) bool {
			c, err := ParseConstraint(input.ConstraintStr)
			if err != nil {
				return true
			}
			if len(input.Versions) < 2 {
				return true
			}

			// Use a subset (first half) and compare with full set
			half := len(input.Versions) / 2
			subset := input.Versions[:half]
			full := input.Versions

			rSubset := c.MaxSatisfying(subset)
			rFull := c.MaxSatisfying(full)

			// If subset yields no result, full can yield anything (or nothing)
			if rSubset == "" {
				return true
			}
			// If full yields no result but subset does — impossible since full ⊇ subset
			if rFull == "" {
				return false
			}
			// Full result must be >= subset result
			return rFull.Compare(rSubset) >= 0
		},
		genConstraintInput(),
	))

	// -----------------------------------------------------------------------
	// Sub-property 4: No match → empty result
	// -----------------------------------------------------------------------
	properties.Property("no satisfying version yields empty result", prop.ForAll(
		func(input constraintInput) bool {
			c, err := ParseConstraint(input.ConstraintStr)
			if err != nil {
				return true
			}
			result := c.MaxSatisfying(input.Versions)
			if result == "" {
				// Verify no version in the set actually matches
				for _, v := range input.Versions {
					if c.Match(v) {
						return false // There IS a match but result is empty — bug!
					}
				}
				return true
			}
			// Result is non-empty — it must satisfy the constraint
			return c.Match(result)
		},
		genConstraintInput(),
	))

	properties.TestingRun(t)
}
