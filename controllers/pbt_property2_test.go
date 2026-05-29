//go:build pbt

// Feature: ai-platform, Property 2: Controller Convergence
//
// Generator: spec + time-window stop-condition
// Oracle: after spec stops changing, within a finite number of reconcile
//         iterations, status.observedGeneration == spec.generation
// Property: P2 / Validates: F2, A3.12, E2.1

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

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/agent"
	"github.com/ai-keeper/ai-keeper/controllers/policy"
	"github.com/ai-keeper/ai-keeper/controllers/skill"
)

// maxConvergenceSteps is the upper bound on reconcile iterations allowed
// for a controller to converge (observedGeneration == generation).
const maxConvergenceSteps = 20

// ---------------------------------------------------------------------------
// Convergence check helpers
// ---------------------------------------------------------------------------

// convergeSkill creates a Skill with the given generation and reconciles up to
// maxConvergenceSteps times, returning whether observedGeneration converged.
func convergeSkill(ctx context.Context, input skillGenInput, generation int64) (converged bool, steps int, err error) {
	scheme := pbtScheme()
	sk := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:       input.Name,
			Namespace:  "default",
			Generation: generation,
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   sharedv1alpha1.SemVer(input.Version),
			Stability: input.Stability,
			Interface: skillv1alpha1.SkillInterface{
				Input:  skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: validJSONSchema}},
				Output: skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: validJSONSchema}},
			},
			Implementation: skillv1alpha1.SkillImplementation{
				Type: "function",
				Runtime: &skillv1alpha1.SkillRuntime{
					Engine:     "aip-runtime/v2",
					Entrypoint: "skills.test.run",
					Image:      "ghcr.io/test/skill:1.0.0",
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sk).
		WithStatusSubresource(sk).
		Build()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := clocktest.NewFakeClock(now)

	rec := &skill.SkillReconciler{
		Client:   cl,
		Scheme:   scheme,
		Clock:    fc,
		Resolver: skill.NoopResolver{},
		Registry: skill.NewMemoryRegistry(),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.Name, Namespace: "default"}}

	for i := 1; i <= maxConvergenceSteps; i++ {
		_, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			return false, i, rerr
		}

		final := &skillv1alpha1.Skill{}
		if err = cl.Get(ctx, req.NamespacedName, final); err != nil {
			return false, i, err
		}
		if final.Status.ObservedGeneration == generation {
			return true, i, nil
		}
	}
	return false, maxConvergenceSteps, nil
}

// convergeAgent creates an Agent with the given generation and reconciles up to
// maxConvergenceSteps times, returning whether observedGeneration converged.
func convergeAgent(ctx context.Context, input agentGenInput, generation int64) (converged bool, steps int, err error) {
	scheme := pbtScheme()

	sk := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-skill",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   "1.0.0",
			Stability: sharedv1alpha1.StageExperimental,
		},
		Status: skillv1alpha1.SkillStatus{
			Phase: sharedv1alpha1.PhaseActive,
		},
	}

	ag := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:       input.Name,
			Namespace:  "default",
			Generation: generation,
		},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: "PBT Agent " + input.Name,
			Identity: agentv1alpha1.AgentIdentity{
				ServiceAccount: "default",
			},
			Skills: []agentv1alpha1.AgentSkillBinding{
				{Ref: sharedv1alpha1.ResourceRef("skill://default/test-skill")},
			},
			Runtime: agentv1alpha1.AgentRuntime{
				Pattern: input.Pattern,
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ag, sk).
		WithStatusSubresource(ag, sk).
		Build()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := clocktest.NewFakeClock(now)

	rec := &agent.AgentReconciler{
		Client:            cl,
		Scheme:            scheme,
		Clock:             fc,
		SkillResolver:     agent.NewClusterSkillResolver(cl),
		DeploymentManager: &agent.FakeDeploymentManager{Replicas: 1, ReadyReplicas: 1},
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.Name, Namespace: "default"}}

	for i := 1; i <= maxConvergenceSteps; i++ {
		_, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			return false, i, rerr
		}

		final := &agentv1alpha1.Agent{}
		if err = cl.Get(ctx, req.NamespacedName, final); err != nil {
			return false, i, err
		}
		if final.Status.ObservedGeneration == generation {
			return true, i, nil
		}
	}
	return false, maxConvergenceSteps, nil
}

// convergePolicy creates a Policy with the given generation and reconciles up to
// maxConvergenceSteps times, returning whether observedGeneration converged.
func convergePolicy(ctx context.Context, input policyGenInput, generation int64) (converged bool, steps int, err error) {
	scheme := pbtScheme()

	priority := input.Priority
	pol := &policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       input.Name,
			Namespace:  "default",
			Generation: generation,
		},
		Spec: policyv1alpha1.PolicySpec{
			Effect:   input.Effect,
			Priority: &priority,
			Subject: policyv1alpha1.SubjectSelector{
				AnyOf: []policyv1alpha1.SubjectEntry{
					{Kind: "User"},
				},
			},
			Action: policyv1alpha1.PolicyAction{
				Verbs: []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{
					AnyOf: []policyv1alpha1.ResourceSelector{
						{Kind: "Skill"},
					},
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(pol).
		WithStatusSubresource(pol).
		Build()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := clocktest.NewFakeClock(now)

	pdpClient := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-0"})

	rec := &policy.PolicyReconciler{
		Client:    cl,
		Scheme:    scheme,
		Clock:     fc,
		PDP:       pdpClient,
		Compiler:  policy.NoopCompiler{},
		Conflicts: policy.NoopConflictDetector{},
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.Name, Namespace: "default"}}

	for i := 1; i <= maxConvergenceSteps; i++ {
		_, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			return false, i, rerr
		}

		final := &policyv1alpha1.Policy{}
		if err = cl.Get(ctx, req.NamespacedName, final); err != nil {
			return false, i, err
		}
		if final.Status.ObservedGeneration == generation {
			return true, i, nil
		}
	}
	return false, maxConvergenceSteps, nil
}

// ---------------------------------------------------------------------------
// Generators for convergence test
// ---------------------------------------------------------------------------

// convergenceGenInput adds a generation field to the spec generators
// to simulate varying metadata.generation values (1–100).
type skillConvergenceInput struct {
	Spec       skillGenInput
	Generation int64
}

type agentConvergenceInput struct {
	Spec       agentGenInput
	Generation int64
}

type policyConvergenceInput struct {
	Spec       policyGenInput
	Generation int64
}

func genSkillConvergence() gopter.Gen {
	return gen.Struct(reflect.TypeOf(skillConvergenceInput{}), map[string]gopter.Gen{
		"Spec":       genSkillSpec(),
		"Generation": gen.Int64Range(1, 100),
	})
}

func genAgentConvergence() gopter.Gen {
	return gen.Struct(reflect.TypeOf(agentConvergenceInput{}), map[string]gopter.Gen{
		"Spec":       genAgentSpec(),
		"Generation": gen.Int64Range(1, 100),
	})
}

func genPolicyConvergence() gopter.Gen {
	return gen.Struct(reflect.TypeOf(policyConvergenceInput{}), map[string]gopter.Gen{
		"Spec":       genPolicySpec(),
		"Generation": gen.Int64Range(1, 100),
	})
}

// ---------------------------------------------------------------------------
// TestProperty2 — Controller Convergence
//
// **Validates: Requirements F2, A3.12, E2.1**
//
// For each controller (Skill, Agent, Policy), verifies that after the spec
// stops changing, within a finite number of reconcile iterations (≤20),
// status.observedGeneration equals metadata.generation.
// ---------------------------------------------------------------------------

func TestProperty2(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	ctx := context.Background()

	// Property 2a: Skill controller converges
	properties.Property("Skill converges: observedGeneration == generation within finite steps", prop.ForAll(
		func(input skillConvergenceInput) (bool, error) {
			converged, steps, err := convergeSkill(ctx, input.Spec, input.Generation)
			if err != nil {
				return false, fmt.Errorf("reconcile error at step %d: %w", steps, err)
			}
			if !converged {
				return false, fmt.Errorf("did not converge after %d steps (generation=%d)", maxConvergenceSteps, input.Generation)
			}
			return true, nil
		},
		genSkillConvergence(),
	))

	// Property 2b: Agent controller converges
	properties.Property("Agent converges: observedGeneration == generation within finite steps", prop.ForAll(
		func(input agentConvergenceInput) (bool, error) {
			converged, steps, err := convergeAgent(ctx, input.Spec, input.Generation)
			if err != nil {
				return false, fmt.Errorf("reconcile error at step %d: %w", steps, err)
			}
			if !converged {
				return false, fmt.Errorf("did not converge after %d steps (generation=%d)", maxConvergenceSteps, input.Generation)
			}
			return true, nil
		},
		genAgentConvergence(),
	))

	// Property 2c: Policy controller converges
	properties.Property("Policy converges: observedGeneration == generation within finite steps", prop.ForAll(
		func(input policyConvergenceInput) (bool, error) {
			converged, steps, err := convergePolicy(ctx, input.Spec, input.Generation)
			if err != nil {
				return false, fmt.Errorf("reconcile error at step %d: %w", steps, err)
			}
			if !converged {
				return false, fmt.Errorf("did not converge after %d steps (generation=%d)", maxConvergenceSteps, input.Generation)
			}
			return true, nil
		},
		genPolicyConvergence(),
	))

	properties.TestingRun(t)
}
