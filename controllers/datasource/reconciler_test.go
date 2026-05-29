package datasource_test

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

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/datasource"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := datav1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register data.ai-keeper.io scheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&datav1alpha1.DataSource{}).
		Build()
	return c, s
}

type dsBuilder struct {
	ds *datav1alpha1.DataSource
}

func newDataSourceBuilder(name string) *dsBuilder {
	return &dsBuilder{
		ds: &datav1alpha1.DataSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "tenant-acme",
				Generation: 1,
			},
			Spec: datav1alpha1.DataSourceSpec{
				Connector: datav1alpha1.DataSourceConnector{
					Kind: "feishu_wiki",
				},
				ACL: &datav1alpha1.DataSourceACL{
					Mode: "inherit_from_source",
				},
			},
		},
	}
}

func (b *dsBuilder) withACL(mode string) *dsBuilder {
	if mode == "" {
		b.ds.Spec.ACL = nil
		return b
	}
	b.ds.Spec.ACL = &datav1alpha1.DataSourceACL{Mode: mode}
	return b
}

func (b *dsBuilder) withDeletionTimestamp() *dsBuilder {
	now := metav1.Now()
	b.ds.DeletionTimestamp = &now
	if !controllerutil.ContainsFinalizer(b.ds, datasource.FinalizerDataSourceProtect) {
		controllerutil.AddFinalizer(b.ds, datasource.FinalizerDataSourceProtect)
	}
	return b
}

func (b *dsBuilder) build() *datav1alpha1.DataSource { return b.ds }

func newReconciler(t *testing.T, connector datasource.ConnectorClient, objs ...client.Object) (*datasource.DataSourceReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &datasource.DataSourceReconciler{
		Client:    c,
		Scheme:    s,
		Connector: connector,
	}, c
}

func reconcileOnce(t *testing.T, r *datasource.DataSourceReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *datasource.DataSourceReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getDataSource(t *testing.T, c client.Client, key types.NamespacedName) *datav1alpha1.DataSource {
	t.Helper()
	got := &datav1alpha1.DataSource{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get datasource: %v", err)
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

// TestReconcile_HappyPath exercises Requirement A7.4: a healthy
// connector + ACL set → Phase=Active with `status.connected=true`,
// document counts populated and `lastSyncAt` non-nil.
func TestReconcile_HappyPath(t *testing.T) {
	t.Parallel()

	connector := datasource.NewNoopConnector()
	connector.Info.DocumentCount = 42
	connector.Info.SizeBytes = 8192
	ds := newDataSourceBuilder("legal-kb-source").build()

	r, c := newReconciler(t, connector, ds)
	key := types.NamespacedName{Namespace: ds.Namespace, Name: ds.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getDataSource(t, c, key)
	if !controllerutil.ContainsFinalizer(got, datasource.FinalizerDataSourceProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.DataSourceConnected); status != metav1.ConditionTrue {
		t.Fatalf("Connected = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.DataSourceACLEnforced); status != metav1.ConditionTrue {
		t.Fatalf("ACLEnforced = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.DataSourceReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.Connected == nil || !*got.Status.Connected {
		t.Fatalf("status.connected = %v, want true", got.Status.Connected)
	}
	if got.Status.DocumentCount != 42 {
		t.Fatalf("status.documentCount = %d, want 42", got.Status.DocumentCount)
	}
	if got.Status.SizeBytes != 8192 {
		t.Fatalf("status.sizeBytes = %d, want 8192", got.Status.SizeBytes)
	}
	if got.Status.LastSyncAt == nil {
		t.Fatalf("status.lastSyncAt is nil")
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
	if last.RequeueAfter != datasource.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, datasource.SteadyStateRequeue)
	}
}

// TestReconcile_NoACL covers the "operator opted out" path: no
// `spec.acl` block → ACLEnforced=Unknown reason=ACLNotConfigured.
// Aggregate Ready stays True so Phase=Active.
func TestReconcile_NoACL(t *testing.T) {
	t.Parallel()

	ds := newDataSourceBuilder("legal-kb-source").withACL("").build()
	r, c := newReconciler(t, datasource.NewNoopConnector(), ds)
	key := types.NamespacedName{Namespace: ds.Namespace, Name: ds.Name}

	reconcileToSteady(t, r, key, 4)

	got := getDataSource(t, c, key)
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.DataSourceACLEnforced); status != metav1.ConditionUnknown {
		t.Fatalf("ACLEnforced = %s, want Unknown", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
}

// TestReconcile_ConnectorFailure exercises the transient backoff path:
// a connector failure flips Connected=False and triggers a backoff
// requeue (sub-30s).
func TestReconcile_ConnectorFailure(t *testing.T) {
	t.Parallel()

	connector := &datasource.NoopConnector{Err: errors.New("connection refused")}
	ds := newDataSourceBuilder("flaky-source").build()
	r, c := newReconciler(t, connector, ds)
	key := types.NamespacedName{Namespace: ds.Namespace, Name: ds.Name}

	// First pass adds finalizer + requeues; second pass fails connect.
	reconcileOnce(t, r, key)
	last := reconcileOnce(t, r, key)

	got := getDataSource(t, c, key)
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.DataSourceConnected); status != metav1.ConditionFalse {
		t.Fatalf("Connected = %s, want False", status)
	}
	if got.Status.Connected == nil || *got.Status.Connected {
		t.Fatalf("status.connected = %v, want false", got.Status.Connected)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseDegraded {
		t.Fatalf("phase = %s, want Degraded", got.Status.Phase)
	}
	if last.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0 on connector failure")
	}
	// Backoff should be tighter than the steady-state cadence.
	if last.RequeueAfter >= datasource.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want < steady-state %s", last.RequeueAfter, datasource.SteadyStateRequeue)
	}
}

// TestReconcile_Idempotent verifies the second steady-state pass does
// not change phase / conditions / observedGeneration.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	connector := datasource.NewNoopConnector()
	ds := newDataSourceBuilder("legal-kb-source").build()
	r, c := newReconciler(t, connector, ds)
	key := types.NamespacedName{Namespace: ds.Namespace, Name: ds.Name}

	reconcileToSteady(t, r, key, 4)
	first := getDataSource(t, c, key).DeepCopy()

	reconcileOnce(t, r, key)
	second := getDataSource(t, c, key)

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

// TestReconcile_Deletion verifies the drain path: finalizer is removed
// and the CR disappears from the API server.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()

	ds := newDataSourceBuilder("legal-kb-source").build()
	r, c := newReconciler(t, datasource.NewNoopConnector(), ds)
	key := types.NamespacedName{Namespace: ds.Namespace, Name: ds.Name}

	reconcileToSteady(t, r, key, 4)
	got := getDataSource(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete datasource: %v", err)
	}

	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}
	if err := c.Get(context.Background(), key, &datav1alpha1.DataSource{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted CR
// returns a clean (no error, no requeue) result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	r, _ := newReconciler(t, datasource.NewNoopConnector())
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing datasource: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing datasource, got %+v", res)
	}
}
