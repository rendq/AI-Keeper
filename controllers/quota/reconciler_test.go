package quota_test

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/quota"
)

// ---------------------------------------------------------------------------
// FakeResourceCounter
// ---------------------------------------------------------------------------

// FakeResourceCounter is an in-memory [quota.ResourceCounter] for tests.
type FakeResourceCounter struct {
	Counts map[string]int64
	Err    error
}

func NewFakeCounter() *FakeResourceCounter {
	return &FakeResourceCounter{Counts: map[string]int64{}}
}

func (f *FakeResourceCounter) Count(_ context.Context, kind, _ string) (int64, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	return f.Counts[kind], nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		policyv1alpha1.AddToScheme,
		agentv1alpha1.AddToScheme,
		skillv1alpha1.AddToScheme,
		modelv1alpha1.AddToScheme,
		datav1alpha1.AddToScheme,
	} {
		if err := add(s); err != nil {
			t.Fatalf("register scheme: %v", err)
		}
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&policyv1alpha1.Quota{}).
		Build()
	return c, s
}

type quotaBuilder struct {
	q *policyv1alpha1.Quota
}

func newQuota(name string) *quotaBuilder {
	return &quotaBuilder{
		q: &policyv1alpha1.Quota{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "tenant-acme",
				Generation: 1,
			},
			Spec: policyv1alpha1.QuotaSpec{
				Scope: policyv1alpha1.QuotaScope{
					Kind: "Tenant",
					Name: "acme",
				},
			},
		},
	}
}

func (b *quotaBuilder) withLimit(kind string, val int) *quotaBuilder {
	if b.q.Spec.Limits == nil {
		b.q.Spec.Limits = map[string]intstr.IntOrString{}
	}
	b.q.Spec.Limits[kind] = intstr.FromInt32(int32(val))
	return b
}

func (b *quotaBuilder) build() *policyv1alpha1.Quota { return b.q }

func newReconciler(t *testing.T, counter quota.ResourceCounter, objs ...client.Object) (*quota.QuotaReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &quota.QuotaReconciler{
		Client:  c,
		Scheme:  s,
		Counter: counter,
	}, c
}

func reconcileOnce(t *testing.T, r *quota.QuotaReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *quota.QuotaReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getQuota(t *testing.T, c client.Client, key types.NamespacedName) *policyv1alpha1.Quota {
	t.Helper()
	got := &policyv1alpha1.Quota{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get quota: %v", err)
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestReconcile_HappyPath exercises Requirement A8.5: a freshly-
// created Quota counts resources, populates status.used, and reaches
// Phase=Active when within limits.
func TestReconcile_HappyPath(t *testing.T) {
	t.Parallel()

	counter := NewFakeCounter()
	counter.Counts[quota.ResourceAgents] = 2
	counter.Counts[quota.ResourceSkills] = 5

	q := newQuota("default").
		withLimit(quota.ResourceAgents, 10).
		withLimit(quota.ResourceSkills, 20).
		build()
	r, c := newReconciler(t, counter, q)
	key := types.NamespacedName{Namespace: q.Namespace, Name: q.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getQuota(t, c, key)
	if !controllerutil.ContainsFinalizer(got, quota.FinalizerQuotaProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.QuotaWithinLimit); status != metav1.ConditionTrue {
		t.Fatalf("WithinLimit = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.QuotaServiceReady); status != metav1.ConditionTrue {
		t.Fatalf("ServiceReady = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.QuotaReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
	// Verify used values.
	if v, ok := got.Status.Used[quota.ResourceAgents]; !ok || v.IntValue() != 2 {
		t.Fatalf("used[agents] = %v, want 2", got.Status.Used[quota.ResourceAgents])
	}
	if v, ok := got.Status.Used[quota.ResourceSkills]; !ok || v.IntValue() != 5 {
		t.Fatalf("used[skills] = %v, want 5", got.Status.Used[quota.ResourceSkills])
	}
	if last.RequeueAfter != quota.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, quota.SteadyStateRequeue)
	}
}

// TestReconcile_Exceeded exercises Requirement A8.5: a Quota whose
// used count reaches the limit flips WithinLimit=False and Phase=Failed.
func TestReconcile_Exceeded(t *testing.T) {
	t.Parallel()

	counter := NewFakeCounter()
	counter.Counts[quota.ResourceAgents] = 10

	q := newQuota("over-quota").
		withLimit(quota.ResourceAgents, 10).
		build()
	r, c := newReconciler(t, counter, q)
	key := types.NamespacedName{Namespace: q.Namespace, Name: q.Name}

	reconcileToSteady(t, r, key, 4)

	got := getQuota(t, c, key)
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.QuotaWithinLimit); status != metav1.ConditionFalse {
		t.Fatalf("WithinLimit = %s, want False", status)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.QuotaReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
}

// TestReconcile_NoLimits verifies the boundary case where limits map
// is empty → WithinLimit=True and Phase=Active.
func TestReconcile_NoLimits(t *testing.T) {
	t.Parallel()

	counter := NewFakeCounter()
	q := newQuota("no-limits").build() // no limits set
	r, c := newReconciler(t, counter, q)
	key := types.NamespacedName{Namespace: q.Namespace, Name: q.Name}
	reconcileToSteady(t, r, key, 4)

	got := getQuota(t, c, key)
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.QuotaWithinLimit); status != metav1.ConditionTrue {
		t.Fatalf("WithinLimit = %s, want True (no limits)", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
}

// TestReconcile_Idempotent verifies that a second steady-state pass
// does not change phase or conditions.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	counter := NewFakeCounter()
	counter.Counts[quota.ResourceAgents] = 3

	q := newQuota("default").withLimit(quota.ResourceAgents, 10).build()
	r, c := newReconciler(t, counter, q)
	key := types.NamespacedName{Namespace: q.Namespace, Name: q.Name}

	reconcileToSteady(t, r, key, 4)
	first := getQuota(t, c, key).DeepCopy()

	reconcileOnce(t, r, key)
	second := getQuota(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	if second.Status.ObservedGeneration != second.Generation {
		t.Fatalf("observedGeneration drift: %d, want %d", second.Status.ObservedGeneration, second.Generation)
	}
}

// TestReconcile_Deletion verifies the drain path: the finalizer is
// removed and the CR disappears from the API server.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()
	counter := NewFakeCounter()
	q := newQuota("default").withLimit(quota.ResourceAgents, 10).build()
	r, c := newReconciler(t, counter, q)
	key := types.NamespacedName{Namespace: q.Namespace, Name: q.Name}

	reconcileToSteady(t, r, key, 4)
	got := getQuota(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete quota: %v", err)
	}
	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}
	if err := c.Get(context.Background(), key, &policyv1alpha1.Quota{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted CR
// returns a clean result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()
	r, _ := newReconciler(t, NewFakeCounter())
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing quota: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing quota, got %+v", res)
	}
}

// TestReconcile_PartialExceed verifies that exceeding one limit while
// another is fine still fails.
func TestReconcile_PartialExceed(t *testing.T) {
	t.Parallel()

	counter := NewFakeCounter()
	counter.Counts[quota.ResourceAgents] = 5  // under limit
	counter.Counts[quota.ResourceSkills] = 20 // at limit

	q := newQuota("partial").
		withLimit(quota.ResourceAgents, 10).
		withLimit(quota.ResourceSkills, 20).
		build()
	r, c := newReconciler(t, counter, q)
	key := types.NamespacedName{Namespace: q.Namespace, Name: q.Name}

	reconcileToSteady(t, r, key, 4)

	got := getQuota(t, c, key)
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.QuotaWithinLimit); status != metav1.ConditionFalse {
		t.Fatalf("WithinLimit = %s, want False (skills at limit)", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
}

// TestReconcile_UnknownResource verifies that an unknown resource key
// doesn't block the reconcile; it just gets counted as 0.
func TestReconcile_UnknownResource(t *testing.T) {
	t.Parallel()

	counter := NewFakeCounter()
	q := newQuota("unknown-kind").
		withLimit("unicorns", 10).
		build()
	r, c := newReconciler(t, counter, q)
	key := types.NamespacedName{Namespace: q.Namespace, Name: q.Name}

	reconcileToSteady(t, r, key, 4)

	got := getQuota(t, c, key)
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if v, ok := got.Status.Used["unicorns"]; !ok || v.IntValue() != 0 {
		t.Fatalf("used[unicorns] = %v, want 0", got.Status.Used["unicorns"])
	}
}
