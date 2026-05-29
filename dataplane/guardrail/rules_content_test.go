package guardrail

import (
	"context"
	"testing"
)

// --- Unit Tests for Toxicity / Bias / Profanity Detection ---

func TestContent_DetectToxicity_Triggers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"kill threat", "I will kill you if you don't comply"},
		{"death threat", "This is a death threat to everyone"},
		{"hate expression", "I hate you and everything about you"},
		{"deserve to die", "You deserve to die for what you said"},
		{"violence against", "Promoting violence against minorities"},
		{"harassment", "This is clear harassment and bullying"},
		{"chinese toxic keyword", "他应该去死"},
		{"chinese violence", "这是暴力行为"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := DetectToxicity(tt.input)
			if !triggered {
				t.Errorf("Expected toxicity to be detected for input: %q", tt.input)
			}
			if score < 0.8 {
				t.Errorf("Score = %f, want >= 0.8", score)
			}
			if reason == "" {
				t.Error("Reason should not be empty when triggered")
			}
		})
	}
}

func TestContent_DetectToxicity_SafeInput(t *testing.T) {
	safeInputs := []string{
		"Hello, how are you today?",
		"Can you help me write a program?",
		"What is the meaning of life?",
		"今天天气真好",
		"请帮我分析这段代码",
	}

	for _, input := range safeInputs {
		t.Run(input, func(t *testing.T) {
			_, triggered, _ := DetectToxicity(input)
			if triggered {
				t.Errorf("Safe input should NOT trigger toxicity: %q", input)
			}
		})
	}
}

func TestContent_DetectToxicity_Empty(t *testing.T) {
	score, triggered, _ := DetectToxicity("")
	if triggered {
		t.Error("Empty input should not trigger")
	}
	if score != 0.0 {
		t.Errorf("Score = %f, want 0.0", score)
	}
}

func TestContent_DetectBias_Triggers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"gender stereotype", "All women are unable to do math"},
		{"racial superiority", "This is about racial superiority"},
		{"women cannot", "Women cannot be good engineers"},
		{"chinese bias keyword", "这是一种种族歧视"},
		{"gender bias keyword", "明显的性别歧视"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := DetectBias(tt.input)
			if !triggered {
				t.Errorf("Expected bias to be detected for input: %q", tt.input)
			}
			if score < 0.8 {
				t.Errorf("Score = %f, want >= 0.8", score)
			}
			if reason == "" {
				t.Error("Reason should not be empty when triggered")
			}
		})
	}
}

func TestContent_DetectBias_SafeInput(t *testing.T) {
	safeInputs := []string{
		"Explain the concept of fairness in machine learning",
		"How to build an inclusive AI system?",
		"介绍多样性和包容性的概念",
	}

	for _, input := range safeInputs {
		t.Run(input, func(t *testing.T) {
			_, triggered, _ := DetectBias(input)
			if triggered {
				t.Errorf("Safe input should NOT trigger bias: %q", input)
			}
		})
	}
}

func TestContent_DetectBias_Empty(t *testing.T) {
	score, triggered, _ := DetectBias("")
	if triggered {
		t.Error("Empty input should not trigger")
	}
	if score != 0.0 {
		t.Errorf("Score = %f, want 0.0", score)
	}
}

func TestContent_DetectProfanity_Triggers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"english profanity fuck", "What the fuck is this?"},
		{"english profanity shit", "This is total shit"},
		{"english profanity bitch", "You are such a bitch"},
		{"chinese profanity 1", "他妈的这什么东西"},
		{"chinese profanity 2", "你个傻逼"},
		{"chinese profanity 3", "草泥马"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := DetectProfanity(tt.input)
			if !triggered {
				t.Errorf("Expected profanity to be detected for input: %q", tt.input)
			}
			if score < 0.8 {
				t.Errorf("Score = %f, want >= 0.8", score)
			}
			if reason == "" {
				t.Error("Reason should not be empty when triggered")
			}
		})
	}
}

func TestContent_DetectProfanity_SafeInput(t *testing.T) {
	safeInputs := []string{
		"Please help me with this task",
		"What is the weather today?",
		"这段代码有什么问题？",
		"帮我翻译这句话",
	}

	for _, input := range safeInputs {
		t.Run(input, func(t *testing.T) {
			_, triggered, _ := DetectProfanity(input)
			if triggered {
				t.Errorf("Safe input should NOT trigger profanity: %q", input)
			}
		})
	}
}

func TestContent_DetectProfanity_Empty(t *testing.T) {
	score, triggered, _ := DetectProfanity("")
	if triggered {
		t.Error("Empty input should not trigger")
	}
	if score != 0.0 {
		t.Errorf("Score = %f, want 0.0", score)
	}
}

// --- Provider Integration Tests ---

func TestContent_AIPBuiltinProvider_Toxicity(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleToxicity,
		Stage:    StageInput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
	}

	req := EvalRequest{Input: "I will kill you for this"}
	score, triggered, reason, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Expected toxicity to be detected")
	}
	if score < 0.8 {
		t.Errorf("Score = %f, want >= 0.8", score)
	}
	if reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestContent_AIPBuiltinProvider_Bias(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleBias,
		Stage:    StageInput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
	}

	req := EvalRequest{Input: "All women are not able to code"}
	score, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Expected bias to be detected")
	}
	if score < 0.8 {
		t.Errorf("Score = %f, want >= 0.8", score)
	}
}

func TestContent_AIPBuiltinProvider_Profanity(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleProfanity,
		Stage:    StageInput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
	}

	req := EvalRequest{Input: "What the fuck is wrong with this?"}
	score, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Expected profanity to be detected")
	}
	if score < 0.8 {
		t.Errorf("Score = %f, want >= 0.8", score)
	}
}

func TestContent_AIPBuiltinProvider_OutputStage(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleToxicity,
		Stage:    StageOutput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
	}

	req := EvalRequest{
		Input:  "safe query",
		Output: "I will murder everyone who disagrees",
	}
	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Expected toxicity in output to be detected")
	}
}

func TestContent_AIPBuiltinProvider_SafeInput(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rules := []Rule{
		{Kind: RuleToxicity, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionBlock},
		{Kind: RuleBias, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionBlock},
		{Kind: RuleProfanity, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionBlock},
	}

	safeReq := EvalRequest{Input: "Please explain how machine learning works"}
	for _, rule := range rules {
		t.Run(string(rule.Kind), func(t *testing.T) {
			_, triggered, _, err := provider.Evaluate(context.Background(), rule, safeReq)
			if err != nil {
				t.Fatalf("Evaluate() error: %v", err)
			}
			if triggered {
				t.Errorf("Safe input should not trigger %s", rule.Kind)
			}
		})
	}
}

// --- Engine Integration Tests ---

func TestContent_Engine_BlocksToxicity(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&AIPBuiltinProvider{})

	rules := []Rule{
		{Kind: RuleToxicity, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionBlock},
	}
	engine := NewEngine(reg, rules)

	result, err := engine.Evaluate(context.Background(), EvalRequest{
		Input: "I will kill you for this",
	})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !result.Blocked {
		t.Error("Expected toxic input to be blocked")
	}
	if result.FinalAction != ActionBlock {
		t.Errorf("FinalAction = %s, want block", result.FinalAction)
	}
}

func TestContent_Engine_BlocksProfanity(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&AIPBuiltinProvider{})

	rules := []Rule{
		{Kind: RuleProfanity, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionBlock},
	}
	engine := NewEngine(reg, rules)

	result, err := engine.Evaluate(context.Background(), EvalRequest{
		Input: "This is total shit and I hate it",
	})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !result.Blocked {
		t.Error("Expected profane input to be blocked")
	}
}
