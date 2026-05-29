package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes Tenant.Status.Conditions for `controllers/common`.
func (t *Tenant) GetConditions() []metav1.Condition {
	if t == nil {
		return nil
	}
	return t.Status.Conditions
}

// SetConditions overwrites Tenant.Status.Conditions.
func (t *Tenant) SetConditions(conds []metav1.Condition) {
	if t == nil {
		return
	}
	t.Status.Conditions = conds
}
