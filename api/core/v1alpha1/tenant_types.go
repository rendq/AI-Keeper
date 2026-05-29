package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Tenant Conditions (design.md §6.5 / aip-controllers-reconcile.md).
const (
	// TenantNamespacesReady is True when the per-tenant namespace(s) and
	// default Budget/Quota templates have been provisioned. Maps to
	// Requirement A7.1.
	TenantNamespacesReady = "NamespacesReady"
	// TenantConnectorsReady is True when the connector templates have
	// been initialised.
	TenantConnectorsReady = "ConnectorsReady"
	// TenantReady is the aggregate Ready condition.
	TenantReady = "Ready"
)

// TenantContact identifies a human contact for the tenant.
type TenantContact struct {
	// Role of the contact.
	//
	// +kubebuilder:validation:Enum=admin;security;billing;dpo
	Role string `json:"role"`

	// Email of the contact.
	//
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=320
	Email string `json:"email"`
}

// TenantDataResidency captures a tenant's primary data location.
type TenantDataResidency struct {
	// PrimaryRegion the tenant data is processed in.
	// +optional
	PrimaryRegion string `json:"primaryRegion,omitempty"`

	// AllowedRegions where data may flow.
	// +optional
	AllowedRegions []string `json:"allowedRegions,omitempty"`

	// ForbidCrossBorder defaults to true.
	// +optional
	ForbidCrossBorder *bool `json:"forbidCrossBorder,omitempty"`
}

// TenantComplianceProfile bundles the compliance posture of a tenant.
type TenantComplianceProfile struct {
	// Tier maps to platform-wide compliance defaults.
	//
	// +kubebuilder:validation:Enum=basic;standard;regulated;classified
	Tier string `json:"tier"`

	// Certifications already obtained.
	// +optional
	Certifications []string `json:"certifications,omitempty"`

	// DataResidency for this tenant.
	// +optional
	DataResidency *TenantDataResidency `json:"dataResidency,omitempty"`
}

// TenantDefaultBudget is the default per-month budget injected into the
// tenant's namespace at provisioning time. UsdPerMonth is a MoneyAmount
// (string-encoded) that admission validates as non-negative via its
// regex; integer caps use Minimum=0.
type TenantDefaultBudget struct {
	// +optional
	UsdPerMonth *shared.MoneyAmount `json:"usdPerMonth,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerMonth *int64 `json:"tokensPerMonth,omitempty"`
}

// TenantDeployment captures the deployment posture of a tenant.
type TenantDeployment struct {
	// +kubebuilder:validation:Enum=saas_shared;saas_dedicated;vpc;on_premise;airgapped
	// +optional
	Mode string `json:"mode,omitempty"`

	// +kubebuilder:validation:Enum=hosted;self_managed
	// +optional
	ControlPlane string `json:"controlPlane,omitempty"`

	// +kubebuilder:validation:Enum=hosted;customer_vpc;on_premise
	// +optional
	DataPlane string `json:"dataPlane,omitempty"`
}

// TenantSpec describes the desired state of a Tenant.
type TenantSpec struct {
	// DisplayName for UI / reporting.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=200
	DisplayName string `json:"displayName"`

	// Description is a free-form long form description.
	//
	// +kubebuilder:validation:MaxLength=2000
	// +optional
	Description string `json:"description,omitempty"`

	// Contacts for the tenant.
	// +optional
	Contacts []TenantContact `json:"contacts,omitempty"`

	// ComplianceProfile drives audit / encryption / backup defaults.
	ComplianceProfile TenantComplianceProfile `json:"complianceProfile"`

	// ModelAllowlist restricts which ModelEndpoints calls in this tenant
	// may route to. Empty means no restriction.
	// +optional
	ModelAllowlist []shared.ResourceRef `json:"modelAllowlist,omitempty"`

	// DefaultBudget seeded into the tenant namespace.
	// +optional
	DefaultBudget *TenantDefaultBudget `json:"defaultBudget,omitempty"`

	// Deployment posture.
	// +optional
	Deployment *TenantDeployment `json:"deployment,omitempty"`
}

// TenantUsage is the running summary metrics surfaced on the status.
type TenantUsage struct {
	// +optional
	ActiveAgents int32 `json:"activeAgents,omitempty"`
	// +optional
	ActiveSkills int32 `json:"activeSkills,omitempty"`
	// +optional
	Last30dInvocations int64 `json:"last30dInvocations,omitempty"`
	// +optional
	Last30dCostUsd shared.MoneyAmount `json:"last30dCostUsd,omitempty"`
}

// TenantCertification captures an obtained certification.
type TenantCertification struct {
	Name      string       `json:"name"`
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// TenantStatus is the observed state of a Tenant.
type TenantStatus struct {
	// Phase is the coarse-grained lifecycle phase.
	// +optional
	Phase shared.Phase `json:"phase,omitempty"`

	// ObservedGeneration is the .metadata.generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions track transitions of the controller state machine.
	//
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Namespaces the controller has provisioned for this tenant.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Usage roll-up.
	// +optional
	Usage *TenantUsage `json:"usage,omitempty"`

	// CertificationsObtained is the controller's view of compliance
	// attestations.
	// +optional
	CertificationsObtained []TenantCertification `json:"certificationsObtained,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=tn,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Tier",type=string,JSONPath=`.spec.complianceProfile.tier`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Tenant is the cluster-scoped isolation unit and compliance domain.
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantList is the canonical list type for Tenant.
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
