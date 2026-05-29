package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetCondition idempotently records a condition on `conditions`. Returns
// true iff the slice was mutated.
//
// Behaviour:
//   - If no condition with `condType` exists, the new condition is
//     appended with `LastTransitionTime = metav1.Now()`.
//   - If a condition with the same `type, status, reason, message`
//     already exists, the slice is left untouched and the function
//     returns false (idempotency).
//   - If the condition's `status` changed, both `LastTransitionTime` and
//     the other tracked fields are updated.
//   - If only `reason` or `message` changed, the existing
//     `LastTransitionTime` is preserved (per K8s convention — only status
//     transitions reset the timer).
//
// Mirrors design.md §5.3 / controllers/common/conditions.go contract.
func SetCondition(conditions *[]metav1.Condition, condType, status, reason, message string) bool {
	if conditions == nil {
		return false
	}
	now := metav1.Now()
	for i := range *conditions {
		c := &(*conditions)[i]
		if c.Type != condType {
			continue
		}
		// No-op when nothing changed.
		if c.Status == metav1.ConditionStatus(status) && c.Reason == reason && c.Message == message {
			return false
		}
		mutated := false
		if c.Status != metav1.ConditionStatus(status) {
			c.Status = metav1.ConditionStatus(status)
			c.LastTransitionTime = now
			mutated = true
		}
		if c.Reason != reason {
			c.Reason = reason
			mutated = true
		}
		if c.Message != message {
			c.Message = message
			mutated = true
		}
		return mutated
	}
	*conditions = append(*conditions, metav1.Condition{
		Type:               condType,
		Status:             metav1.ConditionStatus(status),
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
	return true
}

// GetCondition returns a pointer to the condition with the given type, or
// nil if absent. The returned pointer aliases the slice; callers must not
// mutate it concurrently.
func GetCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

// IsConditionTrue is the small ergonomic wrapper used in reconcile loops.
func IsConditionTrue(conds []metav1.Condition, condType string) bool {
	c := GetCondition(conds, condType)
	return c != nil && c.Status == metav1.ConditionTrue
}

// RemoveCondition removes the condition with the given type. Returns
// true iff a removal happened.
func RemoveCondition(conditions *[]metav1.Condition, condType string) bool {
	if conditions == nil {
		return false
	}
	for i := range *conditions {
		if (*conditions)[i].Type == condType {
			*conditions = append((*conditions)[:i], (*conditions)[i+1:]...)
			return true
		}
	}
	return false
}
