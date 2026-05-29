package datasource

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// FinalizerDataSourceProtect is the finalizer added to every reconciled
// DataSource CR so the controller can drain the connector on deletion
// (Requirement A7.4 — basic finalizer; full sync drain lands in P1).
const FinalizerDataSourceProtect = "ai-keeper.io/datasource-protect"

// SteadyStateRequeue is the periodic connector-check cadence applied
// in the absence of a real sync schedule. Mirrors the 5-minute
// requeue suggested by the task specification.
const SteadyStateRequeue = 5 * time.Minute

// Reason constants surfaced on DataSource conditions and Events.
const (
	// ReasonConnected marks `Connected=True`.
	ReasonConnected = "Connected"
	// ReasonConnectFailed marks `Connected=False`.
	ReasonConnectFailed = "ConnectFailed"

	// ReasonSyncDeferred marks `Syncing=Unknown` while the P0 build
	// has no real sync schedule.
	ReasonSyncDeferred = "SyncDeferred"

	// ReasonACLEnforced marks `ACLEnforced=True` once `spec.acl.mode`
	// has been declared.
	ReasonACLEnforced = "ACLEnforced"
	// ReasonACLNotConfigured marks `ACLEnforced=Unknown` when
	// `spec.acl` is empty — we treat this as "operator opted out" and
	// allow Ready to stay True.
	ReasonACLNotConfigured = "ACLNotConfigured"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. Aggregate Ready=True → Active
//  3. `Connected=False` → Degraded
//  4. Otherwise → Pending
func derivePhase(ds *datav1alpha1.DataSource) sharedv1alpha1.Phase {
	if ds == nil {
		return sharedv1alpha1.PhasePending
	}
	if !ds.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := ds.Status.Conditions
	if isTrue(conds, datav1alpha1.DataSourceReady) {
		return sharedv1alpha1.PhaseActive
	}
	if c := condition(conds, datav1alpha1.DataSourceConnected); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseDegraded
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic from
// design §6.5: Connected=True ∧ ACLEnforced ∈ {True, Unknown}.
func readyFromConditions(ds *datav1alpha1.DataSource) (status, reason, message string) {
	conds := ds.Status.Conditions
	if !isTrue(conds, datav1alpha1.DataSourceConnected) {
		return string(metav1.ConditionFalse), ReasonNotReady, datav1alpha1.DataSourceConnected + " not satisfied"
	}
	acl := condition(conds, datav1alpha1.DataSourceACLEnforced)
	switch {
	case acl == nil:
		return string(metav1.ConditionFalse), ReasonNotReady, datav1alpha1.DataSourceACLEnforced + " missing"
	case acl.Status == metav1.ConditionTrue:
		return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
	case acl.Status == metav1.ConditionUnknown && acl.Reason == ReasonACLNotConfigured:
		return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied (ACL not configured)"
	default:
		return string(metav1.ConditionFalse), ReasonNotReady, datav1alpha1.DataSourceACLEnforced + " not satisfied"
	}
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
