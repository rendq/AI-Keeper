package guardrail

import (
	"context"
	"encoding/json"
	"testing"
)

// --- Hallucination Detection Tests ---

func TestDetectHallucination_Grounded(t *testing.T) {
	output := "The capital of France is Paris and it is located in Europe"
	sources := []string{"France is a country in Europe. Its capital city is Paris."}
	threshold := 0.4

	score, triggered, reason := DetectHallucination(output, sources, threshold)
	if triggered {
		t.Errorf("expected not triggered for grounded output, got triggered with score=%.2f reason=%s", score, reason)
	}
	if score < threshold {
		t.Errorf("expected score >= %.2f, got %.2f", threshold, score)
	}
}

func TestDetectHallucination_NotGrounded(t *testing.T) {
	output := "The weather tomorrow will be sunny with temperatures reaching 30 degrees"
	sources := []string{"France is a country in Europe. Its capital city is Paris."}
	threshold := 0.4

	score, triggered, reason := DetectHallucination(output, sources, threshold)
	if !triggered {
		t.Errorf("expected triggered for ungrounded output, got not triggered with score=%.2f", score)
	}
	if reason == "" {
		t.Error("expected non-empty reason when triggered")
	}
}

func TestDetectHallucination_EmptyInputs(t *testing.T) {
	// Empty output
	score, triggered, _ := DetectHallucination("", []string{"some source"}, 0.4)
	if triggered || score != 0.0 {
		t.Error("expected no trigger for empty output")
	}

	// Empty sources
	score, triggered, _ = DetectHallucination("some output", nil, 0.4)
	if triggered || score != 0.0 {
		t.Error("expected no trigger for empty sources")
	}
}

// --- Grounding Detection Tests ---

func TestDetectGrounding_Consistent(t *testing.T) {
	output := "The database supports SQL queries and transactions"
	kbResults := []string{"Our database system supports standard SQL queries and ACID transactions."}

	score, triggered, _ := DetectGrounding(output, kbResults)
	if triggered {
		t.Errorf("expected not triggered for consistent output, got triggered with score=%.2f", score)
	}
}

func TestDetectGrounding_Inconsistent(t *testing.T) {
	output := "Quantum entanglement allows faster than light communication between particles"
	kbResults := []string{"Our database system supports standard SQL queries and ACID transactions."}

	score, triggered, reason := DetectGrounding(output, kbResults)
	if !triggered {
		t.Errorf("expected triggered for inconsistent output, got score=%.2f", score)
	}
	if reason == "" {
		t.Error("expected non-empty reason when triggered")
	}
}

func TestDetectGrounding_EmptyInputs(t *testing.T) {
	score, triggered, _ := DetectGrounding("", []string{"source"})
	if triggered || score != 0.0 {
		t.Error("expected no trigger for empty output")
	}

	score, triggered, _ = DetectGrounding("output", nil)
	if triggered || score != 0.0 {
		t.Error("expected no trigger for empty kbResults")
	}
}

// --- ClassificationLeak Detection Tests ---

func TestDetectClassificationLeak_NoLeak(t *testing.T) {
	tests := []struct {
		output string
		max    string
	}{
		{"public", "public"},
		{"public", "internal"},
		{"internal", "confidential"},
		{"confidential", "restricted"},
		{"restricted", "secret"},
		{"public", "secret"},
	}

	for _, tt := range tests {
		score, triggered, reason := DetectClassificationLeak(tt.output, tt.max)
		if triggered {
			t.Errorf("DetectClassificationLeak(%q, %q) should not trigger, got score=%.2f reason=%s",
				tt.output, tt.max, score, reason)
		}
	}
}

func TestDetectClassificationLeak_Leak(t *testing.T) {
	tests := []struct {
		output string
		max    string
	}{
		{"internal", "public"},
		{"confidential", "internal"},
		{"restricted", "confidential"},
		{"secret", "restricted"},
		{"secret", "public"},
	}

	for _, tt := range tests {
		score, triggered, reason := DetectClassificationLeak(tt.output, tt.max)
		if !triggered {
			t.Errorf("DetectClassificationLeak(%q, %q) should trigger, got score=%.2f",
				tt.output, tt.max, score)
		}
		if score != 1.0 {
			t.Errorf("expected score 1.0 for leak, got %.2f", score)
		}
		if reason == "" {
			t.Error("expected non-empty reason for leak")
		}
	}
}

func TestDetectClassificationLeak_EmptyOrUnknown(t *testing.T) {
	// Empty values
	_, triggered, _ := DetectClassificationLeak("", "public")
	if triggered {
		t.Error("should not trigger with empty output classification")
	}

	_, triggered, _ = DetectClassificationLeak("public", "")
	if triggered {
		t.Error("should not trigger with empty max classification")
	}

	// Unknown level
	_, triggered, _ = DetectClassificationLeak("unknown", "public")
	if triggered {
		t.Error("should not trigger with unknown classification level")
	}
}

func TestDetectClassificationLeak_CaseInsensitive(t *testing.T) {
	_, triggered, _ := DetectClassificationLeak("Secret", "Public")
	if !triggered {
		t.Error("should trigger regardless of case")
	}
}

// --- Provider Integration Tests ---

func TestAIPBuiltinProvider_Hallucination(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	sources, _ := json.Marshal([]string{"France is a country in Europe. Its capital is Paris."})

	req := EvalRequest{
		Output: "The weather tomorrow will be sunny and warm",
		Metadata: map[string]string{
			"kb.sources": string(sources),
		},
	}
	rule := Rule{
		Kind:   RuleHallucination,
		Stage:  StageOutput,
		Action: ActionBlock,
		Config: map[string]string{"threshold": "0.4"},
	}

	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !triggered {
		t.Error("expected hallucination rule to trigger for ungrounded output")
	}
}

func TestAIPBuiltinProvider_Grounding(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	sources, _ := json.Marshal([]string{"The system uses PostgreSQL database for data storage."})

	req := EvalRequest{
		Output: "Quantum physics describes the behavior of subatomic particles",
		Metadata: map[string]string{
			"kb.sources": string(sources),
		},
	}
	rule := Rule{
		Kind:   RuleGrounding,
		Stage:  StageOutput,
		Action: ActionWarn,
	}

	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !triggered {
		t.Error("expected grounding rule to trigger for inconsistent output")
	}
}

func TestAIPBuiltinProvider_ClassificationLeak(t *testing.T) {
	provider := &AIPBuiltinProvider{}

	req := EvalRequest{
		Output: "This contains secret information",
		Metadata: map[string]string{
			"output.classification":  "secret",
			"input.maxClassification": "internal",
		},
	}
	rule := Rule{
		Kind:   RuleClassificationLeak,
		Stage:  StageOutput,
		Action: ActionBlock,
	}

	score, triggered, reason, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !triggered {
		t.Errorf("expected classification leak to trigger, got score=%.2f reason=%s", score, reason)
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %.2f", score)
	}
}

func TestAIPBuiltinProvider_ClassificationLeak_NoLeak(t *testing.T) {
	provider := &AIPBuiltinProvider{}

	req := EvalRequest{
		Output: "This is public information",
		Metadata: map[string]string{
			"output.classification":  "public",
			"input.maxClassification": "confidential",
		},
	}
	rule := Rule{
		Kind:   RuleClassificationLeak,
		Stage:  StageOutput,
		Action: ActionBlock,
	}

	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if triggered {
		t.Error("expected no classification leak when output level <= max")
	}
}

// --- ParseKBSources Tests ---

func TestParseKBSources(t *testing.T) {
	metadata := map[string]string{
		"kb.sources": `["source one", "source two"]`,
	}
	sources, err := ParseKBSources(metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if sources[0] != "source one" || sources[1] != "source two" {
		t.Errorf("unexpected sources: %v", sources)
	}
}

func TestParseKBSources_Empty(t *testing.T) {
	sources, err := ParseKBSources(map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sources != nil {
		t.Errorf("expected nil sources, got %v", sources)
	}
}

func TestParseKBSources_Invalid(t *testing.T) {
	metadata := map[string]string{
		"kb.sources": "not valid json",
	}
	_, err := ParseKBSources(metadata)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
