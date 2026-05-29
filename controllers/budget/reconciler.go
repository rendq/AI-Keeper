package budget

import (
	"context"
	"errors"
	"fmt"
	"time"

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

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the Budget CR.
const (
	// EventReasonExhausted is published the first time WithinLimit
	// flips True → False.
	EventReasonExhausted = "BudgetExhausted"
	// EventReasonRecovered is published when WithinLimit transitions
	// False → True (e.g. period reset).
	EventReasonRecovered = "BudgetRecovered"
	// EventReasonReady is published the first time aggregate Ready
	// flips True.
	EventReasonReady = "BudgetReady"
)

// Clock is the abstraction the reconciler uses to read wall-clock
// time. Production code uses [SystemClock]; tests inject a frozen
// [FakeClock] so period boundary computations stay deterministic.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production [Clock] backed by time.Now.
type SystemClock struct{}

// Now returns time.Now(). Always UTC-normalised by the caller.
func (SystemClock) Now() time.Time { return time.Now() }

// BudgetReconciler implements the Budget state machine documented
// in design.md §6.5 / Requirement A8.
type BudgetReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// CostTracker is the data-plane client used to look up the
	// current spend for a scope. Defaults to [NewNoopCostTracker]
	// when nil so the controller stays operational while task 13.1
	// is in flight.
	CostTracker CostTrackerClient

	// Clock supplies wall-clock time. Defaults to [SystemClock] when
	// nil.
	Clock Clock
}

// SetupWithManager registers the reconciler with the controller-
// runtime manager.
func (r *BudgetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("budget: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("budget-controller").
		For(&policyv1alpha1.Budget{}).
		Complete(r)
}

// applyDefaults wires the no-op stand-ins when the operator did not
// supply real implementations.
func (r *BudgetReconciler) applyDefaults() {
	if r.CostTracker == nil {
		r.CostTracker = NewNoopCostTracker()
	}
	if r.Clock == nil {
		r.Clock = SystemClock{}
	}
}

// Reconcile runs one reconciliation pass for a Budget CR.
func (r *BudgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("budget", req.NamespacedName)

	b := &policyv1alpha1.Budget{}
	if err := r.Get(ctx, req.NamespacedName, b); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("budget: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path.
	if !b.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, b)
	}

	// 2) Ensure the protect finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, b, FinalizerBudgetProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	wasReady := common.IsReady(b)
	wasWithin := common.IsConditionTrue(b, policyv1alpha1.BudgetWithinLimit)

	// 3) Compute period boundaries.
	now := r.Clock.Now()
	start, end, err := computePeriod(b.Spec.Period, now)
	if err != nil {
		common.SetCondition(b, policyv1alpha1.BudgetEnforcerReady,
			string(metav1.ConditionFalse), ReasonInvalidPeriod, truncateErr(err))
		common.SetCondition(b, policyv1alpha1.BudgetWithinLimit,
			string(metav1.ConditionFalse), ReasonInvalidPeriod, truncateErr(err))
		r.aggregate(b)
		return r.writeStatus(ctx, b, common.RequeueWithBackoff(0))
	}
	startTime := metav1.NewTime(start)
	endTime := metav1.NewTime(end)
	b.Status.PeriodStart = &startTime
	b.Status.PeriodEnd = &endTime
	b.Status.DaysRemaining = daysRemaining(now, end)

	// 4) Pull the current spend snapshot.
	scope := ScopeKey{
		Kind:   b.Spec.Scope.Kind,
		Name:   b.Spec.Scope.Name,
		Period: b.Spec.Period,
	}
	current, err := r.CostTracker.Current(ctx, scope)
	if err != nil {
		// Tracker transient failure — keep the existing snapshot and
		// retry with backoff. The aggregate Ready stays at its prior
		// value because we didn't observe a change.
		logger.V(1).Info("budget: cost tracker error", "err", err.Error())
		return r.writeStatus(ctx, b, common.RequeueWithBackoff(0))
	}
	b.Status.Current = current
	b.Status.BurnRate = classifyBurnRate(b.Spec.Limits, current)

	// 5) WithinLimit gate.
	within := withinLimit(b.Spec.Limits, current)
	if within {
		common.SetCondition(b, policyv1alpha1.BudgetWithinLimit,
			string(metav1.ConditionTrue), ReasonWithinLimit,
			"current spend is below configured limits")
	} else {
		common.SetCondition(b, policyv1alpha1.BudgetWithinLimit,
			string(metav1.ConditionFalse), ReasonExhausted,
			fmt.Sprintf("current spend has reached %q limits (hardCap=%v)", b.Spec.Period, hardCapEffective(b)))
	}
	if wasWithin && !within {
		r.eventf(b, corev1.EventTypeWarning, EventReasonExhausted,
			"Budget %q in scope %s/%s exhausted (hardCap=%v)",
			b.Name, b.Spec.Scope.Kind, b.Spec.Scope.Name, hardCapEffective(b))
	} else if !wasWithin && within && hasCondition(b, policyv1alpha1.BudgetWithinLimit) {
		r.eventf(b, corev1.EventTypeNormal, EventReasonRecovered,
			"Budget %q recovered to within configured limits", b.Name)
	}

	// 6) EnforcerReady gate. P0 placeholder — Budget_Enforcer reads
	//    `status` directly and applies hardCap downstream. We stamp
	//    True so the aggregate Ready can flip and the data plane
	//    can wire in later (task 13.1).
	common.SetCondition(b, policyv1alpha1.BudgetEnforcerReady,
		string(metav1.ConditionTrue), ReasonEnforcerReady,
		"limits published on status; data-plane consumes directly")

	// 7) Aggregate Ready + derive phase.
	r.aggregate(b)
	if !wasReady && common.IsReady(b) {
		r.eventf(b, corev1.EventTypeNormal, EventReasonReady,
			"Budget %q is Ready", b.Name)
	}

	return r.writeStatus(ctx, b, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drives the drain flow: lift the finalizer once the
// CR is being deleted. Real Budget_Enforcer cleanup lands in P1.
func (r *BudgetReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, b *policyv1alpha1.Budget) (ctrl.Result, error) {
	if b.Status.Phase != sharedv1alpha1.PhaseTerminating {
		b.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, b); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("budget: status update on terminating: %w", err)
		}
	}
	if _, err := common.RemoveFinalizer(ctx, r.Client, b, FinalizerBudgetProtect); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("budget: finalizer removed", "name", b.Name)
	return ctrl.Result{}, nil
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *BudgetReconciler) aggregate(b *policyv1alpha1.Budget) {
	status, reason, message := readyFromConditions(b)
	common.SetCondition(b, policyv1alpha1.BudgetReady, status, reason, message)
	b.Status.Phase = derivePhase(b)
	b.Status.ObservedGeneration = b.Generation
}

// writeStatus persists the in-memory status block to the API server.
func (r *BudgetReconciler) writeStatus(ctx context.Context, b *policyv1alpha1.Budget, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, b); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("budget: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *BudgetReconciler) eventf(b *policyv1alpha1.Budget, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(b, eventType, reason, msg, args...)
}

// hasCondition reports whether the named condition is present.
func hasCondition(b *policyv1alpha1.Budget, t string) bool {
	return condition(b.Status.Conditions, t) != nil
}

// hardCapEffective returns the effective hardCap value, defaulting
// to true when `spec.hardCap` is nil. The kubebuilder marker
// documents `defaults to true`.
func hardCapEffective(b *policyv1alpha1.Budget) bool {
	if b.Spec.HardCap == nil {
		return true
	}
	return *b.Spec.HardCap
}

// daysRemaining computes the integer day-count until `end`, clamped
// at zero. Returns nil when the period boundary is unset.
func daysRemaining(now, end time.Time) *int32 {
	if end.IsZero() {
		return nil
	}
	d := end.Sub(now.UTC())
	if d < 0 {
		zero := int32(0)
		return &zero
	}
	days := int32(d.Hours() / 24)
	return &days
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
var _ reconcile.Reconciler = (*BudgetReconciler)(nil)
