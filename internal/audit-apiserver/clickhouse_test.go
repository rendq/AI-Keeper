package auditapiserver

import (
	"testing"
)

func TestBuildGet(t *testing.T) {
	qb := NewQueryBuilder()
	query, args := qb.BuildGet(GetOptions{
		Name:      "evt-abc123",
		Namespace: "legal-copilot",
	})

	expectedQuery := "SELECT name, namespace, tenant_id, invocation_id, occurred_at, agent_name, decision, outcome_status, event_hash, spec_json, labels FROM audit_events WHERE name = ? AND namespace = ? LIMIT 1"
	if query != expectedQuery {
		t.Errorf("unexpected query:\n got:  %s\n want: %s", query, expectedQuery)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "evt-abc123" {
		t.Errorf("args[0] = %v, want evt-abc123", args[0])
	}
	if args[1] != "legal-copilot" {
		t.Errorf("args[1] = %v, want legal-copilot", args[1])
	}
}

func TestBuildList_NoFilters(t *testing.T) {
	qb := NewQueryBuilder()
	query, args, err := qb.BuildList(ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "SELECT name, namespace, tenant_id, invocation_id, occurred_at, agent_name, decision, outcome_status, event_hash, spec_json, labels FROM audit_events  ORDER BY occurred_at DESC LIMIT 1000 OFFSET 0"
	if query != expected {
		t.Errorf("unexpected query:\n got:  %s\n want: %s", query, expected)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}

func TestBuildList_WithNamespace(t *testing.T) {
	qb := NewQueryBuilder()
	query, args, err := qb.BuildList(ListOptions{
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "SELECT name, namespace, tenant_id, invocation_id, occurred_at, agent_name, decision, outcome_status, event_hash, spec_json, labels FROM audit_events WHERE namespace = ? ORDER BY occurred_at DESC LIMIT 1000 OFFSET 0"
	if query != expected {
		t.Errorf("unexpected query:\n got:  %s\n want: %s", query, expected)
	}
	if len(args) != 1 || args[0] != "default" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestBuildList_WithLabelSelector(t *testing.T) {
	qb := NewQueryBuilder()
	query, args, err := qb.BuildList(ListOptions{
		LabelSelector: map[string]string{
			"app": "legal-copilot",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(args) != 1 || args[0] != "legal-copilot" {
		t.Errorf("unexpected args: %v", args)
	}
	// Should contain the labels map access.
	if query == "" {
		t.Error("query should not be empty")
	}
	t.Logf("query: %s", query)
}

func TestBuildList_WithFieldSelector(t *testing.T) {
	qb := NewQueryBuilder()
	query, args, err := qb.BuildList(ListOptions{
		FieldSelector: map[string]string{
			"spec.policy.decision": "deny",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(args) != 1 || args[0] != "deny" {
		t.Errorf("unexpected args: %v", args)
	}
	// Should contain 'decision = ?'.
	if query == "" {
		t.Error("query should not be empty")
	}
	t.Logf("query: %s", query)
}

func TestBuildList_UnsupportedFieldSelector(t *testing.T) {
	qb := NewQueryBuilder()
	_, _, err := qb.BuildList(ListOptions{
		FieldSelector: map[string]string{
			"spec.nonexistent.field": "value",
		},
	})
	if err == nil {
		t.Error("expected error for unsupported field selector")
	}
}

func TestBuildList_WithLimit(t *testing.T) {
	qb := NewQueryBuilder()
	query, _, err := qb.BuildList(ListOptions{
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "SELECT name, namespace, tenant_id, invocation_id, occurred_at, agent_name, decision, outcome_status, event_hash, spec_json, labels FROM audit_events  ORDER BY occurred_at DESC LIMIT 50 OFFSET 0"
	if query != expected {
		t.Errorf("unexpected query:\n got:  %s\n want: %s", query, expected)
	}
}

func TestBuildList_WithContinueToken(t *testing.T) {
	qb := NewQueryBuilder()
	token := encodeContinueToken(100)
	query, _, err := qb.BuildList(ListOptions{
		Limit:    50,
		Continue: token,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "SELECT name, namespace, tenant_id, invocation_id, occurred_at, agent_name, decision, outcome_status, event_hash, spec_json, labels FROM audit_events  ORDER BY occurred_at DESC LIMIT 50 OFFSET 100"
	if query != expected {
		t.Errorf("unexpected query:\n got:  %s\n want: %s", query, expected)
	}
}

func TestBuildList_InvalidContinueToken(t *testing.T) {
	qb := NewQueryBuilder()
	_, _, err := qb.BuildList(ListOptions{
		Continue: "not-valid-base64!!!",
	})
	if err == nil {
		t.Error("expected error for invalid continue token")
	}
}

func TestBuildList_InvalidLabelKey(t *testing.T) {
	qb := NewQueryBuilder()
	_, _, err := qb.BuildList(ListOptions{
		LabelSelector: map[string]string{
			"key'inject": "value",
		},
	})
	if err == nil {
		t.Error("expected error for label key with single quote")
	}
}

func TestBuildList_CombinedFilters(t *testing.T) {
	qb := NewQueryBuilder()
	query, args, err := qb.BuildList(ListOptions{
		Namespace: "prod",
		LabelSelector: map[string]string{
			"team": "legal",
		},
		FieldSelector: map[string]string{
			"spec.policy.decision": "allow",
		},
		Limit: 25,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 args: namespace, label value, field value.
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d: %v", len(args), args)
	}
	t.Logf("query: %s", query)
	t.Logf("args: %v", args)
}

func TestValidateLabelKey(t *testing.T) {
	tests := []struct {
		key     string
		wantErr bool
	}{
		{"app", false},
		{"app.kubernetes.io/name", false},
		{"ai-keeper.io/system", false},
		{"", true},
		{"key'bad", true},
		{"key\"bad", true},
		{"key;bad", true},
		{"key\\bad", true},
		{"key\nbad", true},
	}
	for _, tt := range tests {
		err := validateLabelKey(tt.key)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateLabelKey(%q): got err=%v, wantErr=%v", tt.key, err, tt.wantErr)
		}
	}
}
