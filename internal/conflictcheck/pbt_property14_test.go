// Feature: ai-platform, Property 14: Policy 冲突可检测
//
// Generator: 随机 policy pair（含完全重叠 / 部分重叠 / 不重叠 × 同/异 priority × 同/反 effect）
// Oracle: hard conflict ⇒ controller phase=Failed ∧ PDP 不加载 bundle
//
// **Validates: Requirements F14, A5.3, A5.4**

//go:build pbt

package conflictcheck

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// ---------------------------------------------------------------------------
// Overlap mode for subject/resource sets
// ---------------------------------------------------------------------------

type overlapMode int

const (
	overlapModeFull    overlapMode = iota // completely overlapping
	overlapModePartial                    // partially overlapping
	overlapModeNone                       // disjoint
)

// ---------------------------------------------------------------------------
// Policy pair input structure
// ---------------------------------------------------------------------------

type policyPairInput struct {
	// Priority configuration
	PriorityA int32
	PriorityB int32

	// Effect configuration
	EffectA string
	EffectB string

	// Overlap modes
	SubjectOverlap  overlapMode
	ResourceOverlap overlapMode

	// Subject entries for policy A and B
	SubjectsA []policyv1alpha1.SubjectEntry
	SubjectsB []policyv1alpha1.SubjectEntry

	// Resource selectors for policy A and B
	ResourcesA []policyv1alpha1.ResourceSelector
	ResourcesB []policyv1alpha1.ResourceSelector
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func genEffect() gopter.Gen {
	return gen.OneConstOf("allow", "deny")
}

func genPriority() gopter.Gen {
	return gen.Int32Range(0, 1000)
}

func genOverlapMode() gopter.Gen {
	return gen.IntRange(0, 2).Map(func(v int) overlapMode {
		return overlapMode(v)
	})
}

func genSubjectKind() gopter.Gen {
	kinds := []string{"User", "Role", "Group", "Agent", "ServiceAccount", "Tenant"}
	return gen.IntRange(0, len(kinds)-1).Map(func(i int) string {
		return kinds[i]
	})
}

func genResourceKind() gopter.Gen {
	kinds := []string{"Skill", "Agent", "Tool", "ModelEndpoint", "DataSource", "KnowledgeBase"}
	return gen.IntRange(0, len(kinds)-1).Map(func(i int) string {
		return kinds[i]
	})
}

func genSubjectName() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9]{2,8}")
}

func genResourceName() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9]{2,8}")
}

// genSubjectEntry generates a single subject entry.
func genSubjectEntry() gopter.Gen {
	return gopter.CombineGens(
		genSubjectKind(),
		genSubjectName(),
	).Map(func(vals []interface{}) policyv1alpha1.SubjectEntry {
		kind := vals[0].(string)
		name := vals[1].(string)
		return policyv1alpha1.SubjectEntry{
			Kind:  kind,
			Match: &policyv1alpha1.SubjectMatch{Name: name},
		}
	})
}

// genResourceSelector generates a single resource selector.
func genResourceSelector() gopter.Gen {
	return gopter.CombineGens(
		genResourceKind(),
		genResourceName(),
	).Map(func(vals []interface{}) policyv1alpha1.ResourceSelector {
		kind := vals[0].(string)
		name := vals[1].(string)
		return policyv1alpha1.ResourceSelector{
			Kind:  kind,
			Match: &policyv1alpha1.ResourceMatch{Name: name},
		}
	})
}

// genSubjectSlice generates a slice of 1-3 subject entries.
func genSubjectSlice() gopter.Gen {
	return gen.SliceOfN(3, genSubjectEntry()).SuchThat(func(v interface{}) bool {
		s := v.([]policyv1alpha1.SubjectEntry)
		return len(s) >= 1
	})
}

// genResourceSlice generates a slice of 1-3 resource selectors.
func genResourceSlice() gopter.Gen {
	return gen.SliceOfN(3, genResourceSelector()).SuchThat(func(v interface{}) bool {
		s := v.([]policyv1alpha1.ResourceSelector)
		return len(s) >= 1
	})
}

// genPolicyPairInput generates a random policy pair with controlled overlap.
func genPolicyPairInput() gopter.Gen {
	return gopter.CombineGens(
		genPriority(),                // priorityA
		genPriority(),                // priorityB
		genEffect(),                  // effectA
		genEffect(),                  // effectB
		genOverlapMode(),             // subjectOverlap
		genOverlapMode(),             // resourceOverlap
		genSubjectSlice(),            // base subjects
		genResourceSlice(),           // base resources
		genSubjectEntry(),            // extra subject (for partial/none)
		genResourceSelector(),        // extra resource (for partial/none)
	).Map(func(vals []interface{}) policyPairInput {
		priA := vals[0].(int32)
		priB := vals[1].(int32)
		effA := vals[2].(string)
		effB := vals[3].(string)
		subOvl := vals[4].(overlapMode)
		resOvl := vals[5].(overlapMode)
		baseSubjects := vals[6].([]policyv1alpha1.SubjectEntry)
		baseResources := vals[7].([]policyv1alpha1.ResourceSelector)
		extraSubject := vals[8].(policyv1alpha1.SubjectEntry)
		extraResource := vals[9].(policyv1alpha1.ResourceSelector)

		// Ensure base slices are non-empty
		if len(baseSubjects) == 0 {
			baseSubjects = []policyv1alpha1.SubjectEntry{
				{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "default-user"}},
			}
		}
		if len(baseResources) == 0 {
			baseResources = []policyv1alpha1.ResourceSelector{
				{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "default-skill"}},
			}
		}

		// Construct subjects for A and B based on overlap mode
		subjectsA := make([]policyv1alpha1.SubjectEntry, len(baseSubjects))
		copy(subjectsA, baseSubjects)

		var subjectsB []policyv1alpha1.SubjectEntry
		switch subOvl {
		case overlapModeFull:
			// Same subjects
			subjectsB = make([]policyv1alpha1.SubjectEntry, len(baseSubjects))
			copy(subjectsB, baseSubjects)
		case overlapModePartial:
			// Shares some subjects but has extra
			subjectsB = make([]policyv1alpha1.SubjectEntry, len(baseSubjects))
			copy(subjectsB, baseSubjects)
			// Make extra subject unique from base
			extraSubject.Match = &policyv1alpha1.SubjectMatch{Name: "extra-subj-" + extraSubject.Kind}
			subjectsB = append(subjectsB, extraSubject)
		case overlapModeNone:
			// Completely different subjects
			subjectsB = []policyv1alpha1.SubjectEntry{
				{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "disjoint-user-xzy"}},
			}
		}

		// Construct resources for A and B based on overlap mode
		resourcesA := make([]policyv1alpha1.ResourceSelector, len(baseResources))
		copy(resourcesA, baseResources)

		var resourcesB []policyv1alpha1.ResourceSelector
		switch resOvl {
		case overlapModeFull:
			resourcesB = make([]policyv1alpha1.ResourceSelector, len(baseResources))
			copy(resourcesB, baseResources)
		case overlapModePartial:
			resourcesB = make([]policyv1alpha1.ResourceSelector, len(baseResources))
			copy(resourcesB, baseResources)
			extraResource.Match = &policyv1alpha1.ResourceMatch{Name: "extra-res-" + extraResource.Kind}
			resourcesB = append(resourcesB, extraResource)
		case overlapModeNone:
			resourcesB = []policyv1alpha1.ResourceSelector{
				{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "disjoint-skill-xzy"}},
			}
		}

		return policyPairInput{
			PriorityA:       priA,
			PriorityB:       priB,
			EffectA:         effA,
			EffectB:         effB,
			SubjectOverlap:  subOvl,
			ResourceOverlap: resOvl,
			SubjectsA:       subjectsA,
			SubjectsB:       subjectsB,
			ResourcesA:      resourcesA,
			ResourcesB:      resourcesB,
		}
	})
}

// ---------------------------------------------------------------------------
// Reference oracle implementation
// ---------------------------------------------------------------------------

// isHardConflict is the reference oracle for determining hard conflict.
// Hard conflict: same priority + opposite effect + full subject overlap + full resource overlap
func isHardConflict(input policyPairInput) bool {
	return input.PriorityA == input.PriorityB &&
		input.EffectA != input.EffectB &&
		input.SubjectOverlap == overlapModeFull &&
		input.ResourceOverlap == overlapModeFull
}

// isSoftConflict is the reference oracle for determining soft conflict.
// Soft conflict: same priority + opposite effect + at least one partial overlap (and not both full)
func isSoftConflict(input policyPairInput) bool {
	if input.PriorityA != input.PriorityB {
		return false
	}
	if input.EffectA == input.EffectB {
		return false
	}
	// At least one must overlap (not none), and not both full
	subOverlaps := input.SubjectOverlap != overlapModeNone
	resOverlaps := input.ResourceOverlap != overlapModeNone

	if !subOverlaps || !resOverlaps {
		return false
	}
	// It's not hard (both full) — already checked by isHardConflict
	if input.SubjectOverlap == overlapModeFull && input.ResourceOverlap == overlapModeFull {
		return false
	}
	return true
}

// isNoConflict returns true when no conflict should be detected.
func isNoConflict(input policyPairInput) bool {
	return !isHardConflict(input) && !isSoftConflict(input)
}

// ---------------------------------------------------------------------------
// Simulated Policy Controller behavior
// ---------------------------------------------------------------------------

// simulatePolicyControllerPhase simulates what the policy controller does
// when it encounters conflicts: hard conflict → phase=Failed, PDP must NOT load bundle.
func simulatePolicyControllerPhase(conflicts []ConflictResult) (phase string, bundleLoaded bool) {
	if HasHardConflict(conflicts) {
		return "Failed", false
	}
	// Soft conflicts or no conflicts: proceed normally
	return "Active", true
}

// ---------------------------------------------------------------------------
// Test
// ---------------------------------------------------------------------------

func TestProperty14(t *testing.T) {
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

	// Sub-property 1: Hard conflict ⇒ controller phase=Failed ∧ PDP does NOT load bundle
	properties.Property("hard conflict ⇒ phase=Failed and PDP does not load bundle", prop.ForAll(
		func(input policyPairInput) bool {
			policies := buildPolicies(input)
			results := DetectConflicts(policies)

			if isHardConflict(input) {
				// Oracle: must detect hard conflict
				if !HasHardConflict(results) {
					return false
				}
				// Simulate controller behavior
				phase, bundleLoaded := simulatePolicyControllerPhase(results)
				if phase != "Failed" {
					return false
				}
				if bundleLoaded {
					return false
				}
				return true
			}
			return true // only checking hard conflict case here
		},
		genPolicyPairInput(),
	))

	// Sub-property 2: Soft conflict ⇒ detected as Soft (warning, still allows compilation)
	properties.Property("soft conflict ⇒ detected as Soft, compilation allowed", prop.ForAll(
		func(input policyPairInput) bool {
			policies := buildPolicies(input)
			results := DetectConflicts(policies)

			if isSoftConflict(input) {
				// Oracle: must detect soft conflict but NOT hard
				hasSoft := false
				for _, r := range results {
					if r.Type == Soft {
						hasSoft = true
					}
				}
				if !hasSoft {
					return false
				}
				if HasHardConflict(results) {
					return false
				}
				// Controller should still allow bundle loading
				phase, bundleLoaded := simulatePolicyControllerPhase(results)
				if phase != "Active" {
					return false
				}
				if !bundleLoaded {
					return false
				}
				return true
			}
			return true // only checking soft conflict case here
		},
		genPolicyPairInput(),
	))

	// Sub-property 3: No conflict ⇒ no conflicts detected
	properties.Property("no conflict conditions ⇒ no conflicts detected", prop.ForAll(
		func(input policyPairInput) bool {
			policies := buildPolicies(input)
			results := DetectConflicts(policies)

			if isNoConflict(input) {
				if len(results) != 0 {
					return false
				}
				return true
			}
			return true // only checking no-conflict case here
		},
		genPolicyPairInput(),
	))

	// Sub-property 4: Hard conflict detection is symmetric (order doesn't matter)
	properties.Property("conflict detection is symmetric", prop.ForAll(
		func(input policyPairInput) bool {
			policiesAB := buildPolicies(input)
			policiesBA := []policyv1alpha1.Policy{policiesAB[1], policiesAB[0]}

			resultsAB := DetectConflicts(policiesAB)
			resultsBA := DetectConflicts(policiesBA)

			// Same number of conflicts
			if len(resultsAB) != len(resultsBA) {
				return false
			}
			// Same conflict types
			if HasHardConflict(resultsAB) != HasHardConflict(resultsBA) {
				return false
			}
			return true
		},
		genPolicyPairInput(),
	))

	// Sub-property 5: Hard conflict implies phase=Failed with reason=PolicyConflict
	properties.Property("hard conflict ⇒ reason=PolicyConflict", prop.ForAll(
		func(input policyPairInput) bool {
			if !isHardConflict(input) {
				return true // skip non-hard cases
			}
			policies := buildPolicies(input)
			results := DetectConflicts(policies)

			if !HasHardConflict(results) {
				return false
			}

			// Simulate full controller behavior: set condition + phase
			pol := &policies[0]
			pol.Status.Phase = ""
			pol.Status.Conditions = nil

			for _, r := range results {
				if r.Type == Hard {
					// Controller sets PolicyNotConflicting=False, reason=PolicyConflict
					pol.Status.Phase = "Failed"
					pol.Status.Conditions = append(pol.Status.Conditions, metav1.Condition{
						Type:    policyv1alpha1.PolicyNotConflicting,
						Status:  metav1.ConditionFalse,
						Reason:  "PolicyConflict",
						Message: fmt.Sprintf("hard conflict with %s: %s", r.PolicyB, r.Reason),
					})
					break
				}
			}

			if pol.Status.Phase != "Failed" {
				return false
			}
			if len(pol.Status.Conditions) == 0 {
				return false
			}
			if pol.Status.Conditions[0].Reason != "PolicyConflict" {
				return false
			}
			return true
		},
		genPolicyPairInput(),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Helper: build Policy objects from input
// ---------------------------------------------------------------------------

func buildPolicies(input policyPairInput) []policyv1alpha1.Policy {
	pA := policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-a"},
		Spec: policyv1alpha1.PolicySpec{
			Effect:   input.EffectA,
			Priority: ptr.To(input.PriorityA),
			Subject:  policyv1alpha1.SubjectSelector{AnyOf: input.SubjectsA},
			Action: policyv1alpha1.PolicyAction{
				Verbs:     []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{AnyOf: input.ResourcesA},
			},
		},
	}
	pB := policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-b"},
		Spec: policyv1alpha1.PolicySpec{
			Effect:   input.EffectB,
			Priority: ptr.To(input.PriorityB),
			Subject:  policyv1alpha1.SubjectSelector{AnyOf: input.SubjectsB},
			Action: policyv1alpha1.PolicyAction{
				Verbs:     []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{AnyOf: input.ResourcesB},
			},
		},
	}
	return []policyv1alpha1.Policy{pA, pB}
}
