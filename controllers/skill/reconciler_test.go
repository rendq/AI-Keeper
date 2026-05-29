package skill_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clocktest "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
	"github.com/ai-keeper/ai-keeper/controllers/skill"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

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

// validInputSchema and validOutputSchema are minimal Draft-7 JSON
// schemas used across the table tests.
var (
	validInputSchema  = []byte(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`)
	validOutputSchema = []byte(`{"type":"object","properties":{"answer":{"type":"string"}}}`)
)

type skillBuilder struct {
	skill *skillv1alpha1.Skill
}

func newSkillBuilder(name string) *skillBuilder {
	return &skillBuilder{
		skill: &skillv1alpha1.Skill{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Generation: 1,
			},
			Spec: skillv1alpha1.SkillSpec{
				Version:   sharedv1alpha1.SemVer("1.0.0"),
				Stability: sharedv1alpha1.StageExperimental,
				Interface: skillv1alpha1.SkillInterface{
					Input:  skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: validInputSchema}},
					Output: skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: validOutputSchema}},
				},
				Implementation: skillv1alpha1.SkillImplementation{
					Type: "function",
					Runtime: &skillv1alpha1.SkillRuntime{
						Engine:     "aip-runtime/v2",
						Entrypoint: "skills.contract.review",
						Image:      "ghcr.io/example/contract:1.0.0",
					},
				},
			},
		},
	}
}

func (b *skillBuilder) withInputSchema(raw []byte) *skillBuilder {
	b.skill.Spec.Interface.Input.Schema = &apiextensionsv1.JSON{Raw: raw}
	return b
}

func (b *skillBuilder) withStability(s sharedv1alpha1.Stage) *skillBuilder {
	b.skill.Spec.Stability = s
	return b
}

func (b *skillBuilder) withDeletionTimestamp(t time.Time) *skillBuilder {
	mt := metav1.NewTime(t)
	b.skill.DeletionTimestamp = &mt
	// Objects with deletionTimestamp must carry a finalizer for the fake
	// client to accept them; set one if absent.
	if !controllerutil.ContainsFinalizer(b.skill, skill.FinalizerSkillProtect) {
		controllerutil.AddFinalizer(b.skill, skill.FinalizerSkillProtect)
	}
	return b
}

func (b *skillBuilder) withReferencingAgents(agents ...string) *skillBuilder {
	b.skill.Status.ReferencingAgents = append(b.skill.Status.ReferencingAgents, agents...)
	return b
}

func (b *skillBuilder) withMissingSince(t time.Time) *skillBuilder {
	if b.skill.Annotations == nil {
		b.skill.Annotations = map[string]string{}
	}
	b.skill.Annotations[skill.MissingSinceAnnotation] = t.UTC().Format(time.RFC3339)
	return b
}

func (b *skillBuilder) build() *skillv1alpha1.Skill { return b.skill }

func newReconciler(t *testing.T, opts reconcilerOpts, objs ...client.Object) (*skill.SkillReconciler, client.Client, *common.NoopBus, *clocktest.FakeClock) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&skillv1alpha1.Skill{}).
		Build()
	bus := common.NewNoopBus(logr.Discard())
	fakeClock := clocktest.NewFakeClock(time.Now())
	r := &skill.SkillReconciler{
		Client:   c,
		Scheme:   s,
		Bus:      bus,
		Registry: skill.NewMemoryRegistry(),
		Resolver: skill.NoopResolver{},
		Clock:    fakeClock,
	}
	if opts.resolver != nil {
		r.Resolver = opts.resolver
	}
	if opts.registry != nil {
		r.Registry = opts.registry
	}
	if opts.clock != nil {
		r.Clock = opts.clock
		fakeClock = opts.clock
	}
	return r, c, bus, fakeClock
}

type reconcilerOpts struct {
	resolver skill.Resolver
	registry skill.Registry
	clock    *clocktest.FakeClock
}

func reconcileOnce(t *testing.T, r *skill.SkillReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

// reconcileToSteady drives Reconcile until it returns a non-Requeue
// result (or fails). The first pass typically only stamps the
// finalizer, so most tests need at least two passes.
func reconcileToSteady(t *testing.T, r *skill.SkillReconciler, key types.NamespacedName, max int) reconcile.Result {
	t.Helper()
	var last reconcile.Result
	for i := 0; i < max; i++ {
		last = reconcileOnce(t, r, key)
		if !last.Requeue {
			return last
		}
	}
	t.Fatalf("Reconcile did not reach steady state after %d passes", max)
	return last
}

func getSkill(t *testing.T, c client.Client, key types.NamespacedName) *skillv1alpha1.Skill {
	t.Helper()
	got := &skillv1alpha1.Skill{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	return got
}

func conditionStatus(t *testing.T, conds []metav1.Condition, condType string) metav1.ConditionStatus {
	t.Helper()
	for _, c := range conds {
		if c.Type == condType {
			return c.Status
		}
	}
	return ""
}

func conditionReason(conds []metav1.Condition, condType string) string {
	for _, c := range conds {
		if c.Type == condType {
			return c.Reason
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestReconcile_HappyPath exercises Requirements A3.1, A3.3, A3.7, A3.13:
// finalizer added → schema validated → resolver succeeds → registry
// populated → Ready=True → phase=Active → 10m steady-state requeue.
func TestReconcile_HappyPath(t *testing.T) {
	t.Parallel()

	sk := newSkillBuilder("contract-review").build()
	r, c, bus, _ := newReconciler(t, reconcilerOpts{}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getSkill(t, c, key)
	if !controllerutil.ContainsFinalizer(got, skill.FinalizerSkillProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillSchemaValid); status != metav1.ConditionTrue {
		t.Fatalf("SchemaValid status = %s, want True", status)
	}
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillDependenciesResolved); status != metav1.ConditionTrue {
		t.Fatalf("DependenciesResolved status = %s, want True", status)
	}
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillImplementationReady); status != metav1.ConditionTrue {
		t.Fatalf("ImplementationReady status = %s, want True", status)
	}
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillRegistered); status != metav1.ConditionTrue {
		t.Fatalf("Registered status = %s, want True", status)
	}
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready status = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
	if last.RequeueAfter != skill.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, skill.SteadyStateRequeue)
	}

	// Bus events: SkillRegistered + SkillPromoted should have been
	// published exactly once during the first run.
	kinds := map[common.DomainEventKind]int{}
	for _, ev := range bus.Events() {
		kinds[ev.Kind]++
	}
	if kinds[common.EventSkillRegistered] != 1 {
		t.Fatalf("SkillRegistered events = %d, want 1; got %v", kinds[common.EventSkillRegistered], kinds)
	}
	if kinds[common.EventSkillPromoted] != 1 {
		t.Fatalf("SkillPromoted events = %d, want 1; got %v", kinds[common.EventSkillPromoted], kinds)
	}
}

// TestReconcile_SchemaInvalid verifies Requirement A3.2: an invalid
// JSON Schema flips SchemaValid=False, sets phase=Failed and does not
// retry.
func TestReconcile_SchemaInvalid(t *testing.T) {
	t.Parallel()

	sk := newSkillBuilder("bad-schema").
		withInputSchema([]byte(`{"type":"banana"}`)).
		build()
	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getSkill(t, c, key)
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillSchemaValid); status != metav1.ConditionFalse {
		t.Fatalf("SchemaValid = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, skillv1alpha1.SkillSchemaValid); reason != skill.ReasonInvalidSchema {
		t.Fatalf("SchemaValid reason = %s, want %s", reason, skill.ReasonInvalidSchema)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("requeue = %s, want 0 (no retry on permanent failure)", last.RequeueAfter)
	}
}

// TestReconcile_MissingDependency_WithinWindow exercises Requirement
// A3.4: a missing reference within the 1-hour window keeps the Skill
// Pending and requests a 30-second requeue.
func TestReconcile_MissingDependency_WithinWindow(t *testing.T) {
	t.Parallel()

	missing := []sharedv1alpha1.ResourceRef{"tool://docusign/sign@1.0.0"}
	resolver := skill.FuncResolver(func(_ context.Context, _ *skillv1alpha1.Skill) (skill.ResolveResult, error) {
		return skill.ResolveResult{Missing: missing}, nil
	})
	sk := newSkillBuilder("waiting-for-tool").build()
	r, c, _, _ := newReconciler(t, reconcilerOpts{resolver: resolver}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getSkill(t, c, key)
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillDependenciesResolved); status != metav1.ConditionFalse {
		t.Fatalf("DependenciesResolved = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, skillv1alpha1.SkillDependenciesResolved); reason != skill.ReasonMissingReference {
		t.Fatalf("reason = %s, want %s", reason, skill.ReasonMissingReference)
	}
	if got.Status.Phase != sharedv1alpha1.PhasePending {
		t.Fatalf("phase = %s, want Pending", got.Status.Phase)
	}
	if last.RequeueAfter != skill.MissingDependencyRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, skill.MissingDependencyRequeue)
	}
	if _, ok := got.Annotations[skill.MissingSinceAnnotation]; !ok {
		t.Fatalf("missing-since annotation not stamped: %v", got.Annotations)
	}
}

// TestReconcile_MissingDependency_PastTTL exercises Requirement A3.5:
// a missing reference that has aged past the 1-hour TTL transitions to
// Failed with reason MissingReferencePermanent, and the controller
// stops retrying.
func TestReconcile_MissingDependency_PastTTL(t *testing.T) {
	t.Parallel()

	missing := []sharedv1alpha1.ResourceRef{"tool://docusign/sign@1.0.0"}
	resolver := skill.FuncResolver(func(_ context.Context, _ *skillv1alpha1.Skill) (skill.ResolveResult, error) {
		return skill.ResolveResult{Missing: missing}, nil
	})
	// Use a fake clock that is 90 minutes ahead of the stamped
	// missing-since annotation so the TTL is exceeded.
	now := time.Now().UTC()
	missingSince := now.Add(-90 * time.Minute)

	sk := newSkillBuilder("dead-tool").
		withMissingSince(missingSince).
		build()
	// Pre-stamp the finalizer so the first pass runs the resolver, not
	// the finalizer add path (which short-circuits with Requeue).
	controllerutil.AddFinalizer(sk, skill.FinalizerSkillProtect)

	fakeClock := clocktest.NewFakeClock(now)
	r, c, _, _ := newReconciler(t, reconcilerOpts{resolver: resolver, clock: fakeClock}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getSkill(t, c, key)
	if reason := conditionReason(got.Status.Conditions, skillv1alpha1.SkillDependenciesResolved); reason != skill.ReasonMissingReferencePermanent {
		t.Fatalf("reason = %s, want %s", reason, skill.ReasonMissingReferencePermanent)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", last.RequeueAfter)
	}
}

// TestReconcile_CyclicDependency exercises Requirement A3.6: a cyclic
// dependency graph is a permanent failure with reason CyclicDependency.
func TestReconcile_CyclicDependency(t *testing.T) {
	t.Parallel()

	resolver := skill.FuncResolver(func(_ context.Context, _ *skillv1alpha1.Skill) (skill.ResolveResult, error) {
		return skill.ResolveResult{Cyclic: true}, nil
	})
	sk := newSkillBuilder("cyclic").build()
	r, c, _, _ := newReconciler(t, reconcilerOpts{resolver: resolver}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getSkill(t, c, key)
	if reason := conditionReason(got.Status.Conditions, skillv1alpha1.SkillDependenciesResolved); reason != skill.ReasonCyclicDependency {
		t.Fatalf("reason = %s, want %s", reason, skill.ReasonCyclicDependency)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", last.RequeueAfter)
	}
}

// TestReconcile_DeletionBlocked exercises Requirement A3.11: deletion
// is blocked while at least one Agent in the cluster references the
// Skill (Requirement A6.4 — the back-pointer is recomputed from live
// state, not seeded by the test).
func TestReconcile_DeletionBlocked(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	sk := newSkillBuilder("deleting").
		withDeletionTimestamp(now).
		build()
	refAgent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-a",
			Namespace: "default",
		},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: "agent-a",
			Identity:    agentv1alpha1.AgentIdentity{ServiceAccount: "default"},
			Skills: []agentv1alpha1.AgentSkillBinding{
				{Ref: sharedv1alpha1.ResourceRef("skill://default/deleting")},
			},
			Runtime: agentv1alpha1.AgentRuntime{Pattern: "tool_calling"},
		},
	}
	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk, refAgent)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	res := reconcileOnce(t, r, key)
	if res.RequeueAfter != time.Minute {
		t.Fatalf("RequeueAfter = %s, want 1m", res.RequeueAfter)
	}

	got := getSkill(t, c, key)
	if !controllerutil.ContainsFinalizer(got, skill.FinalizerSkillProtect) {
		t.Fatal("finalizer should remain while at least one Agent references the Skill")
	}
	if got.Status.Phase != sharedv1alpha1.PhaseTerminating {
		t.Fatalf("phase = %s, want Terminating", got.Status.Phase)
	}
	if want := []string{"default/agent-a"}; !equalStringSlice(got.Status.ReferencingAgents, want) {
		t.Fatalf("referencingAgents = %v, want %v", got.Status.ReferencingAgents, want)
	}
}

// equalStringSlice returns true when a and b contain the same strings
// in the same order.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestReconcile_DeletionSuccess exercises the drain happy path: the
// finalizer is removed and the registry entry is dropped.
func TestReconcile_DeletionSuccess(t *testing.T) {
	t.Parallel()

	registry := skill.NewMemoryRegistry()
	sk := newSkillBuilder("retiring").build()
	// Pre-register so we can assert deregister happens.
	if err := registry.Register(context.Background(), sk); err != nil {
		t.Fatalf("seed registry: %v", err)
	}
	now := time.Now().UTC()
	sk = newSkillBuilder("retiring").
		withDeletionTimestamp(now).
		build()
	r, c, _, _ := newReconciler(t, reconcilerOpts{registry: registry}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	res := reconcileOnce(t, r, key)
	if res.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", res.RequeueAfter)
	}

	// Confirm registry entry was removed.
	ref, err := skill.SkillResourceRef(sk)
	if err != nil {
		t.Fatalf("SkillResourceRef: %v", err)
	}
	if registry.Has(ref) {
		t.Fatalf("registry still contains %s after deletion", ref)
	}

	// Object should be gone (fake client deletes when the last
	// finalizer is removed during deletion).
	got := &skillv1alpha1.Skill{}
	err = c.Get(context.Background(), key, got)
	if err == nil && controllerutil.ContainsFinalizer(got, skill.FinalizerSkillProtect) {
		t.Fatalf("finalizer should be removed; got %v", got.Finalizers)
	}
}

// TestReconcile_Idempotent confirms running Reconcile twice on a
// steady-state object yields the same conditions / phase / generation.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	sk := newSkillBuilder("idempotent").build()
	r, c, bus, _ := newReconciler(t, reconcilerOpts{}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	reconcileToSteady(t, r, key, 4)
	first := getSkill(t, c, key)
	firstEvents := len(bus.Events())

	// Snapshot the LastTransitionTime values; idempotent reconciles
	// MUST NOT bump these.
	firstTimestamps := map[string]metav1.Time{}
	for _, c := range first.Status.Conditions {
		firstTimestamps[c.Type] = c.LastTransitionTime
	}

	// Second reconcile — should be a pure no-op on conditions.
	reconcileOnce(t, r, key)
	second := getSkill(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed across reconciles: %s -> %s", first.Status.Phase, second.Status.Phase)
	}
	if first.Status.ObservedGeneration != second.Status.ObservedGeneration {
		t.Fatalf("observedGeneration changed: %d -> %d", first.Status.ObservedGeneration, second.Status.ObservedGeneration)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("conditions count changed: %d -> %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	for _, c := range second.Status.Conditions {
		if firstTimestamps[c.Type] != c.LastTransitionTime {
			t.Fatalf("LastTransitionTime drifted on %s: %s -> %s", c.Type, firstTimestamps[c.Type], c.LastTransitionTime)
		}
	}
	if got := len(bus.Events()); got != firstEvents {
		t.Fatalf("idempotent reconcile published events: %d -> %d", firstEvents, got)
	}
}

// TestReconcile_EvalGate_NonExperimental verifies that non-experimental
// Skills do not reach Ready=True until the EvalRunner is implemented.
func TestReconcile_EvalGate_NonExperimental(t *testing.T) {
	t.Parallel()

	sk := newSkillBuilder("stable-skill").
		withStability(sharedv1alpha1.StageStable).
		build()
	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	reconcileToSteady(t, r, key, 4)

	got := getSkill(t, c, key)
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillEvalPassing); status != metav1.ConditionUnknown {
		t.Fatalf("EvalPassing = %s, want Unknown", status)
	}
	if got.Status.Phase == sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = Active, want non-Active for un-evaluated stable skill")
	}
	if status := conditionStatus(t, got.Status.Conditions, skillv1alpha1.SkillReady); status == metav1.ConditionTrue {
		t.Fatalf("Ready = True, want False until eval runner is implemented")
	}
}

// TestReconcile_TransientResolverError exercises the transient
// resolver path: a non-nil error from Resolve should requeue with
// backoff (>0) and not corrupt status.
func TestReconcile_TransientResolverError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("etcd unavailable")
	resolver := skill.FuncResolver(func(_ context.Context, _ *skillv1alpha1.Skill) (skill.ResolveResult, error) {
		return skill.ResolveResult{}, wantErr
	})
	sk := newSkillBuilder("transient").build()
	r, _, _, _ := newReconciler(t, reconcilerOpts{resolver: resolver}, sk)
	key := types.NamespacedName{Name: sk.Name, Namespace: sk.Namespace}

	// First pass adds the finalizer.
	res := reconcileOnce(t, r, key)
	if !res.Requeue {
		t.Fatal("first pass should request requeue after stamping finalizer")
	}

	// Second pass: resolver returns an error → controller should
	// surface it and request a backoff requeue.
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected resolver error to propagate, got %v", err)
	}
}
