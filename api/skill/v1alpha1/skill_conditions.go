package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes the Status.Conditions slice so that the helpers
// in `controllers/common` can mutate it without reflection. Mirrors the
// other 12 AIP Kinds.
func (s *Skill) GetConditions() []metav1.Condition {
	if s == nil {
		return nil
	}
	return s.Status.Conditions
}

// SetConditions writes the conditions slice back to the underlying
// status block. The receiver must be non-nil; controllers always call
// this on a fetched object so the constraint is naturally satisfied.
func (s *Skill) SetConditions(conds []metav1.Condition) {
	if s == nil {
		return
	}
	s.Status.Conditions = conds
}
