package v1beta1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Skill conditions (carried from v1alpha1 + new ones for v1beta1).
const (
	SkillSchemaValid          = "SchemaValid"
	SkillDependenciesResolved = "DependenciesResolved"
	SkillImplementationReady  = "ImplementationReady"
	SkillRegistered           = "Registered"
	SkillEvalPassing          = "EvalPassing"
	SkillSLOMet               = "SLOMet"
	SkillDeprecating          = "Deprecating"
	SkillReady                = "Ready"
	SkillComplianceVerified   = "ComplianceVerified" // new in v1beta1
)

// SkillExample is an input/output example pair attached to the Skill interface.
type SkillExample struct {
	// +optional
	Note string `json:"note,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Input *apiextensionsv1.JSON `json:"input,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Output *apiextensionsv1.JSON `json:"output,omitempty"`
}

// SkillIO holds a JSON Schema describing input or output of a Skill.
type SkillIO struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	Schema *apiextensionsv1.JSON `json:"schema"`
}

// SkillInterface declares the IO contract of a Skill.
type SkillInterface struct {
	Input  SkillIO `json:"input"`
	Output SkillIO `json:"output"`

	// +optional
	Examples []SkillExample `json:"examples,omitempty"`
}

// SkillRuntime describes how the implementation is deployed.
type SkillRuntime struct {
	// +optional
	Engine string `json:"engine,omitempty"`
	// +optional
	Entrypoint string `json:"entrypoint,omitempty"`
	// +optional
	Image string `json:"image,omitempty"`
}

// SkillPromptTemplate references or inlines a prompt template.
type SkillPromptTemplate struct {
	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`

	// +kubebuilder:validation:MaxLength=65536
	// +optional
	Inline string `json:"inline,omitempty"`
}

// SkillModelDep is a dependency on a model alias.
type SkillModelDep struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Alias string `json:"alias"`

	Ref shared.ResourceRef `json:"ref"`

	// +kubebuilder:validation:Enum=reasoning;embedding;vision;code;classification;rerank
	// +optional
	Purpose string `json:"purpose,omitempty"`

	// +optional
	Fallback []shared.ResourceRef `json:"fallback,omitempty"`
}

// SkillToolDep depends on a Tool resource.
type SkillToolDep struct {
	Ref shared.ResourceRef `json:"ref"`
}

// SkillDataSourceDep depends on a DataSource (or KnowledgeBase).
type SkillDataSourceDep struct {
	Ref shared.ResourceRef `json:"ref"`
}

// SkillSubSkillDep depends on another Skill.
type SkillSubSkillDep struct {
	Ref shared.ResourceRef `json:"ref"`

	// +kubebuilder:validation:MaxLength=128
	// +optional
	VersionConstraint string `json:"versionConstraint,omitempty"`
}

// SkillRequires lists external dependencies of a Skill.
type SkillRequires struct {
	// +optional
	Models []SkillModelDep `json:"models,omitempty"`
	// +optional
	Tools []SkillToolDep `json:"tools,omitempty"`
	// +optional
	DataSources []SkillDataSourceDep `json:"dataSources,omitempty"`
	// +optional
	Skills []SkillSubSkillDep `json:"skills,omitempty"`
}

// SkillImplementation declares how a Skill runs.
type SkillImplementation struct {
	// +kubebuilder:validation:Enum=function;workflow;agentic;mcp_tool;external_api
	Type string `json:"type"`

	// +optional
	Runtime *SkillRuntime `json:"runtime,omitempty"`
	// +optional
	PromptTemplate *SkillPromptTemplate `json:"promptTemplate,omitempty"`
	// +optional
	Requires *SkillRequires `json:"requires,omitempty"`
}

// --- New in v1beta1: Compliance block ---

// SkillComplianceStandard references a specific compliance standard.
type SkillComplianceStandard struct {
	// Name of the compliance standard (e.g. "SOC2", "GDPR", "HIPAA", "ISO27001").
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Name string `json:"name"`

	// Controls lists the specific control IDs satisfied.
	// +optional
	Controls []string `json:"controls,omitempty"`
}

// SkillCompliance captures regulatory compliance metadata for the Skill.
// New in v1beta1 — not present in v1alpha1.
type SkillCompliance struct {
	// Standards lists the compliance frameworks this Skill satisfies.
	// +optional
	Standards []SkillComplianceStandard `json:"standards,omitempty"`

	// DataClassification indicates the highest data sensitivity level.
	//
	// +kubebuilder:validation:Enum=public;internal;confidential;restricted
	// +optional
	DataClassification string `json:"dataClassification,omitempty"`

	// AuditRequired forces forensic-level auditing when true.
	// +optional
	AuditRequired *bool `json:"auditRequired,omitempty"`
}

// --- Enhanced Evaluation in v1beta1 ---

// SkillEvalMetric defines a single eval metric with thresholds.
// New in v1beta1 — provides structured metric definitions.
type SkillEvalMetric struct {
	// Name of the metric (e.g. "accuracy", "latency_p95", "toxicity").
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Name string `json:"name"`

	// Threshold is a CEL expression evaluating whether the metric passes.
	//
	// +kubebuilder:validation:MaxLength=512
	// +optional
	Threshold string `json:"threshold,omitempty"`

	// Weight for composite scoring (0.0–1.0).
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	Weight *float64 `json:"weight,omitempty"`
}

// SkillEvaluation describes the offline eval suite attached to the Skill.
// Extended in v1beta1 with structured metrics.
type SkillEvaluation struct {
	// +optional
	EvalSet *shared.ResourceRef `json:"evalSet,omitempty"`
	// +optional
	RedTeamSet *shared.ResourceRef `json:"redTeamSet,omitempty"`

	// Gates for stage promotion. Outer key = target stage, inner map =
	// metric → CEL expression.
	// +optional
	Gates map[string]map[string]string `json:"gates,omitempty"`

	// +kubebuilder:validation:MaxLength=128
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Metrics provides structured metric definitions with thresholds.
	// New in v1beta1.
	// +optional
	Metrics []SkillEvalMetric `json:"metrics,omitempty"`

	// ContinuousEval enables always-on evaluation in production.
	// New in v1beta1.
	// +optional
	ContinuousEval *bool `json:"continuousEval,omitempty"`
}

// SkillDeprecation captures the lifecycle.deprecation block.
type SkillDeprecation struct {
	// +optional
	Successor *shared.ResourceRef `json:"successor,omitempty"`
	// +optional
	SunsetAt *metav1.Time `json:"sunsetAt,omitempty"`
	// +optional
	MigrationGuide *shared.ResourceRef `json:"migrationGuide,omitempty"`
}

// SkillLifecycle captures the spec.lifecycle block.
type SkillLifecycle struct {
	// +optional
	Deprecation *SkillDeprecation `json:"deprecation,omitempty"`
}

// SkillSpec is the desired state of a Skill (v1beta1).
type SkillSpec struct {
	// Version is the skill version (strict semver).
	Version shared.SemVer `json:"version"`

	// Stability stage (experimental / beta / stable / deprecated).
	Stability shared.Stage `json:"stability"`

	// Interface contract.
	Interface SkillInterface `json:"interface"`

	// Implementation runtime.
	Implementation SkillImplementation `json:"implementation"`

	// Governance block.
	// +optional
	Governance *shared.GovernanceBlock `json:"governance,omitempty"`

	// Cost block.
	// +optional
	Cost *shared.CostBlock `json:"cost,omitempty"`

	// SLO block.
	// +optional
	SLO *shared.SLOBlock `json:"slo,omitempty"`

	// Reliability (timeout / retry / fallback / circuit-breaker).
	// +optional
	Reliability *shared.ReliabilityBlock `json:"reliability,omitempty"`

	// Evaluation suite for stage gating (enhanced in v1beta1).
	// +optional
	Evaluation *SkillEvaluation `json:"evaluation,omitempty"`

	// Lifecycle metadata.
	// +optional
	Lifecycle *SkillLifecycle `json:"lifecycle,omitempty"`

	// Compliance block — new in v1beta1.
	// +optional
	Compliance *SkillCompliance `json:"compliance,omitempty"`
}

// SkillHealth summarises runtime health metrics.
type SkillHealth struct {
	// +optional
	P95LatencyMs *int32 `json:"p95LatencyMs,omitempty"`
	// +optional
	SuccessRate *float64 `json:"successRate,omitempty"`
	// +optional
	CostPerCallUsd *shared.MoneyAmount `json:"costPerCallUsd,omitempty"`
	// +optional
	Last24hInvocations int64 `json:"last24hInvocations,omitempty"`
}

// SkillEvalResults is the most recent evaluation summary.
type SkillEvalResults struct {
	// +optional
	LastRunAt *metav1.Time `json:"lastRunAt,omitempty"`
	// +optional
	Metrics map[string]string `json:"metrics,omitempty"`
	// +optional
	Passed *bool `json:"passed,omitempty"`
}

// SkillResolvedModel records a resolved model dependency.
type SkillResolvedModel struct {
	Alias       string             `json:"alias"`
	ResolvedRef shared.ResourceRef `json:"resolvedRef"`
}

// SkillResolvedDependencies is what the controller wrote back after the
// dependency resolver succeeded.
type SkillResolvedDependencies struct {
	// +optional
	Models []SkillResolvedModel `json:"models,omitempty"`
	// +optional
	Tools []shared.ResourceRef `json:"tools,omitempty"`
	// +optional
	DataSources []shared.ResourceRef `json:"dataSources,omitempty"`
	// +optional
	Skills []shared.ResourceRef `json:"skills,omitempty"`
}

// SkillStatus is the observed state of a Skill.
type SkillStatus struct {
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
	Health *SkillHealth `json:"health,omitempty"`

	// +optional
	EvalResults *SkillEvalResults `json:"evalResults,omitempty"`

	// +optional
	ResolvedDependencies *SkillResolvedDependencies `json:"resolvedDependencies,omitempty"`

	// +optional
	ReferencingAgents []string `json:"referencingAgents,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=sk,categories={aip}
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.spec.stability`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Skill is the platform's unit of reusable business capability (v1beta1).
type Skill struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillSpec   `json:"spec,omitempty"`
	Status SkillStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SkillList is the canonical list type for Skill.
type SkillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Skill `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Skill{}, &SkillList{})
}
