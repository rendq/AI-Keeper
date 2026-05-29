package common_test

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

const testFinalizer = "ai-keeper.io/skill-protect"

func newSkillFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := skillv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("register skill scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestEnsureFinalizer_AddsAndIsIdempotent(t *testing.T) {
	t.Parallel()

	skill := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contract-review",
			Namespace: "default",
		},
	}
	c := newSkillFakeClient(t, skill)
	ctx := context.Background()

	// First call: adds the finalizer and persists.
	added, err := common.EnsureFinalizer(ctx, c, skill, testFinalizer)
	if err != nil {
		t.Fatalf("EnsureFinalizer: %v", err)
	}
	if !added {
		t.Fatal("expected first EnsureFinalizer call to add the finalizer")
	}

	// Re-fetch and confirm persistence.
	got := &skillv1alpha1.Skill{}
	if err := c.Get(ctx, types.NamespacedName{Name: "contract-review", Namespace: "default"}, got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !controllerutil.ContainsFinalizer(got, testFinalizer) {
		t.Fatalf("finalizer not persisted: %v", got.GetFinalizers())
	}

	// Second call: idempotent.
	added, err = common.EnsureFinalizer(ctx, c, got, testFinalizer)
	if err != nil {
		t.Fatalf("EnsureFinalizer (idempotent): %v", err)
	}
	if added {
		t.Fatal("EnsureFinalizer should be idempotent on subsequent calls")
	}
}

func TestRemoveFinalizer(t *testing.T) {
	t.Parallel()

	skill := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "contract-review",
			Namespace:  "default",
			Finalizers: []string{testFinalizer},
		},
	}
	c := newSkillFakeClient(t, skill)
	ctx := context.Background()

	removed, err := common.RemoveFinalizer(ctx, c, skill, testFinalizer)
	if err != nil {
		t.Fatalf("RemoveFinalizer: %v", err)
	}
	if !removed {
		t.Fatal("expected RemoveFinalizer to report removal")
	}
	got := &skillv1alpha1.Skill{}
	if err := c.Get(ctx, types.NamespacedName{Name: "contract-review", Namespace: "default"}, got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if controllerutil.ContainsFinalizer(got, testFinalizer) {
		t.Fatalf("finalizer should have been removed, got %v", got.GetFinalizers())
	}

	// Idempotent on second call.
	removed, err = common.RemoveFinalizer(ctx, c, got, testFinalizer)
	if err != nil {
		t.Fatalf("RemoveFinalizer (idempotent): %v", err)
	}
	if removed {
		t.Fatal("expected idempotent RemoveFinalizer to return false")
	}
}

func TestFinalize_NotDeleting_NoOp(t *testing.T) {
	t.Parallel()

	skill := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "still-alive",
			Namespace:  "default",
			Finalizers: []string{testFinalizer},
		},
	}
	c := newSkillFakeClient(t, skill)

	drainCalled := false
	res, stop, err := common.Finalize(context.Background(), c, skill, testFinalizer, func(_ context.Context) error {
		drainCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if drainCalled {
		t.Fatal("drain func must not run while DeletionTimestamp is zero")
	}
	if stop {
		t.Fatal("stop should be false on the happy reconcile path")
	}
	if res.RequeueAfter != 0 || res.Requeue {
		t.Fatalf("unexpected non-zero result: %+v", res)
	}
}

func TestFinalize_LifecycleDrainAndRemove(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	skill := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting-me",
			Namespace:         "default",
			Finalizers:        []string{testFinalizer},
			DeletionTimestamp: &now,
		},
	}
	c := newSkillFakeClient(t, skill)

	drainCalls := 0
	res, stop, err := common.Finalize(context.Background(), c, skill, testFinalizer, func(_ context.Context) error {
		drainCalls++
		return nil
	})
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if drainCalls != 1 {
		t.Fatalf("drain func must be called exactly once, got %d", drainCalls)
	}
	if !stop {
		t.Fatal("stop must be true while finalizing")
	}
	if res.RequeueAfter != 0 || res.Requeue {
		t.Fatalf("Finalize should not requeue on success: %+v", res)
	}

	// The fake client deletes objects whose finalizers are all removed
	// while DeletionTimestamp is set. A subsequent Get returns NotFound.
	got := &skillv1alpha1.Skill{}
	err = c.Get(context.Background(), types.NamespacedName{Name: "deleting-me", Namespace: "default"}, got)
	if err == nil {
		// If still present (older fake versions), the finalizer must be gone.
		if controllerutil.ContainsFinalizer(got, testFinalizer) {
			t.Fatalf("finalizer should be removed; got %v", got.GetFinalizers())
		}
	}
}

func TestFinalize_DrainErrorBubblesUp(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	skill := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting-me",
			Namespace:         "default",
			Finalizers:        []string{testFinalizer},
			DeletionTimestamp: &now,
		},
	}
	c := newSkillFakeClient(t, skill)

	wantErr := errors.New("audit not yet flushed")
	_, stop, err := common.Finalize(context.Background(), c, skill, testFinalizer, func(_ context.Context) error {
		return wantErr
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error chain should contain drain error, got %v", err)
	}
	if !stop {
		t.Fatal("stop must be true to short-circuit caller after drain error")
	}

	got := &skillv1alpha1.Skill{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "deleting-me", Namespace: "default"}, got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !controllerutil.ContainsFinalizer(got, testFinalizer) {
		t.Fatal("finalizer must remain when drain fails")
	}
}

func TestFinalize_DeletingNoFinalizer_NoOp(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	skill := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "already-cleaned",
			Namespace:         "default",
			DeletionTimestamp: &now,
		},
	}

	drainCalled := false
	// No object in the client store — Finalize should bail before any IO.
	c := newSkillFakeClient(t)
	_, stop, err := common.Finalize(context.Background(), c, skill, testFinalizer, func(_ context.Context) error {
		drainCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if drainCalled {
		t.Fatal("drain must not be invoked when finalizer is absent")
	}
	if !stop {
		t.Fatal("stop must be true on the deletion-already-progressed path")
	}
}

func TestEnsureFinalizer_RejectsEmptyName(t *testing.T) {
	t.Parallel()

	c := newSkillFakeClient(t)
	_, err := common.EnsureFinalizer(context.Background(), c, &skillv1alpha1.Skill{}, "")
	if err == nil {
		t.Fatal("expected error for empty finalizer name")
	}
}
