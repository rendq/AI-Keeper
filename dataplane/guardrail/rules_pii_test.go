package guardrail

import (
	"context"
	"testing"
)

// --- Unit Tests for PII / PIILeak Detection ---

func TestPIILeak_DetectPII_Triggers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"chinese ID card", "我的身份证号是110101199003071234"},
		{"phone number", "请联系我手机号13812345678"},
		{"email address", "发邮件到 test@example.com 联系我"},
		{"bank card 16 digits", "我的银行卡号是6222021234567890"},
		{"multiple PII", "身份证110101199003071234 手机13912345678 邮箱a@b.com"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := DetectPII(tt.input)
			if !triggered {
				t.Errorf("Expected PII to be detected for input: %q", tt.input)
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

func TestPIILeak_DetectPII_SafeInput(t *testing.T) {
	safeInputs := []struct {
		name  string
		input string
	}{
		{"greeting", "你好，今天天气怎么样？"},
		{"question", "什么是人工智能？"},
		{"code", "func main() { fmt.Println(\"hello\") }"},
		{"short number", "今天温度是25度"},
	}

	for _, tt := range safeInputs {
		t.Run(tt.name, func(t *testing.T) {
			_, triggered, _ := DetectPII(tt.input)
			if triggered {
				t.Errorf("Safe input should NOT trigger PII detection: %q", tt.input)
			}
		})
	}
}

func TestPIILeak_DetectPII_EmptyInput(t *testing.T) {
	score, triggered, reason := DetectPII("")
	if triggered {
		t.Error("Empty input should not trigger")
	}
	if score != 0.0 {
		t.Errorf("Score = %f, want 0.0", score)
	}
	if reason != "" {
		t.Errorf("Reason = %q, want empty", reason)
	}
}

func TestPIILeak_DetectPIILeak_Triggers(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"leaks phone", "用户的手机号是13812345678"},
		{"leaks ID card", "该用户身份证号为110101199003071234"},
		{"leaks email", "客户邮箱为user@company.com"},
		{"leaks bank card", "银行卡号6222021234567890123"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			score, triggered, reason := DetectPIILeak(tt.output)
			if !triggered {
				t.Errorf("Expected PIILeak to be detected for output: %q", tt.output)
			}
			if score < 0.9 {
				t.Errorf("Score = %f, want >= 0.9 (PIILeak is high severity)", score)
			}
			if reason == "" {
				t.Error("Reason should not be empty when triggered")
			}
		})
	}
}

func TestPIILeak_DetectPIILeak_SafeOutput(t *testing.T) {
	safeOutputs := []string{
		"以下是查询结果的摘要信息",
		"The weather today is sunny with a high of 25°C",
		"这是一个关于机器学习的回答",
	}

	for _, output := range safeOutputs {
		t.Run(output, func(t *testing.T) {
			_, triggered, _ := DetectPIILeak(output)
			if triggered {
				t.Errorf("Safe output should NOT trigger PIILeak: %q", output)
			}
		})
	}
}

func TestPIILeak_DetectPIILeak_EmptyOutput(t *testing.T) {
	score, triggered, _ := DetectPIILeak("")
	if triggered {
		t.Error("Empty output should not trigger")
	}
	if score != 0.0 {
		t.Errorf("Score = %f, want 0.0", score)
	}
}

// --- Provider Integration Tests ---

func TestPIILeak_AIPBuiltinProvider_PII_InputStage(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RulePII,
		Stage:    StageInput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionWarn,
	}

	req := EvalRequest{Input: "我的手机号是13912345678"}
	score, triggered, reason, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Expected PII to be detected in input")
	}
	if score < 0.8 {
		t.Errorf("Score = %f, want >= 0.8", score)
	}
	if reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestPIILeak_AIPBuiltinProvider_PIILeak_OutputStage(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RulePIILeak,
		Stage:    StageOutput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
	}

	req := EvalRequest{
		Input:  "查询用户信息",
		Output: "用户手机号为13812345678，邮箱test@example.com",
	}
	score, triggered, reason, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !triggered {
		t.Error("Expected PIILeak to be detected in output")
	}
	if score < 0.9 {
		t.Errorf("Score = %f, want >= 0.9", score)
	}
	if reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestPIILeak_AIPBuiltinProvider_PII_SafeInput(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RulePII,
		Stage:    StageInput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
	}

	req := EvalRequest{Input: "今天天气如何？"}
	_, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if triggered {
		t.Error("Safe input should not trigger PII rule")
	}
}

func TestPIILeak_AIPBuiltinProvider_EmptyInput(t *testing.T) {
	provider := &AIPBuiltinProvider{}
	rule := Rule{
		Kind:     RulePII,
		Stage:    StageInput,
		Provider: ProviderAIPBuiltin,
		Action:   ActionBlock,
	}

	req := EvalRequest{Input: ""}
	score, triggered, _, err := provider.Evaluate(context.Background(), rule, req)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if triggered {
		t.Error("Empty input should not trigger")
	}
	if score != 0.0 {
		t.Errorf("Score = %f, want 0.0", score)
	}
}

// --- Engine Integration Tests ---

func TestPIILeak_Engine_BlocksPIILeak(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&AIPBuiltinProvider{})

	rules := []Rule{
		{Kind: RulePIILeak, Stage: StageOutput, Provider: ProviderAIPBuiltin, Action: ActionBlock},
	}
	engine := NewEngine(reg, rules)

	result, err := engine.Evaluate(context.Background(), EvalRequest{
		Input:  "查询用户资料",
		Output: "用户身份证号110101199003071234，手机13812345678",
	})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if !result.Blocked {
		t.Error("Expected PIILeak output to be blocked")
	}
	if result.FinalAction != ActionBlock {
		t.Errorf("FinalAction = %s, want block", result.FinalAction)
	}
}

func TestPIILeak_Engine_WarnsPII(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&AIPBuiltinProvider{})

	rules := []Rule{
		{Kind: RulePII, Stage: StageInput, Provider: ProviderAIPBuiltin, Action: ActionWarn},
	}
	engine := NewEngine(reg, rules)

	result, err := engine.Evaluate(context.Background(), EvalRequest{
		Input: "我的手机号13912345678",
	})
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if result.Blocked {
		t.Error("PII with warn action should not block")
	}
	if result.FinalAction != ActionWarn {
		t.Errorf("FinalAction = %s, want warn", result.FinalAction)
	}
	if len(result.Hits) == 0 {
		t.Error("Expected at least one hit")
	}
}
