package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// ModelEndpoint conditions.
const (
	ModelEndpointHealthy     = "Healthy"
	ModelEndpointDPASigned   = "DPASigned"
	ModelEndpointWithinQuota = "WithinQuota"
	ModelEndpointReady       = "Ready"
)

// ModelEndpointSecretRef holds credential pointer.
type ModelEndpointSecretRef struct {
	Name string `json:"name"`
	// +optional
	Key string `json:"key,omitempty"`
}

// ModelEndpointAuthentication captures auth.
type ModelEndpointAuthentication struct {
	// +kubebuilder:validation:Enum=api_key;oauth2;iam;mtls;none
	Mode string `json:"mode"`

	// +optional
	SecretRef *ModelEndpointSecretRef `json:"secretRef,omitempty"`
}

// ModelEndpointCost captures pricing.
type ModelEndpointCost struct {
	// +optional
	InputUsdPerMTok *shared.MoneyAmount `json:"inputUsdPerMTok,omitempty"`

	// +optional
	OutputUsdPerMTok *shared.MoneyAmount `json:"outputUsdPerMTok,omitempty"`

	// +optional
	CachedInputUsdPerMTok *shared.MoneyAmount `json:"cachedInputUsdPerMTok,omitempty"`

	// +optional
	EmbeddingUsdPerMTok *shared.MoneyAmount `json:"embeddingUsdPerMTok,omitempty"`
}

// ModelEndpointResidency captures residency.
type ModelEndpointResidency struct {
	// +kubebuilder:validation:MaxLength=64
	// +optional
	PrimaryRegion string `json:"primaryRegion,omitempty"`

	// +optional
	Sovereign *bool `json:"sovereign,omitempty"`
}

// ModelEndpointQuota captures provider rate limits.
type ModelEndpointQuota struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	TPM *int64 `json:"tpm,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	RPM *int64 `json:"rpm,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	TPD *int64 `json:"tpd,omitempty"`
}

// ModelEndpointPrivacy captures privacy commitments.
type ModelEndpointPrivacy struct {
	// +optional
	TrainOnInputs *bool `json:"trainOnInputs,omitempty"`

	// +optional
	ZeroRetention *bool `json:"zeroRetention,omitempty"`

	// +optional
	DPASigned *bool `json:"dpaSigned,omitempty"`
}

// ModelEndpointDeployment captures the deployment mode.
type ModelEndpointDeployment struct {
	// +kubebuilder:validation:Enum=public_api;private_api;dedicated;on_premise;edge
	// +optional
	Mode string `json:"mode,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	KserveRef string `json:"kserveRef,omitempty"`
}

// ModelEndpointSpec is the desired state.
type ModelEndpointSpec struct {
	// Provider name.
	//
	// +kubebuilder:validation:Enum=openai;anthropic;azure_openai;bedrock;vertex;aliyun_dashscope;tencent_hunyuan;volcengine_ark;baichuan;moonshot;deepseek;zhipu;minimax;self_hosted;vllm;sglang;tgi;ollama;custom
	Provider string `json:"provider"`

	// Model is the provider-specific model name.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Model string `json:"model"`

	// Endpoint URL.
	//
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=2048
	Endpoint string `json:"endpoint"`

	// +kubebuilder:validation:MaxLength=64
	// +optional
	Region string `json:"region,omitempty"`

	// +optional
	Authentication *ModelEndpointAuthentication `json:"authentication,omitempty"`

	// Capabilities the endpoint supports.
	//
	// +listType=set
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	ContextWindow *int32 `json:"contextWindow,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxOutputTokens *int32 `json:"maxOutputTokens,omitempty"`

	// +optional
	Cost *ModelEndpointCost `json:"cost,omitempty"`

	// Compliance regimes the endpoint is approved for.
	// +optional
	Compliance []string `json:"compliance,omitempty"`

	// +optional
	DataResidency *ModelEndpointResidency `json:"dataResidency,omitempty"`

	// +optional
	Quota *ModelEndpointQuota `json:"quota,omitempty"`

	// +optional
	Privacy *ModelEndpointPrivacy `json:"privacy,omitempty"`

	// Fallback chain.
	// +optional
	Fallback []shared.ResourceRef `json:"fallback,omitempty"`

	// +optional
	Deployment *ModelEndpointDeployment `json:"deployment,omitempty"`
}

// ModelEndpointStatus is the observed state.
type ModelEndpointStatus struct {
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

	// +optional
	Healthy *bool `json:"healthy,omitempty"`

	// +optional
	LastProbeAt *metav1.Time `json:"lastProbeAt,omitempty"`

	// +optional
	CurrentTpm *int64 `json:"currentTpm,omitempty"`

	// +optional
	CurrentRpm *int64 `json:"currentRpm,omitempty"`

	// +optional
	ErrorRate24h *float64 `json:"errorRate24h,omitempty"`

	// +optional
	AvgLatencyMs *int32 `json:"avgLatencyMs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=me,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ModelEndpoint is a concrete model instance reachable by the platform.
type ModelEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelEndpointSpec   `json:"spec,omitempty"`
	Status ModelEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelEndpointList is the canonical list type for ModelEndpoint.
type ModelEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelEndpoint{}, &ModelEndpointList{})
}
