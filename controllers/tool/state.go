package tool

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// FinalizerToolProtect is the finalizer added to every reconciled
// Tool CR so the controller can deregister the entry from the
// Tool_Registry on deletion (Requirement A7.3).
const FinalizerToolProtect = "ai-keeper.io/tool-protect"

// SteadyStateRequeue is the periodic probe cadence mandated by
// Requirement A7.3 ("每 30 秒探测一次").
const SteadyStateRequeue = 30 * time.Second

// SideEffects values mirrored from
// [skillv1alpha1.ToolGovernance.SideEffects] for readability.
const (
	SideEffectsReadOnly    = "read_only"
	SideEffectsWrite       = "write"
	SideEffectsDestructive = "destructive"
	SideEffectsExternal    = "external"
)

// Reason constants surfaced on Tool conditions and Events.
const (
	// ReasonProbeOK marks `EndpointProbed=True`.
	ReasonProbeOK = "ProbeOK"
	// ReasonProbeFailed marks `EndpointProbed=False` after a probe
	// returned an error or a 5xx response.
	ReasonProbeFailed = "ProbeFailed"

	// ReasonSchemaAccepted marks `SchemaParsed=True`.
	ReasonSchemaAccepted = "SchemaAccepted"
	// ReasonSchemaInvalid marks `SchemaParsed=False` (defensive — CRD
	// admission already enforces structure).
	ReasonSchemaInvalid = "SchemaInvalid"

	// ReasonRegistered marks `Registered=True`.
	ReasonRegistered = "Registered"
	// ReasonRegistrationFailed marks `Registered=False`.
	ReasonRegistrationFailed = "RegistrationFailed"

	// ReasonApprovalConfigured marks `ApprovalConfigured=True` for a
	// destructive tool with `requiresApproval=true` or for any
	// non-destructive tool.
	ReasonApprovalConfigured = "ApprovalConfigured"
	// ReasonApprovalMissing marks `ApprovalConfigured=False` when a
	// destructive tool does not enforce HITL approval (defensive
	// against an admission webhook bypass).
	ReasonApprovalMissing = "ApprovalMissing"

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
//  3. `EndpointProbed=False` → Degraded
//  4. `ApprovalConfigured=False` reason=ApprovalMissing → Failed
//  5. Otherwise → Pending
func derivePhase(tool *skillv1alpha1.Tool) sharedv1alpha1.Phase {
	if tool == nil {
		return sharedv1alpha1.PhasePending
	}
	if !tool.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := tool.Status.Conditions
	if c := condition(conds, skillv1alpha1.ToolApprovalConfigured); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonApprovalMissing {
		return sharedv1alpha1.PhaseFailed
	}
	if isTrue(conds, skillv1alpha1.ToolReady) {
		return sharedv1alpha1.PhaseActive
	}
	if c := condition(conds, skillv1alpha1.ToolEndpointProbed); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseDegraded
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic from
// design §6.5: EndpointProbed ∧ SchemaParsed ∧ Registered ∧
// ApprovalConfigured.
func readyFromConditions(tool *skillv1alpha1.Tool) (status, reason, message string) {
	conds := tool.Status.Conditions
	gates := []string{
		skillv1alpha1.ToolEndpointProbed,
		skillv1alpha1.ToolSchemaParsed,
		skillv1alpha1.ToolRegistered,
		skillv1alpha1.ToolApprovalConfigured,
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

// requiresApproval reports whether `governance.requiresApproval` is
// explicitly true. Treats nil pointers as false (the CRD default).
func requiresApproval(tool *skillv1alpha1.Tool) bool {
	if tool == nil {
		return false
	}
	v := tool.Spec.Governance.RequiresApproval
	return v != nil && *v
}
