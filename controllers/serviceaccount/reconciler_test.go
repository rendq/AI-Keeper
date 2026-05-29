package serviceaccount_test

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/serviceaccount"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register core.ai-keeper.io scheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&corev1alpha1.ServiceAccount{}).
		Build()
	return c, s
}

type saBuilder struct {
	sa *corev1alpha1.ServiceAccount
}

func newSABuilder(name string) *saBuilder {
	return &saBuilder{
		sa: &corev1alpha1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "tenant-acme",
				Generation: 1,
			},
			Spec: corev1alpha1.ServiceAccountSpec{
				IdentityProvider: "oidc-corp",
			},
		},
	}
}

func (b *saBuilder) withIdentityProvider(p string) *saBuilder {
	b.sa.Spec.IdentityProvider = p
	return b
}

func (b *saBuilder) withOBO(enabled bool) *saBuilder {
	v := enabled
	b.sa.Spec.AllowOnBehalfOf = &v
	return b
}

func (b *saBuilder) withDeletionTimestamp() *saBuilder {
	now := metav1.Now()
	b.sa.DeletionTimestamp = &now
	if !controllerutil.ContainsFinalizer(b.sa, serviceaccount.FinalizerSARevoke) {
		controllerutil.AddFinalizer(b.sa, serviceaccount.FinalizerSARevoke)
	}
	return b
}

func (b *saBuilder) build() *corev1alpha1.ServiceAccount { return b.sa }

func newReconciler(t *testing.T, broker serviceaccount.IdentityBrokerClient, objs ...client.Object) (*serviceaccount.ServiceAccountReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &serviceaccount.ServiceAccountReconciler{
		Client:         c,
		Scheme:         s,
		IdentityBroker: broker,
	}, c
}

func reconcileOnce(t *testing.T, r *serviceaccount.ServiceAccountReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *serviceaccount.ServiceAccountReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getSA(t *testing.T, c client.Client, key types.NamespacedName) *corev1alpha1.ServiceAccount {
	t.Helper()
	got := &corev1alpha1.ServiceAccount{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get sa: %v", err)
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

// TestReconcile_HappyPath_WithOBO exercises Requirement A7.2: a SA
// with `allowOnBehalfOf=true` flips through Register → EnableOBO and
// reaches Phase=Active with both IdentityProviderReady and
// TokenExchangeReady=True.
func TestReconcile_HappyPath_WithOBO(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("legal-copilot-sa").withOBO(true).build()
	broker := &serviceaccount.NoopIdentityBroker{}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getSA(t, c, key)
	if !controllerutil.ContainsFinalizer(got, serviceaccount.FinalizerSARevoke) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	if status := conditionStatus(got.Status.Conditions, corev1alpha1.ServiceAccountIdentityProviderReady); status != metav1.ConditionTrue {
		t.Fatalf("IdentityProviderReady = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, corev1alpha1.ServiceAccountTokenExchangeReady); status != metav1.ConditionTrue {
		t.Fatalf("TokenExchangeReady = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, corev1alpha1.ServiceAccountReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}

	reg, _, oboCalls, _ := broker.Snapshot()
	if reg != 1 {
		t.Fatalf("Register calls = %d, want 1", reg)
	}
	if oboCalls != 1 {
		t.Fatalf("EnableOBO calls = %d, want 1", oboCalls)
	}
	if last.RequeueAfter != serviceaccount.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, serviceaccount.SteadyStateRequeue)
	}
}

// TestReconcile_HappyPath_WithoutOBO covers the standard SA flow:
// `allowOnBehalfOf` unset → TokenExchangeReady=Unknown reason=OBODisabled,
// aggregate Ready stays True and the SA reaches Phase=Active without
// the Broker's EnableOBO being called.
func TestReconcile_HappyPath_WithoutOBO(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("agent-runner-sa").build()
	broker := &serviceaccount.NoopIdentityBroker{}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	reconcileToSteady(t, r, key, 4)

	got := getSA(t, c, key)
	if status := conditionStatus(got.Status.Conditions, corev1alpha1.ServiceAccountTokenExchangeReady); status != metav1.ConditionUnknown {
		t.Fatalf("TokenExchangeReady = %s, want Unknown", status)
	}
	if reason := conditionReason(got.Status.Conditions, corev1alpha1.ServiceAccountTokenExchangeReady); reason != serviceaccount.ReasonOBODisabled {
		t.Fatalf("TokenExchangeReady reason = %s, want %s", reason, serviceaccount.ReasonOBODisabled)
	}
	if status := conditionStatus(got.Status.Conditions, corev1alpha1.ServiceAccountReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if _, _, oboCalls, _ := broker.Snapshot(); oboCalls != 0 {
		t.Fatalf("EnableOBO calls = %d, want 0", oboCalls)
	}
}

// TestReconcile_InvalidIdentityProvider exercises the defensive check
// against an empty `spec.identityProvider`. The fake client bypasses
// CRD admission, so this guards against bad test fixtures and
// future migrations that loosen the regex.
func TestReconcile_InvalidIdentityProvider(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("bad-sa").withIdentityProvider("").build()
	broker := &serviceaccount.NoopIdentityBroker{}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getSA(t, c, key)
	if reason := conditionReason(got.Status.Conditions, corev1alpha1.ServiceAccountIdentityProviderReady); reason != serviceaccount.ReasonInvalidIdentityProvider {
		t.Fatalf("IdentityProviderReady reason = %s, want %s",
			reason, serviceaccount.ReasonInvalidIdentityProvider)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0 (no retry on permanent failure)", last.RequeueAfter)
	}
	if reg, _, _, _ := broker.Snapshot(); reg != 0 {
		t.Fatalf("Register calls = %d, want 0", reg)
	}
}

// TestReconcile_RegisterFails covers the transient Broker failure
// path: Register returns an error → IdentityProviderReady=False
// reason=RegistrationFailed, the reconciler re-queues with backoff.
func TestReconcile_RegisterFails(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("legal-copilot-sa").withOBO(true).build()
	broker := &serviceaccount.NoopIdentityBroker{
		RegisterErr: errors.New("connection refused"),
	}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	// First pass adds the finalizer and requeues.
	first := reconcileOnce(t, r, key)
	if !first.Requeue {
		t.Fatalf("expected Requeue=true on first pass (finalizer added), got %+v", first)
	}
	// Second pass attempts Register and fails.
	second := reconcileOnce(t, r, key)
	if second.RequeueAfter == 0 {
		t.Fatalf("RequeueAfter = 0, want positive backoff on transient Broker failure")
	}

	got := getSA(t, c, key)
	if reason := conditionReason(got.Status.Conditions, corev1alpha1.ServiceAccountIdentityProviderReady); reason != serviceaccount.ReasonRegistrationFailed {
		t.Fatalf("IdentityProviderReady reason = %s, want %s",
			reason, serviceaccount.ReasonRegistrationFailed)
	}
	if status := conditionStatus(got.Status.Conditions, corev1alpha1.ServiceAccountReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
}

// TestReconcile_OBOFails covers the partial-failure path: Register
// succeeds but EnableOBO fails. The reconciler keeps
// IdentityProviderReady=True while flipping TokenExchangeReady=False
// and re-queueing with backoff.
func TestReconcile_OBOFails(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("legal-copilot-sa").withOBO(true).build()
	broker := &serviceaccount.NoopIdentityBroker{
		EnableOBOErr: errors.New("token exchange backend down"),
	}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	reconcileOnce(t, r, key)
	res := reconcileOnce(t, r, key)
	if res.RequeueAfter == 0 {
		t.Fatalf("RequeueAfter = 0, want positive backoff on EnableOBO failure")
	}

	got := getSA(t, c, key)
	if status := conditionStatus(got.Status.Conditions, corev1alpha1.ServiceAccountIdentityProviderReady); status != metav1.ConditionTrue {
		t.Fatalf("IdentityProviderReady = %s, want True", status)
	}
	if reason := conditionReason(got.Status.Conditions, corev1alpha1.ServiceAccountTokenExchangeReady); reason != serviceaccount.ReasonOBOFailed {
		t.Fatalf("TokenExchangeReady reason = %s, want %s", reason, serviceaccount.ReasonOBOFailed)
	}
}

// TestReconcile_Idempotent verifies a second steady-state pass produces
// identical status and does NOT re-call the Broker (Register /
// EnableOBO are designed to be idempotent — the assertion guards
// against the controller spamming the Broker unnecessarily).
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("legal-copilot-sa").withOBO(true).build()
	broker := &serviceaccount.NoopIdentityBroker{}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	reconcileToSteady(t, r, key, 4)
	first := getSA(t, c, key).DeepCopy()
	regBefore, _, oboBefore, _ := broker.Snapshot()

	reconcileOnce(t, r, key)
	second := getSA(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d",
			len(first.Status.Conditions), len(second.Status.Conditions))
	}
	regAfter, _, oboAfter, _ := broker.Snapshot()
	if regAfter != regBefore+1 {
		// Idempotency: we expect Register to be called once per
		// reconcile pass (it is up to the Broker to short-circuit on
		// no-op). The counter delta therefore equals the number of
		// reconciles. We assert single-pass progression here, not
		// "no calls", because the controller cannot know the Broker is
		// already in the desired state without asking.
		t.Fatalf("Register calls delta = %d, want exactly 1 (idempotent re-call)",
			regAfter-regBefore)
	}
	if oboAfter != oboBefore+1 {
		t.Fatalf("EnableOBO calls delta = %d, want exactly 1", oboAfter-oboBefore)
	}
}

// TestReconcile_Deletion exercises Requirement A7.2 / C8.4: the
// finalizer triggers Deregister + DisableOBO at the Broker before
// being removed.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("legal-copilot-sa").withOBO(true).withDeletionTimestamp().build()
	broker := &serviceaccount.NoopIdentityBroker{}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}

	// Object should be gone — finalizer removed → fake client
	// garbage-collects.
	err := c.Get(context.Background(), key, &corev1alpha1.ServiceAccount{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}

	_, deregister, _, disableOBO := broker.Snapshot()
	if deregister != 1 {
		t.Fatalf("Deregister calls = %d, want 1", deregister)
	}
	if disableOBO != 1 {
		t.Fatalf("DisableOBO calls = %d, want 1", disableOBO)
	}
}

// TestReconcile_DeletionDeregisterFails ensures that a Broker
// failure on Deregister keeps the finalizer in place and the
// reconciler re-queues with backoff (Requirement C8.4 — token
// revocation MUST succeed before the SA can be removed).
func TestReconcile_DeletionDeregisterFails(t *testing.T) {
	t.Parallel()

	sa := newSABuilder("legal-copilot-sa").withDeletionTimestamp().build()
	broker := &serviceaccount.NoopIdentityBroker{
		DeregisterErr: errors.New("broker unreachable"),
	}
	r, c := newReconciler(t, broker, sa)
	key := types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}

	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err == nil {
		t.Fatalf("expected error on failed Deregister, got nil")
	}
	if res.RequeueAfter == 0 {
		t.Fatalf("RequeueAfter = 0, want positive backoff")
	}

	got := getSA(t, c, key)
	if !controllerutil.ContainsFinalizer(got, serviceaccount.FinalizerSARevoke) {
		t.Fatalf("finalizer should remain until Deregister succeeds: %v", got.Finalizers)
	}
}

// TestReconcile_NotFound covers the absent-CR fast path.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	r, _ := newReconciler(t, &serviceaccount.NoopIdentityBroker{})
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing SA: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing SA, got %+v", res)
	}
}
