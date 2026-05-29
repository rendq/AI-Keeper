package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Quota conditions.
const (
	QuotaServiceReady = "ServiceReady"
	QuotaWithinLimit  = "WithinLimit"
	QuotaReady        = "Ready"
)

// QuotaScope identifies which dimension a quota applies to.
type QuotaScope struct {
	// +kubebuilder:validation:Enum=Tenant;Namespace;Team;User
	Kind string `json:"kind"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// QuotaSpec is the desired state of a Quota.
type QuotaSpec struct {
	Scope QuotaScope `json:"scope"`

	// Limits is a free-form map of resource kind → cap. The cap may be
	// either an integer or a quantity-style string (e.g. "100000000").
	// +optional
	Limits map[string]intstr.IntOrString `json:"limits,omitempty"`
}

// QuotaStatus is the observed state of a Quota.
type QuotaStatus struct {
	// +optional
	Phase shared.Phase `json:"phase,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +patchStrategy=merge
	// +patchMergeKey=type
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Used mirrors the shape of Limits.
	// +optional
	Used map[string]intstr.IntOrString `json:"used,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=qa,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Scope",type=string,JSONPath=`.spec.scope.kind`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Quota caps the number of resources a scope may create.
type Quota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   QuotaSpec   `json:"spec,omitempty"`
	Status QuotaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// QuotaList is the canonical list type for Quota.
type QuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Quota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Quota{}, &QuotaList{})
}
