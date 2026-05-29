package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes ModelEndpoint.Status.Conditions for
// `controllers/common`.
func (m *ModelEndpoint) GetConditions() []metav1.Condition {
	if m == nil {
		return nil
	}
	return m.Status.Conditions
}

// SetConditions overwrites ModelEndpoint.Status.Conditions.
func (m *ModelEndpoint) SetConditions(conds []metav1.Condition) {
	if m == nil {
		return
	}
	m.Status.Conditions = conds
}
