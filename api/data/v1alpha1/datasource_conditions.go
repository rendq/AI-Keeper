package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes DataSource.Status.Conditions for `controllers/common`.
func (d *DataSource) GetConditions() []metav1.Condition {
	if d == nil {
		return nil
	}
	return d.Status.Conditions
}

// SetConditions overwrites DataSource.Status.Conditions.
func (d *DataSource) SetConditions(conds []metav1.Condition) {
	if d == nil {
		return
	}
	d.Status.Conditions = conds
}
