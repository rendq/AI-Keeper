package mcp

import "context"

// Transport defines the interface for MCP communication transports.
// Implementations handle sending JSON-RPC 2.0 requests and receiving responses.
type Transport interface {
	// Send sends a JSON-RPC 2.0 request and returns the response.
	Send(ctx context.Context, req *Request) (*Response, error)

	// Close releases any resources held by the transport.
	Close() error
}
