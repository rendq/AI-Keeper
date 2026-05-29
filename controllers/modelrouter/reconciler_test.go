package modelrouter_test

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/modelrouter"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := modelv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register model.ai-keeper.io scheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&modelv1alpha1.ModelRouter{}, &modelv1alpha1.ModelEndpoint{}).
		Build()
	return c, s
}

func newRouter(name, namespace, alias string, refs ...sharedv1alpha1.ResourceRef) *modelv1alpha1.ModelRouter {
	rules := make([]modelv1alpha1.ModelRouterRule, 0, len(refs))
	for _, ref := range refs {
		rules = append(rules, modelv1alpha1.ModelRouterRule{Endpoint: ref})
	}
	return &modelv1alpha1.ModelRouter{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: modelv1alpha1.ModelRouterSpec{
			Alias: alias,
			Rules: rules,
		},
	}
}

func newReadyEndpoint(name, namespace string) *modelv1alpha1.ModelEndpoint {
	ep := &modelv1alpha1.ModelEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: modelv1alpha1.ModelEndpointSpec{
			Provider: "openai",
			Model:    "gpt-4o",
			Endpoint: "https://api.example.com/v1",
		},
		Status: modelv1alpha1.ModelEndpointStatus{
			Phase: sharedv1alpha1.PhaseActive,
			Conditions: []metav1.Condition{
				{
					Type:               modelv1alpha1.ModelEndpointReady,
					Status:             metav1.ConditionTrue,
					Reason:             "Ready",
					Message:            "test fixture",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
	return ep
}

func newReconciler(t *testing.T, pusher modelrouter.RouterPusher, objs ...client.Object) (*modelrouter.ModelRouterReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &modelrouter.ModelRouterReconciler{
		Client: c,
		Scheme: s,
		Pusher: pusher,
	}, c
}

func reconcileOnce(t *testing.T, r *modelrouter.ModelRouterReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *modelrouter.ModelRouterReconciler, key types.NamespacedName, max int) reconcile.Result {
	t.Helper()
	var last reconcile.Result
	for i := 0; i < max; i++ {
		last = reconcileOnce(t, r, key)
		if !last.Requeue && last.RequeueAfter > 0 {
			return last
		}
		if !last.Requeue {
			return last
		}
	}
	t.Fatalf("Reconcile did not reach steady state after %d passes", max)
	return last
}

func getRouter(t *testing.T, c client.Client, key types.NamespacedName) *modelv1alpha1.ModelRouter {
	t.Helper()
	got := &modelv1alpha1.ModelRouter{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get modelrouter: %v", err)
	}
	return got
}

func conditionStatus(conds []metav1.Condition, condType string) metav1.ConditionStatus {
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

// TestReconcile_HappyPath exercises Requirement A7.7: a router with
// one Ready endpoint and one router instance flips through every
// gate, pushes the table, and reaches Phase=Active.
func TestReconcile_HappyPath(t *testing.T) {
	t.Parallel()

	ep := newReadyEndpoint("gpt-4o", "tenant-acme")
	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://gpt-4o"))
	pusher := modelrouter.NewMemoryRouterPusher(modelrouter.Instance{ID: "router-0", Address: "router:9090"})

	r, c := newReconciler(t, pusher, mr, ep)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getRouter(t, c, key)
	if !controllerutil.ContainsFinalizer(got, modelrouter.FinalizerModelRouterProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	gates := []string{
		modelv1alpha1.ModelRouterCompiled,
		modelv1alpha1.ModelRouterDistributed,
		modelv1alpha1.ModelRouterAllReachable,
		modelv1alpha1.ModelRouterReady,
	}
	for _, g := range gates {
		if status := conditionStatus(got.Status.Conditions, g); status != metav1.ConditionTrue {
			t.Fatalf("%s = %s, want True", g, status)
		}
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
	if last.RequeueAfter != modelrouter.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, modelrouter.SteadyStateRequeue)
	}
	// The router-0 instance must have received a non-empty hash.
	tables := pusher.Snapshot()
	if h := tables["router-0"]; h == "" {
		t.Fatalf("expected push to router-0, got %v", tables)
	}
	if pusher.PushCalls < 1 {
		t.Fatalf("PushCalls = %d, want ≥ 1", pusher.PushCalls)
	}
	// Distribution should track the single rule.
	if len(got.Status.Distribution) != 1 {
		t.Fatalf("status.distribution = %v, want 1 entry", got.Status.Distribution)
	}
	if got.Status.Distribution[0].Endpoint != sharedv1alpha1.ResourceRef("model://gpt-4o") {
		t.Fatalf("distribution endpoint = %q, want model://gpt-4o", got.Status.Distribution[0].Endpoint)
	}
}

// TestReconcile_AllUnreachable covers Requirement A7.7 ("endpoint 全
// 不可达置 Degraded"): every endpoint reference is missing →
// AllReachable=False, Phase=Degraded.
func TestReconcile_AllUnreachable(t *testing.T) {
	t.Parallel()

	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://missing-1"),
		sharedv1alpha1.ResourceRef("model://missing-2"))
	pusher := modelrouter.NewMemoryRouterPusher(modelrouter.Instance{ID: "router-0"})

	r, c := newReconciler(t, pusher, mr)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	reconcileToSteady(t, r, key, 4)

	got := getRouter(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterCompiled); status != metav1.ConditionTrue {
		t.Fatalf("Compiled = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterAllReachable); status != metav1.ConditionFalse {
		t.Fatalf("AllReachable = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, modelv1alpha1.ModelRouterAllReachable); reason != modelrouter.ReasonAllUnreachable {
		t.Fatalf("AllReachable.reason = %q, want %q", reason, modelrouter.ReasonAllUnreachable)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseDegraded {
		t.Fatalf("phase = %s, want Degraded", got.Status.Phase)
	}
}

// TestReconcile_PartialReachable covers the "1/2 endpoints Ready"
// case: AllReachable=False reason=Partial but the table is still
// distributed. Aggregate Ready is False (partial != all) but the
// router stays out of Phase=Degraded because traffic can still flow.
func TestReconcile_PartialReachable(t *testing.T) {
	t.Parallel()

	ep := newReadyEndpoint("gpt-4o", "tenant-acme")
	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://gpt-4o"),
		sharedv1alpha1.ResourceRef("model://still-missing"))
	pusher := modelrouter.NewMemoryRouterPusher(modelrouter.Instance{ID: "router-0"})

	r, c := newReconciler(t, pusher, mr, ep)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	reconcileToSteady(t, r, key, 4)

	got := getRouter(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterCompiled); status != metav1.ConditionTrue {
		t.Fatalf("Compiled = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterDistributed); status != metav1.ConditionTrue {
		t.Fatalf("Distributed = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterAllReachable); status != metav1.ConditionFalse {
		t.Fatalf("AllReachable = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, modelv1alpha1.ModelRouterAllReachable); reason != modelrouter.ReasonPartialReachable {
		t.Fatalf("AllReachable.reason = %q, want %q", reason, modelrouter.ReasonPartialReachable)
	}
	if got.Status.Phase == sharedv1alpha1.PhaseDegraded {
		t.Fatalf("phase = Degraded; partial reachability should not degrade")
	}
}

// TestReconcile_NoInstances covers the "router fleet not yet
// registered" case: Discover returns empty → Distributed=Unknown
// reason=NoInstances, but aggregate Ready can still flip True
// because the table is recorded centrally.
func TestReconcile_NoInstances(t *testing.T) {
	t.Parallel()

	ep := newReadyEndpoint("gpt-4o", "tenant-acme")
	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://gpt-4o"))
	pusher := modelrouter.NewMemoryRouterPusher() // no instances

	r, c := newReconciler(t, pusher, mr, ep)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	reconcileToSteady(t, r, key, 4)

	got := getRouter(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterDistributed); status != metav1.ConditionUnknown {
		t.Fatalf("Distributed = %s, want Unknown", status)
	}
	if reason := conditionReason(got.Status.Conditions, modelv1alpha1.ModelRouterDistributed); reason != modelrouter.ReasonNoInstances {
		t.Fatalf("Distributed.reason = %q, want %q", reason, modelrouter.ReasonNoInstances)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True (NoInstances counts as satisfied)", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
}

// TestReconcile_PushFailure covers the transport failure path on
// Push: the controller flips Distributed=False and requeues with
// backoff (sub-steady-state cadence).
func TestReconcile_PushFailure(t *testing.T) {
	t.Parallel()

	ep := newReadyEndpoint("gpt-4o", "tenant-acme")
	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://gpt-4o"))
	pusher := modelrouter.NewMemoryRouterPusher(modelrouter.Instance{ID: "router-0"})
	pusher.PushErr = modelrouter.ErrPushFailed

	r, c := newReconciler(t, pusher, mr, ep)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	reconcileOnce(t, r, key) // adds finalizer, requeues
	last := reconcileOnce(t, r, key)

	got := getRouter(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterCompiled); status != metav1.ConditionTrue {
		t.Fatalf("Compiled = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterDistributed); status != metav1.ConditionFalse {
		t.Fatalf("Distributed = %s, want False", status)
	}
	if last.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0 on push failure")
	}
	if last.RequeueAfter >= modelrouter.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want < steady-state %s", last.RequeueAfter, modelrouter.SteadyStateRequeue)
	}
}

// TestReconcile_Idempotent verifies that a second steady-state pass
// does not change the router's conditions, distribution, or push
// hash. Pushing the same hash twice is acceptable; conditions must
// not flip.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	ep := newReadyEndpoint("gpt-4o", "tenant-acme")
	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://gpt-4o"))
	pusher := modelrouter.NewMemoryRouterPusher(modelrouter.Instance{ID: "router-0"})

	r, c := newReconciler(t, pusher, mr, ep)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	reconcileToSteady(t, r, key, 4)
	first := getRouter(t, c, key).DeepCopy()
	firstHash := pusher.Snapshot()["router-0"]

	reconcileOnce(t, r, key)
	second := getRouter(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	secondHash := pusher.Snapshot()["router-0"]
	if firstHash != secondHash {
		t.Fatalf("pushed hash changed across reconciles: %q → %q", firstHash, secondHash)
	}
	if second.Status.ObservedGeneration != second.Generation {
		t.Fatalf("observedGeneration drift: %d, want %d", second.Status.ObservedGeneration, second.Generation)
	}
}

// TestReconcile_InvalidRef covers the malformed `endpoint` ref path:
// the controller flips Compiled=False reason=CompileFailed and stays
// in Phase=Failed.
func TestReconcile_InvalidRef(t *testing.T) {
	t.Parallel()

	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://a/b/c")) // depth > 2
	pusher := modelrouter.NewMemoryRouterPusher()

	r, c := newReconciler(t, pusher, mr)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	reconcileOnce(t, r, key) // adds finalizer
	reconcileOnce(t, r, key)

	got := getRouter(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelRouterCompiled); status != metav1.ConditionFalse {
		t.Fatalf("Compiled = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
}

// TestReconcile_Deletion verifies the drain path: finalizer is
// removed and the CR disappears from the API server.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()

	ep := newReadyEndpoint("gpt-4o", "tenant-acme")
	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://gpt-4o"))
	pusher := modelrouter.NewMemoryRouterPusher(modelrouter.Instance{ID: "router-0"})

	r, c := newReconciler(t, pusher, mr, ep)
	key := types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}

	reconcileToSteady(t, r, key, 4)
	got := getRouter(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete modelrouter: %v", err)
	}
	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}
	if err := c.Get(context.Background(), key, &modelv1alpha1.ModelRouter{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted CR
// returns a clean (no error, no requeue) result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	r, _ := newReconciler(t, modelrouter.NoopRouterPusher{})
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing router: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing router, got %+v", res)
	}
}

// TestCompileRoutingTable verifies the compiled artefact is stable
// (same input → same hash) and that the hash changes when relevant
// fields change.
func TestCompileRoutingTable(t *testing.T) {
	t.Parallel()

	mr := newRouter("reasoner-router", "tenant-acme", "reasoner",
		sharedv1alpha1.ResourceRef("model://gpt-4o"))
	t1, err := modelrouter.CompileRoutingTable(mr)
	if err != nil {
		t.Fatalf("CompileRoutingTable: %v", err)
	}
	if t1.Hash == "" {
		t.Fatalf("compiled table has empty hash")
	}
	if len(t1.Rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(t1.Rules))
	}

	t2, err := modelrouter.CompileRoutingTable(mr)
	if err != nil {
		t.Fatalf("CompileRoutingTable (2): %v", err)
	}
	if t1.Hash != t2.Hash {
		t.Fatalf("hash changed across calls: %q → %q", t1.Hash, t2.Hash)
	}

	// Mutate alias and re-compile — hash MUST change.
	mr2 := mr.DeepCopy()
	mr2.Spec.Alias = "embedder"
	t3, err := modelrouter.CompileRoutingTable(mr2)
	if err != nil {
		t.Fatalf("CompileRoutingTable (3): %v", err)
	}
	if t1.Hash == t3.Hash {
		t.Fatalf("alias change did not bump hash: %q", t1.Hash)
	}
}
