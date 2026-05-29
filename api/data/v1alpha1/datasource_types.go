package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// DataSource conditions.
const (
	DataSourceConnected   = "Connected"
	DataSourceSyncing     = "Syncing"
	DataSourceACLEnforced = "ACLEnforced"
	DataSourceReady       = "Ready"
)

// DataSourceConnector identifies how the connector is wired.
type DataSourceConnector struct {
	// Kind of connector.
	//
	// +kubebuilder:validation:Enum=feishu_wiki;feishu_doc;wecom_doc;confluence;notion;sharepoint;jira;gitlab;github;postgres;mysql;mongodb;elasticsearch;snowflake;databricks;s3;oss;kafka;mcp_generic;http_api;custom
	Kind string `json:"kind"`

	// Ref to a `connector://` resource (e.g. shared connector template).
	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`

	// Config is connector-specific opaque config.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Config *apiextensionsv1.JSON `json:"config,omitempty"`
}

// DataSourceSecretRef points at the credential.
type DataSourceSecretRef struct {
	Name string `json:"name"`
	// +optional
	Key string `json:"key,omitempty"`
}

// DataSourceAuthentication captures auth.
type DataSourceAuthentication struct {
	// +kubebuilder:validation:Enum=none;api_key;oauth2;oauth2_obo;basic;mtls;iam
	Mode string `json:"mode"`

	// +optional
	SecretRef *DataSourceSecretRef `json:"secretRef,omitempty"`
}

// DataSourceACL controls visibility.
type DataSourceACL struct {
	// +kubebuilder:validation:Enum=open;inherit_from_source;custom;deny_all
	// +optional
	Mode string `json:"mode,omitempty"`

	// +kubebuilder:validation:Enum=pre_filter;post_filter;hybrid
	// +optional
	Enforcement string `json:"enforcement,omitempty"`

	// +optional
	PolicyRef *shared.ResourceRef `json:"policyRef,omitempty"`
}

// DataSourceGovernance extends GovernanceBlock with retention policy.
type DataSourceGovernance struct {
	shared.GovernanceBlock `json:",inline"`

	// +optional
	Retention *shared.Duration `json:"retention,omitempty"`

	// +kubebuilder:validation:Enum=soft;hard;never
	// +optional
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// DataSourceSpec is the desired state.
type DataSourceSpec struct {
	Connector DataSourceConnector `json:"connector"`

	// +optional
	Authentication *DataSourceAuthentication `json:"authentication,omitempty"`

	// AccessMode controls allowed verbs.
	//
	// +kubebuilder:validation:Enum=read_only;read_write;write_only;stream
	// +optional
	AccessMode string `json:"accessMode,omitempty"`

	// +optional
	ACL *DataSourceACL `json:"acl,omitempty"`

	// +optional
	Governance *DataSourceGovernance `json:"governance,omitempty"`
}

// DataSourceStatus is the observed state.
type DataSourceStatus struct {
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
	Connected *bool `json:"connected,omitempty"`

	// +optional
	LastSyncAt *metav1.Time `json:"lastSyncAt,omitempty"`

	// +optional
	DocumentCount int64 `json:"documentCount,omitempty"`

	// +optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=ds,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Connector",type=string,JSONPath=`.spec.connector.kind`
// +kubebuilder:printcolumn:name="Connected",type=boolean,JSONPath=`.status.connected`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DataSource is a connector-backed source of truth.
type DataSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataSourceSpec   `json:"spec,omitempty"`
	Status DataSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DataSourceList is the canonical list type for DataSource.
type DataSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataSource{}, &DataSourceList{})
}
