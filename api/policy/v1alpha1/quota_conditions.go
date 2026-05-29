package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes Quota.Status.Conditions for `controllers/common`.
func (q *Quota) GetConditions() []metav1.Condition {
	if q == nil {
		return nil
	}
	return q.Status.Conditions
}

// SetConditions overwrites Quota.Status.Conditions.
func (q *Quota) SetConditions(conds []metav1.Condition) {
	if q == nil {
		return
	}
	q.Status.Conditions = conds
}
