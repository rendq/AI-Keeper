package tool_test

import (
	"context"
	"errors"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/tool"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := skillv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register skill.ai-keeper.io scheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&skillv1alpha1.Tool{}).
		Build()
	return c, s
}

func emptyJSON() *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(`{}`)}
}

type toolBuilder struct {
	t *skillv1alpha1.Tool
}

func newToolBuilder(name string) *toolBuilder {
	approval := false
	return &toolBuilder{
		t: &skillv1alpha1.Tool{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "tenant-acme",
				Generation: 1,
			},
			Spec: skillv1alpha1.ToolSpec{
				Protocol: "http",
				Endpoint: "https://tool.example.com",
				Schema: skillv1alpha1.ToolSchema{
					Input:  emptyJSON(),
					Output: emptyJSON(),
				},
				Governance: skillv1alpha1.ToolGovernance{
					SideEffects:      tool.SideEffectsReadOnly,
					RequiresApproval: &approval,
				},
			},
		},
	}
}

func (b *toolBuilder) withSideEffects(se string) *toolBuilder {
	b.t.Spec.Governance.SideEffects = se
	return b
}

func (b *toolBuilder) withRequiresApproval(v bool) *toolBuilder {
	b.t.Spec.Governance.RequiresApproval = &v
	return b
}

func (b *toolBuilder) withDeletionTimestamp() *toolBuilder {
	now := metav1.Now()
	b.t.DeletionTimestamp = &now
	if !controllerutil.ContainsFinalizer(b.t, tool.FinalizerToolProtect) {
		controllerutil.AddFinalizer(b.t, tool.FinalizerToolProtect)
	}
	return b
}

func (b *toolBuilder) build() *skillv1alpha1.Tool { return b.t }

func newReconciler(t *testing.T, prober tool.Prober, registry tool.Registry, objs ...client.Object) (*tool.ToolReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &tool.ToolReconciler{
		Client:   c,
		Scheme:   s,
		Prober:   prober,
		Registry: registry,
	}, c
}

func reconcileOnce(t *testing.T, r *tool.ToolReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *tool.ToolReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getTool(t *testing.T, c client.Client, key types.NamespacedName) *skillv1alpha1.Tool {
	t.Helper()
	got := &skillv1alpha1.Tool{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get tool: %v", err)
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

// TestReconcile_HappyPath_ReadOnly exercises Requirement A7.3: a
// reachable read-only Tool flips through every gate, gets registered,
// and reaches Phase=Active with `status.reachable=true`.
func TestReconcile_HappyPath_ReadOnly(t *testing.T) {
	t.Parallel()

	prober := tool.NewNoopProber()
	registry := tool.NewMemoryRegistry()
	tl := newToolBuilder("docusign-mcp").build()

	r, c := newReconciler(t, prober, registry, tl)
	key := types.NamespacedName{Namespace: tl.Namespace, Name: tl.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getTool(t, c, key)
	if !controllerutil.ContainsFinalizer(got, tool.FinalizerToolProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	gates := []string{
		skillv1alpha1.ToolEndpointProbed,
		skillv1alpha1.ToolSchemaParsed,
		skillv1alpha1.ToolRegistered,
		skillv1alpha1.ToolApprovalConfigured,
		skillv1alpha1.ToolReady,
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
	if got.Status.Reachable == nil || !*got.Status.Reachable {
		t.Fatalf("status.reachable = %v, want true", got.Status.Reachable)
	}
	if got.Status.LastProbeAt == nil {
		t.Fatalf("status.lastProbeAt is nil; want non-nil")
	}
	if last.RequeueAfter != tool.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, tool.SteadyStateRequeue)
	}

	// Registry must record the entry exactly once.
	entries := registry.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("registry size = %d, want 1", len(entries))
	}
	if entries[0].Endpoint != tl.Spec.Endpoint {
		t.Fatalf("registry entry endpoint = %q, want %q", entries[0].Endpoint, tl.Spec.Endpoint)
	}

	// Probe must have been called at least once.
	if calls := prober.Snapshot(); calls == 0 {
		t.Fatalf("prober calls = %d, want > 0", calls)
	}
}

// TestReconcile_DestructiveWithApproval covers the safe path for a
// destructive tool: requiresApproval=true → ApprovalConfigured=True →
// Phase=Active.
func TestReconcile_DestructiveWithApproval(t *testing.T) {
	t.Parallel()

	tl := newToolBuilder("docusign-sign").
		withSideEffects(tool.SideEffectsDestructive).
		withRequiresApproval(true).
		build()
	r, c := newReconciler(t, tool.NewNoopProber(), tool.NewMemoryRegistry(), tl)
	key := types.NamespacedName{Namespace: tl.Namespace, Name: tl.Name}

	reconcileToSteady(t, r, key, 4)

	got := getTool(t, c, key)
	if status := conditionStatus(got.Status.Conditions, skillv1alpha1.ToolApprovalConfigured); status != metav1.ConditionTrue {
		t.Fatalf("ApprovalConfigured = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
}

// TestReconcile_DestructiveWithoutApproval is the defensive case:
// admission should block this CR but the reconciler must surface it
// as ApprovalConfigured=False and refuse Ready=True. Phase=Failed.
func TestReconcile_DestructiveWithoutApproval(t *testing.T) {
	t.Parallel()

	tl := newToolBuilder("docusign-sign").
		withSideEffects(tool.SideEffectsDestructive).
		withRequiresApproval(false).
		build()
	r, c := newReconciler(t, tool.NewNoopProber(), tool.NewMemoryRegistry(), tl)
	key := types.NamespacedName{Namespace: tl.Namespace, Name: tl.Name}

	reconcileToSteady(t, r, key, 4)

	got := getTool(t, c, key)
	if status := conditionStatus(got.Status.Conditions, skillv1alpha1.ToolApprovalConfigured); status != metav1.ConditionFalse {
		t.Fatalf("ApprovalConfigured = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, skillv1alpha1.ToolApprovalConfigured); reason != tool.ReasonApprovalMissing {
		t.Fatalf("ApprovalConfigured.reason = %q, want %q", reason, tool.ReasonApprovalMissing)
	}
	if status := conditionStatus(got.Status.Conditions, skillv1alpha1.ToolReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
}

// TestReconcile_UnreachableEndpoint covers Requirement A7.3 (probe
// failure → Degraded). The Tool is still pushed into the Registry so
// the rest of the platform can see the entry, but the aggregate Ready
// gate stays False.
func TestReconcile_UnreachableEndpoint(t *testing.T) {
	t.Parallel()

	prober := &tool.NoopProber{Reachable: false}
	tl := newToolBuilder("flaky-tool").build()
	r, c := newReconciler(t, prober, tool.NewMemoryRegistry(), tl)
	key := types.NamespacedName{Namespace: tl.Namespace, Name: tl.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getTool(t, c, key)
	if status := conditionStatus(got.Status.Conditions, skillv1alpha1.ToolEndpointProbed); status != metav1.ConditionFalse {
		t.Fatalf("EndpointProbed = %s, want False", status)
	}
	if got.Status.Reachable == nil || *got.Status.Reachable {
		t.Fatalf("status.reachable = %v, want false", got.Status.Reachable)
	}
	if status := conditionStatus(got.Status.Conditions, skillv1alpha1.ToolReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseDegraded {
		t.Fatalf("phase = %s, want Degraded", got.Status.Phase)
	}
	// Steady-state requeue still applies on the unreachable path so
	// the next probe runs in 30s.
	if last.RequeueAfter != tool.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, tool.SteadyStateRequeue)
	}
}

// TestReconcile_ProbeError covers the transport-level failure path
// (DNS, TLS, ...). The reconciler MUST requeue with backoff so retries
// happen sooner than the 30s cadence.
func TestReconcile_ProbeError(t *testing.T) {
	t.Parallel()

	prober := &tool.NoopProber{Err: errors.New("dns: lookup failed")}
	tl := newToolBuilder("broken-tool").build()
	r, c := newReconciler(t, prober, tool.NewMemoryRegistry(), tl)
	key := types.NamespacedName{Namespace: tl.Namespace, Name: tl.Name}

	// Drive past the finalizer-add requeue then run one more pass.
	reconcileOnce(t, r, key) // adds finalizer, requeues
	last := reconcileOnce(t, r, key)

	got := getTool(t, c, key)
	if status := conditionStatus(got.Status.Conditions, skillv1alpha1.ToolEndpointProbed); status != metav1.ConditionFalse {
		t.Fatalf("EndpointProbed = %s, want False", status)
	}
	if last.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0 on transient probe error")
	}
}

// TestReconcile_Idempotent verifies that a second steady-state pass
// does not change the Tool's condition / phase / observedGeneration
// or the Registry size.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	prober := tool.NewNoopProber()
	registry := tool.NewMemoryRegistry()
	tl := newToolBuilder("docusign-mcp").build()

	r, c := newReconciler(t, prober, registry, tl)
	key := types.NamespacedName{Namespace: tl.Namespace, Name: tl.Name}

	reconcileToSteady(t, r, key, 4)
	first := getTool(t, c, key).DeepCopy()
	beforeCalls := prober.Snapshot()
	beforeRegistry := len(registry.Snapshot())

	reconcileOnce(t, r, key)
	second := getTool(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed across reconciles: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	if afterRegistry := len(registry.Snapshot()); afterRegistry != beforeRegistry {
		t.Fatalf("registry size changed: %d → %d", beforeRegistry, afterRegistry)
	}
	if calls := prober.Snapshot(); calls <= beforeCalls {
		t.Fatalf("prober calls did not increase: before=%d after=%d", beforeCalls, calls)
	}
	if second.Status.ObservedGeneration != second.Generation {
		t.Fatalf("observedGeneration drift: %d, want %d", second.Status.ObservedGeneration, second.Generation)
	}
}

// TestReconcile_Deletion verifies the drain path: Tool is deregistered
// from the Registry and the finalizer is removed.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()

	prober := tool.NewNoopProber()
	registry := tool.NewMemoryRegistry()
	tl := newToolBuilder("docusign-mcp").build()

	r, c := newReconciler(t, prober, registry, tl)
	key := types.NamespacedName{Namespace: tl.Namespace, Name: tl.Name}

	reconcileToSteady(t, r, key, 4)
	if len(registry.Snapshot()) != 1 {
		t.Fatalf("registry should have 1 entry before deletion, got %d", len(registry.Snapshot()))
	}

	got := getTool(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete tool: %v", err)
	}

	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}

	if err := c.Get(context.Background(), key, &skillv1alpha1.Tool{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}
	if len(registry.Snapshot()) != 0 {
		t.Fatalf("registry size after deletion = %d, want 0", len(registry.Snapshot()))
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted tool
// returns a clean (no error, no requeue) result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	r, _ := newReconciler(t, tool.NewNoopProber(), tool.NewMemoryRegistry())
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing tool: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing tool, got %+v", res)
	}
}
