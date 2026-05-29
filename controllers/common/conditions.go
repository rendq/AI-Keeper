package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// ConditionType is the canonical name of an AIP condition entry. Every
// AIP Kind exports its own typed constants (e.g.
// [api/skill/v1alpha1.SkillReady], [api/agent/v1alpha1.AgentReady]); this
// package treats them as plain strings.
const (
	// ConditionReady is the conventional aggregate Ready condition. Each
	// Kind also exports a typed alias for clarity (e.g. SkillReady,
	// AgentReady) but every alias evaluates to the literal "Ready", which
	// is what [IsReady] checks for here.
	ConditionReady = "Ready"
)

// ConditionsAware is the lowest-common-denominator interface that every
// AIP CRD's `Status` block satisfies. The contract is intentionally
// minimal so the helpers below can be reused across all 13 Kinds.
//
// Implementations live next to the type definitions under
// `api/<group>/v1alpha1/*_conditions.go`. The pointer receivers must
// expose the underlying `Status.Conditions` slice so callers can mutate
// it via [SetCondition] without reaching for reflection.
type ConditionsAware interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

// SetCondition records (or updates) a condition on the typed object. It
// returns true iff the conditions slice was mutated, mirroring the
// shared helper [sharedv1alpha1.SetCondition].
//
// The function reads the current slice via [ConditionsAware.GetConditions],
// applies the change in-place, then writes the (possibly resized) slice
// back via [ConditionsAware.SetConditions]. Callers are expected to
// persist the parent object — for example with `Status().Update(ctx, obj)`
// — when this function reports a mutation.
//
// Status, reason and message follow the K8s convention: `metav1.ConditionTrue`
// / `metav1.ConditionFalse` / `metav1.ConditionUnknown` for `status`, a
// stable CamelCase machine identifier for `reason`, and a free-form
// human-readable string for `message`.
func SetCondition(obj ConditionsAware, condType, status, reason, message string) bool {
	if obj == nil {
		return false
	}
	conds := obj.GetConditions()
	mutated := sharedv1alpha1.SetCondition(&conds, condType, status, reason, message)
	if mutated {
		obj.SetConditions(conds)
	}
	return mutated
}

// IsReady reports whether the object's aggregate `Ready` condition is
// True. It is equivalent to checking
// `IsConditionTrue(obj.Status.Conditions, "Ready")`.
//
// The helper is the cheapest correct way to gate cross-controller
// reconciles (e.g. an Agent waiting for its referenced Skill to become
// Ready) without each controller re-implementing the lookup.
func IsReady(obj ConditionsAware) bool {
	if obj == nil {
		return false
	}
	return sharedv1alpha1.IsConditionTrue(obj.GetConditions(), ConditionReady)
}

// GetCondition returns a pointer to the condition with the given type, or
// nil if absent. The returned pointer aliases the slice held by `obj`;
// callers must not retain it across mutations.
func GetCondition(obj ConditionsAware, condType string) *metav1.Condition {
	if obj == nil {
		return nil
	}
	conds := obj.GetConditions()
	return sharedv1alpha1.GetCondition(conds, condType)
}

// IsConditionTrue is the typed analogue of
// [sharedv1alpha1.IsConditionTrue] for any [ConditionsAware] value.
func IsConditionTrue(obj ConditionsAware, condType string) bool {
	if obj == nil {
		return false
	}
	return sharedv1alpha1.IsConditionTrue(obj.GetConditions(), condType)
}

// RemoveCondition drops the condition with the given type. Returns
// true iff a removal happened.
func RemoveCondition(obj ConditionsAware, condType string) bool {
	if obj == nil {
		return false
	}
	conds := obj.GetConditions()
	if sharedv1alpha1.RemoveCondition(&conds, condType) {
		obj.SetConditions(conds)
		return true
	}
	return false
}
