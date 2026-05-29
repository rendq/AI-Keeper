package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes Budget.Status.Conditions for `controllers/common`.
func (b *Budget) GetConditions() []metav1.Condition {
	if b == nil {
		return nil
	}
	return b.Status.Conditions
}

// SetConditions overwrites Budget.Status.Conditions.
func (b *Budget) SetConditions(conds []metav1.Condition) {
	if b == nil {
		return
	}
	b.Status.Conditions = conds
}
