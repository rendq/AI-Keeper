package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Skill conditions (design.md §5.3 / §6.1.4).
const (
	SkillSchemaValid          = "SchemaValid"
	SkillDependenciesResolved = "DependenciesResolved"
	SkillImplementationReady  = "ImplementationReady"
	SkillRegistered           = "Registered"
	SkillEvalPassing          = "EvalPassing"
	SkillSLOMet               = "SLOMet"
	SkillDeprecating          = "Deprecating"
	SkillReady                = "Ready"
)

// SkillExample is an input/output example pair attached to the Skill
// interface. Inputs and outputs are kept open for forward compatibility
// (`x-kubernetes-preserve-unknown-fields`).
type SkillExample struct {
	// Note explains the example.
	// +optional
	Note string `json:"note,omitempty"`

	// Input is a free-form example payload.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Input *apiextensionsv1.JSON `json:"input,omitempty"`

	// Output is a free-form example payload.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Output *apiextensionsv1.JSON `json:"output,omitempty"`
}

// SkillIO holds a JSON Schema describing input or output of a Skill.
type SkillIO struct {
	// Schema is the JSON Schema. Stored as opaque JSON so we can preserve
	// arbitrary OpenAPI features without taking a hard dep on Draft-7 vs
	// 2020-12 semantics.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	Schema *apiextensionsv1.JSON `json:"schema"`
}

// SkillInterface declares the IO contract of a Skill.
type SkillInterface struct {
	Input  SkillIO `json:"input"`
	Output SkillIO `json:"output"`

	// Examples for the developer-facing UI.
	// +optional
	Examples []SkillExample `json:"examples,omitempty"`
}

// SkillRuntime describes how the implementation is deployed.
type SkillRuntime struct {
	// Engine identifies the runtime engine (e.g. aip-runtime/v2,
	// langgraph, temporal, custom).
	// +optional
	Engine string `json:"engine,omitempty"`

	// Entrypoint is the function symbol to call.
	// +optional
	Entrypoint string `json:"entrypoint,omitempty"`

	// Image is the container image to run.
	// +optional
	Image string `json:"image,omitempty"`
}

// SkillPromptTemplate references or inlines a prompt template.
type SkillPromptTemplate struct {
	// Ref to a prompt://... resource.
	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`

	// Inline template text.
	//
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	Inline string `json:"inline,omitempty"`
}

// SkillModelDep is a dependency on a model alias.
type SkillModelDep struct {
	// Alias used by the runtime (e.g. reasoner, embedder).
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	Alias string `json:"alias"`

	// Ref points at the ModelEndpoint or ModelRouter.
	Ref shared.ResourceRef `json:"ref"`

	// Purpose is the reason this dependency exists.
	//
	// +kubebuilder:validation:Enum=reasoning;embedding;vision;code;classification;rerank
	// +optional
	Purpose string `json:"purpose,omitempty"`

	// Fallback chain used when the primary endpoint fails.
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

// SkillSubSkillDep depends on another Skill with an optional version
// constraint string (npm-style range, validated by the resolver, not at
// admission time).
type SkillSubSkillDep struct {
	Ref shared.ResourceRef `json:"ref"`

	// VersionConstraint is an npm-style semver range.
	//
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
	// Type of implementation.
	//
	// +kubebuilder:validation:Enum=function;workflow;agentic;mcp_tool;external_api
	Type string `json:"type"`

	// Runtime parameters.
	// +optional
	Runtime *SkillRuntime `json:"runtime,omitempty"`

	// PromptTemplate to apply.
	// +optional
	PromptTemplate *SkillPromptTemplate `json:"promptTemplate,omitempty"`

	// Requires lists external resources this Skill depends on.
	// +optional
	Requires *SkillRequires `json:"requires,omitempty"`
}

// SkillEvaluation describes the offline eval suite attached to the Skill.
type SkillEvaluation struct {
	// EvalSet ref.
	// +optional
	EvalSet *shared.ResourceRef `json:"evalSet,omitempty"`

	// RedTeamSet ref.
	// +optional
	RedTeamSet *shared.ResourceRef `json:"redTeamSet,omitempty"`

	// Gates for stage promotion. Outer key = target stage, inner map =
	// metric → CEL expression.
	// +optional
	Gates map[string]map[string]string `json:"gates,omitempty"`

	// Schedule is a cron string controlling Eval_Runner cadence.
	//
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Schedule string `json:"schedule,omitempty"`
}

// SkillDeprecation captures the lifecycle.deprecation block.
type SkillDeprecation struct {
	// Successor Skill ref users should migrate to.
	// +optional
	Successor *shared.ResourceRef `json:"successor,omitempty"`

	// SunsetAt is the cutoff time after which the Skill stops accepting
	// new references.
	// +optional
	SunsetAt *metav1.Time `json:"sunsetAt,omitempty"`

	// MigrationGuide reference.
	// +optional
	MigrationGuide *shared.ResourceRef `json:"migrationGuide,omitempty"`
}

// SkillLifecycle captures the spec.lifecycle block.
type SkillLifecycle struct {
	// +optional
	Deprecation *SkillDeprecation `json:"deprecation,omitempty"`
}

// SkillSpec is the desired state of a Skill.
type SkillSpec struct {
	// Version is the skill version (strict semver).
	Version shared.SemVer `json:"version"`

	// Stability stage (experimental / beta / stable / deprecated).
	Stability shared.Stage `json:"stability"`

	// Interface contract.
	Interface SkillInterface `json:"interface"`

	// Implementation runtime.
	Implementation SkillImplementation `json:"implementation"`

	// Governance block (classification, residency, PII, compliance,
	// retention).
	// +optional
	Governance *shared.GovernanceBlock `json:"governance,omitempty"`

	// Cost block (per-call estimator + budget).
	// +optional
	Cost *shared.CostBlock `json:"cost,omitempty"`

	// SLO block.
	// +optional
	SLO *shared.SLOBlock `json:"slo,omitempty"`

	// Reliability (timeout / retry / fallback / circuit-breaker).
	// +optional
	Reliability *shared.ReliabilityBlock `json:"reliability,omitempty"`

	// Evaluation suite for stage gating.
	// +optional
	Evaluation *SkillEvaluation `json:"evaluation,omitempty"`

	// Lifecycle metadata.
	// +optional
	Lifecycle *SkillLifecycle `json:"lifecycle,omitempty"`
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

	// Metrics is metric → score.
	// +optional
	Metrics map[string]string `json:"metrics,omitempty"`

	// Passed indicates whether gates were satisfied.
	// +optional
	Passed *bool `json:"passed,omitempty"`
}

// SkillResolvedModel records a resolved model dependency.
type SkillResolvedModel struct {
	Alias       string             `json:"alias"`
	ResolvedRef shared.ResourceRef `json:"resolvedRef"`
}

// SkillResolvedDependencies is what the controller wrote back after the
// dependency resolver succeeded (Requirement A3.3).
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

	// ReferencingAgents is the back-pointer used by the deletion finaliser
	// (Requirement A3.11).
	// +optional
	ReferencingAgents []string `json:"referencingAgents,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=sk,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.spec.stability`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Skill is the platform's unit of reusable business capability.
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
