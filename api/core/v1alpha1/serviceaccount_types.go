package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// ServiceAccount Conditions (design.md §6.5).
const (
	ServiceAccountIdentityProviderReady = "IdentityProviderReady"
	ServiceAccountTokenExchangeReady    = "TokenExchangeReady"
	ServiceAccountReady                 = "Ready"
)

// ServiceAccountSpec describes the desired state of an AIP ServiceAccount.
type ServiceAccountSpec struct {
	// IdentityProvider references an IdP configuration name (OIDC / SAML
	// / SPIFFE) configured on the platform.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	IdentityProvider string `json:"identityProvider"`

	// SpiffeID for workload identity (optional).
	//
	// +kubebuilder:validation:Pattern=`^spiffe://[A-Za-z0-9._\-]+(/[A-Za-z0-9._\-]+)*$`
	// +optional
	SpiffeID string `json:"spiffeId,omitempty"`

	// Attributes used by ABAC decisions.
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`

	// TokenLifetime caps the lifetime of issued tokens.
	// +optional
	TokenLifetime *shared.Duration `json:"tokenLifetime,omitempty"`

	// AllowOnBehalfOf enables RFC 8693 token exchange for this SA.
	// +optional
	AllowOnBehalfOf *bool `json:"allowOnBehalfOf,omitempty"`
}

// ServiceAccountStatus describes the observed state.
type ServiceAccountStatus struct {
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

	// IssuedTokens24h is a rolling 24h counter populated by the
	// Identity_Broker.
	// +optional
	IssuedTokens24h int64 `json:"issuedTokens24h,omitempty"`

	// LastUsedAt is the last time a token from this SA was minted.
	// +optional
	LastUsedAt *metav1.Time `json:"lastUsedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=aipsa,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="IdP",type=string,JSONPath=`.spec.identityProvider`
// +kubebuilder:printcolumn:name="OBO",type=boolean,JSONPath=`.spec.allowOnBehalfOf`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServiceAccount is the AIP non-human identity for Agents and Workloads.
type ServiceAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceAccountSpec   `json:"spec,omitempty"`
	Status ServiceAccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceAccountList is the canonical list type for ServiceAccount.
type ServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceAccount `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceAccount{}, &ServiceAccountList{})
}
