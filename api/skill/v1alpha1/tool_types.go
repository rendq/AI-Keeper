package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Tool conditions (design.md §6.5 ToolController).
const (
	ToolEndpointProbed     = "EndpointProbed"
	ToolSchemaParsed       = "SchemaParsed"
	ToolRegistered         = "Registered"
	ToolApprovalConfigured = "ApprovalConfigured"
	ToolReady              = "Ready"
)

// ToolSecretRef references a Vault / KMS / K8s Secret entry.
type ToolSecretRef struct {
	Name string `json:"name"`
	// +optional
	Key string `json:"key,omitempty"`
}

// ToolAuthentication captures how the platform authenticates to the
// tool. The `mode=oauth2_obo` value is special — it requires
// `tokenExchangeRef` to be populated (Requirement B3.3).
type ToolAuthentication struct {
	// Mode of authentication.
	//
	// +kubebuilder:validation:Enum=none;api_key;oauth2_client_credentials;oauth2_obo;mtls;spiffe
	Mode string `json:"mode"`

	// SecretRef holds the credential.
	// +optional
	SecretRef *ToolSecretRef `json:"secretRef,omitempty"`

	// TokenExchangeRef is required when Mode=oauth2_obo.
	//
	// +kubebuilder:validation:MaxLength=253
	// +optional
	TokenExchangeRef string `json:"tokenExchangeRef,omitempty"`
}

// ToolSchema bundles input/output JSON Schemas.
type ToolSchema struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	Input *apiextensionsv1.JSON `json:"input"`

	// +kubebuilder:pruning:PreserveUnknownFields
	Output *apiextensionsv1.JSON `json:"output"`
}

// ToolGovernance extends GovernanceBlock with tool-specific knobs.
type ToolGovernance struct {
	shared.GovernanceBlock `json:",inline"`

	// SideEffects classifies the tool's blast radius.
	//
	// +kubebuilder:validation:Enum=read_only;write;destructive;external
	// +optional
	SideEffects string `json:"sideEffects,omitempty"`

	// RequiresApproval forces HITL on every invocation. Required when
	// SideEffects=destructive (Requirement A9.2 lint rule
	// `tool/destructive-needs-approval`).
	// +optional
	RequiresApproval *bool `json:"requiresApproval,omitempty"`
}

// ToolCost captures per-call cost. PerCallUsd is a MoneyAmount
// (string-encoded); admission enforces non-negativity via the regex.
type ToolCost struct {
	// +optional
	PerCallUsd *shared.MoneyAmount `json:"perCallUsd,omitempty"`
}

// ToolRateLimit caps per-tenant / per-agent throughput.
type ToolRateLimit struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	PerAgentPerMinute *int32 `json:"perAgentPerMinute,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	PerTenantPerMinute *int32 `json:"perTenantPerMinute,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	Burst *int32 `json:"burst,omitempty"`
}

// ToolSpec describes the desired state of a Tool.
type ToolSpec struct {
	// Protocol the tool speaks.
	//
	// +kubebuilder:validation:Enum=mcp;openapi;grpc;builtin;http
	Protocol string `json:"protocol"`

	// Endpoint URL of the tool.
	//
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=2048
	Endpoint string `json:"endpoint"`

	// Authentication block.
	// +optional
	Authentication *ToolAuthentication `json:"authentication,omitempty"`

	// Schema for input/output.
	Schema ToolSchema `json:"schema"`

	// Governance + side-effects + approval.
	Governance ToolGovernance `json:"governance"`

	// Cost block.
	// +optional
	Cost *ToolCost `json:"cost,omitempty"`

	// RateLimit caps.
	// +optional
	RateLimit *ToolRateLimit `json:"rateLimit,omitempty"`

	// Reliability block.
	// +optional
	Reliability *shared.ReliabilityBlock `json:"reliability,omitempty"`
}

// ToolStatus describes the observed state of a Tool.
type ToolStatus struct {
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

	// Reachable is True when the periodic probe last succeeded.
	// +optional
	Reachable *bool `json:"reachable,omitempty"`

	// LastProbeAt is when the controller last probed `endpoint`.
	// +optional
	LastProbeAt *metav1.Time `json:"lastProbeAt,omitempty"`

	// Invocations24h rolling counter.
	// +optional
	Invocations24h int64 `json:"invocations24h,omitempty"`

	// ErrorRate24h reported as a fraction in [0, 1].
	// +optional
	ErrorRate24h *float64 `json:"errorRate24h,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=tl,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Protocol",type=string,JSONPath=`.spec.protocol`
// +kubebuilder:printcolumn:name="Reachable",type=boolean,JSONPath=`.status.reachable`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Tool is an atomic, schema-described capability invoked by Skills.
type Tool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ToolSpec   `json:"spec,omitempty"`
	Status ToolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ToolList is the canonical list type for Tool.
type ToolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tool{}, &ToolList{})
}
