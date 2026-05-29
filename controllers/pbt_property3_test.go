//go:build pbt

// Feature: ai-platform, Property 3: Deletion Safety + Drain Ordering
//
// Generator: Random (Skill, Agent[]) reference graphs
// Oracle:
//   - |referencingAgents| > 0 ⇒ Skill deletion is blocked by finalizer
//   - Agent drain must complete before finalizer is removed
//   - All audit events must be persisted before finalizer removal
// Property: P3 / Validates: F3, A3.11, A4.7, A4.8

package controllers_test

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/agent"
	"github.com/ai-keeper/ai-keeper/controllers/skill"
)

// ---------------------------------------------------------------------------
// Generators for Property 3
// ---------------------------------------------------------------------------

// refGraphInput represents a random reference graph: one Skill referenced
// by N agents.
type refGraphInput struct {
	SkillName  string
	AgentNames []string
}

// genRefGraph generates a random reference graph with 0–5 agents referencing
// a single skill. The skill and agent names are valid DNS-1123 names.
// Agent names are deduplicated to avoid fake-client panics.
func genRefGraph() gopter.Gen {
	return gen.IntRange(0, 5).FlatMap(func(n interface{}) gopter.Gen {
		numAgents := n.(int)
		return gen.Struct(reflect.TypeOf(refGraphInput{}), map[string]gopter.Gen{
			"SkillName":  genDNSName(),
			"AgentNames": genUniqueAgentNames(numAgents),
		})
	}, reflect.TypeOf(refGraphInput{}))
}

// genUniqueAgentNames generates a slice of n unique DNS names by
// appending a suffix index to each generated name.
func genUniqueAgentNames(n int) gopter.Gen {
	if n == 0 {
		return gen.Const([]string{})
	}
	return gen.SliceOfN(n, genDNSName()).Map(func(names []string) []string {
		seen := make(map[string]bool, len(names))
		unique := make([]string, 0, len(names))
		for i, name := range names {
			candidate := fmt.Sprintf("%s-%d", name, i)
			if !seen[candidate] {
				seen[candidate] = true
				unique = append(unique, candidate)
			}
		}
		return unique
	})
}

// ---------------------------------------------------------------------------
// Controllable fakes for drain ordering verification
// ---------------------------------------------------------------------------

// orderTrackingAuditFlusher records the order of Flush calls relative to
// other drain steps. It allows tests to verify audit persistence happens
// before finalizer removal.
type orderTrackingAuditFlusher struct {
	flushed  atomic.Bool
	flushErr error
}

func (f *orderTrackingAuditFlusher) Flush(_ context.Context, _ *agentv1alpha1.Agent) error {
	if f.flushErr != nil {
		return f.flushErr
	}
	f.flushed.Store(true)
	return nil
}

// blockingSessionTracker returns a configurable in-flight count so we can
// test that the finalizer is NOT removed while sessions are draining.
type blockingSessionTracker struct {
	inflight int
}

func (t *blockingSessionTracker) InFlight(_ context.Context, _ *agentv1alpha1.Agent) (int, error) {
	return t.inflight, nil
}

// ---------------------------------------------------------------------------
// TestProperty3 — Deletion Safety + Drain Ordering
//
// **Validates: Requirements F3, A3.11, A4.7, A4.8**
//
// Sub-properties:
//   3a: Skill with referencingAgents > 0 keeps finalizer on deletion
//   3b: Agent drain ordering — finalizer not removed until drain completes
//   3c: Audit events must be persisted before Agent finalizer removal
// ---------------------------------------------------------------------------

func TestProperty3(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	ctx := context.Background()

	// Property 3a: Skill deletion blocked when referencingAgents > 0
	properties.Property("Skill with referencingAgents > 0: deletion blocked by finalizer (A3.11)", prop.ForAll(
		func(input refGraphInput) (bool, error) {
			if len(input.AgentNames) == 0 {
				// No agents referencing → finalizer SHOULD be removed.
				return verifySkillDeletionAllowed(ctx, input)
			}
			// Agents referencing → finalizer MUST remain.
			return verifySkillDeletionBlocked(ctx, input)
		},
		genRefGraph(),
	))

	// Property 3b: Agent finalizer not removed while sessions are in-flight
	properties.Property("Agent finalizer not removed until drain completes (A4.7)", prop.ForAll(
		func(input agentGenInput) (bool, error) {
			return verifyAgentDrainOrdering(ctx, input)
		},
		genAgentSpec(),
	))

	// Property 3c: Audit events must persist before Agent finalizer removal
	properties.Property("Audit events must be persisted before Agent finalizer removal (A4.8)", prop.ForAll(
		func(input agentGenInput) (bool, error) {
			return verifyAuditBeforeFinalizer(ctx, input)
		},
		genAgentSpec(),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Property 3a verification
// ---------------------------------------------------------------------------

// verifySkillDeletionBlocked checks that when agents reference a skill,
// setting deletionTimestamp does NOT remove the finalizer.
func verifySkillDeletionBlocked(ctx context.Context, input refGraphInput) (bool, error) {
	scheme := pbtScheme()

	// Build the skill with finalizer and deletionTimestamp set.
	now := metav1.Now()
	sk := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:              input.SkillName,
			Namespace:         "default",
			Generation:        1,
			DeletionTimestamp: &now,
			Finalizers:        []string{skill.FinalizerSkillProtect},
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   "1.0.0",
			Stability: sharedv1alpha1.StageStable,
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

	// Build agents that reference the skill.
	agents := make([]*agentv1alpha1.Agent, 0, len(input.AgentNames))
	for _, name := range input.AgentNames {
		ag := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Generation: 1,
			},
			Spec: agentv1alpha1.AgentSpec{
				DisplayName: "Agent " + name,
				Identity:    agentv1alpha1.AgentIdentity{ServiceAccount: "default"},
				Skills: []agentv1alpha1.AgentSkillBinding{
					{Ref: sharedv1alpha1.ResourceRef(fmt.Sprintf("skill://default/%s", input.SkillName))},
				},
				Runtime: agentv1alpha1.AgentRuntime{Pattern: "react"},
			},
		}
		agents = append(agents, ag)
	}

	// Build fake client with both skill and agents.
	builder := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sk).
		WithStatusSubresource(sk)
	for _, ag := range agents {
		builder = builder.WithObjects(ag)
	}
	cl := builder.Build()

	fc := clocktest.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	rec := &skill.SkillReconciler{
		Client:   cl,
		Scheme:   scheme,
		Clock:    fc,
		Resolver: skill.NoopResolver{},
		Registry: skill.NewMemoryRegistry(),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.SkillName, Namespace: "default"}}

	// Run reconcile — should NOT remove finalizer.
	_, err := rec.Reconcile(ctx, req)
	if err != nil {
		return false, fmt.Errorf("reconcile error: %w", err)
	}

	// Read back the skill.
	final := &skillv1alpha1.Skill{}
	if err := cl.Get(ctx, req.NamespacedName, final); err != nil {
		return false, fmt.Errorf("get skill: %w", err)
	}

	// Finalizer MUST still be present.
	if !controllerutil.ContainsFinalizer(final, skill.FinalizerSkillProtect) {
		return false, fmt.Errorf("finalizer removed despite %d referencing agents", len(input.AgentNames))
	}

	// Phase must be Terminating.
	if final.Status.Phase != sharedv1alpha1.PhaseTerminating {
		return false, fmt.Errorf("phase = %s, want Terminating", final.Status.Phase)
	}

	return true, nil
}

// verifySkillDeletionAllowed checks that when NO agents reference a skill,
// the finalizer IS removed on deletion.
func verifySkillDeletionAllowed(ctx context.Context, input refGraphInput) (bool, error) {
	scheme := pbtScheme()

	now := metav1.Now()
	sk := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:              input.SkillName,
			Namespace:         "default",
			Generation:        1,
			DeletionTimestamp: &now,
			Finalizers:        []string{skill.FinalizerSkillProtect},
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   "1.0.0",
			Stability: sharedv1alpha1.StageStable,
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

	fc := clocktest.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	rec := &skill.SkillReconciler{
		Client:   cl,
		Scheme:   scheme,
		Clock:    fc,
		Resolver: skill.NoopResolver{},
		Registry: skill.NewMemoryRegistry(),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.SkillName, Namespace: "default"}}

	_, err := rec.Reconcile(ctx, req)
	if err != nil {
		return false, fmt.Errorf("reconcile error: %w", err)
	}

	final := &skillv1alpha1.Skill{}
	if err := cl.Get(ctx, req.NamespacedName, final); err != nil {
		// Object may have been garbage-collected (finalizer removed →
		// API server deletes). That's fine — it means deletion succeeded.
		return true, nil
	}

	// Finalizer should be removed since no agents reference it.
	if controllerutil.ContainsFinalizer(final, skill.FinalizerSkillProtect) {
		return false, fmt.Errorf("finalizer still present despite 0 referencing agents")
	}

	return true, nil
}

// ---------------------------------------------------------------------------
// Property 3b verification — Agent drain ordering
// ---------------------------------------------------------------------------

// verifyAgentDrainOrdering checks that the agent finalizer is NOT removed
// while there are still in-flight sessions (drain incomplete).
func verifyAgentDrainOrdering(ctx context.Context, input agentGenInput) (bool, error) {
	scheme := pbtScheme()

	// Create a skill so the agent has a valid binding.
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

	// Create agent with deletionTimestamp and finalizer set.
	now := metav1.Now()
	ag := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              input.Name,
			Namespace:         "default",
			Generation:        1,
			DeletionTimestamp: &now,
			Finalizers:        []string{agent.FinalizerAgentDrain},
		},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: "PBT Agent " + input.Name,
			Identity:    agentv1alpha1.AgentIdentity{ServiceAccount: "default"},
			Skills: []agentv1alpha1.AgentSkillBinding{
				{Ref: sharedv1alpha1.ResourceRef("skill://default/test-skill")},
			},
			Runtime: agentv1alpha1.AgentRuntime{Pattern: input.Pattern},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ag, sk).
		WithStatusSubresource(ag, sk).
		Build()

	fc := clocktest.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	// Use a session tracker that reports in-flight sessions.
	tracker := &blockingSessionTracker{inflight: 3}

	rec := &agent.AgentReconciler{
		Client:            cl,
		Scheme:            scheme,
		Clock:             fc,
		SkillResolver:     agent.NewClusterSkillResolver(cl),
		DeploymentManager: &agent.FakeDeploymentManager{Replicas: 1, ReadyReplicas: 1},
		SessionTracker:    tracker,
		AuditFlusher:      &orderTrackingAuditFlusher{},
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.Name, Namespace: "default"}}

	// Reconcile with in-flight sessions — finalizer must stay.
	_, err := rec.Reconcile(ctx, req)
	if err != nil {
		return false, fmt.Errorf("reconcile error: %w", err)
	}

	got := &agentv1alpha1.Agent{}
	if err := cl.Get(ctx, req.NamespacedName, got); err != nil {
		return false, fmt.Errorf("get agent: %w", err)
	}

	if !controllerutil.ContainsFinalizer(got, agent.FinalizerAgentDrain) {
		return false, fmt.Errorf("finalizer removed while %d sessions still in-flight", tracker.inflight)
	}

	// Now simulate drain complete (0 in-flight) — finalizer should be removed.
	tracker.inflight = 0
	for i := 0; i < 5; i++ {
		res, err := rec.Reconcile(ctx, req)
		if err != nil {
			return false, fmt.Errorf("reconcile after drain error: %w", err)
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
	}

	got2 := &agentv1alpha1.Agent{}
	if err := cl.Get(ctx, req.NamespacedName, got2); err != nil {
		// Object deleted (finalizer removed → GC). That's success.
		return true, nil
	}

	if controllerutil.ContainsFinalizer(got2, agent.FinalizerAgentDrain) {
		return false, fmt.Errorf("finalizer still present after drain completed (0 in-flight)")
	}

	return true, nil
}

// ---------------------------------------------------------------------------
// Property 3c verification — Audit persistence before finalizer removal
// ---------------------------------------------------------------------------

// verifyAuditBeforeFinalizer checks that when audit flush fails, the
// Agent finalizer is NOT removed (audit must persist first).
func verifyAuditBeforeFinalizer(ctx context.Context, input agentGenInput) (bool, error) {
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

	now := metav1.Now()
	ag := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              input.Name,
			Namespace:         "default",
			Generation:        1,
			DeletionTimestamp: &now,
			Finalizers:        []string{agent.FinalizerAgentDrain},
		},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: "PBT Agent " + input.Name,
			Identity:    agentv1alpha1.AgentIdentity{ServiceAccount: "default"},
			Skills: []agentv1alpha1.AgentSkillBinding{
				{Ref: sharedv1alpha1.ResourceRef("skill://default/test-skill")},
			},
			Runtime: agentv1alpha1.AgentRuntime{Pattern: input.Pattern},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ag, sk).
		WithStatusSubresource(ag, sk).
		Build()

	fc := clocktest.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	// Audit flusher that FAILS — simulates persistence not yet complete.
	failingFlusher := &orderTrackingAuditFlusher{
		flushErr: fmt.Errorf("audit sink unavailable"),
	}

	rec := &agent.AgentReconciler{
		Client:            cl,
		Scheme:            scheme,
		Clock:             fc,
		SkillResolver:     agent.NewClusterSkillResolver(cl),
		DeploymentManager: &agent.FakeDeploymentManager{Replicas: 1, ReadyReplicas: 1},
		SessionTracker:    &blockingSessionTracker{inflight: 0}, // drain complete
		AuditFlusher:      failingFlusher,
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: input.Name, Namespace: "default"}}

	// Reconcile — audit flush fails, so finalizer MUST remain.
	_, _ = rec.Reconcile(ctx, req)

	got := &agentv1alpha1.Agent{}
	if err := cl.Get(ctx, req.NamespacedName, got); err != nil {
		return false, fmt.Errorf("get agent: %w", err)
	}

	if !controllerutil.ContainsFinalizer(got, agent.FinalizerAgentDrain) {
		return false, fmt.Errorf("finalizer removed despite audit flush failure — audit events not persisted")
	}

	// Now fix the flusher and reconcile again — finalizer should be removed.
	failingFlusher.flushErr = nil
	for i := 0; i < 5; i++ {
		res, err := rec.Reconcile(ctx, req)
		if err != nil {
			return false, fmt.Errorf("reconcile after audit fix error: %w", err)
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			break
		}
	}

	got2 := &agentv1alpha1.Agent{}
	if err := cl.Get(ctx, req.NamespacedName, got2); err != nil {
		// Deleted — success.
		return true, nil
	}

	if controllerutil.ContainsFinalizer(got2, agent.FinalizerAgentDrain) {
		return false, fmt.Errorf("finalizer still present after audit flush succeeded")
	}

	// Verify audit was indeed flushed.
	if !failingFlusher.flushed.Load() {
		return false, fmt.Errorf("audit was never flushed before finalizer removal")
	}

	return true, nil
}
