package sdk

import (
	"context"
	"testing"
)

func TestNewSDKAdapter(t *testing.T) {
	adapter := NewSDKAdapter()
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.ActiveStreams() != 0 {
		t.Fatalf("expected 0 active streams, got %d", adapter.ActiveStreams())
	}
}

func TestStreamInvoke_Valid(t *testing.T) {
	adapter := NewSDKAdapter()

	req := &StreamRequest{
		RequestID: "r1",
		TenantID:  "t1",
		AgentID:   "a1",
		Input:     "hello",
	}

	var got *StreamResponse
	err := adapter.StreamInvoke(context.Background(), req, func(resp *StreamResponse) error {
		got = resp
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected response from send callback")
	}
	if got.RequestID != "r1" {
		t.Fatalf("expected request_id r1, got %s", got.RequestID)
	}
	if !got.Done {
		t.Fatal("expected done=true")
	}
}

func TestStreamInvoke_NilRequest(t *testing.T) {
	adapter := NewSDKAdapter()

	err := adapter.StreamInvoke(context.Background(), nil, func(resp *StreamResponse) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestStreamInvoke_MissingTenantID(t *testing.T) {
	adapter := NewSDKAdapter()

	req := &StreamRequest{AgentID: "a1", Input: "hi"}
	err := adapter.StreamInvoke(context.Background(), req, func(resp *StreamResponse) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for missing tenant_id")
	}
}

func TestStreamInvoke_MissingAgentID(t *testing.T) {
	adapter := NewSDKAdapter()

	req := &StreamRequest{TenantID: "t1", Input: "hi"}
	err := adapter.StreamInvoke(context.Background(), req, func(resp *StreamResponse) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for missing agent_id")
	}
}

func TestStreamInvoke_MissingInput(t *testing.T) {
	adapter := NewSDKAdapter()

	req := &StreamRequest{TenantID: "t1", AgentID: "a1"}
	err := adapter.StreamInvoke(context.Background(), req, func(resp *StreamResponse) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestStreamInvoke_CancelledContext(t *testing.T) {
	adapter := NewSDKAdapter()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &StreamRequest{
		TenantID: "t1",
		AgentID:  "a1",
		Input:    "hello",
	}

	err := adapter.StreamInvoke(ctx, req, func(resp *StreamResponse) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
