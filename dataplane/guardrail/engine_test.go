package guardrail

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// --- Test Provider (configurable for testing) ---

type testProvider struct {
	name      ProviderName
	score     float64
	triggered bool
	reason    string
	err       error
}

func (p *testProvider) Name() ProviderName { return p.name }

func (p *testProvider) Evaluate(_ context.Context, _ Rule, _ EvalRequest) (float64, bool, string, error) {
	return p.score, p.triggered, p.reason, p.err
}

// --- Provider Registry Tests ---

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	reg := NewProviderRegistry()

	reg.Register(&AIPBuiltinProvider{})
	reg.Register(&LlamaGuardV3Provider{Endpoint: "http://llamaguard:8080"})
	reg.Register(&NemoGuardrailsProvider{Endpoint: "http://nemo:8080"})
	reg.Register(&CustomProvider{Endpoint: "http://custom:9090"})

	tests := []ProviderName{ProviderAIPBuiltin, ProviderLlamaGuardV3, ProviderNemoGuardrails, ProviderCustom}
	for _, name := range tests {
		p, err := reg.Get(name)
		if err != nil {
			t.Errorf("Get(%s) returned error: %v", name, err)
		}
		if p.Name() != name {
			t.Errorf("Get(%s).Name() = %s, want %s", name, p.Name(), name)
		}
	}

	// Not found
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Error("Get(nonexistent) should return error")
	}
}

func TestProviderRegistry_List(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&AIPBuiltinProvider{})
	reg.Register(&LlamaGuardV3Provider{})

	names := reg.List()
	if len(names) != 2 {
		t.Errorf("List() returned %d providers, want 2", len(names))
	}
}

// --- Engine Tests ---

func TestEngine_EvaluateNoRules(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&AIPBuiltinProvider{})

	engine := NewEngine(reg, nil)
	result, err := engine.Evaluate(context.Background(), EvalRequest{Input: "hello"})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if result.FinalAction != ActionAllow {
		t.Errorf("FinalAction = %s, want allow", result.FinalAction)
	}
	if result.Blocked {
		t.Error("Blocked should be false with no rules")
	}
	if len(result.Hits) != 0 {
		t.Errorf("Hits = %d, want 0", len(result.Hits))
	}
}

func TestEngine_EvaluateStageOrder(t *testing.T) {
	// Create providers that always trigger
	inputProvider := &testProvider{name: "input-prov", score: 0.9, triggered: true, reason: "injection detected"}
	outputProvider := &testProvider{name: "output-prov", score: 0.7, triggered: true, reason: "pii leak"}
	behaviorProvider := &testProvider{name: "behavior-prov", score: 0.5, triggered: true, reason: "blocked topic"}

	reg := NewProviderRegistry()
	reg.Register(inputProvider)
	reg.Register(outputProvider)
	reg.Register(behaviorProvider)

	rules := []Rule{
		{Kind: RulePromptInjection, Stage: StageInput, Provider: "input-prov", Action: ActionWarn},
		{Kind: RulePIILeak, Stage: StageOutput, Provider: "output-prov", Action: ActionWarn},
		{Kind: RuleCustom, Stage: StageBehavior, Provider: "behavior-prov", Action: ActionWarn},
	}

	engine := NewEngine(reg, rules)
	result, err := engine.Evaluate(context.Background(), EvalRequest{Input: "test", Output: "test output"})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}

	// All 3 stages should execute
	if len(result.StagesExecuted) != 3 {
		t.Errorf("StagesExecuted = %v, want 3 stages", result.StagesExecuted)
	}
	if result.StagesExecuted[0] != StageInput {
		t.Errorf("Stage[0] = %s, want input", result.StagesExecuted[0])
	}
	if result.StagesExecuted[1] != StageOutput {
		t.Errorf("Stage[1] = %s, want output", result.StagesExecuted[1])
	}
	if result.StagesExecuted[2] != StageBehavior {
		t.Errorf("Stage[2] = %s, want behavior", result.StagesExecuted[2])
	}

	// All 3 hits recorded
	if len(result.Hits) != 3 {
		t.Errorf("Hits = %d, want 3", len(result.Hits))
	}

	// Final action is warn (all are warn)
	if result.FinalAction != ActionWarn {
		t.Errorf("FinalAction = %s, want warn", result.FinalAction)
	}
}

func TestEngine_InputBlockSkipsLaterStages(t *testing.T) {
	blockProvider := &testProvider{name: "blocker", score: 1.0, triggered: true, reason: "blocked"}
	outputProvider := &testProvider{name: "output-prov", score: 0.8, triggered: true, reason: "should not run"}

	reg := NewProviderRegistry()
	reg.Register(blockProvider)
	reg.Register(outputProvider)

	rules := []Rule{
		{Kind: RulePromptInjection, Stage: StageInput, Provider: "blocker", Action: ActionBlock},
		{Kind: RulePIILeak, Stage: StageOutput, Provider: "output-prov", Action: ActionWarn},
	}

	engine := NewEngine(reg, rules)
	result, err := engine.Evaluate(context.Background(), EvalRequest{Input: "attack"})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}

	// Only input stage executed
	if len(result.StagesExecuted) != 1 {
		t.Errorf("StagesExecuted = %v, want only input", result.StagesExecuted)
	}
	if result.StagesExecuted[0] != StageInput {
		t.Errorf("Stage[0] = %s, want input", result.StagesExecuted[0])
	}

	if !result.Blocked {
		t.Error("Blocked should be true")
	}
	if result.FinalAction != ActionBlock {
		t.Errorf("FinalAction = %s, want block", result.FinalAction)
	}

	// Only 1 hit (output rule never executed)
	if len(result.Hits) != 1 {
		t.Errorf("Hits = %d, want 1", len(result.Hits))
	}
}

func TestEngine_ActionAggregation(t *testing.T) {
	// Test that aggregation follows: block > escalate > mask > warn > allow
	warnProvider := &testProvider{name: "warn-prov", score: 0.6, triggered: true, reason: "minor issue"}
	escalateProvider := &testProvider{name: "esc-prov", score: 0.8, triggered: true, reason: "escalate needed"}

	reg := NewProviderRegistry()
	reg.Register(warnProvider)
	reg.Register(escalateProvider)

	rules := []Rule{
		{Kind: RuleToxicity, Stage: StageOutput, Provider: "warn-prov", Action: ActionWarn},
		{Kind: RuleBias, Stage: StageOutput, Provider: "esc-prov", Action: ActionEscalate},
	}

	engine := NewEngine(reg, rules)
	result, err := engine.Evaluate(context.Background(), EvalRequest{Output: "response"})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}

	if result.FinalAction != ActionEscalate {
		t.Errorf("FinalAction = %s, want escalate (higher priority)", result.FinalAction)
	}
	if len(result.Hits) != 2 {
		t.Errorf("Hits = %d, want 2", len(result.Hits))
	}
}

func TestEngine_ScoreRecorded(t *testing.T) {
	prov := &testProvider{name: "scorer", score: 0.85, triggered: true, reason: "high confidence"}
	reg := NewProviderRegistry()
	reg.Register(prov)

	rules := []Rule{
		{Kind: RuleJailbreak, Stage: StageInput, Provider: "scorer", Action: ActionBlock},
	}

	engine := NewEngine(reg, rules)
	result, err := engine.Evaluate(context.Background(), EvalRequest{Input: "jailbreak attempt"})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}

	if len(result.Hits) != 1 {
		t.Fatalf("Hits = %d, want 1", len(result.Hits))
	}
	hit := result.Hits[0]
	if hit.Score != 0.85 {
		t.Errorf("Hit.Score = %f, want 0.85", hit.Score)
	}
	if hit.Action != ActionBlock {
		t.Errorf("Hit.Action = %s, want block", hit.Action)
	}
	if hit.Reason != "high confidence" {
		t.Errorf("Hit.Reason = %q, want 'high confidence'", hit.Reason)
	}
	if hit.Timestamp.IsZero() {
		t.Error("Hit.Timestamp should not be zero")
	}
}

func TestEngine_SetRulesHotReload(t *testing.T) {
	prov := &testProvider{name: "prov", score: 0.9, triggered: true, reason: "triggered"}
	reg := NewProviderRegistry()
	reg.Register(prov)

	engine := NewEngine(reg, nil)

	// Initially no rules, should allow
	result, _ := engine.Evaluate(context.Background(), EvalRequest{Input: "test"})
	if result.FinalAction != ActionAllow {
		t.Errorf("before reload: FinalAction = %s, want allow", result.FinalAction)
	}

	// Hot reload new rules
	engine.SetRules([]Rule{
		{Kind: RulePromptInjection, Stage: StageInput, Provider: "prov", Action: ActionBlock},
	})

	result, _ = engine.Evaluate(context.Background(), EvalRequest{Input: "test"})
	if result.FinalAction != ActionBlock {
		t.Errorf("after reload: FinalAction = %s, want block", result.FinalAction)
	}
}

func TestEngine_ProviderNotFound(t *testing.T) {
	reg := NewProviderRegistry()
	rules := []Rule{
		{Kind: RuleCustom, Stage: StageInput, Provider: "missing-provider", Action: ActionBlock},
	}

	engine := NewEngine(reg, rules)
	_, err := engine.Evaluate(context.Background(), EvalRequest{Input: "test"})
	if err == nil {
		t.Error("Evaluate() should return error when provider not found")
	}
}

// --- Hot Reload Watcher Tests ---

// mockSubscriber implements Subscriber for testing.
type mockSubscriber struct {
	ch chan []byte
}

func (m *mockSubscriber) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	return m.ch, nil
}

func TestReloadWatcher_AppliesUpdate(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&testProvider{name: ProviderAIPBuiltin, score: 0.9, triggered: true, reason: "hit"})

	engine := NewEngine(reg, nil)
	msgCh := make(chan []byte, 1)
	sub := &mockSubscriber{ch: msgCh}

	watcher := NewReloadWatcher(engine, sub, "guardrail.rules", logr.Discard())

	// Prepare update
	update := RuleSetUpdate{
		Rules: []Rule{
			{Kind: RulePromptInjection, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionBlock},
			{Kind: RulePIILeak, Stage: StageOutput, Provider: ProviderAIPBuiltin, Action: ActionWarn},
		},
		Version:   1,
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(update)

	// Send update and close channel
	msgCh <- data
	close(msgCh)

	ctx := context.Background()
	_ = watcher.Start(ctx)

	// Verify rules were applied
	rules := engine.Rules()
	if len(rules) != 2 {
		t.Fatalf("Rules() = %d, want 2", len(rules))
	}
	if watcher.CurrentVersion() != 1 {
		t.Errorf("CurrentVersion() = %d, want 1", watcher.CurrentVersion())
	}
	if watcher.LastReload().IsZero() {
		t.Error("LastReload() should not be zero")
	}
}

func TestReloadWatcher_SkipsStaleVersion(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&testProvider{name: ProviderAIPBuiltin})

	engine := NewEngine(reg, nil)
	msgCh := make(chan []byte, 2)
	sub := &mockSubscriber{ch: msgCh}

	watcher := NewReloadWatcher(engine, sub, "guardrail.rules", logr.Discard())

	// Send v2 then v1 (stale)
	update2 := RuleSetUpdate{
		Rules:   []Rule{{Kind: RulePII, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionMask}},
		Version: 2,
	}
	update1 := RuleSetUpdate{
		Rules:   []Rule{{Kind: RuleToxicity, Stage: StageOutput, Provider: ProviderAIPBuiltin, Action: ActionWarn}},
		Version: 1,
	}
	data2, _ := json.Marshal(update2)
	data1, _ := json.Marshal(update1)
	msgCh <- data2
	msgCh <- data1
	close(msgCh)

	_ = watcher.Start(context.Background())

	// Only v2 should be applied
	rules := engine.Rules()
	if len(rules) != 1 {
		t.Fatalf("Rules() = %d, want 1", len(rules))
	}
	if rules[0].Kind != RulePII {
		t.Errorf("Rule.Kind = %s, want PII (from v2)", rules[0].Kind)
	}
	if watcher.CurrentVersion() != 2 {
		t.Errorf("CurrentVersion() = %d, want 2", watcher.CurrentVersion())
	}
}

func TestReloadWatcher_HandlesInvalidJSON(t *testing.T) {
	reg := NewProviderRegistry()
	engine := NewEngine(reg, nil)
	msgCh := make(chan []byte, 1)
	sub := &mockSubscriber{ch: msgCh}

	watcher := NewReloadWatcher(engine, sub, "guardrail.rules", logr.Discard())

	// Send invalid JSON
	msgCh <- []byte("not json{{{")
	close(msgCh)

	_ = watcher.Start(context.Background())

	// Engine should still have no rules (update ignored)
	if len(engine.Rules()) != 0 {
		t.Error("Invalid JSON should not update rules")
	}
	if watcher.CurrentVersion() != 0 {
		t.Errorf("CurrentVersion() = %d, want 0", watcher.CurrentVersion())
	}
}

// --- Action Priority Tests ---

func TestActionPriority(t *testing.T) {
	tests := []struct {
		a, b Action
	}{
		{ActionBlock, ActionEscalate},
		{ActionEscalate, ActionMask},
		{ActionMask, ActionWarn},
		{ActionWarn, ActionAllow},
	}
	for _, tt := range tests {
		if ActionPriority(tt.a) <= ActionPriority(tt.b) {
			t.Errorf("ActionPriority(%s) should be > ActionPriority(%s)", tt.a, tt.b)
		}
	}
}

// --- Stage Order Tests ---

func TestStageOrder(t *testing.T) {
	if StageOrder(StageInput) >= StageOrder(StageOutput) {
		t.Error("input should come before output")
	}
	if StageOrder(StageOutput) >= StageOrder(StageBehavior) {
		t.Error("output should come before behavior")
	}
}

// --- All 11 Rule Kinds Exist ---

func TestAllRuleKindsExist(t *testing.T) {
	kinds := []RuleKind{
		RulePromptInjection,
		RuleJailbreak,
		RulePII,
		RulePIILeak,
		RuleToxicity,
		RuleHallucination,
		RuleGrounding,
		RuleClassificationLeak,
		RuleBias,
		RuleProfanity,
		RuleCustom,
	}
	if len(kinds) != 11 {
		t.Errorf("Expected 11 rule kinds, got %d", len(kinds))
	}
	// Verify all are non-empty strings
	for _, k := range kinds {
		if k == "" {
			t.Error("Rule kind should not be empty")
		}
	}
}

// --- All 4 Providers Exist ---

func TestAllProvidersImplementInterface(t *testing.T) {
	providers := []Provider{
		&AIPBuiltinProvider{},
		&LlamaGuardV3Provider{Endpoint: "http://localhost:8080"},
		&NemoGuardrailsProvider{Endpoint: "http://localhost:8081"},
		&CustomProvider{Endpoint: "http://localhost:9090"},
	}

	expected := []ProviderName{ProviderAIPBuiltin, ProviderLlamaGuardV3, ProviderNemoGuardrails, ProviderCustom}

	for i, p := range providers {
		if p.Name() != expected[i] {
			t.Errorf("Provider[%d].Name() = %s, want %s", i, p.Name(), expected[i])
		}
		// Verify Evaluate doesn't panic
		_, _, _, err := p.Evaluate(context.Background(), Rule{}, EvalRequest{})
		if err != nil {
			t.Errorf("Provider %s Evaluate returned unexpected error: %v", p.Name(), err)
		}
	}
}
