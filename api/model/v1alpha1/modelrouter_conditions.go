package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes ModelRouter.Status.Conditions for
// `controllers/common`.
func (m *ModelRouter) GetConditions() []metav1.Condition {
	if m == nil {
		return nil
	}
	return m.Status.Conditions
}

// SetConditions overwrites ModelRouter.Status.Conditions.
func (m *ModelRouter) SetConditions(conds []metav1.Condition) {
	if m == nil {
		return
	}
	m.Status.Conditions = conds
}
