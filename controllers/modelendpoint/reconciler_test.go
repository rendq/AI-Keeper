package modelendpoint_test

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

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/modelendpoint"
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
		WithStatusSubresource(&modelv1alpha1.ModelEndpoint{}).
		Build()
	return c, s
}

type meBuilder struct {
	me *modelv1alpha1.ModelEndpoint
}

func newEndpointBuilder(name string) *meBuilder {
	return &meBuilder{
		me: &modelv1alpha1.ModelEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "tenant-acme",
				Generation: 1,
			},
			Spec: modelv1alpha1.ModelEndpointSpec{
				Provider: "openai",
				Model:    "gpt-4o",
				Endpoint: "https://api.example.com/v1",
			},
		},
	}
}

func (b *meBuilder) withCompliance(tags ...string) *meBuilder {
	b.me.Spec.Compliance = append([]string{}, tags...)
	return b
}

func (b *meBuilder) withDPASigned(v bool) *meBuilder {
	b.me.Spec.Privacy = &modelv1alpha1.ModelEndpointPrivacy{DPASigned: &v}
	return b
}

func (b *meBuilder) withQuota(tpm, rpm int64) *meBuilder {
	q := &modelv1alpha1.ModelEndpointQuota{}
	if tpm > 0 {
		t := tpm
		q.TPM = &t
	}
	if rpm > 0 {
		r := rpm
		q.RPM = &r
	}
	b.me.Spec.Quota = q
	return b
}

func (b *meBuilder) build() *modelv1alpha1.ModelEndpoint { return b.me }

func newReconciler(t *testing.T, prober modelendpoint.Prober, objs ...client.Object) (*modelendpoint.ModelEndpointReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &modelendpoint.ModelEndpointReconciler{
		Client: c,
		Scheme: s,
		Prober: prober,
	}, c
}

func reconcileOnce(t *testing.T, r *modelendpoint.ModelEndpointReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *modelendpoint.ModelEndpointReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getEndpoint(t *testing.T, c client.Client, key types.NamespacedName) *modelv1alpha1.ModelEndpoint {
	t.Helper()
	got := &modelv1alpha1.ModelEndpoint{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get modelendpoint: %v", err)
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

// TestReconcile_HappyPath exercises Requirement A7.6: a reachable
// endpoint with no compliance constraints flips through every gate
// and reaches Phase=Active. `status.healthy=true`,
// `status.avgLatencyMs` is populated, and the rolling counters are
// zero-initialised.
func TestReconcile_HappyPath(t *testing.T) {
	t.Parallel()

	prober := modelendpoint.NewNoopProber()
	prober.Latency = 42
	me := newEndpointBuilder("gpt-4o").build()

	r, c := newReconciler(t, prober, me)
	key := types.NamespacedName{Namespace: me.Namespace, Name: me.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getEndpoint(t, c, key)
	if !controllerutil.ContainsFinalizer(got, modelendpoint.FinalizerModelEndpointProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointHealthy); status != metav1.ConditionTrue {
		t.Fatalf("Healthy = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointDPASigned); status != metav1.ConditionUnknown {
		t.Fatalf("DPASigned = %s, want Unknown (not required)", status)
	}
	if reason := conditionReason(got.Status.Conditions, modelv1alpha1.ModelEndpointDPASigned); reason != modelendpoint.ReasonDPANotRequired {
		t.Fatalf("DPASigned.reason = %q, want %q", reason, modelendpoint.ReasonDPANotRequired)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointWithinQuota); status != metav1.ConditionTrue {
		t.Fatalf("WithinQuota = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.Healthy == nil || !*got.Status.Healthy {
		t.Fatalf("status.healthy = %v, want true", got.Status.Healthy)
	}
	if got.Status.AvgLatencyMs == nil || *got.Status.AvgLatencyMs != 42 {
		t.Fatalf("status.avgLatencyMs = %v, want 42", got.Status.AvgLatencyMs)
	}
	if got.Status.LastProbeAt == nil {
		t.Fatalf("status.lastProbeAt is nil")
	}
	if got.Status.CurrentTpm == nil || *got.Status.CurrentTpm != 0 {
		t.Fatalf("status.currentTpm = %v, want 0", got.Status.CurrentTpm)
	}
	if got.Status.CurrentRpm == nil || *got.Status.CurrentRpm != 0 {
		t.Fatalf("status.currentRpm = %v, want 0", got.Status.CurrentRpm)
	}
	if got.Status.ErrorRate24h == nil || *got.Status.ErrorRate24h != 0 {
		t.Fatalf("status.errorRate24h = %v, want 0", got.Status.ErrorRate24h)
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
	if last.RequeueAfter != modelendpoint.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, modelendpoint.SteadyStateRequeue)
	}
}

// TestReconcile_GDPRWithDPA covers the safe path for a GDPR-listed
// endpoint with `spec.privacy.dpaSigned=true` → DPASigned=True and
// Phase=Active.
func TestReconcile_GDPRWithDPA(t *testing.T) {
	t.Parallel()

	me := newEndpointBuilder("gpt-4o-eu").
		withCompliance("GDPR").
		withDPASigned(true).
		build()
	r, c := newReconciler(t, modelendpoint.NewNoopProber(), me)
	key := types.NamespacedName{Namespace: me.Namespace, Name: me.Name}

	reconcileToSteady(t, r, key, 4)

	got := getEndpoint(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointDPASigned); status != metav1.ConditionTrue {
		t.Fatalf("DPASigned = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
}

// TestReconcile_GDPRWithoutDPA is the rejection case: GDPR compliance
// but `dpaSigned!=true` → DPASigned=False reason=DPAMissing → Ready=False
// and Phase=Failed.
func TestReconcile_GDPRWithoutDPA(t *testing.T) {
	t.Parallel()

	me := newEndpointBuilder("gpt-4o-eu").
		withCompliance("GDPR").
		build()
	r, c := newReconciler(t, modelendpoint.NewNoopProber(), me)
	key := types.NamespacedName{Namespace: me.Namespace, Name: me.Name}

	reconcileToSteady(t, r, key, 4)

	got := getEndpoint(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointDPASigned); status != metav1.ConditionFalse {
		t.Fatalf("DPASigned = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, modelv1alpha1.ModelEndpointDPASigned); reason != modelendpoint.ReasonDPAMissing {
		t.Fatalf("DPASigned.reason = %q, want %q", reason, modelendpoint.ReasonDPAMissing)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
}

// TestReconcile_UnreachableEndpoint covers the probe-fail path: the
// endpoint surfaces as Healthy=False and Phase=Degraded.
func TestReconcile_UnreachableEndpoint(t *testing.T) {
	t.Parallel()

	prober := &modelendpoint.NoopProber{Err: errors.New("dial tcp: connection refused")}
	me := newEndpointBuilder("flaky-endpoint").build()
	r, c := newReconciler(t, prober, me)
	key := types.NamespacedName{Namespace: me.Namespace, Name: me.Name}

	// First pass adds finalizer, second pass probes and fails.
	reconcileOnce(t, r, key)
	last := reconcileOnce(t, r, key)

	got := getEndpoint(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointHealthy); status != metav1.ConditionFalse {
		t.Fatalf("Healthy = %s, want False", status)
	}
	if got.Status.Healthy == nil || *got.Status.Healthy {
		t.Fatalf("status.healthy = %v, want false", got.Status.Healthy)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseDegraded {
		t.Fatalf("phase = %s, want Degraded", got.Status.Phase)
	}
	if last.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0 on probe failure")
	}
}

// TestReconcile_QuotaExceeded covers the WithinQuota gate: a
// ModelEndpoint with `spec.quota.tpm=100` and a pre-populated
// `status.currentTpm=200` flips WithinQuota=False.
func TestReconcile_QuotaExceeded(t *testing.T) {
	t.Parallel()

	me := newEndpointBuilder("over-quota").withQuota(100, 0).build()
	current := int64(200)
	me.Status.CurrentTpm = &current

	r, c := newReconciler(t, modelendpoint.NewNoopProber(), me)
	key := types.NamespacedName{Namespace: me.Namespace, Name: me.Name}

	reconcileToSteady(t, r, key, 4)

	got := getEndpoint(t, c, key)
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointWithinQuota); status != metav1.ConditionFalse {
		t.Fatalf("WithinQuota = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, modelv1alpha1.ModelEndpointWithinQuota); reason != modelendpoint.ReasonQuotaExceeded {
		t.Fatalf("WithinQuota.reason = %q, want %q", reason, modelendpoint.ReasonQuotaExceeded)
	}
	if status := conditionStatus(got.Status.Conditions, modelv1alpha1.ModelEndpointReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
}

// TestReconcile_Idempotent verifies that a second steady-state pass
// does not change phase or conditions, and observedGeneration stays
// consistent.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	prober := modelendpoint.NewNoopProber()
	me := newEndpointBuilder("gpt-4o").build()
	r, c := newReconciler(t, prober, me)
	key := types.NamespacedName{Namespace: me.Namespace, Name: me.Name}

	reconcileToSteady(t, r, key, 4)
	first := getEndpoint(t, c, key).DeepCopy()
	beforeCalls := prober.Snapshot()

	reconcileOnce(t, r, key)
	second := getEndpoint(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	if calls := prober.Snapshot(); calls <= beforeCalls {
		t.Fatalf("prober calls did not increase: before=%d after=%d", beforeCalls, calls)
	}
	if second.Status.ObservedGeneration != second.Generation {
		t.Fatalf("observedGeneration drift: %d, want %d", second.Status.ObservedGeneration, second.Generation)
	}
}

// TestReconcile_Deletion verifies the drain path: the finalizer is
// removed and the CR disappears from the API server.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()

	me := newEndpointBuilder("gpt-4o").build()
	r, c := newReconciler(t, modelendpoint.NewNoopProber(), me)
	key := types.NamespacedName{Namespace: me.Namespace, Name: me.Name}

	reconcileToSteady(t, r, key, 4)
	got := getEndpoint(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete modelendpoint: %v", err)
	}

	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}

	if err := c.Get(context.Background(), key, &modelv1alpha1.ModelEndpoint{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted CR
// returns a clean (no error, no requeue) result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	r, _ := newReconciler(t, modelendpoint.NewNoopProber())
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing endpoint: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing endpoint, got %+v", res)
	}
}
