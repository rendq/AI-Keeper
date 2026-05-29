// Package openapi implements an adapter that maps OpenAPI 3.0 specifications
// to tool invocations with OAuth2 OBO header injection support.
package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ToolSchema describes a tool's input schema (JSON Schema subset).
type ToolSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// ToolInfo describes an available tool discovered from an OpenAPI spec.
type ToolInfo struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	InputSchema ToolSchema `json:"inputSchema"`
}

// Parameter describes an OpenAPI operation parameter.
type Parameter struct {
	Name     string `json:"name"`
	In       string `json:"in"` // "query", "path", "header"
	Required bool   `json:"required,omitempty"`
	Schema   *Schema `json:"schema,omitempty"`
}

// Schema is a simplified JSON Schema used in OpenAPI specs.
type Schema struct {
	Type       string                 `json:"type,omitempty"`
	Properties map[string]*Schema     `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
	Items      *Schema                `json:"items,omitempty"`
	Format     string                 `json:"format,omitempty"`
	Enum       []interface{}          `json:"enum,omitempty"`
}

// RequestBody describes an OpenAPI request body.
type RequestBody struct {
	Content map[string]*MediaType `json:"content,omitempty"`
}

// MediaType describes a media type in a request body.
type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

// Operation describes an OpenAPI operation.
type Operation struct {
	OperationID string       `json:"operationId,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Parameters  []Parameter  `json:"parameters,omitempty"`
	RequestBody *RequestBody `json:"requestBody,omitempty"`
}

// PathItem describes a path in an OpenAPI spec.
type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
}

// Info describes the OpenAPI info object.
type Info struct {
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// Server describes a server in the OpenAPI spec.
type Server struct {
	URL string `json:"url"`
}

// Spec represents a simplified OpenAPI 3.0 specification.
type Spec struct {
	OpenAPI string              `json:"openapi"`
	Info    Info                `json:"info,omitempty"`
	Servers []Server            `json:"servers,omitempty"`
	Paths   map[string]*PathItem `json:"paths,omitempty"`
}

// operationEntry pairs an operation with its path and method for invocation.
type operationEntry struct {
	Path       string
	Method     string
	Operation  *Operation
}

// Adapter maps an OpenAPI 3.0 spec to tool invocations.
type Adapter struct {
	spec       *Spec
	baseURL    string
	operations map[string]*operationEntry
	httpClient *http.Client
}

// Config holds configuration for creating an Adapter.
type Config struct {
	// BaseURL overrides the server URL from the spec.
	BaseURL string
	// HTTPClient is an optional custom HTTP client. If nil, http.DefaultClient is used.
	HTTPClient *http.Client
}

// NewAdapter creates a new OpenAPI tool adapter from the given spec JSON.
func NewAdapter(specJSON []byte, cfg Config) (*Adapter, error) {
	var spec Spec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, fmt.Errorf("openapi: parse spec: %w", err)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" && len(spec.Servers) > 0 {
		baseURL = spec.Servers[0].URL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	a := &Adapter{
		spec:       &spec,
		baseURL:    baseURL,
		operations: make(map[string]*operationEntry),
		httpClient: httpClient,
	}

	// Index operations by operationId.
	for path, item := range spec.Paths {
		a.indexOp(path, "GET", item.Get)
		a.indexOp(path, "POST", item.Post)
		a.indexOp(path, "PUT", item.Put)
		a.indexOp(path, "DELETE", item.Delete)
		a.indexOp(path, "PATCH", item.Patch)
	}

	return a, nil
}

func (a *Adapter) indexOp(path, method string, op *Operation) {
	if op == nil || op.OperationID == "" {
		return
	}
	a.operations[op.OperationID] = &operationEntry{
		Path:      path,
		Method:    method,
		Operation: op,
	}
}

// ListTools returns all discovered tools from the OpenAPI spec.
func (a *Adapter) ListTools() []ToolInfo {
	tools := make([]ToolInfo, 0, len(a.operations))
	for _, entry := range a.operations {
		tools = append(tools, buildToolInfo(entry))
	}
	return tools
}

// InvokeTool calls the named tool with the given arguments. If oboToken is non-empty,
// it injects an Authorization: Bearer header for OAuth2 OBO.
func (a *Adapter) InvokeTool(ctx context.Context, name string, args map[string]interface{}, oboToken string) (string, error) {
	entry, ok := a.operations[name]
	if !ok {
		return "", fmt.Errorf("openapi: unknown tool %q", name)
	}

	// Build URL with path parameters substituted.
	urlPath := entry.Path
	queryParams := make(map[string]string)

	for _, p := range entry.Operation.Parameters {
		val, exists := args[p.Name]
		if !exists {
			continue
		}
		strVal := fmt.Sprintf("%v", val)
		switch p.In {
		case "path":
			urlPath = strings.ReplaceAll(urlPath, "{"+p.Name+"}", strVal)
		case "query":
			queryParams[p.Name] = strVal
		}
	}

	fullURL := a.baseURL + urlPath
	if len(queryParams) > 0 {
		parts := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			parts = append(parts, k+"="+v)
		}
		fullURL += "?" + strings.Join(parts, "&")
	}

	// Build request body for methods that support it.
	var body io.Reader
	if entry.Method == "POST" || entry.Method == "PUT" || entry.Method == "PATCH" {
		bodyArgs := a.extractBodyArgs(entry, args)
		if len(bodyArgs) > 0 {
			data, err := json.Marshal(bodyArgs)
			if err != nil {
				return "", fmt.Errorf("openapi: marshal body: %w", err)
			}
			body = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequestWithContext(ctx, entry.Method, fullURL, body)
	if err != nil {
		return "", fmt.Errorf("openapi: build request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Inject OBO Authorization header if provided.
	if oboToken != "" {
		req.Header.Set("Authorization", "Bearer "+oboToken)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openapi: execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openapi: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("openapi: tool %q returned status %d: %s", name, resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// extractBodyArgs returns args that belong to the request body (not path/query params).
func (a *Adapter) extractBodyArgs(entry *operationEntry, args map[string]interface{}) map[string]interface{} {
	paramNames := make(map[string]bool)
	for _, p := range entry.Operation.Parameters {
		paramNames[p.Name] = true
	}

	bodyArgs := make(map[string]interface{})
	for k, v := range args {
		if !paramNames[k] {
			bodyArgs[k] = v
		}
	}
	return bodyArgs
}

// buildToolInfo converts an operation entry to a ToolInfo.
func buildToolInfo(entry *operationEntry) ToolInfo {
	desc := entry.Operation.Summary
	if desc == "" {
		desc = entry.Operation.Description
	}

	properties := make(map[string]interface{})
	var required []string

	// Add parameters.
	for _, p := range entry.Operation.Parameters {
		prop := map[string]interface{}{
			"type": "string",
		}
		if p.Schema != nil && p.Schema.Type != "" {
			prop["type"] = p.Schema.Type
		}
		properties[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}

	// Add request body properties.
	if entry.Operation.RequestBody != nil {
		if ct, ok := entry.Operation.RequestBody.Content["application/json"]; ok && ct.Schema != nil {
			for name, s := range ct.Schema.Properties {
				prop := map[string]interface{}{
					"type": "string",
				}
				if s != nil && s.Type != "" {
					prop["type"] = s.Type
				}
				properties[name] = prop
			}
			required = append(required, ct.Schema.Required...)
		}
	}

	return ToolInfo{
		Name:        entry.Operation.OperationID,
		Description: desc,
		InputSchema: ToolSchema{
			Type:       "object",
			Properties: properties,
			Required:   required,
		},
	}
}
