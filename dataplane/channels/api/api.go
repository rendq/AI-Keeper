// Package api implements the REST API channel adapter for AIP Gateway.
// It provides API key authentication and standard REST invoke handling
// for programmatic access to agents.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"strings"
)

// APIAdapter handles API key authentication and REST API invocations.
type APIAdapter struct {
	// keys maps API key values to their tenant IDs.
	keys map[string]string
}

// InvokeRequest represents a programmatic agent invocation request.
type InvokeRequest struct {
	// AgentID is the target agent identifier.
	AgentID string `json:"agent_id"`
	// Input is the user prompt or message.
	Input string `json:"input"`
	// SessionID is an optional session for multi-turn conversations.
	SessionID string `json:"session_id,omitempty"`
	// Parameters contains optional invocation parameters.
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// InvokeResponse represents the response from an agent invocation.
type InvokeResponse struct {
	// Output is the agent's response.
	Output string `json:"output"`
	// SessionID is the session identifier for conversation continuity.
	SessionID string `json:"session_id,omitempty"`
	// Usage contains token/cost usage information.
	Usage *UsageInfo `json:"usage,omitempty"`
}

// UsageInfo contains token and cost usage details.
type UsageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Option configures an APIAdapter.
type Option func(*APIAdapter)

// WithAPIKeys sets the API key to tenant ID mapping.
func WithAPIKeys(keys map[string]string) Option {
	return func(a *APIAdapter) {
		a.keys = keys
	}
}

// NewAPIAdapter creates a new REST API channel adapter.
func NewAPIAdapter(opts ...Option) *APIAdapter {
	a := &APIAdapter{
		keys: make(map[string]string),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// ValidateAPIKey validates an API key and returns the associated tenant ID.
// The key is expected in the format "Bearer <key>" or just the raw key value.
func (a *APIAdapter) ValidateAPIKey(key string) (tenantID string, err error) {
	if key == "" {
		return "", fmt.Errorf("API key is required")
	}

	// Strip "Bearer " prefix if present.
	rawKey := strings.TrimPrefix(key, "Bearer ")
	rawKey = strings.TrimSpace(rawKey)

	if rawKey == "" {
		return "", fmt.Errorf("API key is required")
	}

	// Use constant-time comparison for each registered key.
	for registeredKey, tid := range a.keys {
		if subtle.ConstantTimeCompare([]byte(rawKey), []byte(registeredKey)) == 1 {
			return tid, nil
		}
	}

	return "", fmt.Errorf("invalid API key")
}

// HandleInvoke parses a raw JSON body into an InvokeRequest and validates it.
func HandleInvoke(body []byte) (*InvokeRequest, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}

	var req InvokeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse invoke request: %w", err)
	}

	if err := validateInvokeRequest(&req); err != nil {
		return nil, err
	}

	return &req, nil
}

// validateInvokeRequest checks that required fields are present.
func validateInvokeRequest(req *InvokeRequest) error {
	if req.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if req.Input == "" {
		return fmt.Errorf("input is required")
	}
	return nil
}
