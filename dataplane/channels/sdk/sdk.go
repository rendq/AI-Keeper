// Package sdk implements the gRPC SDK channel adapter for AIP Gateway.
// It provides a bidirectional streaming interface for embedded SDK usage.
package sdk

import (
	"context"
	"fmt"
	"sync/atomic"
)

// SDKAdapter handles gRPC bidirectional streaming for embedded SDK clients.
type SDKAdapter struct {
	// active tracks the number of active streaming sessions.
	active int64
}

// StreamRequest represents a single message in a bidirectional stream.
type StreamRequest struct {
	// RequestID is a unique identifier for this stream message.
	RequestID string
	// TenantID identifies the tenant.
	TenantID string
	// AgentID is the target agent.
	AgentID string
	// Input is the user message.
	Input string
	// SessionID is an optional session identifier.
	SessionID string
}

// StreamResponse represents a single response message in a bidirectional stream.
type StreamResponse struct {
	// RequestID echoes the request identifier.
	RequestID string
	// Output is a chunk of the agent's response.
	Output string
	// Done indicates if this is the final chunk.
	Done bool
	// Error contains error details if the request failed.
	Error string
}

// NewSDKAdapter creates a new gRPC SDK channel adapter.
func NewSDKAdapter() *SDKAdapter {
	return &SDKAdapter{}
}

// StreamInvoke processes a bidirectional streaming invocation.
// In a full implementation, this would handle gRPC stream send/recv loops.
// For now, it validates the request and returns a single response via the callback.
func (a *SDKAdapter) StreamInvoke(ctx context.Context, req *StreamRequest, send func(*StreamResponse) error) error {
	if req == nil {
		return fmt.Errorf("stream request must not be nil")
	}
	if req.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if req.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if req.Input == "" {
		return fmt.Errorf("input is required")
	}

	atomic.AddInt64(&a.active, 1)
	defer atomic.AddInt64(&a.active, -1)

	// Check context cancellation.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// In a full implementation, this would route to the agent runtime
	// and stream responses back. For now, send an acknowledgment.
	resp := &StreamResponse{
		RequestID: req.RequestID,
		Output:    "stream accepted",
		Done:      true,
	}

	return send(resp)
}

// ActiveStreams returns the number of active streaming sessions.
func (a *SDKAdapter) ActiveStreams() int64 {
	return atomic.LoadInt64(&a.active)
}
