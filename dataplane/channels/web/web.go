// Package web implements the Web channel adapter for AIP Gateway.
// It provides HTTP REST request-response and WebSocket streaming
// for web UI integrations.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// WebAdapter handles HTTP REST requests and WebSocket streaming sessions.
type WebAdapter struct {
	// mu protects active WebSocket connections.
	mu          sync.RWMutex
	connections map[string]*WSConn
}

// WebRequest represents an inbound HTTP request from a web UI.
type WebRequest struct {
	// RequestID is a unique identifier for the request.
	RequestID string `json:"request_id"`
	// TenantID identifies the tenant making the request.
	TenantID string `json:"tenant_id"`
	// AgentID identifies the target agent.
	AgentID string `json:"agent_id"`
	// Input is the user message or prompt.
	Input string `json:"input"`
	// SessionID is an optional session identifier for conversation continuity.
	SessionID string `json:"session_id,omitempty"`
	// Stream indicates whether the client wants a streaming response.
	Stream bool `json:"stream,omitempty"`
}

// WebResponse represents the response sent back to the web client.
type WebResponse struct {
	// RequestID echoes the request identifier.
	RequestID string `json:"request_id"`
	// Output is the agent's response text.
	Output string `json:"output"`
	// SessionID is the session identifier for conversation continuity.
	SessionID string `json:"session_id,omitempty"`
	// Done indicates if this is the final response chunk (for streaming).
	Done bool `json:"done"`
	// Error contains error details if the request failed.
	Error string `json:"error,omitempty"`
}

// WSConn represents an active WebSocket connection.
type WSConn struct {
	// ID is the connection identifier.
	ID string
	// TenantID identifies the tenant.
	TenantID string
	// ConnectedAt records when the connection was established.
	ConnectedAt time.Time
}

// NewWebAdapter creates a new Web channel adapter.
func NewWebAdapter() *WebAdapter {
	return &WebAdapter{
		connections: make(map[string]*WSConn),
	}
}

// HandleHTTPRequest handles a synchronous HTTP request-response invocation.
// It parses the JSON body into a WebRequest and validates required fields.
func (a *WebAdapter) HandleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WebRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, "", fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := validateWebRequest(&req); err != nil {
		writeErrorResponse(w, req.RequestID, err.Error(), http.StatusBadRequest)
		return
	}

	// In a full implementation, this would route to the agent runtime.
	// For now, return an acknowledgment response.
	resp := WebResponse{
		RequestID: req.RequestID,
		Output:    "request accepted",
		SessionID: req.SessionID,
		Done:      true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleWebSocket handles a streaming WebSocket session.
// It registers the connection and returns the connection metadata.
func (a *WebAdapter) HandleWebSocket(conn *WSConn) error {
	if conn == nil {
		return fmt.Errorf("connection must not be nil")
	}
	if conn.ID == "" {
		return fmt.Errorf("connection ID must not be empty")
	}

	conn.ConnectedAt = time.Now()

	a.mu.Lock()
	a.connections[conn.ID] = conn
	a.mu.Unlock()

	return nil
}

// DisconnectWebSocket removes a WebSocket connection by ID.
func (a *WebAdapter) DisconnectWebSocket(connID string) {
	a.mu.Lock()
	delete(a.connections, connID)
	a.mu.Unlock()
}

// ActiveConnections returns the number of active WebSocket connections.
func (a *WebAdapter) ActiveConnections() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.connections)
}

// ParseRequest parses a raw JSON body into a WebRequest.
func ParseRequest(body []byte) (*WebRequest, error) {
	var req WebRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse web request: %w", err)
	}
	if err := validateWebRequest(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

// validateWebRequest checks that required fields are present.
func validateWebRequest(req *WebRequest) error {
	if req.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if req.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if req.Input == "" {
		return fmt.Errorf("input is required")
	}
	return nil
}

// writeErrorResponse writes a JSON error response.
func writeErrorResponse(w http.ResponseWriter, requestID, errMsg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := WebResponse{
		RequestID: requestID,
		Error:     errMsg,
		Done:      true,
	}
	json.NewEncoder(w).Encode(resp)
}
