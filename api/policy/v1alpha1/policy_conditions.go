package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes Policy.Status.Conditions for `controllers/common`.
func (p *Policy) GetConditions() []metav1.Condition {
	if p == nil {
		return nil
	}
	return p.Status.Conditions
}

// SetConditions overwrites Policy.Status.Conditions.
func (p *Policy) SetConditions(conds []metav1.Condition) {
	if p == nil {
		return
	}
	p.Status.Conditions = conds
}
