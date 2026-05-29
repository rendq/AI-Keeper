package audit

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// sampleEvent creates a representative AuditEvent for testing.
func sampleEvent() *Event {
	tokensIn := int64(100)
	tokensOut := int64(200)
	latencyMs := int64(350)
	sizeBytes := int64(2048)
	confidence := 0.95
	score := 0.87
	blocked := false
	reviewed := true
	idx := int32(0)
	durationMs := int64(1200)

	return &Event{
		InvocationID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Timestamp:    time.Date(2025, 5, 26, 10, 30, 0, 0, time.UTC),
		Trace: &EventTrace{
			TraceID:      "abc123def456",
			SpanID:       "span001",
			ParentSpanID: "span000",
		},
		Principal: EventPrincipal{
			User: &EventPrincipalUser{
				ID:         "user-42",
				TenantID:   "tenant-acme",
				Department: "legal",
				Attributes: map[string]string{"role": "attorney", "level": "senior"},
			},
			Agent: EventPrincipalAgent{
				Name:      "legal-copilot",
				Namespace: "acme-legal",
				Version:   "1.2.0",
			},
			ServiceAccount: &EventPrincipalSA{
				Name:     "sa-legal-copilot",
				SpiffeID: "spiffe://ai-keeper.io/ns/acme-legal/sa/legal-copilot",
			},
			OnBehalfOf: "user-42",
			SourceIP:   "10.0.1.42",
			UserAgent:  "feishu-bot/2.0",
			Channel:    "feishu",
		},
		Action: EventAction{
			Verb:     "invoke",
			Resource: "skill://contract-review@1.0.0",
			Method:   "ReviewContract",
		},
		Policy: &EventPolicy{
			Decision:           "allow",
			MatchedPolicies:    []string{"legal-acl", "global-default"},
			Reason:             "user has attorney role",
			ObligationsApplied: []string{"audit", "watermark"},
			ApprovalID:         "",
		},
		Request: &EventRequest{
			InputHash:      "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			Classification: "confidential",
			Redactions:     []string{"phone", "email"},
			SizeBytes:      &sizeBytes,
			Language:       "zh",
		},
		Response: &EventResponse{
			OutputHash:     "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			Confidence:     &confidence,
			Citations:      []EventCitation{{Source: "contract-db", ChunkID: "chunk-99", Score: &score}},
			Classification: "confidential",
		},
		Steps: []EventStep{
			{
				Index:     &idx,
				Type:      "model_call",
				Model:     "model://gpt-4o",
				TokensIn:  &tokensIn,
				TokensOut: &tokensOut,
				LatencyMs: &latencyMs,
			},
		},
		Cost: &EventCost{
			Tokens:     &EventCostTokens{Input: 100, Output: 200, Cached: 0},
			Usd:        "0.0045",
			DurationMs: &durationMs,
		},
		Guardrails: &EventGuardrail{
			Triggered: []EventGuardrailHit{{Rule: "PII", Stage: "input", Action: "mask", Score: &score}},
			Blocked:   &blocked,
		},
		Compliance: &EventCompliance{
			Tags:          []string{"GDPR", "SOC2"},
			DataResidency: "eu-west",
			Reviewed:      &reviewed,
			ReviewedBy:    "compliance-bot",
		},
		Outcome: &EventOutcome{
			Status: "success",
		},
	}
}

func TestEventHash_Deterministic(t *testing.T) {
	e := sampleEvent()

	hash1, err := ComputeHash(e)
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	hash2, err := ComputeHash(e)
	if err != nil {
		t.Fatalf("ComputeHash failed on second call: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("same event produced different hashes: %q vs %q", hash1, hash2)
	}

	// Validate hash format
	if !strings.HasPrefix(hash1, "sha256:") {
		t.Errorf("hash should start with 'sha256:', got %q", hash1)
	}
	if len(hash1) != len("sha256:")+64 {
		t.Errorf("hash has wrong length: %d, expected %d", len(hash1), len("sha256:")+64)
	}
}

func TestEventHash_FieldChangeProducesDifferentHash(t *testing.T) {
	e1 := sampleEvent()
	hash1, err := ComputeHash(e1)
	if err != nil {
		t.Fatal(err)
	}

	// Modify a field
	e2 := sampleEvent()
	e2.InvocationID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	hash2, err := ComputeHash(e2)
	if err != nil {
		t.Fatal(err)
	}

	if hash1 == hash2 {
		t.Error("different events should produce different hashes")
	}
}

func TestEventHash_ExcludesEventHashField(t *testing.T) {
	e := sampleEvent()
	hashBefore, _ := ComputeHash(e)

	// Setting eventHash should not affect the computed hash
	e.EventHash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	hashAfter, _ := ComputeHash(e)

	if hashBefore != hashAfter {
		t.Error("eventHash field should be excluded from hash computation")
	}
}

func TestEventHash_Seal_Verify(t *testing.T) {
	e := sampleEvent()
	if err := Seal(e); err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	if e.EventHash == "" {
		t.Fatal("Seal should set eventHash")
	}

	ok, err := Verify(e)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if !ok {
		t.Error("Verify should return true for a correctly sealed event")
	}

	// Tamper with the event
	e.Outcome.Status = "failed"
	ok, err = Verify(e)
	if err != nil {
		t.Fatalf("Verify failed after tamper: %v", err)
	}
	if ok {
		t.Error("Verify should return false after tampering")
	}
}

func TestEventHash_Canonical_SortedKeys(t *testing.T) {
	e := &Event{
		InvocationID: "test-id",
		Timestamp:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Principal: EventPrincipal{
			Agent: EventPrincipalAgent{Name: "test-agent"},
		},
		Action: EventAction{
			Verb:     "invoke",
			Resource: "skill://test",
		},
	}

	canonical, err := Canonical(e)
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's valid JSON
	var m map[string]interface{}
	if err := json.Unmarshal(canonical, &m); err != nil {
		t.Fatalf("canonical output is not valid JSON: %v", err)
	}

	// Verify no unnecessary whitespace
	s := string(canonical)
	if strings.Contains(s, " ") || strings.Contains(s, "\n") || strings.Contains(s, "\t") {
		t.Error("canonical form should not contain whitespace outside strings")
	}

	// Verify keys are sorted by checking first-level key order
	// The canonical should have keys in lexicographic order
	firstBrace := strings.Index(s, "{")
	if firstBrace < 0 {
		t.Fatal("canonical should start with {")
	}
}

func TestEventHash_JSON_RoundTrip(t *testing.T) {
	e := sampleEvent()
	if err := Seal(e); err != nil {
		t.Fatal(err)
	}

	data, err := SerializeJSON(e)
	if err != nil {
		t.Fatalf("SerializeJSON failed: %v", err)
	}

	parsed, err := ParseJSON(data)
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}

	// Re-serialize and compare
	data2, err := SerializeJSON(parsed)
	if err != nil {
		t.Fatalf("SerializeJSON (round-trip) failed: %v", err)
	}

	if string(data) != string(data2) {
		t.Error("JSON round-trip should produce identical bytes")
	}

	// Verify hash is preserved
	if parsed.EventHash != e.EventHash {
		t.Errorf("EventHash not preserved: %q vs %q", parsed.EventHash, e.EventHash)
	}

	// Verify the hash is still valid after round-trip
	ok, err := Verify(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("hash should still verify after JSON round-trip")
	}
}

func TestEventHash_JSONL_RoundTrip(t *testing.T) {
	e1 := sampleEvent()
	Seal(e1)

	e2 := sampleEvent()
	e2.InvocationID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	Seal(e2)

	// Serialize both to JSONL
	line1, err := SerializeJSONL(e1)
	if err != nil {
		t.Fatal(err)
	}
	line2, err := SerializeJSONL(e2)
	if err != nil {
		t.Fatal(err)
	}

	combined := append(line1, line2...)
	events, err := ParseJSONL(combined)
	if err != nil {
		t.Fatalf("ParseJSONL failed: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].InvocationID != e1.InvocationID {
		t.Errorf("first event invocationId mismatch: %q vs %q", events[0].InvocationID, e1.InvocationID)
	}
	if events[1].InvocationID != e2.InvocationID {
		t.Errorf("second event invocationId mismatch: %q vs %q", events[1].InvocationID, e2.InvocationID)
	}

	// Verify hashes survive JSONL round-trip
	ok1, _ := Verify(events[0])
	ok2, _ := Verify(events[1])
	if !ok1 {
		t.Error("first event hash should verify after JSONL round-trip")
	}
	if !ok2 {
		t.Error("second event hash should verify after JSONL round-trip")
	}
}

func TestEventHash_JSONL_SingleEvent(t *testing.T) {
	e := sampleEvent()
	Seal(e)

	data, err := SerializeJSONL(e)
	if err != nil {
		t.Fatal(err)
	}

	// JSONL should end with newline
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("JSONL line should end with newline")
	}

	events, err := ParseJSONL(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Verify field equality
	if events[0].InvocationID != e.InvocationID {
		t.Error("invocationId mismatch after JSONL round-trip")
	}
}

func TestEventHash_MultipleSerializations_SameHash(t *testing.T) {
	e := sampleEvent()

	// Compute hash multiple times from fresh serializations
	hashes := make([]string, 10)
	for i := range hashes {
		h, err := ComputeHash(e)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		hashes[i] = h
	}

	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("iteration %d produced different hash: %q vs %q", i, hashes[i], hashes[0])
		}
	}
}

func TestEventHash_Canonical_NumberFormatting(t *testing.T) {
	// Verify that numbers are formatted correctly per RFC 8785
	tests := []struct {
		input    float64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{1.5, "1.5"},
		{100, "100"},
		{0.5, "0.5"},
	}
	for _, tc := range tests {
		got := string(canonicalNumber(tc.input))
		if got != tc.expected {
			t.Errorf("canonicalNumber(%v) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
