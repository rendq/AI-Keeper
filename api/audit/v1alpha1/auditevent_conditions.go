package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes AuditEvent.Status.Conditions for
// `controllers/common`. AuditEvent is an external-storage facade so the
// conditions slice is intentionally short-lived (forwarding state, etc).
func (a *AuditEvent) GetConditions() []metav1.Condition {
	if a == nil {
		return nil
	}
	return a.Status.Conditions
}

// SetConditions overwrites AuditEvent.Status.Conditions.
func (a *AuditEvent) SetConditions(conds []metav1.Condition) {
	if a == nil {
		return
	}
	a.Status.Conditions = conds
}
