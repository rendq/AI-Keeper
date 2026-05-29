package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConditions exposes KnowledgeBase.Status.Conditions for
// `controllers/common`.
func (k *KnowledgeBase) GetConditions() []metav1.Condition {
	if k == nil {
		return nil
	}
	return k.Status.Conditions
}

// SetConditions overwrites KnowledgeBase.Status.Conditions.
func (k *KnowledgeBase) SetConditions(conds []metav1.Condition) {
	if k == nil {
		return
	}
	k.Status.Conditions = conds
}
