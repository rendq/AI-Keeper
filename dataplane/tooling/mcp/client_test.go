package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockTransport implements Transport for testing.
type mockTransport struct {
	handler func(req *Request) (*Response, error)
	closed  bool
}

func (m *mockTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	if m.handler != nil {
		return m.handler(req)
	}
	return nil, fmt.Errorf("no handler configured")
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}

func TestClient_ListTools(t *testing.T) {
	tools := []ToolInfo{
		{
			Name:        "search",
			Description: "Search the web",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "calculate",
			Description: "Perform arithmetic",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"expression": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	mt := &mockTransport{
		handler: func(req *Request) (*Response, error) {
			if req.Method != "tools/list" {
				return nil, fmt.Errorf("unexpected method: %s", req.Method)
			}
			if req.JSONRPC != "2.0" {
				return nil, fmt.Errorf("unexpected jsonrpc version: %s", req.JSONRPC)
			}

			result := struct {
				Tools []ToolInfo `json:"tools"`
			}{Tools: tools}
			data, _ := json.Marshal(result)
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  data,
			}, nil
		},
	}

	c := NewClient(mt)
	defer c.Close()

	got, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("ListTools() returned %d tools, want 2", len(got))
	}
	if got[0].Name != "search" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "search")
	}
	if got[1].Name != "calculate" {
		t.Errorf("got[1].Name = %q, want %q", got[1].Name, "calculate")
	}
}

func TestClient_ListTools_RPCError(t *testing.T) {
	mt := &mockTransport{
		handler: func(req *Request) (*Response, error) {
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &RPCError{
					Code:    -32601,
					Message: "method not found",
				},
			}, nil
		},
	}

	c := NewClient(mt)
	defer c.Close()

	_, err := c.ListTools(context.Background())
	if err == nil {
		t.Fatal("ListTools() expected error, got nil")
	}
}

func TestClient_InvokeTool(t *testing.T) {
	mt := &mockTransport{
		handler: func(req *Request) (*Response, error) {
			if req.Method != "tools/call" {
				return nil, fmt.Errorf("unexpected method: %s", req.Method)
			}

			// Verify params contain name and arguments
			params, _ := json.Marshal(req.Params)
			var p map[string]interface{}
			json.Unmarshal(params, &p)

			if p["name"] != "search" {
				return nil, fmt.Errorf("unexpected tool name: %v", p["name"])
			}

			result := ToolResult{
				Content: []ContentBlock{
					{Type: "text", Text: "Result for: hello world"},
				},
			}
			data, _ := json.Marshal(result)
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  data,
			}, nil
		},
	}

	c := NewClient(mt)
	defer c.Close()

	result, err := c.InvokeTool(context.Background(), "search", map[string]interface{}{
		"query": "hello world",
	})
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("InvokeTool() returned %d content blocks, want 1", len(result.Content))
	}
	if result.Content[0].Text != "Result for: hello world" {
		t.Errorf("content text = %q, want %q", result.Content[0].Text, "Result for: hello world")
	}
	if result.IsError {
		t.Error("InvokeTool() result.IsError = true, want false")
	}
}

func TestClient_InvokeTool_Error(t *testing.T) {
	mt := &mockTransport{
		handler: func(req *Request) (*Response, error) {
			result := ToolResult{
				Content: []ContentBlock{
					{Type: "text", Text: "tool execution failed: connection refused"},
				},
				IsError: true,
			}
			data, _ := json.Marshal(result)
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  data,
			}, nil
		},
	}

	c := NewClient(mt)
	defer c.Close()

	result, err := c.InvokeTool(context.Background(), "broken-tool", nil)
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if !result.IsError {
		t.Error("InvokeTool() result.IsError = false, want true")
	}
}

func TestClient_InvokeTool_TransportError(t *testing.T) {
	mt := &mockTransport{
		handler: func(req *Request) (*Response, error) {
			return nil, fmt.Errorf("connection lost")
		},
	}

	c := NewClient(mt)
	defer c.Close()

	_, err := c.InvokeTool(context.Background(), "any", nil)
	if err == nil {
		t.Fatal("InvokeTool() expected error, got nil")
	}
}

func TestClient_Close(t *testing.T) {
	mt := &mockTransport{}
	c := NewClient(mt)
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !mt.closed {
		t.Error("Close() did not close transport")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	mt := &mockTransport{
		handler: func(req *Request) (*Response, error) {
			return nil, context.Canceled
		},
	}

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.ListTools(ctx)
	if err == nil {
		t.Fatal("ListTools() expected error with cancelled context")
	}
}

func TestHTTPTransport_Integration(t *testing.T) {
	// Create a test HTTP server that simulates an MCP server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}

		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var resp Response
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "tools/list":
			result := struct {
				Tools []ToolInfo `json:"tools"`
			}{
				Tools: []ToolInfo{
					{
						Name:        "echo",
						Description: "Echo input back",
						InputSchema: ToolSchema{Type: "object"},
					},
				},
			}
			resp.Result, _ = json.Marshal(result)
		case "tools/call":
			result := ToolResult{
				Content: []ContentBlock{{Type: "text", Text: "echoed"}},
			}
			resp.Result, _ = json.Marshal(result)
		default:
			resp.Error = &RPCError{Code: -32601, Message: "method not found"}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(HTTPConfig{URL: server.URL})
	c := NewClient(transport)
	defer c.Close()

	// Test ListTools
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Errorf("ListTools() = %v, want 1 tool named 'echo'", tools)
	}

	// Test InvokeTool
	result, err := c.InvokeTool(context.Background(), "echo", map[string]interface{}{"msg": "hi"})
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "echoed" {
		t.Errorf("InvokeTool() = %v, want 'echoed'", result)
	}
}

func TestHTTPTransport_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	transport := NewHTTPTransport(HTTPConfig{URL: server.URL})
	c := NewClient(transport)
	defer c.Close()

	_, err := c.ListTools(context.Background())
	if err == nil {
		t.Fatal("ListTools() expected error for 500 response")
	}
}

func TestHTTPTransport_CustomHeaders(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		resp := Response{JSONRPC: "2.0", ID: 1}
		result := struct {
			Tools []ToolInfo `json:"tools"`
		}{Tools: []ToolInfo{}}
		resp.Result, _ = json.Marshal(result)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(HTTPConfig{
		URL:     server.URL,
		Headers: map[string]string{"Authorization": "Bearer test-token"},
	})
	c := NewClient(transport)
	defer c.Close()

	_, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if receivedAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", receivedAuth, "Bearer test-token")
	}
}

func TestNewRequest(t *testing.T) {
	req := NewRequest(42, "tools/list", struct{}{})
	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", req.JSONRPC, "2.0")
	}
	if req.ID != 42 {
		t.Errorf("ID = %d, want 42", req.ID)
	}
	if req.Method != "tools/list" {
		t.Errorf("Method = %q, want %q", req.Method, "tools/list")
	}
}

func TestRPCError_Error(t *testing.T) {
	e := &RPCError{Code: -32600, Message: "invalid request"}
	if e.Error() != "invalid request" {
		t.Errorf("Error() = %q, want %q", e.Error(), "invalid request")
	}
}
