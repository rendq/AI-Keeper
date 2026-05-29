package conversion

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	agentv1beta1 "github.com/ai-keeper/ai-keeper/api/agent/v1beta1"
)

// ConvertAgentAlphaToBeta converts a v1alpha1.Agent to v1beta1.Agent.
// The returned []string lists lossy annotation strings (empty for
// alpha→beta since beta is a superset).
//
// Validates: Requirements A11.2, A11.3, A11.4.
func ConvertAgentAlphaToBeta(src *agentv1alpha1.Agent) (*agentv1beta1.Agent, []string) {
	if src == nil {
		return nil, nil
	}

	dst := &agentv1beta1.Agent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "agent.ai-keeper.io/v1beta1",
			Kind:       "Agent",
		},
		ObjectMeta: *src.ObjectMeta.DeepCopy(),
	}

	// Spec
	dst.Spec.DisplayName = src.Spec.DisplayName
	dst.Spec.Description = src.Spec.Description
	dst.Spec.Identity = convertAgentIdentityAlphaToBeta(src.Spec.Identity)
	dst.Spec.Skills = convertAgentSkillsAlphaToBeta(src.Spec.Skills)
	if src.Spec.Memory != nil {
		dst.Spec.Memory = convertAgentMemoryAlphaToBeta(src.Spec.Memory)
	}
	dst.Spec.Runtime = convertAgentRuntimeAlphaToBeta(src.Spec.Runtime)
	if src.Spec.Guardrails != nil {
		dst.Spec.Guardrails = convertGuardrailsAlphaToBeta(src.Spec.Guardrails)
	}
	if src.Spec.Audit != nil {
		dst.Spec.Audit = convertAgentAuditAlphaToBeta(src.Spec.Audit)
	}
	if src.Spec.Deployment != nil {
		dst.Spec.Deployment = convertAgentDeploymentAlphaToBeta(src.Spec.Deployment)
	}
	dst.Spec.Channels = convertAgentChannelsAlphaToBeta(src.Spec.Channels)

	// Status
	dst.Status = convertAgentStatusAlphaToBeta(src.Status)

	return dst, nil
}

// ConvertAgentBetaToAlpha converts a v1beta1.Agent to v1alpha1.Agent.
// Fields only in v1beta1 (rollout.analysis) are lost and noted.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func ConvertAgentBetaToAlpha(src *agentv1beta1.Agent) (*agentv1alpha1.Agent, []string) {
	if src == nil {
		return nil, nil
	}

	var lossy []string

	dst := &agentv1alpha1.Agent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "agent.ai-keeper.io/v1alpha1",
			Kind:       "Agent",
		},
		ObjectMeta: *src.ObjectMeta.DeepCopy(),
	}

	// Spec
	dst.Spec.DisplayName = src.Spec.DisplayName
	dst.Spec.Description = src.Spec.Description
	dst.Spec.Identity = convertAgentIdentityBetaToAlpha(src.Spec.Identity)
	dst.Spec.Skills = convertAgentSkillsBetaToAlpha(src.Spec.Skills)
	if src.Spec.Memory != nil {
		dst.Spec.Memory = convertAgentMemoryBetaToAlpha(src.Spec.Memory)
	}
	dst.Spec.Runtime = convertAgentRuntimeBetaToAlpha(src.Spec.Runtime)
	if src.Spec.Guardrails != nil {
		dst.Spec.Guardrails = convertGuardrailsBetaToAlpha(src.Spec.Guardrails)
	}
	if src.Spec.Audit != nil {
		dst.Spec.Audit = convertAgentAuditBetaToAlpha(src.Spec.Audit)
	}
	if src.Spec.Deployment != nil {
		dst.Spec.Deployment, lossy = convertAgentDeploymentBetaToAlpha(src.Spec.Deployment, lossy)
	}
	dst.Spec.Channels = convertAgentChannelsBetaToAlpha(src.Spec.Channels)

	// Status
	dst.Status = convertAgentStatusBetaToAlpha(src.Status)

	return dst, lossy
}

// --- Identity ---

func convertAgentIdentityAlphaToBeta(in agentv1alpha1.AgentIdentity) agentv1beta1.AgentIdentity {
	out := agentv1beta1.AgentIdentity{ServiceAccount: in.ServiceAccount}
	if in.Representation != nil {
		out.Representation = &agentv1beta1.AgentRepresentation{
			Mode:               in.Representation.Mode,
			RequireUserContext: in.Representation.RequireUserContext,
			TokenExchange:      in.Representation.TokenExchange,
		}
	}
	return out
}

func convertAgentIdentityBetaToAlpha(in agentv1beta1.AgentIdentity) agentv1alpha1.AgentIdentity {
	out := agentv1alpha1.AgentIdentity{ServiceAccount: in.ServiceAccount}
	if in.Representation != nil {
		out.Representation = &agentv1alpha1.AgentRepresentation{
			Mode:               in.Representation.Mode,
			RequireUserContext: in.Representation.RequireUserContext,
			TokenExchange:      in.Representation.TokenExchange,
		}
	}
	return out
}

// --- Skills ---

func convertAgentSkillsAlphaToBeta(in []agentv1alpha1.AgentSkillBinding) []agentv1beta1.AgentSkillBinding {
	out := make([]agentv1beta1.AgentSkillBinding, len(in))
	for i, s := range in {
		out[i] = agentv1beta1.AgentSkillBinding{
			Ref:               s.Ref,
			VersionConstraint: s.VersionConstraint,
			Enabled:           s.Enabled,
			Alias:             s.Alias,
		}
	}
	return out
}

func convertAgentSkillsBetaToAlpha(in []agentv1beta1.AgentSkillBinding) []agentv1alpha1.AgentSkillBinding {
	out := make([]agentv1alpha1.AgentSkillBinding, len(in))
	for i, s := range in {
		out[i] = agentv1alpha1.AgentSkillBinding{
			Ref:               s.Ref,
			VersionConstraint: s.VersionConstraint,
			Enabled:           s.Enabled,
			Alias:             s.Alias,
		}
	}
	return out
}

// --- Memory ---

func convertAgentMemoryAlphaToBeta(in *agentv1alpha1.AgentMemory) *agentv1beta1.AgentMemory {
	out := &agentv1beta1.AgentMemory{}
	if in.ShortTerm != nil {
		out.ShortTerm = &agentv1beta1.AgentMemoryShortTerm{
			Type:    in.ShortTerm.Type,
			Window:  in.ShortTerm.Window,
			TTL:     in.ShortTerm.TTL,
			Storage: in.ShortTerm.Storage,
		}
	}
	if in.LongTerm != nil {
		out.LongTerm = &agentv1beta1.AgentMemoryLongTerm{
			Type:        in.LongTerm.Type,
			Ref:         in.LongTerm.Ref,
			Isolation:   in.LongTerm.Isolation,
			WritePolicy: in.LongTerm.WritePolicy,
			Retention:   in.LongTerm.Retention,
		}
	}
	return out
}

func convertAgentMemoryBetaToAlpha(in *agentv1beta1.AgentMemory) *agentv1alpha1.AgentMemory {
	out := &agentv1alpha1.AgentMemory{}
	if in.ShortTerm != nil {
		out.ShortTerm = &agentv1alpha1.AgentMemoryShortTerm{
			Type:    in.ShortTerm.Type,
			Window:  in.ShortTerm.Window,
			TTL:     in.ShortTerm.TTL,
			Storage: in.ShortTerm.Storage,
		}
	}
	if in.LongTerm != nil {
		out.LongTerm = &agentv1alpha1.AgentMemoryLongTerm{
			Type:        in.LongTerm.Type,
			Ref:         in.LongTerm.Ref,
			Isolation:   in.LongTerm.Isolation,
			WritePolicy: in.LongTerm.WritePolicy,
			Retention:   in.LongTerm.Retention,
		}
	}
	return out
}

// --- Runtime ---

func convertAgentRuntimeAlphaToBeta(in agentv1alpha1.AgentRuntime) agentv1beta1.AgentRuntime {
	out := agentv1beta1.AgentRuntime{
		Pattern:      in.Pattern,
		MaxSteps:     in.MaxSteps,
		MaxToolCalls: in.MaxToolCalls,
		Timeout:      in.Timeout,
		Parallelism:  in.Parallelism,
	}
	if in.Determinism != nil {
		out.Determinism = &agentv1beta1.AgentDeterminism{
			Temperature: in.Determinism.Temperature,
			TopP:        in.Determinism.TopP,
			Seed:        in.Determinism.Seed,
		}
	}
	if in.Sandbox != nil {
		out.Sandbox = &agentv1beta1.AgentSandbox{
			Enabled:         in.Sandbox.Enabled,
			Type:            in.Sandbox.Type,
			NetworkPolicy:   in.Sandbox.NetworkPolicy,
			EgressAllowList: in.Sandbox.EgressAllowList,
			CPULimit:        in.Sandbox.CPULimit,
			MemoryLimit:     in.Sandbox.MemoryLimit,
		}
	}
	if in.Budget != nil {
		out.Budget = &agentv1beta1.AgentBudget{
			TokensPerSession: in.Budget.TokensPerSession,
			UsdPerSession:    in.Budget.UsdPerSession,
			TokensPerStep:    in.Budget.TokensPerStep,
			OnExceed:         in.Budget.OnExceed,
		}
	}
	return out
}

func convertAgentRuntimeBetaToAlpha(in agentv1beta1.AgentRuntime) agentv1alpha1.AgentRuntime {
	out := agentv1alpha1.AgentRuntime{
		Pattern:      in.Pattern,
		MaxSteps:     in.MaxSteps,
		MaxToolCalls: in.MaxToolCalls,
		Timeout:      in.Timeout,
		Parallelism:  in.Parallelism,
	}
	if in.Determinism != nil {
		out.Determinism = &agentv1alpha1.AgentDeterminism{
			Temperature: in.Determinism.Temperature,
			TopP:        in.Determinism.TopP,
			Seed:        in.Determinism.Seed,
		}
	}
	if in.Sandbox != nil {
		out.Sandbox = &agentv1alpha1.AgentSandbox{
			Enabled:         in.Sandbox.Enabled,
			Type:            in.Sandbox.Type,
			NetworkPolicy:   in.Sandbox.NetworkPolicy,
			EgressAllowList: in.Sandbox.EgressAllowList,
			CPULimit:        in.Sandbox.CPULimit,
			MemoryLimit:     in.Sandbox.MemoryLimit,
		}
	}
	if in.Budget != nil {
		out.Budget = &agentv1alpha1.AgentBudget{
			TokensPerSession: in.Budget.TokensPerSession,
			UsdPerSession:    in.Budget.UsdPerSession,
			TokensPerStep:    in.Budget.TokensPerStep,
			OnExceed:         in.Budget.OnExceed,
		}
	}
	return out
}

// --- Guardrails ---

func convertGuardrailsAlphaToBeta(in *agentv1alpha1.GuardrailsBlock) *agentv1beta1.GuardrailsBlock {
	out := &agentv1beta1.GuardrailsBlock{}
	for _, r := range in.Input {
		out.Input = append(out.Input, agentv1beta1.GuardrailRule{
			Kind: r.Kind, Provider: r.Provider, Action: r.Action,
			Threshold: r.Threshold, Rule: r.Rule, Config: r.Config,
		})
	}
	for _, r := range in.Output {
		out.Output = append(out.Output, agentv1beta1.GuardrailRule{
			Kind: r.Kind, Provider: r.Provider, Action: r.Action,
			Threshold: r.Threshold, Rule: r.Rule, Config: r.Config,
		})
	}
	if in.Behavior != nil {
		out.Behavior = &agentv1beta1.GuardrailBehavior{
			SystemPrompt:      in.Behavior.SystemPrompt,
			BlockedTopics:     in.Behavior.BlockedTopics,
			AllowedTopics:     in.Behavior.AllowedTopics,
			RequiredCitations: in.Behavior.RequiredCitations,
			LanguageLock:      in.Behavior.LanguageLock,
		}
	}
	return out
}

func convertGuardrailsBetaToAlpha(in *agentv1beta1.GuardrailsBlock) *agentv1alpha1.GuardrailsBlock {
	out := &agentv1alpha1.GuardrailsBlock{}
	for _, r := range in.Input {
		out.Input = append(out.Input, agentv1alpha1.GuardrailRule{
			Kind: r.Kind, Provider: r.Provider, Action: r.Action,
			Threshold: r.Threshold, Rule: r.Rule, Config: r.Config,
		})
	}
	for _, r := range in.Output {
		out.Output = append(out.Output, agentv1alpha1.GuardrailRule{
			Kind: r.Kind, Provider: r.Provider, Action: r.Action,
			Threshold: r.Threshold, Rule: r.Rule, Config: r.Config,
		})
	}
	if in.Behavior != nil {
		out.Behavior = &agentv1alpha1.GuardrailBehavior{
			SystemPrompt:      in.Behavior.SystemPrompt,
			BlockedTopics:     in.Behavior.BlockedTopics,
			AllowedTopics:     in.Behavior.AllowedTopics,
			RequiredCitations: in.Behavior.RequiredCitations,
			LanguageLock:      in.Behavior.LanguageLock,
		}
	}
	return out
}

// --- Audit ---

func convertAgentAuditAlphaToBeta(in *agentv1alpha1.AgentAudit) *agentv1beta1.AgentAudit {
	out := &agentv1beta1.AgentAudit{
		Level:     in.Level,
		Retention: in.Retention,
		RedactPII: in.RedactPII,
	}
	if in.StoreRaw != nil {
		out.StoreRaw = &agentv1beta1.AgentAuditStoreRaw{
			Prompts: in.StoreRaw.Prompts,
			Outputs: in.StoreRaw.Outputs,
			ToolIO:  in.StoreRaw.ToolIO,
		}
	}
	for _, f := range in.Forwarders {
		out.Forwarders = append(out.Forwarders, agentv1beta1.AgentAuditForwarder{
			Kind: f.Kind, Ref: f.Ref,
		})
	}
	return out
}

func convertAgentAuditBetaToAlpha(in *agentv1beta1.AgentAudit) *agentv1alpha1.AgentAudit {
	out := &agentv1alpha1.AgentAudit{
		Level:     in.Level,
		Retention: in.Retention,
		RedactPII: in.RedactPII,
	}
	if in.StoreRaw != nil {
		out.StoreRaw = &agentv1alpha1.AgentAuditStoreRaw{
			Prompts: in.StoreRaw.Prompts,
			Outputs: in.StoreRaw.Outputs,
			ToolIO:  in.StoreRaw.ToolIO,
		}
	}
	for _, f := range in.Forwarders {
		out.Forwarders = append(out.Forwarders, agentv1alpha1.AgentAuditForwarder{
			Kind: f.Kind, Ref: f.Ref,
		})
	}
	return out
}

// --- Deployment ---

func convertAgentDeploymentAlphaToBeta(in *agentv1alpha1.AgentDeployment) *agentv1beta1.AgentDeployment {
	out := &agentv1beta1.AgentDeployment{Replicas: in.Replicas}
	if in.Autoscale != nil {
		out.Autoscale = &agentv1beta1.AgentAutoscale{
			Min: in.Autoscale.Min, Max: in.Autoscale.Max,
			TargetConcurrency: in.Autoscale.TargetConcurrency,
		}
	}
	if in.Placement != nil {
		out.Placement = &agentv1beta1.AgentPlacement{
			Zones: in.Placement.Zones, Compliance: in.Placement.Compliance,
			AirGapped: in.Placement.AirGapped, NodeSelector: in.Placement.NodeSelector,
		}
	}
	if in.Rollout != nil {
		out.Rollout = &agentv1beta1.AgentRollout{
			Strategy:         in.Rollout.Strategy,
			Steps:            in.Rollout.Steps,
			AnalysisInterval: in.Rollout.AnalysisInterval,
			AnalysisRef:      in.Rollout.AnalysisRef,
			// Analysis is new in v1beta1 — left nil.
		}
	}
	return out
}

func convertAgentDeploymentBetaToAlpha(in *agentv1beta1.AgentDeployment, lossy []string) (*agentv1alpha1.AgentDeployment, []string) {
	out := &agentv1alpha1.AgentDeployment{Replicas: in.Replicas}
	if in.Autoscale != nil {
		out.Autoscale = &agentv1alpha1.AgentAutoscale{
			Min: in.Autoscale.Min, Max: in.Autoscale.Max,
			TargetConcurrency: in.Autoscale.TargetConcurrency,
		}
	}
	if in.Placement != nil {
		out.Placement = &agentv1alpha1.AgentPlacement{
			Zones: in.Placement.Zones, Compliance: in.Placement.Compliance,
			AirGapped: in.Placement.AirGapped, NodeSelector: in.Placement.NodeSelector,
		}
	}
	if in.Rollout != nil {
		out.Rollout = &agentv1alpha1.AgentRollout{
			Strategy:         in.Rollout.Strategy,
			Steps:            in.Rollout.Steps,
			AnalysisInterval: in.Rollout.AnalysisInterval,
			AnalysisRef:      in.Rollout.AnalysisRef,
		}
		if in.Rollout.Analysis != nil {
			lossy = append(lossy, "v1beta1→v1alpha1: spec.deployment.rollout.analysis dropped (no v1alpha1 equivalent)")
		}
	}
	return out, lossy
}

// --- Channels ---

func convertAgentChannelsAlphaToBeta(in []agentv1alpha1.AgentChannel) []agentv1beta1.AgentChannel {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentv1beta1.AgentChannel, len(in))
	for i, c := range in {
		out[i] = agentv1beta1.AgentChannel{
			Kind: c.Kind, Ref: c.Ref, Auth: c.Auth,
		}
		if c.RateLimit != nil {
			out[i].RateLimit = &agentv1beta1.AgentChannelRateLimit{
				RequestsPerMinute:  c.RateLimit.RequestsPerMinute,
				ConcurrentSessions: c.RateLimit.ConcurrentSessions,
			}
		}
	}
	return out
}

func convertAgentChannelsBetaToAlpha(in []agentv1beta1.AgentChannel) []agentv1alpha1.AgentChannel {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentv1alpha1.AgentChannel, len(in))
	for i, c := range in {
		out[i] = agentv1alpha1.AgentChannel{
			Kind: c.Kind, Ref: c.Ref, Auth: c.Auth,
		}
		if c.RateLimit != nil {
			out[i].RateLimit = &agentv1alpha1.AgentChannelRateLimit{
				RequestsPerMinute:  c.RateLimit.RequestsPerMinute,
				ConcurrentSessions: c.RateLimit.ConcurrentSessions,
			}
		}
	}
	return out
}

// --- Status ---

func convertAgentStatusAlphaToBeta(in agentv1alpha1.AgentStatus) agentv1beta1.AgentStatus {
	out := agentv1beta1.AgentStatus{
		Phase:              shared.Phase(in.Phase),
		ObservedGeneration: in.ObservedGeneration,
		Replicas:           in.Replicas,
		ReadyReplicas:      in.ReadyReplicas,
		AttachedSkills:     in.AttachedSkills,
		EffectivePolicies:  in.EffectivePolicies,
	}
	for _, c := range in.Conditions {
		out.Conditions = append(out.Conditions, c)
	}
	if in.Metrics != nil {
		out.Metrics = &agentv1beta1.AgentMetrics{ActiveUsers: in.Metrics.ActiveUsers}
		if in.Metrics.Today != nil {
			out.Metrics.Today = &agentv1beta1.AgentTodayMetrics{
				Invocations:  in.Metrics.Today.Invocations,
				CostUsd:      in.Metrics.Today.CostUsd,
				P95LatencyMs: in.Metrics.Today.P95LatencyMs,
				ErrorRate:    in.Metrics.Today.ErrorRate,
			}
		}
	}
	if in.RolloutStatus != nil {
		out.RolloutStatus = &agentv1beta1.AgentRolloutStatus{
			Phase:         in.RolloutStatus.Phase,
			CurrentStep:   in.RolloutStatus.CurrentStep,
			TrafficWeight: in.RolloutStatus.TrafficWeight,
		}
	}
	return out
}

func convertAgentStatusBetaToAlpha(in agentv1beta1.AgentStatus) agentv1alpha1.AgentStatus {
	out := agentv1alpha1.AgentStatus{
		Phase:              shared.Phase(in.Phase),
		ObservedGeneration: in.ObservedGeneration,
		Replicas:           in.Replicas,
		ReadyReplicas:      in.ReadyReplicas,
		AttachedSkills:     in.AttachedSkills,
		EffectivePolicies:  in.EffectivePolicies,
	}
	for _, c := range in.Conditions {
		out.Conditions = append(out.Conditions, c)
	}
	if in.Metrics != nil {
		out.Metrics = &agentv1alpha1.AgentMetrics{ActiveUsers: in.Metrics.ActiveUsers}
		if in.Metrics.Today != nil {
			out.Metrics.Today = &agentv1alpha1.AgentTodayMetrics{
				Invocations:  in.Metrics.Today.Invocations,
				CostUsd:      in.Metrics.Today.CostUsd,
				P95LatencyMs: in.Metrics.Today.P95LatencyMs,
				ErrorRate:    in.Metrics.Today.ErrorRate,
			}
		}
	}
	if in.RolloutStatus != nil {
		out.RolloutStatus = &agentv1alpha1.AgentRolloutStatus{
			Phase:         in.RolloutStatus.Phase,
			CurrentStep:   in.RolloutStatus.CurrentStep,
			TrafficWeight: in.RolloutStatus.TrafficWeight,
		}
	}
	return out
}
