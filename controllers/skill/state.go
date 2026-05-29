package skill

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// derivePhase maps the current Conditions slice to the canonical
// `status.phase` value for a Skill. The mapping follows design.md §6.1
// state machine and Requirement A3.7.
//
// Precedence (most specific first):
//
//  1. DeletionTimestamp set        → Terminating
//  2. Any condition False with a permanent reason
//     (`InvalidSchema`, `MissingReferencePermanent`, `CyclicDependency`,
//     `RegistrationFailed`)         → Failed
//  3. Deprecating=True              → Deprecated
//  4. Ready=True                    → Active
//  5. SLOMet=False (Ready=True path consumed above) → Degraded
//  6. SchemaValid=False (transient) → Validating
//  7. DependenciesResolved=False    → Resolving (or Pending when reason
//     is `MissingReference` per A3.4)
//  8. ImplementationReady=False     → Building
//  9. Registered=False              → Registering
//
// 10. EvalPassing=Unknown/False     → Evaluating
// 11. default                        → Pending
func derivePhase(skill *skillv1alpha1.Skill) sharedv1alpha1.Phase {
	if skill == nil {
		return sharedv1alpha1.PhasePending
	}
	if !skill.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := skill.Status.Conditions

	if c := condition(conds, skillv1alpha1.SkillSchemaValid); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonInvalidSchema {
		return sharedv1alpha1.PhaseFailed
	}
	if c := condition(conds, skillv1alpha1.SkillDependenciesResolved); c != nil &&
		c.Status == metav1.ConditionFalse {
		switch c.Reason {
		case ReasonMissingReferencePermanent, ReasonCyclicDependency:
			return sharedv1alpha1.PhaseFailed
		case ReasonMissingReference:
			return sharedv1alpha1.PhasePending
		default:
			return sharedv1alpha1.PhaseResolving
		}
	}
	if c := condition(conds, skillv1alpha1.SkillRegistered); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonRegistrationFailed {
		return sharedv1alpha1.PhaseFailed
	}

	// Lifecycle deprecation has higher precedence than Ready / Degraded
	// because a deprecated Skill is still allowed to serve existing
	// references (Requirement A3.10).
	if isTrue(conds, skillv1alpha1.SkillDeprecating) {
		return sharedv1alpha1.PhaseDeprecated
	}
	if isTrue(conds, skillv1alpha1.SkillReady) {
		// Degraded is reported when SLOMet=False alongside Ready=True
		// per Requirement A3.9; keep the Active path as the default.
		if c := condition(conds, skillv1alpha1.SkillSLOMet); c != nil && c.Status == metav1.ConditionFalse {
			return sharedv1alpha1.PhaseDegraded
		}
		return sharedv1alpha1.PhaseActive
	}

	// Walk the condition pyramid downward to surface the most advanced
	// in-progress phase.
	if c := condition(conds, skillv1alpha1.SkillSchemaValid); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseValidating
	}
	if c := condition(conds, skillv1alpha1.SkillDependenciesResolved); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseResolving
	}
	if c := condition(conds, skillv1alpha1.SkillImplementationReady); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseBuilding
	}
	if c := condition(conds, skillv1alpha1.SkillRegistered); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseRegistering
	}
	if c := condition(conds, skillv1alpha1.SkillEvalPassing); c == nil || c.Status != metav1.ConditionTrue {
		return sharedv1alpha1.PhaseEvaluating
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic from
// Requirement A3.7:
//
//	SchemaValid ∧ DependenciesResolved ∧ ImplementationReady ∧ Registered
//	  ∧ (EvalPassing ∨ stability=experimental)
func readyFromConditions(skill *skillv1alpha1.Skill) (status, reason, message string) {
	conds := skill.Status.Conditions
	gates := []string{
		skillv1alpha1.SkillSchemaValid,
		skillv1alpha1.SkillDependenciesResolved,
		skillv1alpha1.SkillImplementationReady,
		skillv1alpha1.SkillRegistered,
	}
	for _, t := range gates {
		if !isTrue(conds, t) {
			return string(metav1.ConditionFalse), ReasonNotReady, t + " not satisfied"
		}
	}
	if skill.Spec.Stability == sharedv1alpha1.StageExperimental {
		return string(metav1.ConditionTrue), ReasonReady, "experimental stage auto-passes evaluation gate"
	}
	if !isTrue(conds, skillv1alpha1.SkillEvalPassing) {
		return string(metav1.ConditionFalse), ReasonNotReady, "EvalPassing not satisfied"
	}
	return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
}

// condition is a small helper that returns a pointer to the named
// condition, or nil if absent.
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
