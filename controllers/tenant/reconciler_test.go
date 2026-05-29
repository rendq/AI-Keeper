package tenant_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/tenant"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("register core/v1 scheme: %v", err)
	}
	if err := corev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register core.ai-keeper.io scheme: %v", err)
	}
	if err := policyv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register policy.ai-keeper.io scheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(
			&corev1alpha1.Tenant{},
			&policyv1alpha1.Budget{},
			&policyv1alpha1.Quota{},
		).
		Build()
	return c, s
}

func newTenant(name string, opts ...func(*corev1alpha1.Tenant)) *corev1alpha1.Tenant {
	t := &corev1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: corev1alpha1.TenantSpec{
			DisplayName: "Test " + name,
			ComplianceProfile: corev1alpha1.TenantComplianceProfile{
				Tier: "standard",
			},
		},
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

func withDefaultBudget(usd string, tokens int64) func(*corev1alpha1.Tenant) {
	return func(t *corev1alpha1.Tenant) {
		amount := sharedv1alpha1.MoneyAmount(usd)
		t.Spec.DefaultBudget = &corev1alpha1.TenantDefaultBudget{
			UsdPerMonth:    &amount,
			TokensPerMonth: &tokens,
		}
	}
}

func reconcileOnce(t *testing.T, r *tenant.TenantReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *tenant.TenantReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getTenant(t *testing.T, c client.Client, key types.NamespacedName) *corev1alpha1.Tenant {
	t.Helper()
	got := &corev1alpha1.Tenant{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get tenant: %v", err)
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

// TestReconcile_HappyPath_WithBudget exercises Requirement A7.1: a
// Tenant CR with `spec.defaultBudget` flips through the gate pyramid
// and reaches Phase=Active with Namespace + Budget + Quota in place
// and `status.namespaces` populated.
func TestReconcile_HappyPath_WithBudget(t *testing.T) {
	t.Parallel()

	tn := newTenant("acme", withDefaultBudget("1000.00", 5_000_000))
	c, s := newFakeClient(t, tn)
	r := &tenant.TenantReconciler{Client: c, Scheme: s}
	key := types.NamespacedName{Name: tn.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getTenant(t, c, key)
	if !controllerutil.ContainsFinalizer(got, tenant.FinalizerTenantCleanup) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	gates := []string{
		corev1alpha1.TenantNamespacesReady,
		corev1alpha1.TenantConnectorsReady,
		corev1alpha1.TenantReady,
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

	wantNS := "tenant-" + tn.Name
	if len(got.Status.Namespaces) != 1 || got.Status.Namespaces[0] != wantNS {
		t.Fatalf("status.namespaces = %v, want [%q]", got.Status.Namespaces, wantNS)
	}

	// Namespace must exist with the tenant + managed-by labels.
	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: wantNS}, ns); err != nil {
		t.Fatalf("Get namespace %q: %v", wantNS, err)
	}
	if ns.Labels[tenant.LabelTenant] != tn.Name {
		t.Fatalf("namespace label %s = %q, want %q",
			tenant.LabelTenant, ns.Labels[tenant.LabelTenant], tn.Name)
	}
	if ns.Labels[tenant.LabelManagedBy] != tenant.ManagerName {
		t.Fatalf("namespace label %s = %q, want %q",
			tenant.LabelManagedBy, ns.Labels[tenant.LabelManagedBy], tenant.ManagerName)
	}

	// Budget must exist with USD + Tokens caps wired through.
	budget := &policyv1alpha1.Budget{}
	bKey := types.NamespacedName{Name: tenant.DefaultBudgetName, Namespace: wantNS}
	if err := c.Get(context.Background(), bKey, budget); err != nil {
		t.Fatalf("Get default budget: %v", err)
	}
	if budget.Spec.Period != tenant.DefaultBudgetPeriod {
		t.Fatalf("budget.Period = %q, want %q", budget.Spec.Period, tenant.DefaultBudgetPeriod)
	}
	if budget.Spec.Scope.Kind != "Tenant" || budget.Spec.Scope.Name != tn.Name {
		t.Fatalf("budget.Scope = %+v, want {Tenant, %s}", budget.Spec.Scope, tn.Name)
	}
	if budget.Spec.Limits.Usd == nil || string(*budget.Spec.Limits.Usd) != "1000.00" {
		t.Fatalf("budget.Limits.Usd = %v, want 1000.00", budget.Spec.Limits.Usd)
	}
	if budget.Spec.Limits.Tokens == nil || *budget.Spec.Limits.Tokens != 5_000_000 {
		t.Fatalf("budget.Limits.Tokens = %v, want 5_000_000", budget.Spec.Limits.Tokens)
	}
	if budget.Spec.HardCap == nil || !*budget.Spec.HardCap {
		t.Fatalf("budget.HardCap = %v, want true", budget.Spec.HardCap)
	}

	// Quota must exist with non-empty placeholder limits.
	quota := &policyv1alpha1.Quota{}
	qKey := types.NamespacedName{Name: tenant.DefaultQuotaName, Namespace: wantNS}
	if err := c.Get(context.Background(), qKey, quota); err != nil {
		t.Fatalf("Get default quota: %v", err)
	}
	if quota.Spec.Scope.Kind != "Tenant" || quota.Spec.Scope.Name != tn.Name {
		t.Fatalf("quota.Scope = %+v, want {Tenant, %s}", quota.Spec.Scope, tn.Name)
	}
	if got, want := len(quota.Spec.Limits), len(tenant.DefaultQuotaLimits); got != want {
		t.Fatalf("quota.Limits len = %d, want %d", got, want)
	}
	agentsLimit := quota.Spec.Limits["agents"]
	if got := agentsLimit.IntValue(); got != 100 {
		t.Fatalf("quota.Limits[agents] = %d, want 100", got)
	}

	if last.RequeueAfter != tenant.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, tenant.SteadyStateRequeue)
	}
}

// TestReconcile_NoDefaultBudget covers the case where `spec.defaultBudget`
// is nil — the controller should skip Budget seeding silently and still
// reach Ready=True.
func TestReconcile_NoDefaultBudget(t *testing.T) {
	t.Parallel()

	tn := newTenant("globex")
	c, s := newFakeClient(t, tn)
	r := &tenant.TenantReconciler{Client: c, Scheme: s}
	key := types.NamespacedName{Name: tn.Name}

	reconcileToSteady(t, r, key, 4)

	got := getTenant(t, c, key)
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	wantNS := "tenant-" + tn.Name
	if err := c.Get(context.Background(), types.NamespacedName{Name: tenant.DefaultBudgetName, Namespace: wantNS},
		&policyv1alpha1.Budget{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected default Budget to be absent, err = %v", err)
	}
	// Quota must still exist.
	if err := c.Get(context.Background(), types.NamespacedName{Name: tenant.DefaultQuotaName, Namespace: wantNS},
		&policyv1alpha1.Quota{}); err != nil {
		t.Fatalf("Get default quota: %v", err)
	}
}

// TestReconcile_Idempotent verifies that a second steady-state pass
// produces identical status (Requirements F1, F2). We compare condition
// statuses + phase + namespaces because timestamps differ between
// passes.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	tn := newTenant("acme", withDefaultBudget("1000.00", 5_000_000))
	c, s := newFakeClient(t, tn)
	r := &tenant.TenantReconciler{Client: c, Scheme: s}
	key := types.NamespacedName{Name: tn.Name}

	reconcileToSteady(t, r, key, 4)
	first := getTenant(t, c, key).DeepCopy()

	// Capture the operation result counts indirectly by snapshotting
	// generation + resourceVersion of the seeded objects. They must
	// remain stable across the second pass.
	wantNS := "tenant-" + tn.Name
	bKey := types.NamespacedName{Name: tenant.DefaultBudgetName, Namespace: wantNS}
	qKey := types.NamespacedName{Name: tenant.DefaultQuotaName, Namespace: wantNS}
	preBudget := &policyv1alpha1.Budget{}
	if err := c.Get(context.Background(), bKey, preBudget); err != nil {
		t.Fatalf("Get budget: %v", err)
	}
	preQuota := &policyv1alpha1.Quota{}
	if err := c.Get(context.Background(), qKey, preQuota); err != nil {
		t.Fatalf("Get quota: %v", err)
	}

	reconcileOnce(t, r, key)
	second := getTenant(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed across reconciles: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	if got, want := len(second.Status.Namespaces), 1; got != want {
		t.Fatalf("status.namespaces len = %d, want %d", got, want)
	}
	postBudget := &policyv1alpha1.Budget{}
	if err := c.Get(context.Background(), bKey, postBudget); err != nil {
		t.Fatalf("Get budget after 2nd pass: %v", err)
	}
	if preBudget.ResourceVersion != postBudget.ResourceVersion {
		t.Fatalf("budget RV mutated across reconciles: %s → %s",
			preBudget.ResourceVersion, postBudget.ResourceVersion)
	}
	postQuota := &policyv1alpha1.Quota{}
	if err := c.Get(context.Background(), qKey, postQuota); err != nil {
		t.Fatalf("Get quota after 2nd pass: %v", err)
	}
	if preQuota.ResourceVersion != postQuota.ResourceVersion {
		t.Fatalf("quota RV mutated across reconciles: %s → %s",
			preQuota.ResourceVersion, postQuota.ResourceVersion)
	}
}

// TestReconcile_PreservesOperatorOverrides verifies that operator
// overrides on the seeded Budget/Quota survive subsequent reconciles
// — only the spec fields the controller manages get re-applied.
func TestReconcile_PreservesOperatorOverrides(t *testing.T) {
	t.Parallel()

	tn := newTenant("acme", withDefaultBudget("1000.00", 5_000_000))
	c, s := newFakeClient(t, tn)
	r := &tenant.TenantReconciler{Client: c, Scheme: s}
	key := types.NamespacedName{Name: tn.Name}

	reconcileToSteady(t, r, key, 4)

	wantNS := "tenant-" + tn.Name
	// Operator narrows the Quota limits — controller must NOT clobber
	// the override on the next pass.
	quota := &policyv1alpha1.Quota{}
	qKey := types.NamespacedName{Name: tenant.DefaultQuotaName, Namespace: wantNS}
	if err := c.Get(context.Background(), qKey, quota); err != nil {
		t.Fatalf("Get quota: %v", err)
	}
	override := tightenQuotaLimits(quota.Spec.Limits)
	quota.Spec.Limits = override
	if err := c.Update(context.Background(), quota); err != nil {
		t.Fatalf("Update quota override: %v", err)
	}

	// Bump generation to drive a fresh reconcile.
	tn = getTenant(t, c, key)
	tn.Generation++
	if err := c.Update(context.Background(), tn); err != nil {
		t.Fatalf("Update tenant: %v", err)
	}
	reconcileToSteady(t, r, key, 4)

	got := &policyv1alpha1.Quota{}
	if err := c.Get(context.Background(), qKey, got); err != nil {
		t.Fatalf("Get quota after override: %v", err)
	}
	gotLimit := got.Spec.Limits["agents"]
	wantLimit := override["agents"]
	if gotLimit.IntValue() != wantLimit.IntValue() {
		t.Fatalf("agents limit clobbered: got %v, want %v",
			got.Spec.Limits["agents"], override["agents"])
	}
}

// TestReconcile_DeletionRemovesFinalizer covers the deletion path:
// `status.namespaces` is cleared, the finalizer is removed, and the
// underlying Namespace is intentionally left in place for operator
// review.
func TestReconcile_DeletionRemovesFinalizer(t *testing.T) {
	t.Parallel()

	tn := newTenant("acme")
	c, s := newFakeClient(t, tn)
	r := &tenant.TenantReconciler{Client: c, Scheme: s}
	key := types.NamespacedName{Name: tn.Name}

	// Drive to steady state, then mark for deletion.
	reconcileToSteady(t, r, key, 4)

	got := getTenant(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete tenant: %v", err)
	}
	// Drive deletion path to completion. Fake client may keep the
	// object as long as the finalizer is present.
	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}

	// Object should now be gone from the API server (finalizer
	// removed → fake client garbage-collects).
	err := c.Get(context.Background(), key, &corev1alpha1.Tenant{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}

	// Namespace MUST still exist — operators own its lifecycle.
	wantNS := "tenant-" + tn.Name
	if err := c.Get(context.Background(), types.NamespacedName{Name: wantNS}, &corev1.Namespace{}); err != nil {
		t.Fatalf("namespace deleted by tenant controller (should be left for operator): %v", err)
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted tenant
// returns a clean (no error, no requeue) result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	c, s := newFakeClient(t)
	r := &tenant.TenantReconciler{Client: c, Scheme: s}
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing tenant: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing tenant, got %+v", res)
	}
}

// tightenQuotaLimits returns a copy of `in` with all integer values
// reduced to 1 — a non-trivial operator override the controller must
// preserve.
func tightenQuotaLimits(in map[string]intstr.IntOrString) map[string]intstr.IntOrString {
	out := make(map[string]intstr.IntOrString, len(in))
	for k := range in {
		out[k] = intstr.FromInt32(1)
	}
	return out
}
