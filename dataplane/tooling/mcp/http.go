package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// HTTPTransport communicates with an MCP server via HTTP POST (JSON-RPC 2.0).
type HTTPTransport struct {
	// URL is the MCP server endpoint.
	URL string
	// Client is the HTTP client used for requests. If nil, http.DefaultClient is used.
	Client *http.Client
	// Headers are optional additional headers sent with each request.
	Headers map[string]string

	mu sync.Mutex
}

// HTTPConfig holds configuration for the HTTP transport.
type HTTPConfig struct {
	// URL is the MCP server endpoint (e.g., "http://localhost:8080/rpc").
	URL string
	// Client is an optional HTTP client. If nil, http.DefaultClient is used.
	Client *http.Client
	// Headers are optional additional headers.
	Headers map[string]string
}

// NewHTTPTransport creates an HTTP transport for MCP communication.
func NewHTTPTransport(cfg HTTPConfig) *HTTPTransport {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTransport{
		URL:     cfg.URL,
		Client:  client,
		Headers: cfg.Headers,
	}
}

// Send sends a JSON-RPC 2.0 request via HTTP POST and returns the response.
func (t *HTTPTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("mcp: create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.Headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := t.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp: http request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("mcp: http status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("mcp: read response body: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal response: %w", err)
	}

	return &resp, nil
}

// Close is a no-op for HTTP transport (no persistent connection to close).
func (t *HTTPTransport) Close() error {
	return nil
}
