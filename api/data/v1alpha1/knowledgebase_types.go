package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// KnowledgeBase conditions.
const (
	KnowledgeBaseSourcesReady = "SourcesReady"
	KnowledgeBaseIndexed      = "Indexed"
	KnowledgeBaseSynced       = "Synced"
	KnowledgeBaseReady        = "Ready"
)

// KBSourceSync controls per-source sync cadence.
type KBSourceSync struct {
	// +kubebuilder:validation:Enum=full;incremental;cdc;manual
	// +optional
	Mode string `json:"mode,omitempty"`

	// +kubebuilder:validation:MaxLength=128
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	WatermarkField string `json:"watermarkField,omitempty"`
}

// KBSource is one DataSource binding.
type KBSource struct {
	Ref shared.ResourceRef `json:"ref"`

	// +optional
	Sync *KBSourceSync `json:"sync,omitempty"`
}

// KBChunking config.
type KBChunking struct {
	// +kubebuilder:validation:Enum=fixed;semantic;recursive;structural;hybrid
	// +optional
	Strategy string `json:"strategy,omitempty"`

	// +kubebuilder:validation:Minimum=64
	// +kubebuilder:validation:Maximum=8192
	// +optional
	MaxTokens *int32 `json:"maxTokens,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	Overlap *int32 `json:"overlap,omitempty"`
}

// KBEmbedding config.
type KBEmbedding struct {
	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +optional
	Dimensions *int32 `json:"dimensions,omitempty"`
}

// KBPipeline config.
type KBPipeline struct {
	// +optional
	Chunking *KBChunking `json:"chunking,omitempty"`

	// +optional
	Embedding *KBEmbedding `json:"embedding,omitempty"`

	// Enrichment passes (subset of allowed names; controllers tolerate
	// unknown values for forward compatibility).
	//
	// +listType=set
	// +optional
	Enrichment []string `json:"enrichment,omitempty"`
}

// KBIndex captures the storage backends.
type KBIndex struct {
	VectorStore shared.ResourceRef `json:"vectorStore"`

	// +optional
	GraphStore *shared.ResourceRef `json:"graphStore,omitempty"`

	// +optional
	FullTextStore *shared.ResourceRef `json:"fullTextStore,omitempty"`

	// +optional
	HybridSearch *bool `json:"hybridSearch,omitempty"`

	// +optional
	MultiTenant *bool `json:"multiTenant,omitempty"`
}

// KBRetrieval captures the retrieval block.
type KBRetrieval struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TopK *int32 `json:"topK,omitempty"`

	// +optional
	Reranker *shared.ResourceRef `json:"reranker,omitempty"`

	// Filters are CEL expressions over chunk metadata.
	// +optional
	Filters []string `json:"filters,omitempty"`
}

// KBACL controls KB-level access enforcement.
type KBACL struct {
	// +kubebuilder:validation:Enum=inherit_from_source;custom
	// +optional
	Mode string `json:"mode,omitempty"`

	// +kubebuilder:validation:Enum=pre_filter;post_filter
	// +optional
	Enforcement string `json:"enforcement,omitempty"`
}

// KBGovernance is GovernanceBlock + retention.
type KBGovernance struct {
	shared.GovernanceBlock `json:",inline"`

	// +optional
	Retention *shared.Duration `json:"retention,omitempty"`
}

// KnowledgeBaseSpec is the desired state.
type KnowledgeBaseSpec struct {
	// +kubebuilder:validation:MinItems=1
	Sources []KBSource `json:"sources"`

	Pipeline KBPipeline `json:"pipeline"`

	Index KBIndex `json:"index"`

	// +optional
	Retrieval *KBRetrieval `json:"retrieval,omitempty"`

	// +optional
	ACL *KBACL `json:"acl,omitempty"`

	// +optional
	Governance *KBGovernance `json:"governance,omitempty"`
}

// KBQualityMetrics surfaces eval results from the retrieval QA loop.
type KBQualityMetrics struct {
	// +optional
	AvgRecall *float64 `json:"avgRecall,omitempty"`

	// +optional
	AvgMRR *float64 `json:"avgMRR,omitempty"`

	// +optional
	LastEvalAt *metav1.Time `json:"lastEvalAt,omitempty"`
}

// KnowledgeBaseStatus is the observed state.
type KnowledgeBaseStatus struct {
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
	DocumentCount int64 `json:"documentCount,omitempty"`

	// +optional
	ChunkCount int64 `json:"chunkCount,omitempty"`

	// +optional
	IndexSizeBytes int64 `json:"indexSizeBytes,omitempty"`

	// +optional
	LastIndexedAt *metav1.Time `json:"lastIndexedAt,omitempty"`

	// +optional
	QualityMetrics *KBQualityMetrics `json:"qualityMetrics,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=kb,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Documents",type=integer,JSONPath=`.status.documentCount`
// +kubebuilder:printcolumn:name="Chunks",type=integer,JSONPath=`.status.chunkCount`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KnowledgeBase is a retrieval-ready aggregation of DataSources.
type KnowledgeBase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KnowledgeBaseSpec   `json:"spec,omitempty"`
	Status KnowledgeBaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KnowledgeBaseList is the canonical list type for KnowledgeBase.
type KnowledgeBaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KnowledgeBase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KnowledgeBase{}, &KnowledgeBaseList{})
}
