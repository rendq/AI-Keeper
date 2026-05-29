package guardrail

import "time"

// RuleKind represents the 11 built-in guardrail rule kinds.
type RuleKind string

const (
	RulePromptInjection    RuleKind = "PromptInjection"
	RuleJailbreak          RuleKind = "Jailbreak"
	RulePII                RuleKind = "PII"
	RulePIILeak            RuleKind = "PIILeak"
	RuleToxicity           RuleKind = "Toxicity"
	RuleHallucination      RuleKind = "Hallucination"
	RuleGrounding          RuleKind = "Grounding"
	RuleClassificationLeak RuleKind = "ClassificationLeak"
	RuleBias               RuleKind = "Bias"
	RuleProfanity          RuleKind = "Profanity"
	RuleCustom             RuleKind = "Custom"
)

// Stage defines when a guardrail rule is evaluated in the request lifecycle.
type Stage string

const (
	StageInput    Stage = "input"
	StageOutput   Stage = "output"
	StageBehavior Stage = "behavior"
)

// StageOrder returns the execution order index for a stage.
func StageOrder(s Stage) int {
	switch s {
	case StageInput:
		return 0
	case StageOutput:
		return 1
	case StageBehavior:
		return 2
	default:
		return 99
	}
}

// Action defines the enforcement action when a rule is triggered.
type Action string

const (
	ActionAllow    Action = "allow"
	ActionWarn     Action = "warn"
	ActionMask     Action = "mask"
	ActionEscalate Action = "escalate"
	ActionBlock    Action = "block"
)

// ActionPriority returns the priority of an action (higher = more severe).
// Aggregation uses: block > escalate > mask > warn > allow.
func ActionPriority(a Action) int {
	switch a {
	case ActionBlock:
		return 5
	case ActionEscalate:
		return 4
	case ActionMask:
		return 3
	case ActionWarn:
		return 2
	case ActionAllow:
		return 1
	default:
		return 0
	}
}

// ProviderName identifies a guardrail provider.
type ProviderName string

const (
	ProviderAIPBuiltin     ProviderName = "aip-builtin"
	ProviderLlamaGuardV3   ProviderName = "llamaguard-v3"
	ProviderNemoGuardrails ProviderName = "nemo-guardrails"
	ProviderCustom         ProviderName = "custom"
)

// Rule defines a single guardrail rule configuration.
type Rule struct {
	Kind     RuleKind     `json:"kind"`
	Stage    Stage        `json:"stage"`
	Provider ProviderName `json:"provider"`
	Action   Action       `json:"action"`
	// Config holds provider-specific configuration (thresholds, endpoints, etc.)
	Config map[string]string `json:"config,omitempty"`
}

// EvalRequest is the input to guardrail evaluation.
type EvalRequest struct {
	// Input is the user prompt (used in input stage).
	Input string
	// Output is the model response (used in output stage).
	Output string
	// Metadata carries additional context (agent name, session id, etc.)
	Metadata map[string]string
}

// Hit records a single guardrail rule trigger with its score and action.
type Hit struct {
	Rule      Rule      `json:"rule"`
	Score     float64   `json:"score"`
	Action    Action    `json:"action"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EvalResult is the aggregated output of guardrail evaluation.
type EvalResult struct {
	// FinalAction is the highest-priority action from all hits.
	FinalAction Action `json:"finalAction"`
	// Blocked is true if FinalAction == ActionBlock.
	Blocked bool `json:"blocked"`
	// Hits contains all triggered rules with scores.
	Hits []Hit `json:"hits"`
	// StagesExecuted records which stages were executed.
	StagesExecuted []Stage `json:"stagesExecuted"`
}
