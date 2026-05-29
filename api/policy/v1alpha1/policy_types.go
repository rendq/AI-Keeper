package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Policy conditions (design.md §6.3.4).
const (
	PolicySyntaxValid           = "SyntaxValid"
	PolicyReferencesResolved    = "ReferencesResolved"
	PolicyNotConflicting        = "NotConflicting"
	PolicyCompiled              = "Compiled"
	PolicyDistributed           = "Distributed"
	PolicyFullyDistributed      = "FullyDistributed"
	PolicyWithinEffectiveWindow = "WithinEffectiveWindow"
	PolicyActive                = "Active"
	PolicyReady                 = "Ready"
)

// PolicyEffectiveWindow constrains when the policy is in force.
type PolicyEffectiveWindow struct {
	// +optional
	NotBefore *metav1.Time `json:"notBefore,omitempty"`

	// +optional
	NotAfter *metav1.Time `json:"notAfter,omitempty"`
}

// SubjectMatch captures the labels / attributes / name match of a
// SubjectSelector entry.
type SubjectMatch struct {
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Name string `json:"name,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Attributes is an open-ended ABAC bag.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Attributes *apiextensionsv1.JSON `json:"attributes,omitempty"`
}

// SubjectEntry is one principal class.
type SubjectEntry struct {
	// +kubebuilder:validation:Enum=User;Role;Group;Agent;ServiceAccount;Tenant;Anonymous
	Kind string `json:"kind"`

	// +optional
	Match *SubjectMatch `json:"match,omitempty"`
}

// SubjectSelector is the policy's subject side.
type SubjectSelector struct {
	// +kubebuilder:validation:MinItems=1
	AnyOf []SubjectEntry `json:"anyOf"`
}

// ResourceMatch matches resources by name / labels / classification.
type ResourceMatch struct {
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Name string `json:"name,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Classification is a comparison expression like '<= confidential'.
	//
	// +kubebuilder:validation:MaxLength=64
	// +optional
	Classification string `json:"classification,omitempty"`
}

// ResourceSelector is one entry in `action.resources.anyOf`.
type ResourceSelector struct {
	// +kubebuilder:validation:Enum=Skill;Agent;Tool;ModelEndpoint;DataSource;KnowledgeBase;Channel;Any
	Kind string `json:"kind"`

	// +optional
	Match *ResourceMatch `json:"match,omitempty"`
}

// PolicyActionResources is the action's resource side.
type PolicyActionResources struct {
	// +kubebuilder:validation:MinItems=1
	AnyOf []ResourceSelector `json:"anyOf"`
}

// PolicyAction declares what the policy applies to.
type PolicyAction struct {
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	Verbs []string `json:"verbs"`

	Resources PolicyActionResources `json:"resources"`
}

// ConditionTimeWindow captures schedule-style conditions.
type ConditionTimeWindow struct {
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// +kubebuilder:validation:MaxLength=64
	// +optional
	Timezone string `json:"timezone,omitempty"`
}

// ConditionLocation captures geo / IP conditions.
type ConditionLocation struct {
	// +optional
	Countries []string `json:"countries,omitempty"`

	// +optional
	Regions []string `json:"regions,omitempty"`

	// +optional
	IPAllowList []string `json:"ipAllowList,omitempty"`

	// +optional
	IPDenyList []string `json:"ipDenyList,omitempty"`
}

// ConditionRiskScore caps the allowable risk score.
type ConditionRiskScore struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	Min *float64 `json:"min,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	Max *float64 `json:"max,omitempty"`
}

// ConditionRequire captures step-up auth requirements.
type ConditionRequire struct {
	// +optional
	MFA *bool `json:"mfa,omitempty"`

	// +optional
	DeviceCompliant *bool `json:"deviceCompliant,omitempty"`

	// +optional
	SSO *bool `json:"sso,omitempty"`

	// +optional
	StepUpAuth *bool `json:"stepUpAuth,omitempty"`
}

// ConditionItem is one branch of conditions.allOf / anyOf / noneOf.
type ConditionItem struct {
	// +optional
	TimeWindow *ConditionTimeWindow `json:"timeWindow,omitempty"`

	// +optional
	Location *ConditionLocation `json:"location,omitempty"`

	// +optional
	DataClassificationCeiling *shared.Classification `json:"dataClassificationCeiling,omitempty"`

	// +optional
	RiskScore *ConditionRiskScore `json:"riskScore,omitempty"`

	// +optional
	Require *ConditionRequire `json:"require,omitempty"`

	// Expression is a CEL expression.
	//
	// +kubebuilder:validation:MaxLength=4096
	// +optional
	Expression string `json:"expression,omitempty"`
}

// ConditionSet is the conditions block.
type ConditionSet struct {
	// +optional
	AllOf []ConditionItem `json:"allOf,omitempty"`

	// +optional
	AnyOf []ConditionItem `json:"anyOf,omitempty"`

	// +optional
	NoneOf []ConditionItem `json:"noneOf,omitempty"`
}

// ConstraintBudget caps token / USD throughput.
type ConstraintBudget struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerUserPerDay *int64 `json:"tokensPerUserPerDay,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerUserPerMonth *int64 `json:"tokensPerUserPerMonth,omitempty"`

	// +optional
	UsdPerUserPerMonth *shared.MoneyAmount `json:"usdPerUserPerMonth,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerRequest *int64 `json:"tokensPerRequest,omitempty"`
}

// ConstraintRateLimit caps requests.
type ConstraintRateLimit struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	RequestsPerMinute *int32 `json:"requestsPerMinute,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	RequestsPerHour *int32 `json:"requestsPerHour,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	ConcurrentSessions *int32 `json:"concurrentSessions,omitempty"`
}

// PolicyConstraints is the constraints block.
type PolicyConstraints struct {
	// +optional
	Budget *ConstraintBudget `json:"budget,omitempty"`

	// +optional
	RateLimit *ConstraintRateLimit `json:"rateLimit,omitempty"`

	// +optional
	Quota *shared.ResourceRef `json:"quota,omitempty"`
}

// ApprovalWhen captures the trigger of an ApprovalSpec.
type ApprovalWhen struct {
	// +kubebuilder:validation:MaxLength=4096
	// +optional
	Expression string `json:"expression,omitempty"`
}

// ApprovalApprover identifies the principal authorised to approve.
type ApprovalApprover struct {
	// +kubebuilder:validation:Enum=User;Role;Group
	Kind string `json:"kind"`

	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// ApprovalSpec is one approval requirement.
type ApprovalSpec struct {
	// When is a CEL trigger expression.
	When ApprovalWhen `json:"when"`

	// Approver identifies the principal authorised to approve.
	Approver ApprovalApprover `json:"approver"`

	// +optional
	Timeout *shared.Duration `json:"timeout,omitempty"`

	// IfTimeout decides what happens when no answer arrives.
	//
	// +kubebuilder:validation:Enum=allow;deny;escalate
	// +optional
	IfTimeout string `json:"ifTimeout,omitempty"`
}

// ObligationAudit declares audit obligations.
type ObligationAudit struct {
	// +kubebuilder:validation:Enum=off;basic;high;forensic
	// +optional
	Level string `json:"level,omitempty"`

	// +optional
	IncludePromptHashes *bool `json:"includePromptHashes,omitempty"`

	// +optional
	ForwardTo []shared.ResourceRef `json:"forwardTo,omitempty"`
}

// ObligationRedact declares DLP redaction obligations.
type ObligationRedact struct {
	// +optional
	PatternsRef *shared.ResourceRef `json:"patternsRef,omitempty"`

	// +optional
	Fields []string `json:"fields,omitempty"`
}

// ObligationNotifyMatch is one notify trigger.
type ObligationNotifyMatch struct {
	// +kubebuilder:validation:MaxLength=4096
	Condition string `json:"condition"`

	// +kubebuilder:validation:MaxLength=253
	Channel string `json:"channel"`
}

// ObligationNotify declares notify obligations.
type ObligationNotify struct {
	// +optional
	OnMatch []ObligationNotifyMatch `json:"onMatch,omitempty"`
}

// ObligationWatermark declares watermark obligations.
type ObligationWatermark struct {
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +kubebuilder:validation:Enum=visible;invisible;both
	// +optional
	Mode string `json:"mode,omitempty"`

	// +kubebuilder:validation:MaxLength=512
	// +optional
	Text string `json:"text,omitempty"`
}

// PolicyObligations is the obligations block.
type PolicyObligations struct {
	// +optional
	Audit *ObligationAudit `json:"audit,omitempty"`

	// +optional
	Redact *ObligationRedact `json:"redact,omitempty"`

	// +optional
	Notify *ObligationNotify `json:"notify,omitempty"`

	// +optional
	Watermark *ObligationWatermark `json:"watermark,omitempty"`
}

// PolicySpec is the desired state of a Policy.
type PolicySpec struct {
	// Effect of the policy.
	//
	// +kubebuilder:validation:Enum=allow;deny
	Effect string `json:"effect"`

	// Priority — higher wins; ties go to deny.
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// Enabled defaults to true.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// EffectiveWindow restricts when the policy is in force.
	// +optional
	EffectiveWindow *PolicyEffectiveWindow `json:"effectiveWindow,omitempty"`

	// Subject the policy applies to.
	Subject SubjectSelector `json:"subject"`

	// Action describes verbs and resources.
	Action PolicyAction `json:"action"`

	// Conditions narrows when the policy fires.
	// +optional
	Conditions *ConditionSet `json:"conditions,omitempty"`

	// Constraints applied when the policy matches.
	// +optional
	Constraints *PolicyConstraints `json:"constraints,omitempty"`

	// Approvals required when matched.
	// +optional
	Approvals []ApprovalSpec `json:"approvals,omitempty"`

	// Obligations attached to allow / deny decisions.
	// +optional
	Obligations *PolicyObligations `json:"obligations,omitempty"`
}

// PolicyDecisions24h surfaces decision counters.
type PolicyDecisions24h struct {
	// +optional
	Allow int64 `json:"allow,omitempty"`

	// +optional
	Deny int64 `json:"deny,omitempty"`

	// +optional
	RequireApproval int64 `json:"requireApproval,omitempty"`
}

// PolicyConflict describes a conflict between Policies.
type PolicyConflict struct {
	// +kubebuilder:validation:MaxLength=512
	ConflictsWith string `json:"conflictsWith"`

	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Reason string `json:"reason,omitempty"`
}

// PolicyDistributionStatus tracks per-PDP rollout.
type PolicyDistributionStatus struct {
	// PDPInstance identifier.
	// +kubebuilder:validation:MaxLength=253
	PDPInstance string `json:"pdpInstance"`

	// AckedBundleHash records the hash the PDP currently has loaded.
	// +kubebuilder:validation:MaxLength=72
	// +optional
	AckedBundleHash string `json:"ackedBundleHash,omitempty"`

	// AckedAt records the last successful ack.
	// +optional
	AckedAt *metav1.Time `json:"ackedAt,omitempty"`
}

// PolicyStatus is the observed state of a Policy.
type PolicyStatus struct {
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

	// BundleVersion is the monotonic counter (Requirement A5.5).
	// +optional
	BundleVersion *int64 `json:"bundleVersion,omitempty"`

	// BundleHash of the latest compiled bundle (sha256:...).
	// +kubebuilder:validation:MaxLength=72
	// +optional
	BundleHash string `json:"bundleHash,omitempty"`

	// Distribution lists the per-PDP ack state.
	// +optional
	Distribution []PolicyDistributionStatus `json:"distribution,omitempty"`

	// EvaluationCount24h rolling counter.
	// +optional
	EvaluationCount24h int64 `json:"evaluationCount24h,omitempty"`

	// Decisions24h breakdown.
	// +optional
	Decisions24h *PolicyDecisions24h `json:"decisions24h,omitempty"`

	// Conflicts surfaces detected conflicts with other Policies.
	// +optional
	Conflicts []PolicyConflict `json:"conflicts,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=pol,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Effect",type=string,JSONPath=`.spec.effect`
// +kubebuilder:printcolumn:name="Priority",type=integer,JSONPath=`.spec.priority`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Policy is the ABAC + Obligations declarative authorization rule.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicySpec   `json:"spec,omitempty"`
	Status PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PolicyList is the canonical list type for Policy.
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Policy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
}
