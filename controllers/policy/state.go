package policy

import (
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// effectiveWindowState enumerates the three positions the current
// wall-clock time can occupy relative to a Policy's `effectiveWindow`.
type effectiveWindowState int

const (
	effectiveWindowWithin effectiveWindowState = iota
	effectiveWindowNotYet
	effectiveWindowExpired
)

// evaluateEffectiveWindow projects the supplied time onto the Policy's
// effective window. A nil window is treated as `[-∞, +∞]` per the AIP
// API contract, in which case the function always returns
// [effectiveWindowWithin].
func evaluateEffectiveWindow(pol *policyv1alpha1.Policy, now time.Time) effectiveWindowState {
	if pol == nil || pol.Spec.EffectiveWindow == nil {
		return effectiveWindowWithin
	}
	w := pol.Spec.EffectiveWindow
	if w.NotBefore != nil && now.Before(w.NotBefore.Time) {
		return effectiveWindowNotYet
	}
	if w.NotAfter != nil && !now.Before(w.NotAfter.Time) {
		// `now == NotAfter` is treated as expired so the boundary
		// condition is closed at the upper end. This matches the
		// reading in design.md §6.3.1 / Requirement A5.9.
		return effectiveWindowExpired
	}
	return effectiveWindowWithin
}

// derivePhase maps the Policy's Conditions / Spec onto the canonical
// `status.phase`. The mapping mirrors the state machine in design.md
// §6.3.1.
//
// Precedence (most specific first):
//
//  1. DeletionTimestamp set        → Terminating
//  2. SyntaxValid=False             → Failed
//  3. NotConflicting=False (Hard)   → Failed
//  4. Compiled=False (CompileError) → Failed
//  5. WithinEffectiveWindow=False (Expired)        → Expired
//  6. WithinEffectiveWindow=False (NotYetEffective) → Suspended
//  7. spec.enabled=false            → Suspended
//  8. FullyDistributed=True         → Active
//  9. Distributed=True              → Active (≥90% acked)
//
// 10. default                        → Pending
func derivePhase(pol *policyv1alpha1.Policy) sharedv1alpha1.Phase {
	if pol == nil {
		return sharedv1alpha1.PhasePending
	}
	if !pol.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := pol.Status.Conditions

	if c := conditionByType(conds, policyv1alpha1.PolicySyntaxValid); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseFailed
	}
	if c := conditionByType(conds, policyv1alpha1.PolicyNotConflicting); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonHardConflict {
		return sharedv1alpha1.PhaseFailed
	}
	if c := conditionByType(conds, policyv1alpha1.PolicyCompiled); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonCompileError {
		return sharedv1alpha1.PhaseFailed
	}
	if c := conditionByType(conds, policyv1alpha1.PolicyWithinEffectiveWindow); c != nil &&
		c.Status == metav1.ConditionFalse {
		switch c.Reason {
		case ReasonExpired:
			return sharedv1alpha1.PhaseExpired
		case ReasonNotYetEffective:
			return sharedv1alpha1.PhaseSuspended
		}
	}
	if pol.Spec.Enabled != nil && !*pol.Spec.Enabled {
		return sharedv1alpha1.PhaseSuspended
	}
	if isTrue(conds, policyv1alpha1.PolicyFullyDistributed) {
		return sharedv1alpha1.PhaseActive
	}
	if isTrue(conds, policyv1alpha1.PolicyDistributed) {
		return sharedv1alpha1.PhaseActive
	}
	return sharedv1alpha1.PhasePending
}

// conditionByType returns a pointer to the named condition, or nil
// when absent.
func conditionByType(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}

// isTrue reports whether the named condition is present and True.
func isTrue(conds []metav1.Condition, t string) bool {
	c := conditionByType(conds, t)
	return c != nil && c.Status == metav1.ConditionTrue
}

// filterConflictsForKey returns conflicts that involve `key`.
func filterConflictsForKey(all []Conflict, key string) []Conflict {
	if len(all) == 0 {
		return nil
	}
	out := make([]Conflict, 0, len(all))
	for _, c := range all {
		if c.Involves(key) {
			out = append(out, c)
		}
	}
	return out
}

// firstHardConflict returns the first hard conflict in the slice, or
// nil when none exists.
func firstHardConflict(conflicts []Conflict) *Conflict {
	for i := range conflicts {
		if conflicts[i].IsHard() {
			return &conflicts[i]
		}
	}
	return nil
}

// peerKey returns the peer policy key in a binary conflict relative
// to `selfKey`. Falls back to whichever non-empty side is present (so
// a tautology with B="" still reads sensibly).
func peerKey(c *Conflict, selfKey string) string {
	if c == nil {
		return ""
	}
	if c.A == selfKey {
		return c.B
	}
	if c.B == selfKey {
		return c.A
	}
	if c.B != "" {
		return c.B
	}
	return c.A
}

// projectConflicts converts the in-process [Conflict] slice to the
// public `policyv1alpha1.PolicyConflict` form written to status.
func projectConflicts(conflicts []Conflict, selfKey string) []policyv1alpha1.PolicyConflict {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]policyv1alpha1.PolicyConflict, 0, len(conflicts))
	for _, c := range conflicts {
		out = append(out, policyv1alpha1.PolicyConflict{
			ConflictsWith: peerKey(&c, selfKey),
			Reason:        string(c.Type) + ": " + c.Reason,
		})
	}
	return out
}

// policyResourceRef builds the canonical `policy://<ns>/<name>` ref
// for use on the event bus.
func policyResourceRef(pol *policyv1alpha1.Policy) (sharedv1alpha1.ResourceRef, error) {
	if pol == nil {
		return "", errors.New("policy: nil")
	}
	if pol.Name == "" {
		return "", errors.New("policy: empty metadata.name")
	}
	ns := pol.Namespace
	if ns == "" {
		ns = "default"
	}
	return sharedv1alpha1.FormatResourceRef(sharedv1alpha1.SchemePolicy, ns+"/"+pol.Name, "")
}
