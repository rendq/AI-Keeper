package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Agent conditions (design.md §6.2.5).
const (
	AgentSpecValid            = "SpecValid"
	AgentSkillsResolved       = "SkillsResolved"
	AgentPolicyAttached       = "PolicyAttached"
	AgentIdentityReady        = "IdentityReady"
	AgentDeployed             = "Deployed"
	AgentChannelsHealthy      = "ChannelsHealthy"
	AgentGuardrailsHealthy    = "GuardrailsHealthy"
	AgentSandboxReady         = "SandboxReady"
	AgentRolloutComplete      = "RolloutComplete"
	AgentBudgetWithinLimit    = "BudgetWithinLimit"
	AgentUsingDeprecatedSkill = "UsingDeprecatedSkill"
	AgentReady                = "Ready"
)

// AgentRepresentation captures `identity.representation`.
type AgentRepresentation struct {
	// Mode of identity representation.
	//
	// +kubebuilder:validation:Enum=self;service_account;on_behalf_of
	// +optional
	Mode string `json:"mode,omitempty"`

	// RequireUserContext rejects requests lacking a user identity when
	// true.
	// +optional
	RequireUserContext *bool `json:"requireUserContext,omitempty"`

	// TokenExchange is the IdP / exchanger configuration name.
	//
	// +kubebuilder:validation:MaxLength=253
	// +optional
	TokenExchange string `json:"tokenExchange,omitempty"`
}

// AgentIdentity declares the agent's runtime identity.
type AgentIdentity struct {
	// ServiceAccount is the name of the AIP ServiceAccount in the same
	// namespace.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	ServiceAccount string `json:"serviceAccount"`

	// Representation overrides default `service_account` mode.
	// +optional
	Representation *AgentRepresentation `json:"representation,omitempty"`
}

// AgentSkillBinding attaches a Skill to the agent with a version
// constraint.
type AgentSkillBinding struct {
	// Ref is the skill://name form.
	Ref shared.ResourceRef `json:"ref"`

	// VersionConstraint is an npm-style range; resolved by the controller.
	//
	// +kubebuilder:validation:MaxLength=128
	// +optional
	VersionConstraint string `json:"versionConstraint,omitempty"`

	// Enabled defaults to true.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Alias to use within the agent.
	//
	// +kubebuilder:validation:MaxLength=64
	// +optional
	Alias string `json:"alias,omitempty"`
}

// AgentMemoryShortTerm describes the short-term memory backend.
type AgentMemoryShortTerm struct {
	// +kubebuilder:validation:Enum=conversation;summary;none
	// +optional
	Type string `json:"type,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	Window *int32 `json:"window,omitempty"`

	// +optional
	TTL *shared.Duration `json:"ttl,omitempty"`

	// Storage backend ref.
	// +optional
	Storage *shared.ResourceRef `json:"storage,omitempty"`
}

// AgentMemoryLongTerm describes the long-term memory backend.
type AgentMemoryLongTerm struct {
	// +kubebuilder:validation:Enum=vector;kv;graph;none
	// +optional
	Type string `json:"type,omitempty"`

	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`

	// Isolation governs cross-tenant / cross-user visibility.
	//
	// +kubebuilder:validation:Enum=shared;per_user;per_session;per_tenant
	// +optional
	Isolation string `json:"isolation,omitempty"`

	// WritePolicy controls what the runtime is allowed to commit.
	//
	// +kubebuilder:validation:Enum=auto;explicit_only;manual_review
	// +optional
	WritePolicy string `json:"writePolicy,omitempty"`

	// +optional
	Retention *shared.Duration `json:"retention,omitempty"`
}

// AgentMemory bundles short and long term memory.
type AgentMemory struct {
	// +optional
	ShortTerm *AgentMemoryShortTerm `json:"shortTerm,omitempty"`

	// +optional
	LongTerm *AgentMemoryLongTerm `json:"longTerm,omitempty"`
}

// AgentDeterminism configures LLM determinism knobs.
type AgentDeterminism struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2
	// +optional
	Temperature *float64 `json:"temperature,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	TopP *float64 `json:"topP,omitempty"`

	// Seed for reproducibility (nullable).
	// +optional
	Seed *int64 `json:"seed,omitempty"`
}

// AgentSandbox configures isolation for code or destructive tool runs.
type AgentSandbox struct {
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +kubebuilder:validation:Enum=none;gvisor;firecracker;kata;e2b
	// +optional
	Type string `json:"type,omitempty"`

	// +kubebuilder:validation:Enum=deny_all;allow_list;allow_all
	// +optional
	NetworkPolicy string `json:"networkPolicy,omitempty"`

	// +optional
	EgressAllowList []string `json:"egressAllowList,omitempty"`

	// +kubebuilder:validation:MaxLength=32
	// +optional
	CPULimit string `json:"cpuLimit,omitempty"`

	// +kubebuilder:validation:MaxLength=32
	// +optional
	MemoryLimit string `json:"memoryLimit,omitempty"`
}

// AgentBudget enforces per-session caps inside the runtime.
type AgentBudget struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerSession *int64 `json:"tokensPerSession,omitempty"`

	// +optional
	UsdPerSession *shared.MoneyAmount `json:"usdPerSession,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerStep *int64 `json:"tokensPerStep,omitempty"`

	// OnExceed action.
	//
	// +kubebuilder:validation:Enum=warn;terminate;request_approval
	// +optional
	OnExceed string `json:"onExceed,omitempty"`
}

// AgentRuntime captures the runtime caps and pattern.
type AgentRuntime struct {
	// Pattern selects the FSM.
	//
	// +kubebuilder:validation:Enum=react;plan_execute;reflection;workflow;tool_calling;multi_agent
	Pattern string `json:"pattern"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	MaxSteps *int32 `json:"maxSteps,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=200
	// +optional
	MaxToolCalls *int32 `json:"maxToolCalls,omitempty"`

	// +optional
	Timeout *shared.Duration `json:"timeout,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32
	// +optional
	Parallelism *int32 `json:"parallelism,omitempty"`

	// +optional
	Determinism *AgentDeterminism `json:"determinism,omitempty"`

	// +optional
	Sandbox *AgentSandbox `json:"sandbox,omitempty"`

	// +optional
	Budget *AgentBudget `json:"budget,omitempty"`
}

// GuardrailRule is one rule inside the chain.
type GuardrailRule struct {
	// Kind names the built-in guardrail.
	//
	// +kubebuilder:validation:Enum=PromptInjection;Jailbreak;PII;PIILeak;Toxicity;Hallucination;Grounding;ClassificationLeak;Bias;Profanity;Custom
	Kind string `json:"kind"`

	// Provider name (aip-builtin, llamaguard-v3, nemo-guardrails, custom).
	//
	// +kubebuilder:validation:MaxLength=64
	// +optional
	Provider string `json:"provider,omitempty"`

	// Action when matched.
	//
	// +kubebuilder:validation:Enum=allow;mask;block;warn;escalate
	// +optional
	Action string `json:"action,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	Threshold *float64 `json:"threshold,omitempty"`

	// Rule is a CEL/regex string for kind=Custom.
	//
	// +kubebuilder:validation:MaxLength=4096
	// +optional
	Rule string `json:"rule,omitempty"`

	// Config is provider-specific opaque config.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Config *apiextensionsv1.JSON `json:"config,omitempty"`
}

// GuardrailBehavior captures behavior-stage rules.
type GuardrailBehavior struct {
	// +optional
	SystemPrompt *shared.ResourceRef `json:"systemPrompt,omitempty"`

	// +optional
	BlockedTopics []string `json:"blockedTopics,omitempty"`

	// +optional
	AllowedTopics []string `json:"allowedTopics,omitempty"`

	// +optional
	RequiredCitations *bool `json:"requiredCitations,omitempty"`

	// LanguageLock is a list of BCP-47 codes the agent may answer in.
	// +optional
	LanguageLock []string `json:"languageLock,omitempty"`
}

// GuardrailsBlock collects input / output / behavior chains.
type GuardrailsBlock struct {
	// +optional
	Input []GuardrailRule `json:"input,omitempty"`

	// +optional
	Output []GuardrailRule `json:"output,omitempty"`

	// +optional
	Behavior *GuardrailBehavior `json:"behavior,omitempty"`
}

// AgentAuditStoreRaw declares prompt/output/tool-IO storage policies.
type AgentAuditStoreRaw struct {
	// +kubebuilder:validation:Enum=full;hashed;none
	// +optional
	Prompts string `json:"prompts,omitempty"`

	// +kubebuilder:validation:Enum=full;hashed;none
	// +optional
	Outputs string `json:"outputs,omitempty"`

	// +kubebuilder:validation:Enum=full;hashed;none
	// +optional
	ToolIO string `json:"toolIo,omitempty"`
}

// AgentAuditForwarder describes a sink the audit pipeline copies to.
type AgentAuditForwarder struct {
	// +kubebuilder:validation:Enum=SIEM;Webhook;Kafka;S3;Elasticsearch
	Kind string `json:"kind"`

	Ref shared.ResourceRef `json:"ref"`
}

// AgentAudit describes audit policy for the agent.
type AgentAudit struct {
	// +kubebuilder:validation:Enum=off;basic;high;forensic
	// +optional
	Level string `json:"level,omitempty"`

	// +optional
	Retention *shared.Duration `json:"retention,omitempty"`

	// +optional
	RedactPII *bool `json:"redactPII,omitempty"`

	// +optional
	StoreRaw *AgentAuditStoreRaw `json:"storeRaw,omitempty"`

	// +optional
	Forwarders []AgentAuditForwarder `json:"forwarders,omitempty"`
}

// AgentAutoscale configures HPA-like behaviour.
type AgentAutoscale struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	Min *int32 `json:"min,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	Max *int32 `json:"max,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +optional
	TargetConcurrency *int32 `json:"targetConcurrency,omitempty"`
}

// AgentPlacement is the deployment-placement block.
type AgentPlacement struct {
	// +optional
	Zones []string `json:"zones,omitempty"`

	// +optional
	Compliance []string `json:"compliance,omitempty"`

	// +optional
	AirGapped *bool `json:"airGapped,omitempty"`

	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// AgentRollout configures the rollout strategy.
type AgentRollout struct {
	// +kubebuilder:validation:Enum=recreate;rolling;canary;blue_green
	// +optional
	Strategy string `json:"strategy,omitempty"`

	// Steps as percentages, e.g. ["10%", "30%", "100%"].
	// +optional
	Steps []string `json:"steps,omitempty"`

	// +optional
	AnalysisInterval *shared.Duration `json:"analysisInterval,omitempty"`

	// +optional
	AnalysisRef *shared.ResourceRef `json:"analysisRef,omitempty"`
}

// AgentDeployment captures the deployment block.
type AgentDeployment struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	Autoscale *AgentAutoscale `json:"autoscale,omitempty"`

	// +optional
	Placement *AgentPlacement `json:"placement,omitempty"`

	// +optional
	Rollout *AgentRollout `json:"rollout,omitempty"`
}

// AgentChannelRateLimit caps per-channel throughput.
type AgentChannelRateLimit struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	RequestsPerMinute *int32 `json:"requestsPerMinute,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	ConcurrentSessions *int32 `json:"concurrentSessions,omitempty"`
}

// AgentChannel declares a channel binding.
type AgentChannel struct {
	// Kind of channel.
	//
	// +kubebuilder:validation:Enum=feishu;wecom;dingtalk;slack;teams;web;api;sdk;voice;email
	Kind string `json:"kind"`

	// +optional
	Ref *shared.ResourceRef `json:"ref,omitempty"`

	// Auth secret reference name.
	//
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Auth string `json:"auth,omitempty"`

	// +optional
	RateLimit *AgentChannelRateLimit `json:"rateLimit,omitempty"`
}

// AgentSpec declares the desired state of an Agent.
type AgentSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=200
	DisplayName string `json:"displayName"`

	// +kubebuilder:validation:MaxLength=2000
	// +optional
	Description string `json:"description,omitempty"`

	Identity AgentIdentity `json:"identity"`

	// +kubebuilder:validation:MinItems=1
	Skills []AgentSkillBinding `json:"skills"`

	// +optional
	Memory *AgentMemory `json:"memory,omitempty"`

	Runtime AgentRuntime `json:"runtime"`

	// +optional
	Guardrails *GuardrailsBlock `json:"guardrails,omitempty"`

	// +optional
	Audit *AgentAudit `json:"audit,omitempty"`

	// +optional
	Deployment *AgentDeployment `json:"deployment,omitempty"`

	// +optional
	Channels []AgentChannel `json:"channels,omitempty"`
}

// AgentTodayMetrics is the rolling 24h metric snapshot.
type AgentTodayMetrics struct {
	// +optional
	Invocations int64 `json:"invocations,omitempty"`

	// +optional
	CostUsd *shared.MoneyAmount `json:"costUsd,omitempty"`

	// +optional
	P95LatencyMs *int32 `json:"p95LatencyMs,omitempty"`

	// +optional
	ErrorRate *float64 `json:"errorRate,omitempty"`
}

// AgentMetrics surfaces user / cost / latency rollups.
type AgentMetrics struct {
	// +optional
	ActiveUsers int32 `json:"activeUsers,omitempty"`

	// +optional
	Today *AgentTodayMetrics `json:"today,omitempty"`
}

// AgentRolloutStatus surfaces the current rollout step.
type AgentRolloutStatus struct {
	// +kubebuilder:validation:Enum=Progressing;Promoting;Paused;Aborted;Succeeded
	// +optional
	Phase string `json:"phase,omitempty"`

	// +optional
	CurrentStep *int32 `json:"currentStep,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	TrafficWeight *int32 `json:"trafficWeight,omitempty"`
}

// AgentStatus is the observed state of the agent.
type AgentStatus struct {
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

	// Replicas is the desired replica count last observed.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas reported by the underlying Deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AttachedSkills are the resolved skill refs.
	// +optional
	AttachedSkills []shared.ResourceRef `json:"attachedSkills,omitempty"`

	// EffectivePolicies attached at the PDP.
	// +optional
	EffectivePolicies []string `json:"effectivePolicies,omitempty"`

	// +optional
	Metrics *AgentMetrics `json:"metrics,omitempty"`

	// +optional
	RolloutStatus *AgentRolloutStatus `json:"rolloutStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=ag,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pattern",type=string,JSONPath=`.spec.runtime.pattern`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Agent is the user-facing execution entity composed of Skills, identity,
// runtime constraints, guardrails and channel bindings.
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentList is the canonical list type for Agent.
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
