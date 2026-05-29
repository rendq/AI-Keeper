package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// ToolSchema describes an MCP tool's input schema.
type ToolSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// ToolInfo describes an available MCP tool.
type ToolInfo struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	InputSchema ToolSchema `json:"inputSchema"`
}

// ToolResult represents the result of a tool invocation.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents a content block in a tool result.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Client provides methods to interact with an MCP server.
type Client interface {
	// ListTools discovers available tools from the MCP server.
	ListTools(ctx context.Context) ([]ToolInfo, error)

	// InvokeTool calls a tool by name with the given arguments.
	InvokeTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error)

	// Close releases resources held by the client.
	Close() error
}

// client is the default MCP client implementation.
type client struct {
	transport Transport
	idCounter atomic.Int64
}

// NewClient creates an MCP client with the given transport.
func NewClient(transport Transport) Client {
	return &client{transport: transport}
}

func (c *client) nextID() int64 {
	return c.idCounter.Add(1)
}

// ListTools calls "tools/list" on the MCP server and returns discovered tools.
func (c *client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	req := NewRequest(c.nextID(), "tools/list", struct{}{})

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mcp: list tools: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcp: list tools: %w", resp.Error)
	}

	var result struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal tools list: %w", err)
	}

	return result.Tools, nil
}

// InvokeTool calls "tools/call" on the MCP server with the given tool name and arguments.
func (c *client) InvokeTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}
	req := NewRequest(c.nextID(), "tools/call", params)

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mcp: invoke tool %q: %w", name, err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcp: invoke tool %q: %w", name, resp.Error)
	}

	var result ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal tool result: %w", err)
	}

	return &result, nil
}

// Close releases the underlying transport.
func (c *client) Close() error {
	return c.transport.Close()
}
