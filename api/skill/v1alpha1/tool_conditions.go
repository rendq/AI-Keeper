package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes Tool.Status.Conditions for `controllers/common`.
func (t *Tool) GetConditions() []metav1.Condition {
	if t == nil {
		return nil
	}
	return t.Status.Conditions
}

// SetConditions overwrites Tool.Status.Conditions.
func (t *Tool) SetConditions(conds []metav1.Condition) {
	if t == nil {
		return
	}
	t.Status.Conditions = conds
}
