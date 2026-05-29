package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// AuditEventTrace captures OTel correlation ids.
type AuditEventTrace struct {
	// +kubebuilder:validation:MaxLength=64
	// +optional
	TraceID string `json:"traceId,omitempty"`

	// +kubebuilder:validation:MaxLength=32
	// +optional
	SpanID string `json:"spanId,omitempty"`

	// +kubebuilder:validation:MaxLength=32
	// +optional
	ParentSpanID string `json:"parentSpanId,omitempty"`
}

// AuditPrincipalUser captures the human user (if any).
type AuditPrincipalUser struct {
	// +kubebuilder:validation:MaxLength=253
	// +optional
	ID string `json:"id,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	TenantID string `json:"tenantId,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	Department string `json:"department,omitempty"`

	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`
}

// AuditPrincipalAgent captures the Agent that handled the call.
type AuditPrincipalAgent struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// +kubebuilder:validation:MaxLength=64
	// +optional
	Version string `json:"version,omitempty"`
}

// AuditPrincipalSA captures the ServiceAccount.
type AuditPrincipalSA struct {
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Name string `json:"name,omitempty"`

	// +kubebuilder:validation:Pattern=`^spiffe://[A-Za-z0-9._\-]+(/[A-Za-z0-9._\-]+)*$`
	// +optional
	SpiffeID string `json:"spiffeId,omitempty"`
}

// AuditPrincipal captures the "who".
type AuditPrincipal struct {
	// +optional
	User *AuditPrincipalUser `json:"user,omitempty"`

	// Agent is required.
	Agent AuditPrincipalAgent `json:"agent"`

	// +optional
	ServiceAccount *AuditPrincipalSA `json:"serviceAccount,omitempty"`

	// OnBehalfOf records the end-user id under OBO.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	OnBehalfOf string `json:"onBehalfOf,omitempty"`

	// +kubebuilder:validation:MaxLength=64
	// +optional
	SourceIP string `json:"sourceIp,omitempty"`

	// +kubebuilder:validation:MaxLength=512
	// +optional
	UserAgent string `json:"userAgent,omitempty"`

	// +kubebuilder:validation:Enum=feishu;wecom;dingtalk;slack;teams;web;api;sdk;voice;email;internal
	// +optional
	Channel string `json:"channel,omitempty"`
}

// AuditAction captures the "what".
type AuditAction struct {
	// +kubebuilder:validation:Enum=invoke;read;write;delete;admin;list;watch
	Verb string `json:"verb"`

	Resource shared.ResourceRef `json:"resource"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	Method string `json:"method,omitempty"`
}

// AuditPolicy captures the decision.
type AuditPolicy struct {
	// +kubebuilder:validation:Enum=allow;deny;require_approval;error
	Decision string `json:"decision"`

	// +optional
	MatchedPolicies []string `json:"matchedPolicies,omitempty"`

	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Reason string `json:"reason,omitempty"`

	// +optional
	ObligationsApplied []string `json:"obligationsApplied,omitempty"`

	// +kubebuilder:validation:MaxLength=128
	// +optional
	ApprovalID string `json:"approvalId,omitempty"`
}

// AuditRequest captures inbound IO metadata (no raw bodies).
type AuditRequest struct {
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	// +optional
	InputHash string `json:"inputHash,omitempty"`

	// +optional
	InputRef *shared.ResourceRef `json:"inputRef,omitempty"`

	// +optional
	Classification *shared.Classification `json:"classification,omitempty"`

	// Redactions lists detected PII type names (no values).
	// +optional
	Redactions []string `json:"redactions,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	SizeBytes *int64 `json:"sizeBytes,omitempty"`

	// +kubebuilder:validation:Pattern=`^[a-z]{2}(-[A-Z]{2})?$`
	// +optional
	Language string `json:"language,omitempty"`
}

// AuditCitation describes one retrieval citation.
type AuditCitation struct {
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Source string `json:"source,omitempty"`

	// +kubebuilder:validation:MaxLength=128
	// +optional
	ChunkID string `json:"chunkId,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	Score *float64 `json:"score,omitempty"`
}

// AuditResponse captures outbound IO metadata.
type AuditResponse struct {
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	// +optional
	OutputHash string `json:"outputHash,omitempty"`

	// +optional
	OutputRef *shared.ResourceRef `json:"outputRef,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	Confidence *float64 `json:"confidence,omitempty"`

	// +optional
	Citations []AuditCitation `json:"citations,omitempty"`

	// +optional
	Classification *shared.Classification `json:"classification,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	SizeBytes *int64 `json:"sizeBytes,omitempty"`
}

// AuditStep is one entry in the agent thought-tree.
type AuditStep struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	Index *int32 `json:"index,omitempty"`

	// +kubebuilder:validation:Enum=thought;tool_call;model_call;observation;final
	// +optional
	Type string `json:"type,omitempty"`

	// +optional
	Tool *shared.ResourceRef `json:"tool,omitempty"`

	// +optional
	Model *shared.ResourceRef `json:"model,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensIn *int64 `json:"tokensIn,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensOut *int64 `json:"tokensOut,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	LatencyMs *int64 `json:"latencyMs,omitempty"`

	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Error string `json:"error,omitempty"`
}

// AuditCostTokens is the per-call token breakdown.
type AuditCostTokens struct {
	// +optional
	Input int64 `json:"input,omitempty"`

	// +optional
	Output int64 `json:"output,omitempty"`

	// +optional
	Cached int64 `json:"cached,omitempty"`
}

// AuditCost is the per-call cost.
type AuditCost struct {
	// +optional
	Tokens *AuditCostTokens `json:"tokens,omitempty"`

	// +optional
	Usd *shared.MoneyAmount `json:"usd,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	DurationMs *int64 `json:"durationMs,omitempty"`
}

// AuditGuardrailHit records a guardrail rule trip.
type AuditGuardrailHit struct {
	// +kubebuilder:validation:MaxLength=128
	Rule string `json:"rule"`

	// +kubebuilder:validation:Enum=input;output;behavior
	// +optional
	Stage string `json:"stage,omitempty"`

	// +kubebuilder:validation:Enum=allow;mask;block;warn;escalate
	// +optional
	Action string `json:"action,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	Score *float64 `json:"score,omitempty"`
}

// AuditGuardrails captures guardrail outcome.
type AuditGuardrails struct {
	// +optional
	Triggered []AuditGuardrailHit `json:"triggered,omitempty"`

	// +optional
	Blocked *bool `json:"blocked,omitempty"`
}

// AuditCompliance captures compliance metadata.
type AuditCompliance struct {
	// +optional
	Tags []string `json:"tags,omitempty"`

	// +kubebuilder:validation:MaxLength=64
	// +optional
	DataResidency string `json:"dataResidency,omitempty"`

	// +optional
	Reviewed *bool `json:"reviewed,omitempty"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	ReviewedBy string `json:"reviewedBy,omitempty"`

	// +optional
	ReviewedAt *metav1.Time `json:"reviewedAt,omitempty"`

	// Holds is the legal-hold tag set. Non-empty values prevent retention
	// GC (Requirement D4 / F22).
	// +optional
	Holds []string `json:"holds,omitempty"`
}

// AuditOutcome captures terminal status.
type AuditOutcome struct {
	// +kubebuilder:validation:Enum=success;partial;failed;timeout;blocked;cancelled
	// +optional
	Status string `json:"status,omitempty"`

	// +kubebuilder:validation:MaxLength=64
	// +optional
	ErrorCode string `json:"errorCode,omitempty"`

	// +kubebuilder:validation:MaxLength=2048
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// AuditEventSpec is the AuditEvent payload. Per Requirement A1.5 the API
// server rejects user-initiated CREATE/UPDATE/DELETE; this type still
// declares no `status` because the AuditEvent is itself the status.
type AuditEventSpec struct {
	// InvocationID is a UUID assigned by the Gateway.
	//
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	InvocationID string `json:"invocationId"`

	// Timestamp the event was emitted.
	Timestamp metav1.Time `json:"timestamp"`

	// +optional
	Trace *AuditEventTrace `json:"trace,omitempty"`

	// Principal is required.
	Principal AuditPrincipal `json:"principal"`

	// Action is required.
	Action AuditAction `json:"action"`

	// +optional
	Policy *AuditPolicy `json:"policy,omitempty"`

	// +optional
	Request *AuditRequest `json:"request,omitempty"`

	// +optional
	Response *AuditResponse `json:"response,omitempty"`

	// +optional
	Steps []AuditStep `json:"steps,omitempty"`

	// +optional
	Cost *AuditCost `json:"cost,omitempty"`

	// +optional
	Guardrails *AuditGuardrails `json:"guardrails,omitempty"`

	// +optional
	Compliance *AuditCompliance `json:"compliance,omitempty"`

	// +optional
	Outcome *AuditOutcome `json:"outcome,omitempty"`

	// EventHash is the canonical sha256 of the event envelope. Required
	// for tamper detection (Requirement F22).
	//
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	// +optional
	EventHash string `json:"eventHash,omitempty"`

	// RawObjectURI points at the immutable S3/MinIO object containing the
	// untruncated event body.
	//
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	RawObjectURI string `json:"rawObjectUri,omitempty"`
}

// AuditEventStatus is intentionally minimal. AuditEvent is itself an
// audit record so the spec block carries the data; status is reserved
// for system bookkeeping (e.g. forwarding state, retention windows)
// once task 12.3 lands the read-only aggregated API server.
type AuditEventStatus struct {
	// ObservedGeneration is the .metadata.generation last observed by
	// the audit pipeline.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions track the lifecycle of the audit record (e.g.
	// `PersistedToS3`, `ForwardedToSIEM`).
	//
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=ae,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Decision",type=string,JSONPath=`.spec.policy.decision`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.spec.outcome.status`
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.principal.agent.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AuditEvent is a structured audit record produced for every AI
// invocation. The API surface is read-only for end users; only system
// components may write (admission webhook in task 2.3).
type AuditEvent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuditEventSpec   `json:"spec,omitempty"`
	Status AuditEventStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AuditEventList is the canonical list type for AuditEvent.
type AuditEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuditEvent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuditEvent{}, &AuditEventList{})
}
