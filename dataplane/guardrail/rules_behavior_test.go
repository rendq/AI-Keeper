package guardrail

import (
	"context"
	"testing"
)

// --- Unit Tests for Behavior Stage Rules ---

func TestBehavior_DetectMissingCitations_WithCitations(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"numbered citation [1]", "According to the study [1], climate change is real."},
		{"numbered citation [2]", "Results indicate positive outcomes [2] and growth [3]."},
		{"source marker", "The data confirms this [source]."},
		{"Source marker", "Based on evidence [Source]."},
		{"citation marker", "As stated [citation] previously."},
		{"ref marker", "See [ref] for details."},
		{"chinese citation", "根据研究「引用」显示结果"},
		{"chinese source", "引用来源表明这是正确的"},
		{"url citation", "See https://example.com/paper for more details."},
		{"source prefix", "source: arxiv.org/abs/1234"},
		{"reference prefix", "reference: doi.org/10.1234/test"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, triggered, _ := DetectMissingCitations(tt.output)
			if triggered {
				t.Errorf("Output with citations should NOT trigger: %q", tt.output)
			}
		})
	}
}

func TestBehavior_DetectMissingCitations_WithoutCitations(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"plain text", "The sky is blue and water is wet."},
		{"no markers", "Machine learning is a subset of artificial intelligence."},
		{"chinese no citation", "今天天气很好，适合出门散步。"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := DetectMissingCitations(tt.output)
			if !triggered {
				t.Errorf("Output without citations should trigger: %q", tt.output)
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

func TestBehavior_DetectMissingCitations_Empty(t *testing.T) {
	score, triggered, _ := DetectMissingCitations("")
	if triggered {
		t.Error("Empty input should not trigger")
	}
	if score != 0.0 {
		t.Errorf("Score = %f, want 0.0", score)
	}
}

func TestBehavior_DetectBlockedTopics_Matched(t *testing.T) {
	blockedTopics := []string{"politics", "gambling", "drugs"}

	cases := []struct {
		name string
		text string
	}{
		{"politics topic", "Let me tell you about politics and elections"},
		{"gambling topic", "Here are some tips for gambling at casinos"},
		{"drugs topic", "The effects of drugs on the human body"},
		{"case insensitive", "POLITICS is a controversial topic"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := DetectBlockedTopics(tt.text, blockedTopics)
			if !triggered {
				t.Errorf("Text matching blocked topic should trigger: %q", tt.text)
			}
			if score < 0.9 {
				t.Errorf("Score = %f, want >= 0.9", score)
			}
			if reason == "" {
				t.Error("Reason should not be empty when triggered")
			}
		})
	}
}

func TestBehavior_DetectBlockedTopics_NotMatched(t *testing.T) {
	blockedTopics := []string{"politics", "gambling", "drugs"}

	safeInputs := []string{
		"How to build a web application?",
		"Explain quantum computing",
		"What is the weather forecast?",
	}

	for _, text := range safeInputs {
		t.Run(text, func(t *testing.T) {
			_, triggered, _ := DetectBlockedTopics(text, blockedTopics)
			if triggered {
				t.Errorf("Safe text should NOT trigger: %q", text)
			}
		})
	}
}

func TestBehavior_DetectBlockedTopics_EmptyInputs(t *testing.T) {
	// Empty text
	_, triggered, _ := DetectBlockedTopics("", []string{"politics"})
	if triggered {
		t.Error("Empty text should not trigger")
	}

	// Empty topics
	_, triggered, _ = DetectBlockedTopics("some text about politics", []string{})
	if triggered {
		t.Error("Empty topics list should not trigger")
	}

	// Nil topics
	_, triggered, _ = DetectBlockedTopics("some text", nil)
	if triggered {
		t.Error("Nil topics list should not trigger")
	}
}

func TestBehavior_EvaluateCustomCEL_Triggered(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		expression string
	}{
		{"contains keyword", "this contains secret data", `text.contains("secret")`},
		{"size check", "hi", `size(text) < 5`},
		{"starts with", "ALERT: something happened", `text.startsWith("ALERT")`},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := EvaluateCustomCEL(tt.text, tt.expression)
			if !triggered {
				t.Errorf("CEL expression should trigger for text: %q, expr: %s", tt.text, tt.expression)
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

func TestBehavior_EvaluateCustomCEL_NotTriggered(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		expression string
	}{
		{"no keyword", "this is safe text", `text.contains("secret")`},
		{"size ok", "this is a longer sentence", `size(text) < 5`},
		{"no match", "hello world", `text.startsWith("ALERT")`},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, triggered, _ := EvaluateCustomCEL(tt.text, tt.expression)
			if triggered {
				t.Errorf("CEL expression should NOT trigger for text: %q, expr: %s", tt.text, tt.expression)
			}
		})
	}
}

func TestBehavior_EvaluateCustomCEL_EmptyExpression(t *testing.T) {
	_, triggered, _ := EvaluateCustomCEL("some text", "")
	if triggered {
		t.Error("Empty expression should not trigger")
	}
}

func TestBehavior_EvaluateCustomCEL_InvalidExpression(t *testing.T) {
	_, triggered, reason := EvaluateCustomCEL("some text", "invalid !! syntax")
	if triggered {
		t.Error("Invalid expression should not trigger")
	}
	if reason == "" {
		t.Error("Should return error reason for invalid expression")
	}
}

// --- Provider Integration Tests for Behavior ---

func TestBehavior_AIPBuiltinProvider_RequiredCitations_Passes(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleCustom,
		Stage:    StageBehavior,
		Provider: ProviderAIPBuiltin,
		Action:   ActionWarn,
		Config:   map[string]string{"requiredCitations": "true"},
	}

	req := EvalRequest{
		Input:  "Tell me about climate change",
		Output: "Climate change is a global issue [1]. Studies show rising temperatures [2].",
	}
	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if triggered {
		t.Error("Output with citations should not trigger requiredCitations rule")
	}
}

func TestBehavior_AIPBuiltinProvider_RequiredCitations_Triggers(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleCustom,
		Stage:    StageBehavior,
		Provider: ProviderAIPBuiltin,
		Action:   ActionWarn,
		Config:   map[string]string{"requiredCitations": "true"},
	}

	req := EvalRequest{
		Input:  "Tell me about climate change",
		Output: "Climate change is a global issue. Studies show rising temperatures.",
	}
	score, triggered, reason, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Output without citations should trigger requiredCitations rule")
	}
	if score < 0.8 {
		t.Errorf("Score = %f, want >= 0.8", score)
	}
	if reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestBehavior_AIPBuiltinProvider_BlockedTopics_Triggers(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleCustom,
		Stage:    StageBehavior,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
		Config:   map[string]string{"blockedTopics": `["politics","gambling"]`},
	}

	req := EvalRequest{
		Input:  "Tell me about gambling strategies",
		Output: "Here are some gambling tips",
	}
	score, triggered, reason, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Text matching blocked topic should trigger")
	}
	if score < 0.9 {
		t.Errorf("Score = %f, want >= 0.9", score)
	}
	if reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestBehavior_AIPBuiltinProvider_BlockedTopics_Passes(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleCustom,
		Stage:    StageBehavior,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
		Config:   map[string]string{"blockedTopics": `["politics","gambling"]`},
	}

	req := EvalRequest{
		Input:  "How to build a REST API?",
		Output: "Here is how to build a REST API with Go",
	}
	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if triggered {
		t.Error("Safe text should not trigger blocked topics")
	}
}

func TestBehavior_AIPBuiltinProvider_CustomCEL_Triggers(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleCustom,
		Stage:    StageBehavior,
		Provider: ProviderAIPBuiltin,
		Action:   ActionEscalate,
		Config:   map[string]string{"expression": `text.contains("confidential")`},
	}

	req := EvalRequest{
		Output: "This document is confidential and should not be shared.",
	}
	score, triggered, reason, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Custom CEL expression should trigger")
	}
	if score < 0.8 {
		t.Errorf("Score = %f, want >= 0.8", score)
	}
	if reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestBehavior_AIPBuiltinProvider_CustomCEL_Passes(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RuleCustom,
		Stage:    StageBehavior,
		Provider: ProviderAIPBuiltin,
		Action:   ActionEscalate,
		Config:   map[string]string{"expression": `text.contains("confidential")`},
	}

	req := EvalRequest{
		Output: "This is a public document about open source.",
	}
	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if triggered {
		t.Error("Custom CEL expression should not trigger for safe text")
	}
}
