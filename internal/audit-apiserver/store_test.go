package auditapiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	auditv1alpha1 "github.com/ai-keeper/ai-keeper/api/audit/v1alpha1"
)

// mockClickHouseClient is a test double for ClickHouseClient.
type mockClickHouseClient struct {
	rows []AuditEventRow
	err  error
	// lastQuery and lastArgs capture the most recent call for assertion.
	lastQuery string
	lastArgs  []interface{}
}

func (m *mockClickHouseClient) QueryAuditEvents(_ context.Context, query string, args []interface{}) ([]AuditEventRow, error) {
	m.lastQuery = query
	m.lastArgs = args
	return m.rows, m.err
}

func makeSpecJSON(invocationID, decision, agentName string) string {
	spec := auditv1alpha1.AuditEventSpec{
		InvocationID: invocationID,
		Principal: auditv1alpha1.AuditPrincipal{
			Agent: auditv1alpha1.AuditPrincipalAgent{
				Name: agentName,
			},
		},
		Policy: &auditv1alpha1.AuditPolicy{
			Decision: decision,
		},
	}
	b, _ := json.Marshal(spec)
	return string(b)
}

func TestStoreGet_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mock := &mockClickHouseClient{
		rows: []AuditEventRow{
			{
				Name:         "evt-001",
				Namespace:    "legal",
				TenantID:     "tenant-a",
				InvocationID: "inv-001",
				OccurredAt:   now,
				AgentName:    "legal-copilot",
				Decision:     "allow",
				OutcomeStatus: "success",
				EventHash:    "sha256:abc123",
				SpecJSON:     makeSpecJSON("inv-001", "allow", "legal-copilot"),
				Labels:       map[string]string{"app": "legal"},
			},
		},
	}

	store := NewStore(mock)
	ae, err := store.Get(context.Background(), GetOptions{
		Name:      "evt-001",
		Namespace: "legal",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ae.Name != "evt-001" {
		t.Errorf("Name = %q, want evt-001", ae.Name)
	}
	if ae.Namespace != "legal" {
		t.Errorf("Namespace = %q, want legal", ae.Namespace)
	}
	if ae.Spec.InvocationID != "inv-001" {
		t.Errorf("InvocationID = %q, want inv-001", ae.Spec.InvocationID)
	}
	if ae.Spec.Principal.Agent.Name != "legal-copilot" {
		t.Errorf("AgentName = %q, want legal-copilot", ae.Spec.Principal.Agent.Name)
	}
}

func TestStoreGet_NotFound(t *testing.T) {
	mock := &mockClickHouseClient{rows: nil}
	store := NewStore(mock)
	_, err := store.Get(context.Background(), GetOptions{
		Name:      "nonexistent",
		Namespace: "default",
	})
	if err == nil {
		t.Error("expected not found error")
	}
}

func TestStoreGet_QueryError(t *testing.T) {
	mock := &mockClickHouseClient{err: fmt.Errorf("connection refused")}
	store := NewStore(mock)
	_, err := store.Get(context.Background(), GetOptions{
		Name:      "evt-001",
		Namespace: "default",
	})
	if err == nil {
		t.Error("expected error on query failure")
	}
}

func TestStoreList_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mock := &mockClickHouseClient{
		rows: []AuditEventRow{
			{
				Name:         "evt-001",
				Namespace:    "legal",
				InvocationID: "inv-001",
				OccurredAt:   now,
				AgentName:    "legal-copilot",
				Decision:     "allow",
				SpecJSON:     makeSpecJSON("inv-001", "allow", "legal-copilot"),
			},
			{
				Name:         "evt-002",
				Namespace:    "legal",
				InvocationID: "inv-002",
				OccurredAt:   now.Add(-time.Minute),
				AgentName:    "legal-copilot",
				Decision:     "deny",
				SpecJSON:     makeSpecJSON("inv-002", "deny", "legal-copilot"),
			},
		},
	}

	store := NewStore(mock)
	result, err := store.List(context.Background(), ListOptions{
		Namespace: "legal",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Name != "evt-001" {
		t.Errorf("Items[0].Name = %q, want evt-001", result.Items[0].Name)
	}
	if result.Items[1].Spec.Policy.Decision != "deny" {
		t.Errorf("Items[1].Decision = %q, want deny", result.Items[1].Spec.Policy.Decision)
	}
}

func TestStoreList_Pagination(t *testing.T) {
	// Simulate a full page (limit=2) which should produce a continue token.
	mock := &mockClickHouseClient{
		rows: []AuditEventRow{
			{Name: "evt-001", Namespace: "ns", SpecJSON: makeSpecJSON("i1", "allow", "a")},
			{Name: "evt-002", Namespace: "ns", SpecJSON: makeSpecJSON("i2", "allow", "a")},
		},
	}

	store := NewStore(mock)
	result, err := store.List(context.Background(), ListOptions{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContinueToken == "" {
		t.Error("expected a continue token for a full page")
	}

	// Decode the token and verify it's offset=2.
	offset, err := decodeContinueToken(result.ContinueToken)
	if err != nil {
		t.Fatalf("failed to decode continue token: %v", err)
	}
	if offset != 2 {
		t.Errorf("offset = %d, want 2", offset)
	}
}

func TestStoreList_NoContinueForPartialPage(t *testing.T) {
	mock := &mockClickHouseClient{
		rows: []AuditEventRow{
			{Name: "evt-001", Namespace: "ns", SpecJSON: makeSpecJSON("i1", "allow", "a")},
		},
	}

	store := NewStore(mock)
	result, err := store.List(context.Background(), ListOptions{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContinueToken != "" {
		t.Error("should not have continue token for partial page")
	}
}

func TestStoreList_QueryBuildError(t *testing.T) {
	mock := &mockClickHouseClient{}
	store := NewStore(mock)
	_, err := store.List(context.Background(), ListOptions{
		FieldSelector: map[string]string{"bad.field": "x"},
	})
	if err == nil {
		t.Error("expected error for unsupported field selector")
	}
}
