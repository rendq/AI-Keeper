package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// ModelRouter conditions.
const (
	ModelRouterCompiled     = "Compiled"
	ModelRouterDistributed  = "Distributed"
	ModelRouterAllReachable = "AllReachable"
	ModelRouterReady        = "Ready"
)

// ModelRouterRuleWhen captures rule predicates.
type ModelRouterRuleWhen struct {
	// Expression is a CEL predicate.
	//
	// +kubebuilder:validation:MaxLength=4096
	// +optional
	Expression string `json:"expression,omitempty"`

	// +kubebuilder:validation:Enum=chat;classify;extract;summarize;code;math;vision;embedding
	// +optional
	TaskType string `json:"taskType,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	ContextLengthMin *int32 `json:"contextLengthMin,omitempty"`

	// +optional
	CostSensitive *bool `json:"costSensitive,omitempty"`

	// +optional
	LatencySensitive *bool `json:"latencySensitive,omitempty"`

	// +optional
	Compliance []string `json:"compliance,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	Tenant string `json:"tenant,omitempty"`
}

// ModelRouterRule is one routing rule.
type ModelRouterRule struct {
	// +optional
	When *ModelRouterRuleWhen `json:"when,omitempty"`

	Endpoint shared.ResourceRef `json:"endpoint"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	Weight *int32 `json:"weight,omitempty"`
}

// ModelRouterCache configures semantic caching.
type ModelRouterCache struct {
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +kubebuilder:validation:Enum=exact;semantic;hybrid
	// +optional
	Mode string `json:"mode,omitempty"`

	// +optional
	TTL *shared.Duration `json:"ttl,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	SimilarityThreshold *float64 `json:"similarityThreshold,omitempty"`

	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`
}

// ModelRouterSpec is the desired state.
type ModelRouterSpec struct {
	// Alias the upstream uses (e.g. "reasoner", "embedder").
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Alias string `json:"alias"`

	// +optional
	DefaultEndpoint *shared.ResourceRef `json:"defaultEndpoint,omitempty"`

	// +kubebuilder:validation:MinItems=1
	Rules []ModelRouterRule `json:"rules"`

	// +optional
	Cache *ModelRouterCache `json:"cache,omitempty"`

	// LoadBalancing strategy.
	//
	// +kubebuilder:validation:Enum=round_robin;least_latency;least_cost;weighted;sticky_session
	// +optional
	LoadBalancing string `json:"loadBalancing,omitempty"`
}

// ModelRouterDistribution is per-endpoint routing distribution.
type ModelRouterDistribution struct {
	Endpoint shared.ResourceRef `json:"endpoint"`

	// +optional
	Weight *int32 `json:"weight,omitempty"`

	// +optional
	Requests24h int64 `json:"requests24h,omitempty"`
}

// ModelRouterStatus is the observed state.
type ModelRouterStatus struct {
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
	RequestsRouted24h int64 `json:"requestsRouted24h,omitempty"`

	// +optional
	CacheHitRate *float64 `json:"cacheHitRate,omitempty"`

	// +optional
	Distribution []ModelRouterDistribution `json:"distribution,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=mr,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Alias",type=string,JSONPath=`.spec.alias`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ModelRouter routes a logical model alias to one or more concrete
// ModelEndpoints.
type ModelRouter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelRouterSpec   `json:"spec,omitempty"`
	Status ModelRouterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelRouterList is the canonical list type for ModelRouter.
type ModelRouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelRouter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelRouter{}, &ModelRouterList{})
}
