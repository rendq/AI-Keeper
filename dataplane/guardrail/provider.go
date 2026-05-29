package guardrail

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// parseFloat is a helper to parse a string into float64.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// Provider is the interface that all guardrail providers must implement.
// Each provider evaluates content against a specific rule and returns a score + action.
type Provider interface {
	// Name returns the provider identifier.
	Name() ProviderName
	// Evaluate checks content against the given rule and returns a score [0.0, 1.0]
	// and whether the rule was triggered.
	Evaluate(ctx context.Context, rule Rule, req EvalRequest) (score float64, triggered bool, reason string, err error)
}

// ProviderRegistry manages available guardrail providers.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[ProviderName]Provider
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[ProviderName]Provider),
	}
}

// Register adds a provider to the registry. Overwrites if name already exists.
func (r *ProviderRegistry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Get retrieves a provider by name. Returns error if not found.
func (r *ProviderRegistry) Get(name ProviderName) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("guardrail provider not found: %s", name)
	}
	return p, nil
}

// List returns all registered provider names.
func (r *ProviderRegistry) List() []ProviderName {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]ProviderName, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// --- Built-in Provider (aip-builtin) ---

// AIPBuiltinProvider implements basic rule evaluation using pattern matching and heuristics.
type AIPBuiltinProvider struct{}

func (p *AIPBuiltinProvider) Name() ProviderName { return ProviderAIPBuiltin }

func (p *AIPBuiltinProvider) Evaluate(ctx context.Context, rule Rule, req EvalRequest) (float64, bool, string, error) {
	// Behavior-stage custom rules handle their own text selection
	if rule.Kind == RuleCustom {
		return p.evaluateBehaviorRules(rule, req)
	}

	text := req.Input
	if rule.Stage == StageOutput || rule.Stage == StageBehavior {
		text = req.Output
	}
	if text == "" {
		return 0.0, false, "", nil
	}

	switch rule.Kind {
	case RulePromptInjection:
		score, triggered, reason := DetectPromptInjection(text)
		return score, triggered, reason, nil
	case RuleJailbreak:
		score, triggered, reason := DetectJailbreak(text)
		return score, triggered, reason, nil
	case RulePII:
		score, triggered, reason := DetectPII(text)
		return score, triggered, reason, nil
	case RulePIILeak:
		score, triggered, reason := DetectPIILeak(text)
		return score, triggered, reason, nil
	case RuleToxicity:
		score, triggered, reason := DetectToxicity(text)
		return score, triggered, reason, nil
	case RuleBias:
		score, triggered, reason := DetectBias(text)
		return score, triggered, reason, nil
	case RuleProfanity:
		score, triggered, reason := DetectProfanity(text)
		return score, triggered, reason, nil
	case RuleHallucination:
		sources, err := ParseKBSources(req.Metadata)
		if err != nil {
			return 0.0, false, "", err
		}
		threshold := 0.4 // default threshold
		if t, ok := rule.Config["threshold"]; ok {
			if v, err2 := parseFloat(t); err2 == nil {
				threshold = v
			}
		}
		score, triggered, reason := DetectHallucination(req.Output, sources, threshold)
		return score, triggered, reason, nil
	case RuleGrounding:
		sources, err := ParseKBSources(req.Metadata)
		if err != nil {
			return 0.0, false, "", err
		}
		score, triggered, reason := DetectGrounding(req.Output, sources)
		return score, triggered, reason, nil
	case RuleClassificationLeak:
		outClass := req.Metadata["output.classification"]
		maxClass := req.Metadata["input.maxClassification"]
		score, triggered, reason := DetectClassificationLeak(outClass, maxClass)
		return score, triggered, reason, nil
	default:
		// Other rule kinds are not yet implemented in aip-builtin
		return 0.0, false, "", nil
	}
}

// --- Behavior Rules Helper ---

// evaluateBehaviorRules handles behavior-stage rules (requiredCitations, blockedTopics, custom CEL).
func (p *AIPBuiltinProvider) evaluateBehaviorRules(rule Rule, req EvalRequest) (float64, bool, string, error) {
	// requiredCitations: check output for citation markers
	if rule.Config["requiredCitations"] == "true" {
		text := req.Output
		if text == "" {
			return 0.0, false, "", nil
		}
		score, triggered, reason := DetectMissingCitations(text)
		return score, triggered, reason, nil
	}

	// blockedTopics: check input text against blocked topic list
	if topicsJSON, ok := rule.Config["blockedTopics"]; ok {
		topics, err := parseBehaviorBlockedTopics(topicsJSON)
		if err != nil {
			return 0.0, false, "", err
		}
		// Check both input and output for blocked topics
		text := req.Input
		if rule.Stage == StageOutput || rule.Stage == StageBehavior {
			text = req.Output
			if req.Input != "" {
				text = req.Input + " " + req.Output
			}
		}
		if text == "" {
			return 0.0, false, "", nil
		}
		score, triggered, reason := DetectBlockedTopics(text, topics)
		return score, triggered, reason, nil
	}

	// Custom CEL expression
	if expr, ok := rule.Config["expression"]; ok {
		text := req.Input
		if rule.Stage == StageOutput || rule.Stage == StageBehavior {
			text = req.Output
		}
		if text == "" {
			return 0.0, false, "", nil
		}
		score, triggered, reason := EvaluateCustomCEL(text, expr)
		return score, triggered, reason, nil
	}

	return 0.0, false, "", nil
}

// --- LlamaGuard V3 Provider ---

// LlamaGuardV3Provider calls an external LlamaGuard model endpoint for evaluation via gRPC.
type LlamaGuardV3Provider struct {
	Endpoint string
	// conn is a lazily-initialized gRPC connection (nil until first use).
	conn *grpc.ClientConn
	mu   sync.Mutex
}

func (p *LlamaGuardV3Provider) Name() ProviderName { return ProviderLlamaGuardV3 }

// getConn returns a gRPC connection, creating one if needed.
func (p *LlamaGuardV3Provider) getConn() (*grpc.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		return p.conn, nil
	}
	if p.Endpoint == "" {
		return nil, fmt.Errorf("llamaguard-v3: endpoint not configured")
	}
	conn, err := grpc.NewClient(p.Endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("llamaguard-v3: failed to connect to %s: %w", p.Endpoint, err)
	}
	p.conn = conn
	return conn, nil
}

// Close releases the gRPC connection.
func (p *LlamaGuardV3Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}
	return nil
}

func (p *LlamaGuardV3Provider) Evaluate(ctx context.Context, rule Rule, req EvalRequest) (float64, bool, string, error) {
	// Only handle PromptInjection and Jailbreak
	if rule.Kind != RulePromptInjection && rule.Kind != RuleJailbreak {
		return 0.0, false, "", nil
	}

	text := req.Input
	if rule.Stage == StageOutput {
		text = req.Output
	}
	if text == "" {
		return 0.0, false, "", nil
	}

	// Build the gRPC request to the LlamaGuard model service.
	// The actual gRPC call is wrapped to handle connection failures gracefully.
	conn, err := p.getConn()
	if err != nil {
		return 0.0, false, "", err
	}

	// Construct the evaluation payload for the LlamaGuard model.
	// Uses the standard LlamaGuard prompt format with safety categories.
	category := "S1" // S1 = Violence / Criminal, reused for injection detection
	if rule.Kind == RuleJailbreak {
		category = "S2" // S2 = Jailbreak
	}

	// Call the LlamaGuard model via gRPC Predict/Infer endpoint.
	// The format follows: [INST] task + conversation + categories [/INST]
	prompt := buildLlamaGuardPrompt(text, category)

	score, unsafe, reason, err := callLlamaGuardGRPC(ctx, conn, prompt, string(rule.Kind))
	if err != nil {
		return 0.0, false, "", fmt.Errorf("llamaguard-v3 gRPC call failed: %w", err)
	}

	return score, unsafe, reason, nil
}

// buildLlamaGuardPrompt constructs the LlamaGuard classification prompt.
func buildLlamaGuardPrompt(text string, category string) string {
	var sb strings.Builder
	sb.WriteString("[INST] Task: Check if there is unsafe content in the user message according to our safety policy.\n\n")
	sb.WriteString("<BEGIN UNSAFE CONTENT CATEGORIES>\n")
	sb.WriteString(category)
	sb.WriteString(": Prompt Injection and Jailbreak Attempts\n")
	sb.WriteString("<END UNSAFE CONTENT CATEGORIES>\n\n")
	sb.WriteString("<BEGIN USER MESSAGE>\n")
	sb.WriteString(text)
	sb.WriteString("\n<END USER MESSAGE>\n")
	sb.WriteString("[/INST]")
	return sb.String()
}

// callLlamaGuardGRPC makes the gRPC call to the LlamaGuard model.
// This uses a generic inference service pattern (predict unary RPC).
// In production this would use a generated protobuf client; here we use
// grpc.Invoke directly with a simple request/response structure.
func callLlamaGuardGRPC(ctx context.Context, conn *grpc.ClientConn, prompt string, ruleKind string) (float64, bool, string, error) {
	// Define the request/response for the inference call.
	type inferRequest struct {
		Prompt   string `json:"prompt"`
		RuleKind string `json:"rule_kind"`
	}
	type inferResponse struct {
		Label  string  `json:"label"`  // "safe" or "unsafe"
		Score  float64 `json:"score"`  // confidence score [0,1]
		Reason string  `json:"reason"` // explanation
	}

	req := &inferRequest{Prompt: prompt, RuleKind: ruleKind}
	resp := &inferResponse{}

	// Call the LlamaGuard model service using the standard method path.
	err := conn.Invoke(ctx, "/llamaguard.v3.SafetyService/Classify", req, resp)
	if err != nil {
		return 0.0, false, "", err
	}

	unsafe := resp.Label == "unsafe"
	return resp.Score, unsafe, resp.Reason, nil
}

// --- NeMo Guardrails Provider ---

// NemoGuardrailsProvider integrates with NVIDIA NeMo Guardrails.
type NemoGuardrailsProvider struct {
	Endpoint string
}

func (p *NemoGuardrailsProvider) Name() ProviderName { return ProviderNemoGuardrails }

func (p *NemoGuardrailsProvider) Evaluate(ctx context.Context, rule Rule, req EvalRequest) (float64, bool, string, error) {
	// Stub: real implementation calls NeMo Guardrails API.
	return 0.0, false, "", nil
}

// --- Custom Provider ---

// CustomProvider delegates evaluation to a user-defined HTTP/gRPC webhook.
type CustomProvider struct {
	Endpoint string
}

func (p *CustomProvider) Name() ProviderName { return ProviderCustom }

func (p *CustomProvider) Evaluate(ctx context.Context, rule Rule, req EvalRequest) (float64, bool, string, error) {
	// Stub: real implementation calls the configured custom webhook.
	return 0.0, false, "", nil
}
