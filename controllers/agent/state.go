package agent

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Reason constants surfaced on Agent conditions and Events. The names
// mirror design.md §6.2.5 / the bullet list in tasks.md §3.4.
const (
	// ReasonReady is the boilerplate "all gates satisfied" reason.
	ReasonReady = "Ready"
	// ReasonNotReady marks an aggregate-Ready failure where at least
	// one gate is False.
	ReasonNotReady = "NotReady"

	// ReasonUnsupportedPattern is set on `SpecValid=False` when
	// `runtime.pattern` is not one of the P0-supported values.
	ReasonUnsupportedPattern = "UnsupportedPattern"
	// ReasonSpecValid is the success reason for `SpecValid=True`.
	ReasonSpecValid = "SpecValid"

	// ReasonSkillsResolved is the success reason for `SkillsResolved=True`.
	ReasonSkillsResolved = "SkillsResolved"
	// ReasonMissingSkill marks `SkillsResolved=False` when one or more
	// referenced Skills cannot be found in the cluster.
	ReasonMissingSkill = "MissingSkill"
	// ReasonUnsatisfiableConstraint marks `SkillsResolved=False` when
	// the supplied `versionConstraint` does not match any candidate
	// version (Requirement A4.2).
	ReasonUnsatisfiableConstraint = "UnsatisfiableConstraint"
	// ReasonInvalidSkillRef marks `SkillsResolved=False` when one of
	// the `spec.skills[]` refs is malformed.
	ReasonInvalidSkillRef = "InvalidSkillRef"

	// ReasonPolicyAttached is the success reason for `PolicyAttached=True`.
	ReasonPolicyAttached = "PolicyAttached"
	// ReasonPolicyBindFailed is the failure reason for `PolicyAttached=False`.
	ReasonPolicyBindFailed = "PolicyBindFailed"

	// ReasonIdentityReady is the success reason for `IdentityReady=True`.
	ReasonIdentityReady = "IdentityReady"
	// ReasonIdentityFailed marks `IdentityReady=False`.
	ReasonIdentityFailed = "IdentityFailed"

	// ReasonDeploymentReady is the success reason for `Deployed=True`.
	ReasonDeploymentReady = "DeploymentReady"
	// ReasonDeploymentProgressing marks `Deployed=False` while the
	// underlying Deployment is rolling out.
	ReasonDeploymentProgressing = "DeploymentProgressing"
	// ReasonDeploymentFailed marks `Deployed=False` on a hard failure.
	ReasonDeploymentFailed = "DeploymentFailed"

	// ReasonChannelsHealthy is the success reason for `ChannelsHealthy=True`.
	ReasonChannelsHealthy = "ChannelsHealthy"
	// ReasonChannelRegisterFailed marks `ChannelsHealthy=False`.
	ReasonChannelRegisterFailed = "ChannelRegisterFailed"

	// ReasonGuardrailsHealthy is defaulted True in P0.
	ReasonGuardrailsHealthy = "GuardrailsHealthy"

	// ReasonSandboxReady is the success reason for `SandboxReady=True`.
	ReasonSandboxReady = "SandboxReady"
	// ReasonSandboxDisabled is reported when the spec disables sandbox;
	// the gate is treated as satisfied (Requirement A4 — A4.3 only
	// requires the sandbox when `enabled=true`).
	ReasonSandboxDisabled = "SandboxDisabled"
	// ReasonSandboxUnavailable marks `SandboxReady=False` (Requirement
	// A4.3) and drives phase=Failed.
	ReasonSandboxUnavailable = "SandboxUnavailable"

	// ReasonRolloutComplete is the success reason for
	// `RolloutComplete=True` (P0 always 100 %).
	ReasonRolloutComplete = "RolloutComplete"

	// ReasonBudgetWithinLimit defaults True in P0.
	ReasonBudgetWithinLimit = "BudgetWithinLimit"

	// ReasonUsingDeprecatedSkill marks `UsingDeprecatedSkill=True`
	// (Requirement A4.6 / A6.2).
	ReasonUsingDeprecatedSkill = "UsingDeprecatedSkill"
	// ReasonNoDeprecatedSkill clears the condition when no resolved
	// Skill is in the Deprecating phase.
	ReasonNoDeprecatedSkill = "NoDeprecatedSkill"

	// ReasonDraining is reported on Phase=Terminating.
	ReasonDraining = "Draining"
)

// FinalizerAgentDrain is the finalizer the Agent controller adds to
// every reconciled Agent CR (Requirement A4.1 / A4.7).
const FinalizerAgentDrain = "ai-keeper.io/agent-drain"

// AnnotationDraining is set on the Agent's metadata once
// `deletionTimestamp` has been observed and the controller has
// stopped accepting new sessions (Requirement A4.7 — "停止接受新会话").
// Channel adapters and the runtime watch this key to bail out new
// requests.
const AnnotationDraining = "ai-keeper.io/agent-draining"

// DrainTimeout is the hard upper bound on the drain path
// (Requirement A4.8). After this much wall-clock time elapses the
// reconciler stops waiting for in-flight sessions, but still requires
// that audit events have flushed before removing the finalizer.
const DrainTimeout = 5 * time.Minute

// InFlightWaitTimeout caps how long the reconciler waits for
// in-flight sessions before forcing the Deployment scale-down
// (Requirement A4.7 — "等待 in-flight ≤120s").
const InFlightWaitTimeout = 120 * time.Second

// SteadyStateRequeue is the long-tail requeue applied at the end of
// a successful reconcile (mirrors the Skill controller's value).
const SteadyStateRequeue = 10 * time.Minute

// supportedPatterns enumerates the runtime patterns the Agent
// controller currently honours. Anything outside this set causes
// `SpecValid=False reason=UnsupportedPattern` and Phase=Failed.
//
// Validates: tasks.md §3.4 ("仅 pattern ∈ {react, tool_calling} 接收").
var supportedPatterns = map[string]struct{}{
	"react":        {},
	"tool_calling": {},
}

// patternSupported reports whether the supplied runtime pattern is in
// the P0 allow-list.
func patternSupported(pattern string) bool {
	_, ok := supportedPatterns[pattern]
	return ok
}

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.2.1. Precedence (most specific first):
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. `SpecValid=False reason=UnsupportedPattern` → Failed
//  3. `SandboxReady=False reason=SandboxUnavailable` → Failed
//  4. `SkillsResolved=False reason=UnsatisfiableConstraint` → Failed
//  5. Aggregate Ready=True → Running (or Degraded when
//     `BudgetWithinLimit=False` / `GuardrailsHealthy=False`)
//  6. Walk the gates downward to surface the most advanced in-progress
//     phase: Validating → ResolvingSkills → AttachingPolicies →
//     Provisioning → Deploying → Configuring → RollingOut.
func derivePhase(agent *agentv1alpha1.Agent) sharedv1alpha1.Phase {
	if agent == nil {
		return sharedv1alpha1.PhasePending
	}
	if !agent.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := agent.Status.Conditions

	// Permanent failure cases — these short-circuit the pyramid walk
	// because the gate that failed is never going to resolve without
	// a spec change.
	if c := condition(conds, agentv1alpha1.AgentSpecValid); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonUnsupportedPattern {
		return sharedv1alpha1.PhaseFailed
	}
	if c := condition(conds, agentv1alpha1.AgentSandboxReady); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonSandboxUnavailable {
		return sharedv1alpha1.PhaseFailed
	}
	if c := condition(conds, agentv1alpha1.AgentSkillsResolved); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonUnsatisfiableConstraint {
		return sharedv1alpha1.PhaseFailed
	}

	// Aggregate Ready=True drives Running / Degraded.
	if isTrue(conds, agentv1alpha1.AgentReady) {
		if isFalse(conds, agentv1alpha1.AgentBudgetWithinLimit) ||
			isFalse(conds, agentv1alpha1.AgentGuardrailsHealthy) {
			return sharedv1alpha1.PhaseDegraded
		}
		return sharedv1alpha1.PhaseRunning
	}

	// Walk the gate pyramid downward to surface the most advanced
	// in-progress phase. The order tracks design.md §6.2.1.
	if c := condition(conds, agentv1alpha1.AgentSpecValid); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseValidating
	}
	if c := condition(conds, agentv1alpha1.AgentSkillsResolved); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseResolvingSkills
	}
	if c := condition(conds, agentv1alpha1.AgentPolicyAttached); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseAttachingPolicies
	}
	if c := condition(conds, agentv1alpha1.AgentIdentityReady); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseProvisioning
	}
	if c := condition(conds, agentv1alpha1.AgentDeployed); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseDeploying
	}
	if c := condition(conds, agentv1alpha1.AgentChannelsHealthy); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseConfiguring
	}
	if c := condition(conds, agentv1alpha1.AgentRolloutComplete); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseRollingOut
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic from design
// §6.2.5: SpecValid ∧ SkillsResolved ∧ PolicyAttached ∧ IdentityReady ∧
// Deployed ∧ ChannelsHealthy ∧ GuardrailsHealthy ∧ SandboxReady ∧
// RolloutComplete. `BudgetWithinLimit` and `UsingDeprecatedSkill` are
// not Ready gates — they only modulate the phase (Degraded / warn).
func readyFromConditions(agent *agentv1alpha1.Agent) (status, reason, message string) {
	conds := agent.Status.Conditions
	gates := []string{
		agentv1alpha1.AgentSpecValid,
		agentv1alpha1.AgentSkillsResolved,
		agentv1alpha1.AgentPolicyAttached,
		agentv1alpha1.AgentIdentityReady,
		agentv1alpha1.AgentDeployed,
		agentv1alpha1.AgentChannelsHealthy,
		agentv1alpha1.AgentGuardrailsHealthy,
		agentv1alpha1.AgentSandboxReady,
		agentv1alpha1.AgentRolloutComplete,
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

// isFalse reports whether the named condition is present and False.
func isFalse(conds []metav1.Condition, t string) bool {
	c := condition(conds, t)
	return c != nil && c.Status == metav1.ConditionFalse
}
