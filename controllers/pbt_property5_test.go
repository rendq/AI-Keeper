//go:build pbt

// Feature: ai-platform, Property 5: Controller Phase Ordering Invariant
//
// Generator: Randomly inject stage failures at various phases
// Oracle: When an earlier phase condition is False, later phases are NOT executed
// Property: P5 / Validates: A3.1, A4.1, A5.1

package controllers_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clocktest "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/skill"
)

// ---------------------------------------------------------------------------
// Phase ordering for Skill controller:
//   Validating → Resolving → Building → Registering → Evaluating → Active
//
// Conditions in order:
//   SchemaValid → DependenciesResolved → ImplementationReady → Registered → EvalPassing → Ready
//
// Invariant: if condition[i] is False, then condition[j] for j > i must NOT be True.
// ---------------------------------------------------------------------------

// failureStage represents which phase to inject a failure at.
type failureStage int

const (
	failAtSchema         failureStage = iota // SchemaValid = False
	failAtDependencies                       // DependenciesResolved = False
	failAtImplementation                     // ImplementationReady = False
	failAtRegistration                       // Registered = False
	numFailureStages                         // sentinel
)

// phaseOrderInput is the generator output for Property 5.
type phaseOrderInput struct {
	Name         string
	FailAt       failureStage
	Version      string
	Stability    sharedv1alpha1.Stage
}

// conditionOrder defines the expected ordering of conditions.
// Each condition at index i must be False/Unknown if the condition at
// a lower index is False.
var conditionOrder = []string{
	skillv1alpha1.SkillSchemaValid,
	skillv1alpha1.SkillDependenciesResolved,
	skillv1alpha1.SkillImplementationReady,
	skillv1alpha1.SkillRegistered,
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func genPhaseOrderInput() gopter.Gen {
	stabilities := []sharedv1alpha1.Stage{
		sharedv1alpha1.StageExperimental,
		sharedv1alpha1.StageBeta,
		sharedv1alpha1.StageStable,
	}

	return gen.Struct(reflect.TypeOf(phaseOrderInput{}), map[string]gopter.Gen{
		"Name":      genDNSName(),
		"FailAt":    gen.IntRange(0, int(numFailureStages)-1).Map(func(v int) failureStage { return failureStage(v) }),
		"Version":   genSemVer(),
		"Stability": gen.OneConstOf(stabilities[0], stabilities[1], stabilities[2]),
	})
}

// ---------------------------------------------------------------------------
// Failure-injecting stubs
// ---------------------------------------------------------------------------

// invalidSchemaJSON is a schema that fails JSON Schema compilation.
var invalidSchemaJSON = []byte(`{"type":"INVALID_NOT_A_TYPE"}`)

// failingResolver always reports missing dependencies (cyclic for
// permanent failure).
type failingResolver struct{}

func (failingResolver) Resolve(_ context.Context, _ *skillv1alpha1.Skill) (skill.ResolveResult, error) {
	return skill.ResolveResult{
		Missing: []sharedv1alpha1.ResourceRef{"skill://default/missing-dep"},
	}, nil
}

// failingRegistry always returns an error on Register.
type failingRegistry struct{}

func (failingRegistry) Register(_ context.Context, _ *skillv1alpha1.Skill) error {
	return fmt.Errorf("registry unavailable: simulated failure")
}

func (failingRegistry) Deregister(_ context.Context, _ sharedv1alpha1.ResourceRef) error {
	return nil
}

// ---------------------------------------------------------------------------
// Reconcile helper for Property 5
// ---------------------------------------------------------------------------

func reconcileSkillWithFailure(ctx context.Context, input phaseOrderInput) (map[string]metav1.ConditionStatus, error) {
	scheme := pbtScheme()

	// Build the Skill spec based on which stage should fail.
	var inputSchema []byte
	if input.FailAt == failAtSchema {
		inputSchema = invalidSchemaJSON
	} else {
		inputSchema = validJSONSchema
	}

	// For failAtImplementation, we create a skill with no runtime info
	// (empty image and entrypoint).
	var impl skillv1alpha1.SkillImplementation
	if input.FailAt == failAtImplementation {
		impl = skillv1alpha1.SkillImplementation{
			Type: "function",
			Runtime: &skillv1alpha1.SkillRuntime{
				Engine:     "aip-runtime/v2",
				Entrypoint: "", // empty → fails ensureImplementation
				Image:      "", // empty → fails ensureImplementation
			},
		}
	} else {
		impl = skillv1alpha1.SkillImplementation{
			Type: "function",
			Runtime: &skillv1alpha1.SkillRuntime{
				Engine:     "aip-runtime/v2",
				Entrypoint: "skills.test.run",
				Image:      "ghcr.io/test/skill:1.0.0",
			},
		}
	}

	sk := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:       input.Name,
			Namespace:  "default",
			Generation: 1,
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   sharedv1alpha1.SemVer(input.Version),
			Stability: input.Stability,
			Interface: skillv1alpha1.SkillInterface{
				Input:  skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: inputSchema}},
				Output: skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: validJSONSchema}},
			},
			Implementation: impl,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sk).
		WithStatusSubresource(sk).
		Build()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := clocktest.NewFakeClock(now)

	// Wire up the appropriate resolver and registry based on failure stage.
	var resolver skill.Resolver
	var registry skill.Registry

	switch input.FailAt {
	case failAtDependencies:
		resolver = failingResolver{}
		registry = skill.NewMemoryRegistry()
	case failAtRegistration:
		resolver = skill.NoopResolver{}
		registry = failingRegistry{}
	default:
		resolver = skill.NoopResolver{}
		registry = skill.NewMemoryRegistry()
	}

	rec := &skill.SkillReconciler{
		Client:   cl,
		Scheme:   scheme,
		Clock:    fc,
		Resolver: resolver,
		Registry: registry,
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.Name, Namespace: "default"}}

	// Run reconcile until stable (max 10 passes to allow finalizer + requeues).
	for i := 0; i < 10; i++ {
		res, err := rec.Reconcile(ctx, req)
		if err != nil {
			// Transient errors from resolver are expected; continue reconciling.
			continue
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		// For RequeueAfter > 0 without explicit Requeue, treat as stabilized
		// for testing purposes (the controller will settle at that state).
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	// Read back final state.
	final := &skillv1alpha1.Skill{}
	if err := cl.Get(ctx, req.NamespacedName, final); err != nil {
		return nil, err
	}

	// Extract condition statuses.
	result := make(map[string]metav1.ConditionStatus, len(conditionOrder))
	for _, condType := range conditionOrder {
		found := false
		for _, c := range final.Status.Conditions {
			if c.Type == condType {
				result[condType] = c.Status
				found = true
				break
			}
		}
		if !found {
			// Condition not set → treat as Unknown (not evaluated).
			result[condType] = metav1.ConditionUnknown
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// TestProperty5 — Controller Phase Ordering Invariant
//
// **Validates: Requirements A3.1, A4.1, A5.1**
//
// For the Skill controller, verifies the phase ordering invariant:
// when an earlier phase condition is False, subsequent phase conditions
// must NOT be True. This ensures the controller never skips phases.
//
// Phase order: Validating → Resolving → Building → Registering → Evaluating → Active
// Condition order: SchemaValid → DependenciesResolved → ImplementationReady → Registered
// ---------------------------------------------------------------------------

func TestProperty5(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	ctx := context.Background()

	properties.Property("Phase ordering invariant: earlier condition False ⇒ later conditions NOT True", prop.ForAll(
		func(input phaseOrderInput) (bool, error) {
			condStatuses, err := reconcileSkillWithFailure(ctx, input)
			if err != nil {
				return false, fmt.Errorf("reconcile error: %w", err)
			}

			// Find the first condition that is False.
			failedIdx := -1
			for i, condType := range conditionOrder {
				if condStatuses[condType] == metav1.ConditionFalse {
					failedIdx = i
					break
				}
			}

			// If no condition is False, the invariant is trivially satisfied.
			if failedIdx == -1 {
				return true, nil
			}

			// Verify: all conditions after the failed one must NOT be True.
			for j := failedIdx + 1; j < len(conditionOrder); j++ {
				laterCond := conditionOrder[j]
				if condStatuses[laterCond] == metav1.ConditionTrue {
					return false, fmt.Errorf(
						"phase ordering violated: %s is False but later condition %s is True (failAt=%d, conditions=%v)",
						conditionOrder[failedIdx], laterCond, input.FailAt, condStatuses,
					)
				}
			}

			return true, nil
		},
		genPhaseOrderInput(),
	))

	properties.TestingRun(t)
}
