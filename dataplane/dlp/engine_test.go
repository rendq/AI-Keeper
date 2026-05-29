package dlp

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestDetectAndMask_IDCard(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "用户身份证号是110101199001011234，请核实",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationConfidential,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Blocked {
		t.Fatal("should not be blocked in mask mode")
	}
	if !strings.Contains(result.Text, "<PII:ID_CARD>") {
		t.Errorf("expected masked ID card, got: %s", result.Text)
	}
	if strings.Contains(result.Text, "110101199001011234") {
		t.Errorf("original ID card should be replaced, got: %s", result.Text)
	}
	assertContainsKind(t, result.Redactions, PIIKindIDCard)
	if result.Classification != ClassificationConfidential {
		t.Errorf("classification should propagate, got: %s", result.Classification)
	}
}

func TestDetectAndMask_Phone(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "联系电话：13812345678",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationInternal,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Text, "<PII:PHONE>") {
		t.Errorf("expected masked phone, got: %s", result.Text)
	}
	assertContainsKind(t, result.Redactions, PIIKindPhone)
}

func TestDetectAndMask_Email(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "邮箱是user@example.com，请联系",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationPublic,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Text, "<PII:EMAIL>") {
		t.Errorf("expected masked email, got: %s", result.Text)
	}
	assertContainsKind(t, result.Redactions, PIIKindEmail)
}

func TestDetectAndMask_BankCard(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "银行卡号：6222021234567890123",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationRestricted,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Text, "<PII:BANK_CARD>") {
		t.Errorf("expected masked bank card, got: %s", result.Text)
	}
	assertContainsKind(t, result.Redactions, PIIKindBankCard)
}

func TestDetectAndMask_PostalCode(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "邮编 100000 对应北京",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationPublic,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Text, "<PII:POSTAL_CODE>") {
		t.Errorf("expected masked postal code, got: %s", result.Text)
	}
	assertContainsKind(t, result.Redactions, PIIKindPostalCode)
}

func TestDetectAndBlock_Phone(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "我的手机号是13900001111",
		Mode:           ModeDetectAndBlock,
		Classification: ClassificationConfidential,
	}
	result, err := engine.Inspect(req)
	if !errors.Is(err, ErrBlockedByDLP) {
		t.Fatalf("expected ErrBlockedByDLP, got: %v", err)
	}
	if !result.Blocked {
		t.Fatal("result should indicate blocked")
	}
	assertContainsKind(t, result.Redactions, PIIKindPhone)
}

func TestDetectAndBlock_NoPII(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "这是一条没有任何敏感信息的普通文本",
		Mode:           ModeDetectAndBlock,
		Classification: ClassificationPublic,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Blocked {
		t.Fatal("should not be blocked when no PII")
	}
	if result.Text != req.Text {
		t.Errorf("text should pass through unchanged, got: %s", result.Text)
	}
	if len(result.Redactions) != 0 {
		t.Errorf("no redactions expected, got: %v", result.Redactions)
	}
}

func TestDetectAndMask_NoPII(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "普通文本无PII",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationInternal,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != req.Text {
		t.Errorf("text should pass through unchanged, got: %s", result.Text)
	}
}

func TestDetectAndMask_MultiplePII(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "身份证110101199001011234，电话13812345678，邮箱foo@bar.com",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationSecret,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Text, "110101199001011234") {
		t.Error("ID card should be masked")
	}
	if strings.Contains(result.Text, "13812345678") {
		t.Error("phone should be masked")
	}
	if strings.Contains(result.Text, "foo@bar.com") {
		t.Error("email should be masked")
	}
	if len(result.Redactions) < 3 {
		t.Errorf("expected at least 3 redaction kinds, got: %v", result.Redactions)
	}
}

func TestInspectOutput_Block(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "系统回复包含手机号13900001111",
		Mode:           ModeDetectAndBlock,
		Classification: ClassificationConfidential,
	}
	result, err := engine.InspectOutput(req)
	if !errors.Is(err, ErrBlockedByDLP) {
		t.Fatalf("output inspection should block, got: %v", err)
	}
	if !result.Blocked {
		t.Fatal("result should indicate blocked")
	}
}

func TestInspectOutput_Mask(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "回复中有邮箱test@domain.org",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationInternal,
	}
	result, err := engine.InspectOutput(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Text, "<PII:EMAIL>") {
		t.Errorf("output should be masked, got: %s", result.Text)
	}
}

func TestClassificationPropagation(t *testing.T) {
	engine := NewEngine(nil)
	classifications := []Classification{
		ClassificationPublic,
		ClassificationInternal,
		ClassificationConfidential,
		ClassificationRestricted,
		ClassificationSecret,
	}
	for _, cls := range classifications {
		req := InspectRequest{
			Text:           "无PII文本",
			Mode:           ModeDetectAndMask,
			Classification: cls,
		}
		result, err := engine.Inspect(req)
		if err != nil {
			t.Fatalf("unexpected error for classification %s: %v", cls, err)
		}
		if result.Classification != cls {
			t.Errorf("classification %s should propagate, got: %s", cls, result.Classification)
		}
	}
}

func TestRedactionsContainOnlyTypes(t *testing.T) {
	engine := NewEngine(nil)
	originalPhone := "13812345678"
	req := InspectRequest{
		Text:           "电话" + originalPhone,
		Mode:           ModeDetectAndMask,
		Classification: ClassificationInternal,
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify redactions contain only PIIKind (type), not the original value
	for _, kind := range result.Redactions {
		if string(kind) == originalPhone {
			t.Error("redactions should not contain original PII value")
		}
	}
	assertContainsKind(t, result.Redactions, PIIKindPhone)
}

func TestCustomPatternsRef(t *testing.T) {
	registry := NewPatternRegistry()
	// Register a custom pattern for IP addresses
	registry.Register("custom-ip", []Pattern{
		{
			Kind:        "IP_ADDRESS",
			Regex:       mustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
			Placeholder: "<PII:IP_ADDRESS>",
		},
	})

	engine := NewEngine(registry)
	req := InspectRequest{
		Text:           "服务器IP是192.168.1.100",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationInternal,
		PatternsRef:    "ref://patterns/custom-ip",
	}
	result, err := engine.Inspect(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Text, "<PII:IP_ADDRESS>") {
		t.Errorf("expected custom pattern to mask IP, got: %s", result.Text)
	}
	assertContainsKind(t, result.Redactions, "IP_ADDRESS")
}

func TestCustomPatternsRef_NotFound(t *testing.T) {
	registry := NewPatternRegistry()
	engine := NewEngine(registry)
	req := InspectRequest{
		Text:           "some text",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationPublic,
		PatternsRef:    "ref://patterns/nonexistent",
	}
	_, err := engine.Inspect(req)
	if err == nil {
		t.Fatal("expected error for unknown patternsRef")
	}
}

func TestCustomPatternsRef_InvalidFormat(t *testing.T) {
	registry := NewPatternRegistry()
	engine := NewEngine(registry)
	req := InspectRequest{
		Text:           "some text",
		Mode:           ModeDetectAndMask,
		Classification: ClassificationPublic,
		PatternsRef:    "invalid://format",
	}
	_, err := engine.Inspect(req)
	if err == nil {
		t.Fatal("expected error for invalid patternsRef format")
	}
}

func TestUnsupportedMode(t *testing.T) {
	engine := NewEngine(nil)
	req := InspectRequest{
		Text:           "电话13812345678",
		Mode:           "unknown_mode",
		Classification: ClassificationPublic,
	}
	_, err := engine.Inspect(req)
	if err == nil {
		t.Fatal("expected error for unsupported mode")
	}
}

// --- helpers ---

func assertContainsKind(t *testing.T, kinds []PIIKind, expected PIIKind) {
	t.Helper()
	for _, k := range kinds {
		if k == expected {
			return
		}
	}
	t.Errorf("expected redactions to contain %s, got: %v", expected, kinds)
}

func mustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
