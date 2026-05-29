package knowledgebase_test

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
	"github.com/ai-keeper/ai-keeper/controllers/knowledgebase"
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
		WithStatusSubresource(
			&datav1alpha1.KnowledgeBase{},
			&datav1alpha1.DataSource{},
		).
		Build()
	return c, s
}

func newDataSource(name, namespace string) *datav1alpha1.DataSource {
	return &datav1alpha1.DataSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: datav1alpha1.DataSourceSpec{
			Connector: datav1alpha1.DataSourceConnector{Kind: "feishu_wiki"},
		},
	}
}

type kbBuilder struct {
	kb *datav1alpha1.KnowledgeBase
}

func newKBBuilder(name string) *kbBuilder {
	return &kbBuilder{
		kb: &datav1alpha1.KnowledgeBase{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "tenant-acme",
				Generation: 1,
			},
			Spec: datav1alpha1.KnowledgeBaseSpec{
				Sources: []datav1alpha1.KBSource{
					{Ref: sharedv1alpha1.ResourceRef("data://legal-kb-source")},
				},
				Pipeline: datav1alpha1.KBPipeline{},
				Index: datav1alpha1.KBIndex{
					VectorStore: sharedv1alpha1.ResourceRef("ref://stores/qdrant"),
				},
				ACL: &datav1alpha1.KBACL{Mode: knowledgebase.ACLModeInheritFromSource},
			},
		},
	}
}

func (b *kbBuilder) withSources(refs ...string) *kbBuilder {
	b.kb.Spec.Sources = nil
	for _, r := range refs {
		b.kb.Spec.Sources = append(b.kb.Spec.Sources, datav1alpha1.KBSource{
			Ref: sharedv1alpha1.ResourceRef(r),
		})
	}
	return b
}

func (b *kbBuilder) withClassification(c sharedv1alpha1.Classification) *kbBuilder {
	if b.kb.Spec.Governance == nil {
		b.kb.Spec.Governance = &datav1alpha1.KBGovernance{}
	}
	b.kb.Spec.Governance.Classification = &c
	return b
}

func (b *kbBuilder) withACL(mode string) *kbBuilder {
	if mode == "" {
		b.kb.Spec.ACL = nil
		return b
	}
	b.kb.Spec.ACL = &datav1alpha1.KBACL{Mode: mode}
	return b
}

func (b *kbBuilder) build() *datav1alpha1.KnowledgeBase { return b.kb }

func newReconciler(t *testing.T, pipeline knowledgebase.Pipeline, objs ...client.Object) (*knowledgebase.KnowledgeBaseReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &knowledgebase.KnowledgeBaseReconciler{
		Client:   c,
		Scheme:   s,
		Pipeline: pipeline,
	}, c
}

func reconcileOnce(t *testing.T, r *knowledgebase.KnowledgeBaseReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *knowledgebase.KnowledgeBaseReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getKB(t *testing.T, c client.Client, key types.NamespacedName) *datav1alpha1.KnowledgeBase {
	t.Helper()
	got := &datav1alpha1.KnowledgeBase{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get knowledgebase: %v", err)
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

// TestReconcile_HappyPath exercises Requirement A7.5: a KB referencing
// one DataSource, with a non-empty pipeline, reaches Phase=Active.
func TestReconcile_HappyPath(t *testing.T) {
	t.Parallel()

	pipeline := knowledgebase.NewNoopPipeline()
	pipeline.Info.ChunkCount = 1024
	pipeline.Info.IndexSizeBytes = 4 * 1024 * 1024

	kb := newKBBuilder("legal-kb").build()
	ds := newDataSource("legal-kb-source", kb.Namespace)

	r, c := newReconciler(t, pipeline, kb, ds)
	key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getKB(t, c, key)
	if !controllerutil.ContainsFinalizer(got, knowledgebase.FinalizerKnowledgeBaseProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	gates := []string{
		datav1alpha1.KnowledgeBaseSourcesReady,
		datav1alpha1.KnowledgeBaseIndexed,
		datav1alpha1.KnowledgeBaseReady,
	}
	for _, g := range gates {
		if status := conditionStatus(got.Status.Conditions, g); status != metav1.ConditionTrue {
			t.Fatalf("%s = %s, want True", g, status)
		}
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.ChunkCount != 1024 {
		t.Fatalf("status.chunkCount = %d, want 1024", got.Status.ChunkCount)
	}
	if got.Status.IndexSizeBytes != 4*1024*1024 {
		t.Fatalf("status.indexSizeBytes = %d, want %d", got.Status.IndexSizeBytes, 4*1024*1024)
	}
	if got.Status.LastIndexedAt == nil {
		t.Fatalf("status.lastIndexedAt is nil")
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
	if last.RequeueAfter != knowledgebase.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, knowledgebase.SteadyStateRequeue)
	}
}

// TestReconcile_MissingDataSource covers Requirement A7.5: a KB with
// an unresolvable `sources[].ref` flips SourcesReady=False and stays
// Phase=Degraded with a backoff requeue so the KB recovers as soon as
// the DataSource appears.
func TestReconcile_MissingDataSource(t *testing.T) {
	t.Parallel()

	kb := newKBBuilder("legal-kb").build()
	r, c := newReconciler(t, knowledgebase.NewNoopPipeline(), kb)
	key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Name}

	// First pass adds finalizer + requeues; second pass fails source resolve.
	reconcileOnce(t, r, key)
	last := reconcileOnce(t, r, key)

	got := getKB(t, c, key)
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.KnowledgeBaseSourcesReady); status != metav1.ConditionFalse {
		t.Fatalf("SourcesReady = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, datav1alpha1.KnowledgeBaseSourcesReady); reason != knowledgebase.ReasonSourceMissing {
		t.Fatalf("SourcesReady.reason = %q, want %q", reason, knowledgebase.ReasonSourceMissing)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseDegraded {
		t.Fatalf("phase = %s, want Degraded", got.Status.Phase)
	}
	if last.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0 on missing DataSource")
	}
}

// TestReconcile_InvalidACL covers the lint rule `kb/acl-not-open`:
// classification ≥ confidential combined with `acl.mode=open` is
// rejected → Phase=Failed, SourcesReady=False reason=InvalidACL.
func TestReconcile_InvalidACL(t *testing.T) {
	t.Parallel()

	kb := newKBBuilder("legal-kb").
		withClassification(sharedv1alpha1.ClassificationConfidential).
		withACL(knowledgebase.ACLModeOpen).
		build()
	r, c := newReconciler(t, knowledgebase.NewNoopPipeline(), kb)
	key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Name}

	reconcileToSteady(t, r, key, 4)

	got := getKB(t, c, key)
	if reason := conditionReason(got.Status.Conditions, datav1alpha1.KnowledgeBaseSourcesReady); reason != knowledgebase.ReasonInvalidACL {
		t.Fatalf("SourcesReady.reason = %q, want %q", reason, knowledgebase.ReasonInvalidACL)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.KnowledgeBaseReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
}

// TestReconcile_InvalidACL_RestrictedSecret verifies the same rule
// applies to `restricted` and `secret` classifications.
func TestReconcile_InvalidACL_RestrictedSecret(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		classification sharedv1alpha1.Classification
	}{
		{"restricted", sharedv1alpha1.ClassificationRestricted},
		{"secret", sharedv1alpha1.ClassificationSecret},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			kb := newKBBuilder("legal-kb").
				withClassification(tc.classification).
				withACL(knowledgebase.ACLModeOpen).
				build()
			r, c := newReconciler(t, knowledgebase.NewNoopPipeline(), kb)
			key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Name}

			reconcileToSteady(t, r, key, 4)
			got := getKB(t, c, key)
			if got.Status.Phase != sharedv1alpha1.PhaseFailed {
				t.Fatalf("phase = %s, want Failed", got.Status.Phase)
			}
		})
	}
}

// TestReconcile_PipelineFailure covers transient pipeline outages.
func TestReconcile_PipelineFailure(t *testing.T) {
	t.Parallel()

	pipeline := &knowledgebase.NoopPipeline{Err: errors.New("vector store unhealthy")}
	kb := newKBBuilder("legal-kb").build()
	ds := newDataSource("legal-kb-source", kb.Namespace)

	r, c := newReconciler(t, pipeline, kb, ds)
	key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Name}

	reconcileOnce(t, r, key)
	last := reconcileOnce(t, r, key)

	got := getKB(t, c, key)
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.KnowledgeBaseSourcesReady); status != metav1.ConditionTrue {
		t.Fatalf("SourcesReady = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.KnowledgeBaseIndexed); status != metav1.ConditionFalse {
		t.Fatalf("Indexed = %s, want False", status)
	}
	if status := conditionStatus(got.Status.Conditions, datav1alpha1.KnowledgeBaseReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if last.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0 on pipeline failure")
	}
}

// TestReconcile_Idempotent verifies the second steady-state pass does
// not change phase / conditions / observedGeneration.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	pipeline := knowledgebase.NewNoopPipeline()
	kb := newKBBuilder("legal-kb").build()
	ds := newDataSource("legal-kb-source", kb.Namespace)

	r, c := newReconciler(t, pipeline, kb, ds)
	key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Name}

	reconcileToSteady(t, r, key, 4)
	first := getKB(t, c, key).DeepCopy()
	beforeCalls := pipeline.Snapshot()

	reconcileOnce(t, r, key)
	second := getKB(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	if calls := pipeline.Snapshot(); calls <= beforeCalls {
		t.Fatalf("pipeline calls did not increase: before=%d after=%d", beforeCalls, calls)
	}
	if second.Status.ObservedGeneration != second.Generation {
		t.Fatalf("observedGeneration drift: %d, want %d", second.Status.ObservedGeneration, second.Generation)
	}
}

// TestReconcile_Deletion verifies the drain path: finalizer is removed
// and the CR disappears from the API server.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()

	kb := newKBBuilder("legal-kb").build()
	ds := newDataSource("legal-kb-source", kb.Namespace)
	r, c := newReconciler(t, knowledgebase.NewNoopPipeline(), kb, ds)
	key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Name}

	reconcileToSteady(t, r, key, 4)
	got := getKB(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete kb: %v", err)
	}

	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}
	if err := c.Get(context.Background(), key, &datav1alpha1.KnowledgeBase{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted CR
// returns a clean result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	r, _ := newReconciler(t, knowledgebase.NewNoopPipeline())
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing kb: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing kb, got %+v", res)
	}
}
