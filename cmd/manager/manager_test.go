package main

import (
	"bytes"
	"context"
	"os"
	"sort"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	agentctrl "github.com/ai-keeper/ai-keeper/controllers/agent"
	skillctrl "github.com/ai-keeper/ai-keeper/controllers/skill"
)

// TestInformerWiring verifies the cross-controller informer graph
// task 3.6 wires up. Because envtest binaries are not guaranteed to be
// installed in the developer / CI sandbox, the test exercises the
// mapping functions and predicates registered by SetupWithManager
// directly. The semantic guarantees are:
//
//  1. A Skill change enqueues every Agent that references it
//     (Requirements A6.1, A6.2).
//  2. The status-only predicate suppresses spec-only churn so unrelated
//     edits don't flood the Agent workqueue.
//  3. An Agent change enqueues every Skill it references so the
//     Skill controller can recompute `status.referencingAgents`
//     (Requirement A6.4).
//  4. The Skill reconciler's recompute pass derives
//     `status.referencingAgents` from live Agent objects, sorted
//     deterministically.
//  5. The Policy controller deliberately does NOT enqueue Agents on
//     spec changes (Requirement A6.3) — this is expressed by an
//     architectural assertion, not a runtime mapping.
//
// The cmd/manager target in `Verification:` runs this single test.
func TestInformerWiring(t *testing.T) {
	t.Parallel()

	t.Run("SkillChange_EnqueuesReferencingAgents", testSkillChangeEnqueuesAgents)
	t.Run("SkillStatusPredicate_FiltersSpecChurn", testSkillStatusPredicateFiltersSpecChurn)
	t.Run("AgentChange_EnqueuesReferencedSkills", testAgentChangeEnqueuesSkills)
	t.Run("SkillReconciler_DerivesReferencingAgents", testSkillReconcilerDerivesReferencingAgents)
	t.Run("PolicyController_DoesNotWatchAgents", testPolicyDoesNotWatchAgents)
}

// testSkillChangeEnqueuesAgents asserts the mapping function returns
// reconcile.Requests for every Agent in the Skill's namespace whose
// `spec.skills[].ref` resolves to the Skill (Requirements A6.1, A6.2).
func testSkillChangeEnqueuesAgents(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		newAgent("legal-copilot", "default", "skill://default/contract-review"),
		newAgent("scoring-bot", "default", "skill://default/contract-review@2.0.0"),
		newAgent("unrelated", "default", "skill://default/other"),
		newAgent("disabled-binding", "default", ""), // disabled binding below
	)

	// Disable the binding on `disabled-binding` so it should NOT be
	// enqueued.
	disabled := false
	dba := &agentv1alpha1.Agent{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "disabled-binding"}, dba); err != nil {
		t.Fatalf("get disabled-binding: %v", err)
	}
	dba.Spec.Skills = []agentv1alpha1.AgentSkillBinding{
		{
			Ref:     sharedv1alpha1.ResourceRef("skill://default/contract-review"),
			Enabled: &disabled,
		},
	}
	if err := c.Update(context.Background(), dba); err != nil {
		t.Fatalf("update disabled-binding: %v", err)
	}

	mapper := agentctrl.EnqueueAgentsForSkill(c)
	skill := newSkill("contract-review", "default")
	got := mapper(context.Background(), skill)

	want := []types.NamespacedName{
		{Namespace: "default", Name: "legal-copilot"},
		{Namespace: "default", Name: "scoring-bot"},
	}
	assertRequestsEqual(t, got, want)
}

// testSkillStatusPredicateFiltersSpecChurn exercises Requirement A6.1
// (status flips trigger Agent reconciles) and the negative case from
// design.md §6.4: spec-only edits MUST NOT flood the Agent workqueue.
func testSkillStatusPredicateFiltersSpecChurn(t *testing.T) {
	t.Parallel()
	pred := agentctrl.SkillStatusChangedPredicate()

	base := newSkill("contract-review", "default")
	base.Spec.Version = "1.0.0"
	base.Status.Phase = sharedv1alpha1.PhaseActive
	base.Status.Conditions = []metav1.Condition{
		{Type: skillv1alpha1.SkillReady, Status: metav1.ConditionTrue, Reason: "Ready"},
	}

	// Status flip: SkillReady True → False should fire.
	flipped := base.DeepCopy()
	flipped.Status.Conditions = []metav1.Condition{
		{Type: skillv1alpha1.SkillReady, Status: metav1.ConditionFalse, Reason: "NotReady"},
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: flipped}) {
		t.Fatalf("status flip should trigger Agent reconcile")
	}

	// Phase flip: Active → Deprecated should fire.
	deprecated := base.DeepCopy()
	deprecated.Status.Phase = sharedv1alpha1.PhaseDeprecated
	deprecated.Status.Conditions = append([]metav1.Condition{},
		metav1.Condition{Type: skillv1alpha1.SkillDeprecating, Status: metav1.ConditionTrue, Reason: "Deprecated"},
		metav1.Condition{Type: skillv1alpha1.SkillReady, Status: metav1.ConditionTrue, Reason: "Ready"},
	)
	if !pred.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: deprecated}) {
		t.Fatalf("Deprecating True should trigger Agent reconcile")
	}

	// Spec annotation churn (no status change, no version change)
	// should NOT fire — Requirement A6.3 mirror: avoid unrelated
	// re-enqueues.
	specChurn := base.DeepCopy()
	specChurn.Annotations = map[string]string{"unrelated": "yes"}
	if pred.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: specChurn}) {
		t.Fatalf("annotation-only change should NOT trigger Agent reconcile")
	}

	// Version bump should fire (Agent resolver re-picks the highest).
	bumped := base.DeepCopy()
	bumped.Spec.Version = "1.0.1"
	if !pred.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: bumped}) {
		t.Fatalf("version bump should trigger Agent reconcile")
	}

	// Create / Delete events always fire.
	if !pred.Create(event.CreateEvent{Object: base}) {
		t.Fatalf("create event should fire predicate")
	}
	if !pred.Delete(event.DeleteEvent{Object: base}) {
		t.Fatalf("delete event should fire predicate")
	}
}

// testAgentChangeEnqueuesSkills exercises Requirement A6.4: every
// Agent create / update / delete enqueues the referenced Skills so the
// Skill controller can recompute `status.referencingAgents`.
func testAgentChangeEnqueuesSkills(t *testing.T) {
	t.Parallel()
	mapper := skillctrl.EnqueueSkillsForAgentBindings()

	a := newAgent("legal-copilot", "default", "skill://default/contract-review")
	a.Spec.Skills = append(a.Spec.Skills, agentv1alpha1.AgentSkillBinding{
		Ref: sharedv1alpha1.ResourceRef("skill://default/contract-review@2.0.0"), // duplicate name
	})
	a.Spec.Skills = append(a.Spec.Skills, agentv1alpha1.AgentSkillBinding{
		Ref: sharedv1alpha1.ResourceRef("skill://default/translate"),
	})
	disabled := false
	a.Spec.Skills = append(a.Spec.Skills, agentv1alpha1.AgentSkillBinding{
		Ref:     sharedv1alpha1.ResourceRef("skill://default/disabled-skill"),
		Enabled: &disabled,
	})

	got := mapper(context.Background(), a)
	want := []types.NamespacedName{
		{Namespace: "default", Name: "contract-review"},
		{Namespace: "default", Name: "translate"},
		{Namespace: "default", Name: "disabled-skill"},
	}
	assertRequestsEqual(t, got, want)

	// Non-Agent objects must produce nil, not a panic.
	if got := mapper(context.Background(), newSkill("foo", "default")); got != nil {
		t.Fatalf("non-Agent object should yield nil, got %v", got)
	}

	// A bare Agent with no bindings must yield no requests.
	bare := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "bare", Namespace: "default"},
	}
	if got := mapper(context.Background(), bare); len(got) != 0 {
		t.Fatalf("bare agent should yield no requests, got %v", got)
	}
}

// testSkillReconcilerDerivesReferencingAgents validates the Skill
// reconciler's recompute pass: given two Agents referencing one Skill,
// the next reconcile populates `status.referencingAgents` with the
// sorted list of Agent FQNs (Requirement A6.4).
func testSkillReconcilerDerivesReferencingAgents(t *testing.T) {
	t.Parallel()
	a1 := newAgent("agent-zeta", "default", "skill://default/contract-review")
	a2 := newAgent("agent-alpha", "default", "skill://default/contract-review")
	skill := newReadySkill("contract-review", "default")

	c := newFakeClient(t, skill, a1, a2)
	r := &skillctrl.SkillReconciler{
		Client:   c,
		Scheme:   mustScheme(t),
		Resolver: skillctrl.NoopResolver{},
		Registry: skillctrl.NewMemoryRegistry(),
	}

	key := types.NamespacedName{Namespace: "default", Name: "contract-review"}
	for i := 0; i < 4; i++ {
		res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
		if err != nil {
			t.Fatalf("Reconcile pass %d: %v", i, err)
		}
		if !res.Requeue {
			break
		}
	}

	got := &skillv1alpha1.Skill{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get skill: %v", err)
	}
	want := []string{"default/agent-alpha", "default/agent-zeta"}
	if len(got.Status.ReferencingAgents) != len(want) {
		t.Fatalf("referencingAgents = %v, want %v", got.Status.ReferencingAgents, want)
	}
	for i, fqn := range want {
		if got.Status.ReferencingAgents[i] != fqn {
			t.Fatalf("referencingAgents[%d] = %q, want %q", i, got.Status.ReferencingAgents[i], fqn)
		}
	}
}

// testPolicyDoesNotWatchAgents documents Requirement A6.3 at the test
// level: the Policy controller's SetupWithManager hook must NOT watch
// the Agent kind. controller-runtime does not expose a public API to
// list registered watches, so we instead assert by reflection over the
// production source of truth — controllers/policy/reconciler.go — that
// no `Watches(&agentv1alpha1.Agent{}, ...)` call is wired in.
//
// This keeps the contract enforceable without requiring envtest while
// still failing loudly if a future change accidentally adds the watch.
func testPolicyDoesNotWatchAgents(t *testing.T) {
	t.Parallel()
	policySrc, err := os.ReadFile("../../controllers/policy/reconciler.go")
	if err != nil {
		t.Fatalf("read policy reconciler source: %v", err)
	}
	if bytes.Contains(policySrc, []byte("agentv1alpha1")) {
		t.Fatalf("policy reconciler imports agentv1alpha1; Requirement A6.3 forbids the Policy controller from watching Agents")
	}
	if bytes.Contains(policySrc, []byte("Watches(")) {
		// The Policy controller is allowed to grow its own Watches in
		// the future — for example, a Tenant or PDP CRD — but it must
		// never enqueue Agents. Surface the line for manual review so
		// the test fails fast on accidental wiring.
		for _, line := range bytes.Split(policySrc, []byte("\n")) {
			if bytes.Contains(line, []byte("Watches(")) && bytes.Contains(line, []byte("Agent")) {
				t.Fatalf("policy reconciler appears to register an Agent watch: %q", line)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newAgent(name, namespace, skillRef string) *agentv1alpha1.Agent {
	a := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: name,
			Identity:    agentv1alpha1.AgentIdentity{ServiceAccount: "default"},
			Runtime:     agentv1alpha1.AgentRuntime{Pattern: "tool_calling"},
		},
	}
	if skillRef != "" {
		a.Spec.Skills = []agentv1alpha1.AgentSkillBinding{
			{Ref: sharedv1alpha1.ResourceRef(skillRef)},
		}
	}
	return a
}

func newSkill(name, namespace string) *skillv1alpha1.Skill {
	return &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   sharedv1alpha1.SemVer("1.0.0"),
			Stability: sharedv1alpha1.StageStable,
		},
	}
}

func newReadySkill(name, namespace string) *skillv1alpha1.Skill {
	s := newSkill(name, namespace)
	s.Generation = 1
	s.Status.Phase = sharedv1alpha1.PhaseActive
	return s
}

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := skillv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register skill scheme: %v", err)
	}
	if err := agentv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register agent scheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(mustScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&skillv1alpha1.Skill{}, &agentv1alpha1.Agent{}).
		Build()
}

func assertRequestsEqual(t *testing.T, got []reconcile.Request, want []types.NamespacedName) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("request count = %d, want %d (got %v, want %v)", len(got), len(want), got, want)
	}
	gotKeys := make([]string, 0, len(got))
	for _, r := range got {
		gotKeys = append(gotKeys, r.NamespacedName.String())
	}
	wantKeys := make([]string, 0, len(want))
	for _, w := range want {
		wantKeys = append(wantKeys, w.String())
	}
	sort.Strings(gotKeys)
	sort.Strings(wantKeys)
	for i := range gotKeys {
		if gotKeys[i] != wantKeys[i] {
			t.Fatalf("request[%d] = %q, want %q (got %v, want %v)", i, gotKeys[i], wantKeys[i], gotKeys, wantKeys)
		}
	}
}
