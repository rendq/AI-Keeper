package quota

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the Quota CR.
const (
	// EventReasonExceeded is published when WithinLimit flips True → False.
	EventReasonExceeded = "QuotaExceeded"
	// EventReasonRecovered is published when WithinLimit transitions
	// False → True.
	EventReasonRecovered = "QuotaRecovered"
	// EventReasonReady is published the first time aggregate Ready
	// flips True.
	EventReasonReady = "QuotaReady"
)

// ResourceCounter is the abstraction the Quota controller depends on
// for counting resources per kind in a namespace. The default
// implementation uses the controller-runtime client to list CRs.
// Tests can inject a fake counter.
type ResourceCounter interface {
	// Count returns the number of resources of the given kind in the
	// given namespace. An empty namespace means count across all
	// namespaces (cluster-scoped counting).
	Count(ctx context.Context, kind, namespace string) (int64, error)
}

// QuotaReconciler implements the Quota state machine documented in
// design.md §6.5 / Requirement A8.5.
type QuotaReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil.
	Recorder record.EventRecorder

	// Counter counts resources per kind. Defaults to
	// [NewClientResourceCounter] when nil.
	Counter ResourceCounter
}

// SetupWithManager registers the reconciler with the controller-
// runtime manager.
func (r *QuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("quota: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("quota-controller").
		For(&policyv1alpha1.Quota{}).
		Complete(r)
}

// applyDefaults wires the default implementations when not provided.
func (r *QuotaReconciler) applyDefaults() {
	if r.Counter == nil {
		r.Counter = NewClientResourceCounter(r.Client)
	}
}

// Reconcile runs one reconciliation pass for a Quota CR.
func (r *QuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("quota", req.NamespacedName)

	q := &policyv1alpha1.Quota{}
	if err := r.Get(ctx, req.NamespacedName, q); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("quota: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path.
	if !q.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, q)
	}

	// 2) Ensure the protect finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, q, FinalizerQuotaProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	wasReady := common.IsReady(q)
	wasWithin := common.IsConditionTrue(q, policyv1alpha1.QuotaWithinLimit)

	// 3) Count resources per spec.limits keys.
	ns := q.Namespace
	used := make(map[string]intstr.IntOrString, len(q.Spec.Limits))
	for kind := range q.Spec.Limits {
		count, err := r.Counter.Count(ctx, kind, ns)
		if err != nil {
			logger.V(1).Info("quota: count error", "kind", kind, "err", err.Error())
			// On transient errors, use 0 and keep going so we still
			// populate other keys.
			count = 0
		}
		used[kind] = intstr.FromInt32(int32(count))
	}
	q.Status.Used = used

	// 4) WithinLimit gate.
	within := withinLimit(q.Spec.Limits, used)
	if within {
		common.SetCondition(q, policyv1alpha1.QuotaWithinLimit,
			string(metav1.ConditionTrue), ReasonWithinLimit,
			"all resource counts within configured limits")
	} else {
		exceeded := exceededKeys(q.Spec.Limits, used)
		common.SetCondition(q, policyv1alpha1.QuotaWithinLimit,
			string(metav1.ConditionFalse), ReasonExceeded,
			fmt.Sprintf("resource(s) exceeded: %v", exceeded))
	}
	if wasWithin && !within {
		r.eventf(q, corev1.EventTypeWarning, EventReasonExceeded,
			"Quota %q exceeded in namespace %s", q.Name, q.Namespace)
	} else if !wasWithin && within && hasCondition(q, policyv1alpha1.QuotaWithinLimit) {
		r.eventf(q, corev1.EventTypeNormal, EventReasonRecovered,
			"Quota %q recovered to within limits", q.Name)
	}

	// 5) ServiceReady gate. P0 placeholder — admission reads status
	//    directly. Stamp True so the aggregate Ready can flip.
	common.SetCondition(q, policyv1alpha1.QuotaServiceReady,
		string(metav1.ConditionTrue), ReasonServiceReady,
		"limits published on status; admission consumes directly")

	// 6) Aggregate Ready + derive phase.
	r.aggregate(q)
	if !wasReady && common.IsReady(q) {
		r.eventf(q, corev1.EventTypeNormal, EventReasonReady,
			"Quota %q is Ready", q.Name)
	}

	return r.writeStatus(ctx, q, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drives the deletion: lift the finalizer.
func (r *QuotaReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, q *policyv1alpha1.Quota) (ctrl.Result, error) {
	if q.Status.Phase != sharedv1alpha1.PhaseTerminating {
		q.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, q); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("quota: status update on terminating: %w", err)
		}
	}
	if _, err := common.RemoveFinalizer(ctx, r.Client, q, FinalizerQuotaProtect); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("quota: finalizer removed", "name", q.Name)
	return ctrl.Result{}, nil
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *QuotaReconciler) aggregate(q *policyv1alpha1.Quota) {
	status, reason, message := readyFromConditions(q)
	common.SetCondition(q, policyv1alpha1.QuotaReady, status, reason, message)
	q.Status.Phase = derivePhase(q)
	q.Status.ObservedGeneration = q.Generation
}

// writeStatus persists the in-memory status block to the API server.
func (r *QuotaReconciler) writeStatus(ctx context.Context, q *policyv1alpha1.Quota, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, q); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("quota: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *QuotaReconciler) eventf(q *policyv1alpha1.Quota, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(q, eventType, reason, msg, args...)
}

// hasCondition reports whether the named condition is present.
func hasCondition(q *policyv1alpha1.Quota, t string) bool {
	return condition(q.Status.Conditions, t) != nil
}

// withinLimit reports whether every `used` value is strictly below its
// corresponding `limit`. A missing limit is treated as unlimited.
func withinLimit(limits, used map[string]intstr.IntOrString) bool {
	for k, lim := range limits {
		limVal := int64(lim.IntValue())
		if limVal <= 0 {
			continue // unlimited
		}
		u, ok := used[k]
		if !ok {
			continue
		}
		if int64(u.IntValue()) >= limVal {
			return false
		}
	}
	return true
}

// exceededKeys returns the list of keys where used >= limit.
func exceededKeys(limits, used map[string]intstr.IntOrString) []string {
	var exceeded []string
	for _, k := range sortedLimitKeys(limits) {
		lim := limits[k]
		limVal := int64(lim.IntValue())
		if limVal <= 0 {
			continue
		}
		u, ok := used[k]
		if !ok {
			continue
		}
		if int64(u.IntValue()) >= limVal {
			exceeded = append(exceeded, k)
		}
	}
	return exceeded
}

// sortedLimitKeys returns the keys of an IntOrString map in sorted
// order for deterministic messages.
func sortedLimitKeys(m map[string]intstr.IntOrString) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---------------------------------------------------------------------------
// ClientResourceCounter — default ResourceCounter using k8s client.
// ---------------------------------------------------------------------------

// ClientResourceCounter counts resources by listing CRs via the
// controller-runtime client.
type ClientResourceCounter struct {
	client client.Client
}

// NewClientResourceCounter returns a [ResourceCounter] backed by the
// supplied controller-runtime client.
func NewClientResourceCounter(c client.Client) *ClientResourceCounter {
	return &ClientResourceCounter{client: c}
}

// Count lists resources of the given kind in the given namespace and
// returns the count.
func (c *ClientResourceCounter) Count(ctx context.Context, kind, namespace string) (int64, error) {
	switch kind {
	case ResourceAgents:
		return c.countList(ctx, &agentv1alpha1.AgentList{}, namespace)
	case ResourceSkills:
		return c.countList(ctx, &skillv1alpha1.SkillList{}, namespace)
	case ResourceTools:
		return c.countList(ctx, &skillv1alpha1.ToolList{}, namespace)
	case ResourceModelEndpoints:
		return c.countList(ctx, &modelv1alpha1.ModelEndpointList{}, namespace)
	case ResourceKnowledgeBases:
		return c.countList(ctx, &datav1alpha1.KnowledgeBaseList{}, namespace)
	case ResourceDataSources:
		return c.countList(ctx, &datav1alpha1.DataSourceList{}, namespace)
	default:
		// Unknown kind — return 0 so we don't block reconcile.
		return 0, nil
	}
}

func (c *ClientResourceCounter) countList(ctx context.Context, list client.ObjectList, namespace string) (int64, error) {
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := c.client.List(ctx, list, opts...); err != nil {
		return 0, fmt.Errorf("quota: list: %w", err)
	}
	// Use the generic Items approach via reflection-free length
	// extraction. controller-runtime's ObjectList implementations all
	// embed a `Items []T` field, but we need a type switch.
	switch l := list.(type) {
	case *agentv1alpha1.AgentList:
		return int64(len(l.Items)), nil
	case *skillv1alpha1.SkillList:
		return int64(len(l.Items)), nil
	case *skillv1alpha1.ToolList:
		return int64(len(l.Items)), nil
	case *modelv1alpha1.ModelEndpointList:
		return int64(len(l.Items)), nil
	case *datav1alpha1.KnowledgeBaseList:
		return int64(len(l.Items)), nil
	case *datav1alpha1.DataSourceList:
		return int64(len(l.Items)), nil
	default:
		return 0, fmt.Errorf("quota: unknown list type %T", list)
	}
}

// Compile-time interface assertions.
var (
	_ reconcile.Reconciler = (*QuotaReconciler)(nil)
	_ ResourceCounter      = (*ClientResourceCounter)(nil)
)
