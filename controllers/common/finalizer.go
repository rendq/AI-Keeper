package common

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DrainFunc is the per-controller cleanup hook invoked by [Finalize] once
// `metadata.deletionTimestamp` has been set. The function should be
// idempotent — Finalize re-invokes it every reconcile until it returns
// nil and the finalizer can be removed safely.
//
// design.md §6.1.5 / §6.2.4 spell out the controller-specific drain steps;
// this signature is what those drain orchestrators must implement.
type DrainFunc func(ctx context.Context) error

// EnsureFinalizer adds `name` to `obj.GetFinalizers()` (if missing) and
// patches the change back to the API server. Returns `added=true` only
// when the call mutated the object; in that case the controller should
// short-circuit and requeue so the patched generation drives the next
// reconcile pass.
//
// The patch is applied via [client.Client.Update] (not Patch) because
// finalizers live on the object metadata and benefit from optimistic
// concurrency — a stale write should fail fast and be retried by the
// controller-runtime workqueue rather than silently merge.
func EnsureFinalizer(ctx context.Context, c client.Client, obj client.Object, name string) (added bool, err error) {
	if obj == nil {
		return false, fmt.Errorf("EnsureFinalizer: nil object")
	}
	if name == "" {
		return false, fmt.Errorf("EnsureFinalizer: empty finalizer name")
	}
	if controllerutil.ContainsFinalizer(obj, name) {
		return false, nil
	}
	if !controllerutil.AddFinalizer(obj, name) {
		// AddFinalizer returns false either if already present (covered
		// above) or if the object cannot accept finalizers — we treat
		// the second case as a controller programming error.
		return false, fmt.Errorf("EnsureFinalizer: AddFinalizer rejected %q on %T", name, obj)
	}
	if err := c.Update(ctx, obj); err != nil {
		return false, fmt.Errorf("EnsureFinalizer: update %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
	}
	return true, nil
}

// RemoveFinalizer is the symmetric counterpart of [EnsureFinalizer]. It
// reports `removed=true` iff the finalizer was present and the patch was
// accepted by the server.
func RemoveFinalizer(ctx context.Context, c client.Client, obj client.Object, name string) (removed bool, err error) {
	if obj == nil {
		return false, fmt.Errorf("RemoveFinalizer: nil object")
	}
	if name == "" {
		return false, fmt.Errorf("RemoveFinalizer: empty finalizer name")
	}
	if !controllerutil.ContainsFinalizer(obj, name) {
		return false, nil
	}
	if !controllerutil.RemoveFinalizer(obj, name) {
		return false, fmt.Errorf("RemoveFinalizer: RemoveFinalizer rejected %q on %T", name, obj)
	}
	if err := c.Update(ctx, obj); err != nil {
		return false, fmt.Errorf("RemoveFinalizer: update %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
	}
	return true, nil
}

// Finalize orchestrates the deletion path that is shared across every
// AIP controller (design.md §6.1.5 — Skill / §6.2.4 — Agent / etc):
//
//   - When `obj.DeletionTimestamp` is nil, the function is a no-op and
//     returns `(ctrl.Result{}, nil)` so the caller can continue with the
//     normal reconcile path.
//   - When `obj.DeletionTimestamp` is set but the finalizer is absent,
//     deletion is already in flight and there is nothing left for this
//     controller to do; the function returns `(ctrl.Result{}, nil)`.
//   - When the finalizer is present, the function invokes `do(ctx)` (the
//     controller-specific drain logic) and, on success, removes the
//     finalizer via [RemoveFinalizer]. A drain error is bubbled up
//     unchanged so the controller-runtime workqueue can apply backoff.
//
// The boolean second return value (`stop`) tells the caller whether the
// reconcile is finished. `stop=true` means the caller MUST return the
// `ctrl.Result` immediately and skip the rest of its reconcile.
func Finalize(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	name string,
	do DrainFunc,
) (result ctrl.Result, stop bool, err error) {
	if obj == nil {
		return ctrl.Result{}, false, fmt.Errorf("Finalize: nil object")
	}
	if name == "" {
		return ctrl.Result{}, false, fmt.Errorf("Finalize: empty finalizer name")
	}

	// Not being deleted — the caller continues with normal reconcile.
	if obj.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, false, nil
	}

	// Deleting, finalizer already removed — nothing to do.
	if !controllerutil.ContainsFinalizer(obj, name) {
		return ctrl.Result{}, true, nil
	}

	// Run the per-controller drain. Errors return without removing the
	// finalizer; controller-runtime retries with workqueue backoff.
	if do != nil {
		if err := do(ctx); err != nil {
			return ctrl.Result{}, true, fmt.Errorf("Finalize: drain %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
		}
	}

	if _, err := RemoveFinalizer(ctx, c, obj, name); err != nil {
		return ctrl.Result{}, true, err
	}
	return ctrl.Result{}, true, nil
}
