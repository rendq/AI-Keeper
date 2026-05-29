//go:build pbt

// Feature: ai-platform, Property 1: Controller Idempotency
//
// Generator: Random valid Skill / Agent / Policy specs
// Oracle: reconcile(reconcile(spec)).status == reconcile(spec).status, no cumulative side effects
// Property: P1 / Validates: F1, A3.13, A4 全, A5 全

package controllers_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func pbtScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = skillv1alpha1.AddToScheme(s)
	_ = agentv1alpha1.AddToScheme(s)
	_ = policyv1alpha1.AddToScheme(s)
	return s
}

func pbtSeed() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// statusConditionsMap extracts conditions into a comparable map.
func statusConditionsMap(conditions []metav1.Condition) map[string]string {
	m := make(map[string]string, len(conditions))
	for _, c := range conditions {
		m[c.Type] = fmt.Sprintf("%s/%s/%s", c.Status, c.Reason, c.Message)
	}
	return m
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

var validJSONSchema = []byte(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`)

// genDNSName generates valid DNS-1123 subdomain names (lowercase alpha + digits, 1-63 chars).
func genDNSName() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9]{0,15}")
}

func genSkillSpec() gopter.Gen {
	stabilities := []sharedv1alpha1.Stage{
		sharedv1alpha1.StageExperimental,
		sharedv1alpha1.StageBeta,
		sharedv1alpha1.StageStable,
	}

	return gen.Struct(reflect.TypeOf(skillGenInput{}), map[string]gopter.Gen{
		"Name":      genDNSName(),
		"Version":   genSemVer(),
		"Stability": gen.OneConstOf(stabilities[0], stabilities[1], stabilities[2]),
	})
}

type skillGenInput struct {
	Name      string
	Version   string
	Stability sharedv1alpha1.Stage
}

func genSemVer() gopter.Gen {
	return gen.UInt8Range(0, 9).FlatMap(func(major interface{}) gopter.Gen {
		return gen.UInt8Range(0, 9).FlatMap(func(minor interface{}) gopter.Gen {
			return gen.UInt8Range(0, 9).Map(func(patch uint8) string {
				return fmt.Sprintf("%d.%d.%d", major.(uint8), minor.(uint8), patch)
			})
		}, reflect.TypeOf(""))
	}, reflect.TypeOf(""))
}

func genAgentSpec() gopter.Gen {
	patterns := []string{"react", "tool_calling"}
	return gen.Struct(reflect.TypeOf(agentGenInput{}), map[string]gopter.Gen{
		"Name":    genDNSName(),
		"Pattern": gen.OneConstOf(patterns[0], patterns[1]),
	})
}

type agentGenInput struct {
	Name    string
	Pattern string
}

func genPolicySpec() gopter.Gen {
	effects := []string{"allow", "deny"}
	return gen.Struct(reflect.TypeOf(policyGenInput{}), map[string]gopter.Gen{
		"Name":     genDNSName(),
		"Effect":   gen.OneConstOf(effects[0], effects[1]),
		"Priority": gen.Int32Range(0, 1000),
	})
}

type policyGenInput struct {
	Name     string
	Effect   string
	Priority int32
}

// ---------------------------------------------------------------------------
// Reconcile wrappers (simulate single reconcile pass and return status)
// ---------------------------------------------------------------------------

func reconcileSkill(ctx context.Context, input skillGenInput) (sharedv1alpha1.Phase, map[string]string, error) {
	scheme := pbtScheme()
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

	// Run reconcile (may need multiple passes to stabilize, e.g. finalizer add then requeue)
	for i := 0; i < 5; i++ {
		res, err := rec.Reconcile(ctx, req)
		if err != nil {
			return "", nil, err
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		// For Requeue cases with RequeueAfter > 0 that indicate steady state, break.
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	// Read back the final state
	final := &skillv1alpha1.Skill{}
	if err := cl.Get(ctx, req.NamespacedName, final); err != nil {
		return "", nil, err
	}
	return final.Status.Phase, statusConditionsMap(final.Status.Conditions), nil
}

func reconcileAgent(ctx context.Context, input agentGenInput) (sharedv1alpha1.Phase, map[string]string, error) {
	scheme := pbtScheme()

	// Create a matching skill so the agent can resolve its bindings
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
			Generation: 1,
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

	for i := 0; i < 5; i++ {
		res, err := rec.Reconcile(ctx, req)
		if err != nil {
			return "", nil, err
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	final := &agentv1alpha1.Agent{}
	if err := cl.Get(ctx, req.NamespacedName, final); err != nil {
		return "", nil, err
	}
	return final.Status.Phase, statusConditionsMap(final.Status.Conditions), nil
}

func reconcilePolicy(ctx context.Context, input policyGenInput) (sharedv1alpha1.Phase, map[string]string, error) {
	scheme := pbtScheme()

	priority := input.Priority
	pol := &policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       input.Name,
			Namespace:  "default",
			Generation: 1,
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

	for i := 0; i < 5; i++ {
		res, err := rec.Reconcile(ctx, req)
		if err != nil {
			return "", nil, err
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	final := &policyv1alpha1.Policy{}
	if err := cl.Get(ctx, req.NamespacedName, final); err != nil {
		return "", nil, err
	}
	return final.Status.Phase, statusConditionsMap(final.Status.Conditions), nil
}

// reconcileSkillTwice performs two full reconcile runs on the same object and
// returns both statuses for comparison.
func reconcileSkillTwice(ctx context.Context, input skillGenInput) (phase1, phase2 sharedv1alpha1.Phase, conds1, conds2 map[string]string, err error) {
	scheme := pbtScheme()
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

	// First reconcile run (stabilize)
	for i := 0; i < 5; i++ {
		res, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			return "", "", nil, nil, rerr
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	s1 := &skillv1alpha1.Skill{}
	if err = cl.Get(ctx, req.NamespacedName, s1); err != nil {
		return
	}
	phase1 = s1.Status.Phase
	conds1 = statusConditionsMap(s1.Status.Conditions)

	// Second reconcile run (should be idempotent)
	for i := 0; i < 5; i++ {
		res, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			err = rerr
			return
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	s2 := &skillv1alpha1.Skill{}
	if err = cl.Get(ctx, req.NamespacedName, s2); err != nil {
		return
	}
	phase2 = s2.Status.Phase
	conds2 = statusConditionsMap(s2.Status.Conditions)
	return
}

func reconcileAgentTwice(ctx context.Context, input agentGenInput) (phase1, phase2 sharedv1alpha1.Phase, conds1, conds2 map[string]string, err error) {
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
			Generation: 1,
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

	for i := 0; i < 5; i++ {
		res, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			err = rerr
			return
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	a1 := &agentv1alpha1.Agent{}
	if err = cl.Get(ctx, req.NamespacedName, a1); err != nil {
		return
	}
	phase1 = a1.Status.Phase
	conds1 = statusConditionsMap(a1.Status.Conditions)

	for i := 0; i < 5; i++ {
		res, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			err = rerr
			return
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	a2 := &agentv1alpha1.Agent{}
	if err = cl.Get(ctx, req.NamespacedName, a2); err != nil {
		return
	}
	phase2 = a2.Status.Phase
	conds2 = statusConditionsMap(a2.Status.Conditions)
	return
}

func reconcilePolicyTwice(ctx context.Context, input policyGenInput) (phase1, phase2 sharedv1alpha1.Phase, conds1, conds2 map[string]string, err error) {
	scheme := pbtScheme()

	priority := input.Priority
	pol := &policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       input.Name,
			Namespace:  "default",
			Generation: 1,
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

	for i := 0; i < 5; i++ {
		res, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			err = rerr
			return
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	p1 := &policyv1alpha1.Policy{}
	if err = cl.Get(ctx, req.NamespacedName, p1); err != nil {
		return
	}
	phase1 = p1.Status.Phase
	conds1 = statusConditionsMap(p1.Status.Conditions)

	for i := 0; i < 5; i++ {
		res, rerr := rec.Reconcile(ctx, req)
		if rerr != nil {
			err = rerr
			return
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
		if res.RequeueAfter > 0 && !res.Requeue {
			break
		}
	}

	p2 := &policyv1alpha1.Policy{}
	if err = cl.Get(ctx, req.NamespacedName, p2); err != nil {
		return
	}
	phase2 = p2.Status.Phase
	conds2 = statusConditionsMap(p2.Status.Conditions)
	return
}

// ---------------------------------------------------------------------------
// TestProperty1 — Controller Idempotency
//
// **Validates: Requirements F1, A3.13, A4 全, A5 全**
//
// For each controller (Skill, Agent, Policy), verifies that running
// reconcile twice on the same spec produces identical status output.
// This ensures no cumulative side effects from repeated reconciliation.
// ---------------------------------------------------------------------------

func TestProperty1(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	ctx := context.Background()

	// Property 1a: Skill controller idempotency
	properties.Property("Skill reconcile is idempotent: reconcile(reconcile(spec)).status == reconcile(spec).status", prop.ForAll(
		func(input skillGenInput) (bool, error) {
			phase1, phase2, conds1, conds2, err := reconcileSkillTwice(ctx, input)
			if err != nil {
				return false, fmt.Errorf("reconcile error: %w", err)
			}
			if phase1 != phase2 {
				return false, fmt.Errorf("phase mismatch: first=%s second=%s", phase1, phase2)
			}
			if !reflect.DeepEqual(conds1, conds2) {
				return false, fmt.Errorf("conditions mismatch:\n  first=%v\n  second=%v", conds1, conds2)
			}
			return true, nil
		},
		genSkillSpec(),
	))

	// Property 1b: Agent controller idempotency
	properties.Property("Agent reconcile is idempotent: reconcile(reconcile(spec)).status == reconcile(spec).status", prop.ForAll(
		func(input agentGenInput) (bool, error) {
			phase1, phase2, conds1, conds2, err := reconcileAgentTwice(ctx, input)
			if err != nil {
				return false, fmt.Errorf("reconcile error: %w", err)
			}
			if phase1 != phase2 {
				return false, fmt.Errorf("phase mismatch: first=%s second=%s", phase1, phase2)
			}
			if !reflect.DeepEqual(conds1, conds2) {
				return false, fmt.Errorf("conditions mismatch:\n  first=%v\n  second=%v", conds1, conds2)
			}
			return true, nil
		},
		genAgentSpec(),
	))

	// Property 1c: Policy controller idempotency
	properties.Property("Policy reconcile is idempotent: reconcile(reconcile(spec)).status == reconcile(spec).status", prop.ForAll(
		func(input policyGenInput) (bool, error) {
			phase1, phase2, conds1, conds2, err := reconcilePolicyTwice(ctx, input)
			if err != nil {
				return false, fmt.Errorf("reconcile error: %w", err)
			}
			if phase1 != phase2 {
				return false, fmt.Errorf("phase mismatch: first=%s second=%s", phase1, phase2)
			}
			if !reflect.DeepEqual(conds1, conds2) {
				return false, fmt.Errorf("conditions mismatch:\n  first=%v\n  second=%v", conds1, conds2)
			}
			return true, nil
		},
		genPolicySpec(),
	))

	properties.TestingRun(t)
}
