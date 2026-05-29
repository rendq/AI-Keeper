package api

import (
	"testing"
)

func TestValidateAPIKey_Valid(t *testing.T) {
	adapter := NewAPIAdapter(WithAPIKeys(map[string]string{
		"key-abc-123": "tenant-1",
	}))

	tid, err := adapter.ValidateAPIKey("key-abc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tid != "tenant-1" {
		t.Fatalf("expected tenant-1, got %s", tid)
	}
}

func TestValidateAPIKey_BearerPrefix(t *testing.T) {
	adapter := NewAPIAdapter(WithAPIKeys(map[string]string{
		"key-abc-123": "tenant-1",
	}))

	tid, err := adapter.ValidateAPIKey("Bearer key-abc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tid != "tenant-1" {
		t.Fatalf("expected tenant-1, got %s", tid)
	}
}

func TestValidateAPIKey_Empty(t *testing.T) {
	adapter := NewAPIAdapter()

	_, err := adapter.ValidateAPIKey("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestValidateAPIKey_Invalid(t *testing.T) {
	adapter := NewAPIAdapter(WithAPIKeys(map[string]string{
		"key-abc-123": "tenant-1",
	}))

	_, err := adapter.ValidateAPIKey("wrong-key")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestHandleInvoke_Valid(t *testing.T) {
	body := []byte(`{"agent_id":"agent-1","input":"hello world"}`)
	req, err := HandleInvoke(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.AgentID != "agent-1" {
		t.Fatalf("expected agent-1, got %s", req.AgentID)
	}
	if req.Input != "hello world" {
		t.Fatalf("expected 'hello world', got %s", req.Input)
	}
}

func TestHandleInvoke_WithParameters(t *testing.T) {
	body := []byte(`{"agent_id":"a1","input":"hi","parameters":{"temp":0.7}}`)
	req, err := HandleInvoke(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Parameters == nil {
		t.Fatal("expected parameters to be set")
	}
}

func TestHandleInvoke_EmptyBody(t *testing.T) {
	_, err := HandleInvoke([]byte{})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestHandleInvoke_MissingAgentID(t *testing.T) {
	body := []byte(`{"input":"hello"}`)
	_, err := HandleInvoke(body)
	if err == nil {
		t.Fatal("expected error for missing agent_id")
	}
}

func TestHandleInvoke_MissingInput(t *testing.T) {
	body := []byte(`{"agent_id":"a1"}`)
	_, err := HandleInvoke(body)
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestHandleInvoke_InvalidJSON(t *testing.T) {
	_, err := HandleInvoke([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
