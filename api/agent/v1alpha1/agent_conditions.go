package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes Agent.Status.Conditions for `controllers/common`.
func (a *Agent) GetConditions() []metav1.Condition {
	if a == nil {
		return nil
	}
	return a.Status.Conditions
}

// SetConditions overwrites Agent.Status.Conditions.
func (a *Agent) SetConditions(conds []metav1.Condition) {
	if a == nil {
		return
	}
	a.Status.Conditions = conds
}
