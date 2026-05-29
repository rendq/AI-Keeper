package tool

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the Tool CR.
const (
	// EventReasonRegistered is published the first time the Tool is
	// upserted into Tool_Registry.
	EventReasonRegistered = "Registered"
	// EventReasonDegraded is published when EndpointProbed transitions
	// from True to False.
	EventReasonDegraded = "Degraded"
	// EventReasonRecovered is published when EndpointProbed transitions
	// from False back to True.
	EventReasonRecovered = "Recovered"
	// EventReasonReady is published the first time aggregate Ready
	// flips to True.
	EventReasonReady = "ToolReady"
)

// ToolReconciler implements the Tool state machine documented in
// design.md §6.5 / Requirement A7.3.
type ToolReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// Prober checks endpoint reachability. Defaults to a [NoopProber]
	// when nil so unit tests can construct the reconciler from a bare
	// struct.
	Prober Prober

	// Registry persists Tool entries. Defaults to a process-local
	// [MemoryRegistry] when nil.
	Registry Registry
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
func (r *ToolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("tool: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("tool-controller").
		For(&skillv1alpha1.Tool{}).
		Complete(r)
}

// applyDefaults wires the no-op stand-ins when the operator did not
// supply real implementations.
func (r *ToolReconciler) applyDefaults() {
	if r.Prober == nil {
		r.Prober = NewNoopProber()
	}
	if r.Registry == nil {
		r.Registry = NewMemoryRegistry()
	}
}

// Reconcile runs one reconciliation pass for a Tool CR.
func (r *ToolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("tool", req.NamespacedName)

	tool := &skillv1alpha1.Tool{}
	if err := r.Get(ctx, req.NamespacedName, tool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("tool: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path.
	if !tool.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, tool)
	}

	// 2) Ensure the protect finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, tool, FinalizerToolProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	wasReady := common.IsReady(tool)

	// 3) Endpoint probe.
	wasReachable := common.IsConditionTrue(tool, skillv1alpha1.ToolEndpointProbed)
	reachable, probeErr := r.Prober.Probe(ctx, tool)
	now := metav1.Now()
	tool.Status.LastProbeAt = &now
	if probeErr != nil {
		// Transport-level / programming error — record it as False but
		// requeue with backoff so transient failures recover quickly.
		flag := false
		tool.Status.Reachable = &flag
		common.SetCondition(tool, skillv1alpha1.ToolEndpointProbed,
			string(metav1.ConditionFalse), ReasonProbeFailed, truncateErr(probeErr))
		r.aggregate(tool)
		if wasReachable {
			r.eventf(tool, corev1.EventTypeWarning, EventReasonDegraded,
				"endpoint %q probe failed: %v", tool.Spec.Endpoint, probeErr)
		}
		return r.writeStatus(ctx, tool, common.RequeueWithBackoff(0))
	}
	tool.Status.Reachable = boolPtr(reachable)
	if reachable {
		common.SetCondition(tool, skillv1alpha1.ToolEndpointProbed,
			string(metav1.ConditionTrue), ReasonProbeOK,
			fmt.Sprintf("endpoint %q reachable", tool.Spec.Endpoint))
		if !wasReachable && hasCondition(tool, skillv1alpha1.ToolEndpointProbed) {
			r.eventf(tool, corev1.EventTypeNormal, EventReasonRecovered,
				"endpoint %q reachable again", tool.Spec.Endpoint)
		}
	} else {
		common.SetCondition(tool, skillv1alpha1.ToolEndpointProbed,
			string(metav1.ConditionFalse), ReasonProbeFailed,
			fmt.Sprintf("endpoint %q reported a server error", tool.Spec.Endpoint))
		if wasReachable {
			r.eventf(tool, corev1.EventTypeWarning, EventReasonDegraded,
				"endpoint %q reported a server error", tool.Spec.Endpoint)
		}
	}

	// 4) Schema parsed — admission already validated; declare True.
	common.SetCondition(tool, skillv1alpha1.ToolSchemaParsed,
		string(metav1.ConditionTrue), ReasonSchemaAccepted,
		"schema accepted at admission")

	// 5) Registry upsert. We push regardless of reachability so the
	//    Registry can surface the entry as `unreachable` via its own
	//    health view.
	wasRegistered := common.IsConditionTrue(tool, skillv1alpha1.ToolRegistered)
	if err := r.Registry.Register(ctx, tool); err != nil {
		common.SetCondition(tool, skillv1alpha1.ToolRegistered,
			string(metav1.ConditionFalse), ReasonRegistrationFailed, truncateErr(err))
		r.aggregate(tool)
		return r.writeStatus(ctx, tool, common.RequeueWithBackoff(0))
	}
	common.SetCondition(tool, skillv1alpha1.ToolRegistered,
		string(metav1.ConditionTrue), ReasonRegistered,
		fmt.Sprintf("registered at Tool_Registry as %q", refForTool(tool)))
	if !wasRegistered {
		r.eventf(tool, corev1.EventTypeNormal, EventReasonRegistered,
			"Tool registered at Tool_Registry")
	}

	// 6) Approval gate (Requirement A9.2 lint mirrored at runtime).
	if tool.Spec.Governance.SideEffects == SideEffectsDestructive && !requiresApproval(tool) {
		// Defensive — admission webhook should already block this. Stay
		// in Failed phase until the operator flips requiresApproval.
		common.SetCondition(tool, skillv1alpha1.ToolApprovalConfigured,
			string(metav1.ConditionFalse), ReasonApprovalMissing,
			"sideEffects=destructive requires governance.requiresApproval=true")
	} else {
		common.SetCondition(tool, skillv1alpha1.ToolApprovalConfigured,
			string(metav1.ConditionTrue), ReasonApprovalConfigured,
			"approval policy consistent with governance.sideEffects")
	}

	// 7) Aggregate Ready + derive phase.
	r.aggregate(tool)
	if !wasReady && common.IsReady(tool) {
		r.eventf(tool, corev1.EventTypeNormal, EventReasonReady,
			"Tool %q is Ready", tool.Name)
	}

	return r.writeStatus(ctx, tool, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drives the drain flow: deregister from Tool_Registry
// and remove the finalizer.
func (r *ToolReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, tool *skillv1alpha1.Tool) (ctrl.Result, error) {
	if tool.Status.Phase != sharedv1alpha1.PhaseTerminating {
		tool.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, tool); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("tool: status update on terminating: %w", err)
		}
	}

	ref, err := ToolResourceRef(tool)
	if err == nil {
		if derr := r.Registry.Deregister(ctx, ref); derr != nil && !errors.Is(derr, ErrToolNotRegistered) {
			return common.RequeueWithBackoff(0), fmt.Errorf("tool: deregister: %w", derr)
		}
	}

	if _, err := common.RemoveFinalizer(ctx, r.Client, tool, FinalizerToolProtect); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("tool: finalizer removed", "name", tool.Name)
	return ctrl.Result{}, nil
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *ToolReconciler) aggregate(tool *skillv1alpha1.Tool) {
	status, reason, message := readyFromConditions(tool)
	common.SetCondition(tool, skillv1alpha1.ToolReady, status, reason, message)
	tool.Status.Phase = derivePhase(tool)
	tool.Status.ObservedGeneration = tool.Generation
}

// writeStatus persists the in-memory status block to the API server.
func (r *ToolReconciler) writeStatus(ctx context.Context, tool *skillv1alpha1.Tool, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, tool); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("tool: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *ToolReconciler) eventf(tool *skillv1alpha1.Tool, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(tool, eventType, reason, msg, args...)
}

// hasCondition reports whether the named condition is present.
func hasCondition(tool *skillv1alpha1.Tool, t string) bool {
	return condition(tool.Status.Conditions, t) != nil
}

// refForTool returns the canonical ResourceRef string for messages.
func refForTool(tool *skillv1alpha1.Tool) string {
	ref, err := ToolResourceRef(tool)
	if err != nil {
		return tool.Namespace + "/" + tool.Name
	}
	return string(ref)
}

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

// truncateErr clips long messages so they fit in a Condition message
// field without exceeding K8s API server payload limits.
func truncateErr(err error) string {
	const max = 240
	s := err.Error()
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Compile-time interface assertions.
var _ reconcile.Reconciler = (*ToolReconciler)(nil)
