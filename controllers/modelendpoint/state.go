package modelendpoint

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// FinalizerModelEndpointProtect is the finalizer added to every
// reconciled ModelEndpoint CR so the controller can drain registry
// state on deletion (Requirement A7.6 — basic finalizer; real drain
// in P1).
const FinalizerModelEndpointProtect = "ai-keeper.io/modelendpoint-protect"

// SteadyStateRequeue is the periodic probe cadence mandated by
// Requirement A7.6 ("每 30 秒探测一次").
const SteadyStateRequeue = 30 * time.Second

// Compliance regimes that require an executed DPA before the
// endpoint may be marked Ready (mirrors Requirement A7.6 / lint rule
// `model-endpoint/dpa-required`).
const (
	ComplianceGDPR  = "GDPR"
	ComplianceHIPAA = "HIPAA"
)

// Reason constants surfaced on ModelEndpoint conditions and Events.
const (
	// ReasonProbeOK marks `Healthy=True`.
	ReasonProbeOK = "ProbeOK"
	// ReasonProbeFailed marks `Healthy=False` after a probe error or
	// a 5xx response.
	ReasonProbeFailed = "ProbeFailed"

	// ReasonDPASigned marks `DPASigned=True`.
	ReasonDPASigned = "DPASigned"
	// ReasonDPAMissing marks `DPASigned=False` when compliance ∋
	// {GDPR, HIPAA} but `spec.privacy.dpaSigned` is not true.
	ReasonDPAMissing = "DPAMissing"
	// ReasonDPANotRequired marks `DPASigned=Unknown` when the
	// endpoint is not subject to GDPR/HIPAA — the gate is treated as
	// satisfied for aggregation purposes.
	ReasonDPANotRequired = "NotRequired"

	// ReasonWithinQuota marks `WithinQuota=True`.
	ReasonWithinQuota = "WithinQuota"
	// ReasonQuotaExceeded marks `WithinQuota=False`.
	ReasonQuotaExceeded = "QuotaExceeded"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. `DPASigned=False reason=DPAMissing` → Failed
//  3. Aggregate Ready=True → Active
//  4. `Healthy=False` → Degraded
//  5. Otherwise → Pending
func derivePhase(me *modelv1alpha1.ModelEndpoint) sharedv1alpha1.Phase {
	if me == nil {
		return sharedv1alpha1.PhasePending
	}
	if !me.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := me.Status.Conditions
	if c := condition(conds, modelv1alpha1.ModelEndpointDPASigned); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonDPAMissing {
		return sharedv1alpha1.PhaseFailed
	}
	if isTrue(conds, modelv1alpha1.ModelEndpointReady) {
		return sharedv1alpha1.PhaseActive
	}
	if c := condition(conds, modelv1alpha1.ModelEndpointHealthy); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseDegraded
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic from
// design §6.5: Healthy=True ∧ DPASigned ∈ {True, Unknown(NotRequired)}
// ∧ WithinQuota=True.
func readyFromConditions(me *modelv1alpha1.ModelEndpoint) (status, reason, message string) {
	conds := me.Status.Conditions
	if !isTrue(conds, modelv1alpha1.ModelEndpointHealthy) {
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelEndpointHealthy + " not satisfied"
	}
	dpa := condition(conds, modelv1alpha1.ModelEndpointDPASigned)
	switch {
	case dpa == nil:
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelEndpointDPASigned + " missing"
	case dpa.Status == metav1.ConditionTrue:
		// satisfied
	case dpa.Status == metav1.ConditionUnknown && dpa.Reason == ReasonDPANotRequired:
		// satisfied
	default:
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelEndpointDPASigned + " not satisfied"
	}
	if !isTrue(conds, modelv1alpha1.ModelEndpointWithinQuota) {
		return string(metav1.ConditionFalse), ReasonNotReady, modelv1alpha1.ModelEndpointWithinQuota + " not satisfied"
	}
	return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
}

// requiresDPA reports whether `spec.compliance` lists any regime that
// mandates a signed Data Processing Agreement.
func requiresDPA(me *modelv1alpha1.ModelEndpoint) bool {
	if me == nil {
		return false
	}
	for _, tag := range me.Spec.Compliance {
		switch strings.ToUpper(strings.TrimSpace(tag)) {
		case ComplianceGDPR, ComplianceHIPAA:
			return true
		}
	}
	return false
}

// dpaSigned reports whether `spec.privacy.dpaSigned` is explicitly true.
func dpaSigned(me *modelv1alpha1.ModelEndpoint) bool {
	if me == nil || me.Spec.Privacy == nil || me.Spec.Privacy.DPASigned == nil {
		return false
	}
	return *me.Spec.Privacy.DPASigned
}

// quotaTPM returns the configured per-minute token quota, or 0 when
// `spec.quota.tpm` is omitted.
func quotaTPM(me *modelv1alpha1.ModelEndpoint) int64 {
	if me == nil || me.Spec.Quota == nil || me.Spec.Quota.TPM == nil {
		return 0
	}
	return *me.Spec.Quota.TPM
}

// quotaRPM returns the configured per-minute request quota, or 0 when
// `spec.quota.rpm` is omitted.
func quotaRPM(me *modelv1alpha1.ModelEndpoint) int64 {
	if me == nil || me.Spec.Quota == nil || me.Spec.Quota.RPM == nil {
		return 0
	}
	return *me.Spec.Quota.RPM
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
