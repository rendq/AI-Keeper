package modelrouter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the ModelRouter CR.
const (
	// EventReasonCompiled is published the first time the router
	// compiles its rules into a routing table.
	EventReasonCompiled = "Compiled"
	// EventReasonDistributed is published the first time the table is
	// pushed to every router instance.
	EventReasonDistributed = "Distributed"
	// EventReasonDegraded is published when AllReachable transitions
	// True → False.
	EventReasonDegraded = "Degraded"
	// EventReasonReady is published the first time aggregate Ready
	// flips True.
	EventReasonReady = "ModelRouterReady"
)

// ModelRouterReconciler implements the ModelRouter state machine
// documented in design.md §6.5 / Requirement A7.7.
type ModelRouterReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// Pusher distributes compiled routing tables to live router
	// instances. Defaults to [NoopRouterPusher] when nil so the
	// controller stays operational while task 11.1 is in flight.
	Pusher RouterPusher
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
func (r *ModelRouterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("modelrouter: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("modelrouter-controller").
		For(&modelv1alpha1.ModelRouter{}).
		Complete(r)
}

// applyDefaults wires the no-op stand-ins when the operator did not
// supply real implementations.
func (r *ModelRouterReconciler) applyDefaults() {
	if r.Pusher == nil {
		r.Pusher = NoopRouterPusher{}
	}
}

// Reconcile runs one reconciliation pass for a ModelRouter CR.
func (r *ModelRouterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("modelrouter", req.NamespacedName)

	mr := &modelv1alpha1.ModelRouter{}
	if err := r.Get(ctx, req.NamespacedName, mr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("modelrouter: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path.
	if !mr.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, mr)
	}

	// 2) Ensure the protect finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, mr, FinalizerModelRouterProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	wasReady := common.IsReady(mr)
	wasReachable := common.IsConditionTrue(mr, modelv1alpha1.ModelRouterAllReachable)
	wasCompiled := common.IsConditionTrue(mr, modelv1alpha1.ModelRouterCompiled)
	wasDistributed := common.IsConditionTrue(mr, modelv1alpha1.ModelRouterDistributed)

	// 3) Endpoint resolution & reachability.
	reachableCount, totalCount, distribution, resolveErr := r.resolveEndpoints(ctx, mr)
	if resolveErr != nil {
		// Malformed ref or transport failure — record on Compiled.
		common.SetCondition(mr, modelv1alpha1.ModelRouterCompiled,
			string(metav1.ConditionFalse), ReasonCompileFailed, truncateErr(resolveErr))
		common.SetCondition(mr, modelv1alpha1.ModelRouterDistributed,
			string(metav1.ConditionFalse), ReasonDistributeFailed,
			"compile failed; nothing to distribute")
		common.SetCondition(mr, modelv1alpha1.ModelRouterAllReachable,
			string(metav1.ConditionFalse), ReasonAllUnreachable,
			"compile failed; reachability not evaluated")
		r.aggregate(mr)
		return r.writeStatus(ctx, mr, common.RequeueWithBackoff(0))
	}
	mr.Status.Distribution = distribution

	// 4) Compile routing table.
	table, compileErr := CompileRoutingTable(mr)
	if compileErr != nil {
		common.SetCondition(mr, modelv1alpha1.ModelRouterCompiled,
			string(metav1.ConditionFalse), ReasonCompileFailed, truncateErr(compileErr))
		common.SetCondition(mr, modelv1alpha1.ModelRouterDistributed,
			string(metav1.ConditionFalse), ReasonDistributeFailed,
			"compile failed; nothing to distribute")
		common.SetCondition(mr, modelv1alpha1.ModelRouterAllReachable,
			r.reachableConditionStatus(reachableCount, totalCount), reachableReason(reachableCount, totalCount),
			reachableMessage(reachableCount, totalCount))
		r.aggregate(mr)
		return r.writeStatus(ctx, mr, common.RequeueWithBackoff(0))
	}
	common.SetCondition(mr, modelv1alpha1.ModelRouterCompiled,
		string(metav1.ConditionTrue), ReasonCompiled,
		fmt.Sprintf("%d rule(s) compiled (hash=%s)", len(table.Rules), shortHash(table.Hash)))
	if !wasCompiled {
		r.eventf(mr, corev1.EventTypeNormal, EventReasonCompiled,
			"ModelRouter %q compiled %d rule(s)", mr.Spec.Alias, len(table.Rules))
	}

	// 5) Distribute to router instances.
	instances, discErr := r.Pusher.Discover(ctx)
	if discErr != nil {
		common.SetCondition(mr, modelv1alpha1.ModelRouterDistributed,
			string(metav1.ConditionFalse), ReasonDistributeFailed, truncateErr(discErr))
		common.SetCondition(mr, modelv1alpha1.ModelRouterAllReachable,
			r.reachableConditionStatus(reachableCount, totalCount), reachableReason(reachableCount, totalCount),
			reachableMessage(reachableCount, totalCount))
		r.aggregate(mr)
		return r.writeStatus(ctx, mr, common.RequeueWithBackoff(0))
	}
	if len(instances) == 0 {
		common.SetCondition(mr, modelv1alpha1.ModelRouterDistributed,
			string(metav1.ConditionUnknown), ReasonNoInstances,
			"no router instances registered yet")
	} else {
		var pushErrs []string
		for _, inst := range instances {
			if err := r.Pusher.Push(ctx, inst, table); err != nil {
				pushErrs = append(pushErrs, fmt.Sprintf("%s: %v", inst.ID, err))
			}
		}
		if len(pushErrs) > 0 {
			common.SetCondition(mr, modelv1alpha1.ModelRouterDistributed,
				string(metav1.ConditionFalse), ReasonDistributeFailed,
				fmt.Sprintf("%d/%d push(es) failed: %s", len(pushErrs), len(instances), strings.Join(pushErrs, "; ")))
			common.SetCondition(mr, modelv1alpha1.ModelRouterAllReachable,
				r.reachableConditionStatus(reachableCount, totalCount), reachableReason(reachableCount, totalCount),
				reachableMessage(reachableCount, totalCount))
			r.aggregate(mr)
			return r.writeStatus(ctx, mr, common.RequeueWithBackoff(0))
		}
		common.SetCondition(mr, modelv1alpha1.ModelRouterDistributed,
			string(metav1.ConditionTrue), ReasonDistributed,
			fmt.Sprintf("pushed to %d router instance(s)", len(instances)))
		if !wasDistributed {
			r.eventf(mr, corev1.EventTypeNormal, EventReasonDistributed,
				"ModelRouter %q distributed to %d router instance(s)", mr.Spec.Alias, len(instances))
		}
	}

	// 6) Reachability gate.
	common.SetCondition(mr, modelv1alpha1.ModelRouterAllReachable,
		r.reachableConditionStatus(reachableCount, totalCount), reachableReason(reachableCount, totalCount),
		reachableMessage(reachableCount, totalCount))
	if wasReachable && reachableCount == 0 {
		r.eventf(mr, corev1.EventTypeWarning, EventReasonDegraded,
			"every endpoint referenced by ModelRouter %q is unreachable", mr.Spec.Alias)
	}

	// 7) Aggregate Ready + derive phase.
	r.aggregate(mr)
	if !wasReady && common.IsReady(mr) {
		r.eventf(mr, corev1.EventTypeNormal, EventReasonReady,
			"ModelRouter %q is Ready", mr.Name)
	}

	return r.writeStatus(ctx, mr, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// resolveEndpoints walks every `spec.rules[].endpoint` ref, looks up
// the addressed ModelEndpoint, and counts how many are Ready=True.
// It returns the (reachable, total) pair, the per-rule distribution
// snapshot, and a parse / transport error if any ref is malformed or
// unreachable for non-NotFound reasons.
func (r *ModelRouterReconciler) resolveEndpoints(ctx context.Context, mr *modelv1alpha1.ModelRouter) (int, int, []modelv1alpha1.ModelRouterDistribution, error) {
	total := len(mr.Spec.Rules)
	reachable := 0
	distribution := make([]modelv1alpha1.ModelRouterDistribution, 0, total)
	for _, rule := range mr.Spec.Rules {
		key, err := modelEndpointKeyFor(rule.Endpoint, mr.Namespace)
		if err != nil {
			return 0, total, nil, fmt.Errorf("invalid endpoint ref %q: %w", rule.Endpoint, err)
		}
		ep := &modelv1alpha1.ModelEndpoint{}
		entry := modelv1alpha1.ModelRouterDistribution{
			Endpoint: rule.Endpoint,
			Weight:   rule.Weight,
		}
		switch err := r.Get(ctx, key, ep); {
		case apierrors.IsNotFound(err):
			// Endpoint missing — counts as unreachable but not fatal.
		case err != nil:
			return 0, total, nil, fmt.Errorf("get ModelEndpoint %s: %w", key, err)
		default:
			if endpointReady(ep) {
				reachable++
			}
		}
		distribution = append(distribution, entry)
	}
	return reachable, total, distribution, nil
}

// endpointReady reports whether the endpoint's aggregate Ready
// condition is True.
func endpointReady(ep *modelv1alpha1.ModelEndpoint) bool {
	for _, c := range ep.Status.Conditions {
		if c.Type == modelv1alpha1.ModelEndpointReady {
			return c.Status == metav1.ConditionTrue
		}
	}
	return false
}

// modelEndpointKeyFor turns a `model://<path>[@<version>]` ResourceRef
// into a [types.NamespacedName] usable with controller-runtime client.
// The path may be `<name>` (defaults to the router's namespace) or
// `<namespace>/<name>`. Anything deeper is rejected for P0.
func modelEndpointKeyFor(ref sharedv1alpha1.ResourceRef, fallbackNamespace string) (types.NamespacedName, error) {
	scheme, path, _, err := ref.Parse()
	if err != nil {
		return types.NamespacedName{}, err
	}
	if scheme != sharedv1alpha1.SchemeModel {
		return types.NamespacedName{}, fmt.Errorf("expected scheme %q, got %q", sharedv1alpha1.SchemeModel, scheme)
	}
	parts := strings.Split(path, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return types.NamespacedName{}, errors.New("empty model ref path")
		}
		return types.NamespacedName{Namespace: fallbackNamespace, Name: parts[0]}, nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return types.NamespacedName{}, errors.New("empty namespace or name in model ref")
		}
		return types.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
	default:
		return types.NamespacedName{}, fmt.Errorf("model ref path must be <name> or <namespace>/<name>, got %q", path)
	}
}

// reachableConditionStatus maps the (reachable, total) pair to the
// AllReachable condition status.
func (r *ModelRouterReconciler) reachableConditionStatus(reachable, total int) string {
	if total == 0 {
		return string(metav1.ConditionFalse)
	}
	if reachable == total {
		return string(metav1.ConditionTrue)
	}
	return string(metav1.ConditionFalse)
}

// reachableReason maps the (reachable, total) pair to the AllReachable
// condition reason.
func reachableReason(reachable, total int) string {
	switch {
	case total == 0:
		return ReasonAllUnreachable
	case reachable == 0:
		return ReasonAllUnreachable
	case reachable == total:
		return ReasonAllReachable
	default:
		return ReasonPartialReachable
	}
}

// reachableMessage formats the (reachable, total) pair into a human
// readable message.
func reachableMessage(reachable, total int) string {
	if total == 0 {
		return "spec.rules is empty; nothing to route"
	}
	return fmt.Sprintf("%d/%d referenced ModelEndpoint(s) Ready", reachable, total)
}

// shortHash truncates the routing-table hash for log / message use.
func shortHash(full string) string {
	if len(full) <= 12 {
		return full
	}
	return full[:12]
}

// reconcileDelete drives the drain flow: lift the finalizer once the
// CR is being deleted. Real router cleanup lands in P1.
func (r *ModelRouterReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, mr *modelv1alpha1.ModelRouter) (ctrl.Result, error) {
	if mr.Status.Phase != sharedv1alpha1.PhaseTerminating {
		mr.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, mr); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("modelrouter: status update on terminating: %w", err)
		}
	}
	if _, err := common.RemoveFinalizer(ctx, r.Client, mr, FinalizerModelRouterProtect); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("modelrouter: finalizer removed", "name", mr.Name)
	return ctrl.Result{}, nil
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *ModelRouterReconciler) aggregate(mr *modelv1alpha1.ModelRouter) {
	status, reason, message := readyFromConditions(mr)
	common.SetCondition(mr, modelv1alpha1.ModelRouterReady, status, reason, message)
	mr.Status.Phase = derivePhase(mr)
	mr.Status.ObservedGeneration = mr.Generation
	// Keep the distribution slice ordered deterministically so the
	// status diff stays stable across reconciles.
	sort.SliceStable(mr.Status.Distribution, func(i, j int) bool {
		return string(mr.Status.Distribution[i].Endpoint) < string(mr.Status.Distribution[j].Endpoint)
	})
}

// writeStatus persists the in-memory status block to the API server.
func (r *ModelRouterReconciler) writeStatus(ctx context.Context, mr *modelv1alpha1.ModelRouter, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, mr); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("modelrouter: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *ModelRouterReconciler) eventf(mr *modelv1alpha1.ModelRouter, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(mr, eventType, reason, msg, args...)
}

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
var _ reconcile.Reconciler = (*ModelRouterReconciler)(nil)
