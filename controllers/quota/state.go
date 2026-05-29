package quota

import (
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// FinalizerQuotaProtect is the finalizer added to every reconciled
// Quota CR so the controller can drain Quota_Service state on
// deletion (Requirement A8.5 — basic finalizer; real drain lands in
// P1).
const FinalizerQuotaProtect = "ai-keeper.io/quota-protect"

// SteadyStateRequeue is the periodic settlement cadence applied
// while the quota is stable. 1 minute mirrors the cadence used by
// the Budget controller so dashboards see fresh `used` values
// quickly.
const SteadyStateRequeue = time.Minute

// Reason constants surfaced on Quota conditions and Events.
const (
	// ReasonServiceReady marks `ServiceReady=True`. P0 placeholder
	// — admission reads `status` directly.
	ReasonServiceReady = "ServiceReady"

	// ReasonWithinLimit marks `WithinLimit=True`.
	ReasonWithinLimit = "WithinLimit"
	// ReasonExceeded marks `WithinLimit=False`.
	ReasonExceeded = "Exceeded"

	// ReasonInvalidScope marks `ServiceReady=False` when
	// `spec.scope.kind` is not one of the supported values
	// (kubebuilder admission already gates this; the constant exists
	// so the controller can surface a deterministic reason if the
	// enum ever widens).
	ReasonInvalidScope = "InvalidScope"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// Resource keys recognised on `spec.limits`. The constants mirror the
// camelCase aliases used by aikctl / Helm values; lower-cased
// equivalents are accepted to match the kubebuilder marker on
// [DefaultQuotaLimits] in the tenant package.
const (
	ResourceAgents         = "agents"
	ResourceSkills         = "skills"
	ResourceTools          = "tools"
	ResourceModelEndpoints = "modelEndpoints"
	ResourceKnowledgeBases = "knowledgeBases"
	ResourceDataSources    = "dataSources"
)

// SupportedResources is the canonical set of `spec.limits` keys the
// P0 controller knows how to count. Exposed for the aikctl `lint`
// rule and for tests.
var SupportedResources = []string{
	ResourceAgents,
	ResourceSkills,
	ResourceTools,
	ResourceModelEndpoints,
	ResourceKnowledgeBases,
	ResourceDataSources,
}

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. Aggregate Ready=True → Active
//  3. `WithinLimit=False` → Failed (operator action required)
//  4. Otherwise → Pending
func derivePhase(q *policyv1alpha1.Quota) sharedv1alpha1.Phase {
	if q == nil {
		return sharedv1alpha1.PhasePending
	}
	if !q.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := q.Status.Conditions
	if isTrue(conds, policyv1alpha1.QuotaReady) {
		return sharedv1alpha1.PhaseActive
	}
	if c := condition(conds, policyv1alpha1.QuotaWithinLimit); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseFailed
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic for
// Quota: ServiceReady=True ∧ WithinLimit=True.
func readyFromConditions(q *policyv1alpha1.Quota) (status, reason, message string) {
	conds := q.Status.Conditions
	gates := []string{
		policyv1alpha1.QuotaServiceReady,
		policyv1alpha1.QuotaWithinLimit,
	}
	for _, t := range gates {
		if !isTrue(conds, t) {
			return string(metav1.ConditionFalse), ReasonNotReady, t + " not satisfied"
		}
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

// sortedKeys returns the keys of `m` sorted ascending so log /
// condition messages stay deterministic.
func sortedKeys(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
