package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes ServiceAccount.Status.Conditions for
// `controllers/common`.
func (s *ServiceAccount) GetConditions() []metav1.Condition {
	if s == nil {
		return nil
	}
	return s.Status.Conditions
}

// SetConditions overwrites ServiceAccount.Status.Conditions.
func (s *ServiceAccount) SetConditions(conds []metav1.Condition) {
	if s == nil {
		return
	}
	s.Status.Conditions = conds
}
