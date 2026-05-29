package serviceaccount

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Reason constants surfaced on ServiceAccount conditions and Events.
const (
	// ReasonRegistered marks `IdentityProviderReady=True`.
	ReasonRegistered = "Registered"
	// ReasonRegistering marks `IdentityProviderReady=False` while the
	// Broker call is in flight (or has failed transiently).
	ReasonRegistering = "Registering"
	// ReasonRegistrationFailed marks `IdentityProviderReady=False`
	// after a non-transient Broker failure.
	ReasonRegistrationFailed = "RegistrationFailed"
	// ReasonInvalidIdentityProvider marks `IdentityProviderReady=False`
	// when `spec.identityProvider` is empty.
	ReasonInvalidIdentityProvider = "InvalidIdentityProvider"

	// ReasonOBOEnabled marks `TokenExchangeReady=True`.
	ReasonOBOEnabled = "OBOEnabled"
	// ReasonOBODisabled marks `TokenExchangeReady=Unknown` when the
	// SA has `allowOnBehalfOf=false`. Unknown lets the aggregate Ready
	// condition stay True without making the gate sticky.
	ReasonOBODisabled = "OBODisabled"
	// ReasonOBOFailed marks `TokenExchangeReady=False` after the Broker
	// rejected an EnableOBO call.
	ReasonOBOFailed = "OBOFailed"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// FinalizerSARevoke is the finalizer added to every reconciled
// ServiceAccount CR so the controller can revoke tokens on deletion
// (Requirement A7.2 / C8.4 — "30 秒内回收 token").
const FinalizerSARevoke = "ai-keeper.io/serviceaccount-revoke"

// SteadyStateRequeue mirrors the long-tail requeue used by the other
// AIP controllers so steady-state reconciles eventually re-evaluate
// drift.
const SteadyStateRequeue = 10 * time.Minute

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. `IdentityProviderReady=False reason=InvalidIdentityProvider` → Failed
//  3. Aggregate Ready=True → Active
//  4. `IdentityProviderReady` not yet True → Registering
//  5. Otherwise → Pending
func derivePhase(sa *corev1alpha1.ServiceAccount) sharedv1alpha1.Phase {
	if sa == nil {
		return sharedv1alpha1.PhasePending
	}
	if !sa.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := sa.Status.Conditions
	if c := condition(conds, corev1alpha1.ServiceAccountIdentityProviderReady); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonInvalidIdentityProvider {
		return sharedv1alpha1.PhaseFailed
	}
	if isTrue(conds, corev1alpha1.ServiceAccountReady) {
		return sharedv1alpha1.PhaseActive
	}
	if !isTrue(conds, corev1alpha1.ServiceAccountIdentityProviderReady) {
		return sharedv1alpha1.PhaseRegistering
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic from
// design §6.5: IdentityProviderReady=True ∧ TokenExchangeReady ∈
// {True, Unknown(reason=OBODisabled)}.
func readyFromConditions(sa *corev1alpha1.ServiceAccount) (status, reason, message string) {
	conds := sa.Status.Conditions
	if !isTrue(conds, corev1alpha1.ServiceAccountIdentityProviderReady) {
		return string(metav1.ConditionFalse), ReasonNotReady, corev1alpha1.ServiceAccountIdentityProviderReady + " not satisfied"
	}
	tx := condition(conds, corev1alpha1.ServiceAccountTokenExchangeReady)
	switch {
	case tx == nil:
		return string(metav1.ConditionFalse), ReasonNotReady, corev1alpha1.ServiceAccountTokenExchangeReady + " missing"
	case tx.Status == metav1.ConditionTrue:
		return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
	case tx.Status == metav1.ConditionUnknown && tx.Reason == ReasonOBODisabled:
		return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied (OBO disabled)"
	default:
		return string(metav1.ConditionFalse), ReasonNotReady, corev1alpha1.ServiceAccountTokenExchangeReady + " not satisfied"
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
