package modelrouter

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// FinalizerModelRouterProtect is the finalizer added to every
// reconciled ModelRouter CR so the controller can drop the alias
// from every router instance on deletion (Requirement A7.7).
const FinalizerModelRouterProtect = "ai-keeper.io/modelrouter-protect"

// SteadyStateRequeue is the periodic re-validation cadence applied
// while the rule set is stable. 5 minutes mirrors the cadence used
// by the other reconcile-only controllers (Tool / DataSource / KB).
const SteadyStateRequeue = 5 * time.Minute

// Reason constants surfaced on ModelRouter conditions and Events.
const (
	// ReasonCompiled marks `Compiled=True`.
	ReasonCompiled = "Compiled"
	// ReasonCompileFailed marks `Compiled=False`.
	ReasonCompileFailed = "CompileFailed"

	// ReasonDistributed marks `Distributed=True`.
	ReasonDistributed = "Distributed"
	// ReasonDistributeFailed marks `Distributed=False`.
	ReasonDistributeFailed = "DistributeFailed"
	// ReasonNoInstances marks `Distributed=Unknown` when no router
	// instances have been registered yet — treated as satisfied for
	// aggregation since the table is still recorded centrally.
	ReasonNoInstances = "NoInstances"

	// ReasonAllReachable marks `AllReachable=True` when every
	// referenced endpoint is Ready.
	ReasonAllReachable = "AllReachable"
	// ReasonPartialReachable marks `AllReachable=False reason=Partial`
	// when at least one endpoint is reachable but not all of them.
	// The aggregate Ready stays False but Phase remains Active because
	// traffic can still flow.
	ReasonPartialReachable = "Partial"
	// ReasonAllUnreachable marks `AllReachable=False reason=AllUnreachable`
	// — every referenced endpoint is missing or not Ready.
	ReasonAllUnreachable = "AllUnreachable"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. `Compiled=False` → Failed
//  3. `AllReachable=False reason=AllUnreachable` → Degraded
//  4. Aggregate Ready=True → Active
//  5. Otherwise → Pending
func derivePhase(mr *modelv1alpha1.ModelRouter) sharedv1alpha1.Phase {
	if mr == nil {
		return sharedv1alpha1.PhasePending
	}
	if !mr.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := mr.Status.Conditions
	if c := condition(conds, modelv1alpha1.ModelRouterCompiled); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseFailed
	}
	if c := condition(conds, modelv1alpha1.ModelRouterAllReachable); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonAllUnreachable {
		return sharedv1alpha1.PhaseDegraded
	}
	if isTrue(conds, modelv1alpha1.ModelRouterReady) {
		return sharedv1alpha1.PhaseActive
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic: Compiled
// ∧ Distributed ∈ {True, Unknown(NoInstances)} ∧ AllReachable=True.
func readyFromConditions(mr *modelv1alpha1.ModelRouter) (status, reason, message string) {
	conds := mr.Status.Conditions
	if !isTrue(conds, modelv1alpha1.ModelRouterCompiled) {
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelRouterCompiled + " not satisfied"
	}
	dist := condition(conds, modelv1alpha1.ModelRouterDistributed)
	switch {
	case dist == nil:
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelRouterDistributed + " missing"
	case dist.Status == metav1.ConditionTrue:
		// satisfied
	case dist.Status == metav1.ConditionUnknown && dist.Reason == ReasonNoInstances:
		// satisfied — table is recorded even though no router has
		// pulled it yet.
	default:
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelRouterDistributed + " not satisfied"
	}
	if !isTrue(conds, modelv1alpha1.ModelRouterAllReachable) {
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelRouterAllReachable + " not satisfied"
	}
	return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
}

// condition returns a pointer to the named condition, or nil.
func condition(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}

// isTrue reports whether the named condition is present and True.
func isTrue(conds []metav1.Condition, t string) bool {
	c := condition(conds, t)
	return c != nil && c.Status == metav1.ConditionTrue
}
