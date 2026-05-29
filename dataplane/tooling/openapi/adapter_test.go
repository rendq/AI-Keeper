package openapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func sampleSpec(baseURL string) []byte {
	spec := map[string]interface{}{
		"openapi": "3.0.3",
		"info":    map[string]interface{}{"title": "Test API", "version": "1.0.0"},
		"servers": []map[string]interface{}{{"url": baseURL}},
		"paths": map[string]interface{}{
			"/users/{userId}": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "getUser",
					"summary":     "Get a user by ID",
					"parameters": []map[string]interface{}{
						{
							"name":     "userId",
							"in":       "path",
							"required": true,
							"schema":   map[string]interface{}{"type": "string"},
						},
					},
				},
			},
			"/users": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listUsers",
					"summary":     "List users",
					"parameters": []map[string]interface{}{
						{
							"name":   "limit",
							"in":     "query",
							"schema": map[string]interface{}{"type": "integer"},
						},
						{
							"name":   "offset",
							"in":     "query",
							"schema": map[string]interface{}{"type": "integer"},
						},
					},
				},
				"post": map[string]interface{}{
					"operationId": "createUser",
					"summary":     "Create a user",
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"name":  map[string]interface{}{"type": "string"},
										"email": map[string]interface{}{"type": "string"},
									},
									"required": []interface{}{"name", "email"},
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(spec)
	return data
}

func TestNewAdapter_ParsesSpec(t *testing.T) {
	adapter, err := NewAdapter(sampleSpec("http://localhost:8080"), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	if adapter == nil {
		t.Fatal("NewAdapter() returned nil")
	}
}

func TestNewAdapter_InvalidJSON(t *testing.T) {
	_, err := NewAdapter([]byte("not json"), Config{})
	if err == nil {
		t.Fatal("NewAdapter() expected error for invalid JSON")
	}
}

func TestNewAdapter_BaseURLOverride(t *testing.T) {
	adapter, err := NewAdapter(sampleSpec("http://original.com"), Config{
		BaseURL: "http://override.com",
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	if adapter.baseURL != "http://override.com" {
		t.Errorf("baseURL = %q, want %q", adapter.baseURL, "http://override.com")
	}
}

func TestListTools(t *testing.T) {
	adapter, err := NewAdapter(sampleSpec("http://localhost:8080"), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	tools := adapter.ListTools()
	if len(tools) != 3 {
		t.Fatalf("ListTools() returned %d tools, want 3", len(tools))
	}

	// Sort for deterministic testing.
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	// createUser
	if tools[0].Name != "createUser" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "createUser")
	}
	if tools[0].InputSchema.Type != "object" {
		t.Errorf("createUser InputSchema.Type = %q, want %q", tools[0].InputSchema.Type, "object")
	}
	if _, ok := tools[0].InputSchema.Properties["name"]; !ok {
		t.Error("createUser missing 'name' property")
	}
	if _, ok := tools[0].InputSchema.Properties["email"]; !ok {
		t.Error("createUser missing 'email' property")
	}

	// getUser
	if tools[1].Name != "getUser" {
		t.Errorf("tools[1].Name = %q, want %q", tools[1].Name, "getUser")
	}
	if tools[1].Description != "Get a user by ID" {
		t.Errorf("getUser Description = %q, want %q", tools[1].Description, "Get a user by ID")
	}

	// listUsers
	if tools[2].Name != "listUsers" {
		t.Errorf("tools[2].Name = %q, want %q", tools[2].Name, "listUsers")
	}
}

func TestInvokeTool_GET_PathParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/users/123" {
			t.Errorf("path = %q, want /users/123", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"123","name":"Alice"}`))
	}))
	defer server.Close()

	adapter, err := NewAdapter(sampleSpec(server.URL), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	result, err := adapter.InvokeTool(context.Background(), "getUser", map[string]interface{}{
		"userId": "123",
	}, "")
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if !strings.Contains(result, "Alice") {
		t.Errorf("result = %q, expected to contain 'Alice'", result)
	}
}

func TestInvokeTool_GET_QueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("query limit = %q, want 10", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("offset") != "5" {
			t.Errorf("query offset = %q, want 5", r.URL.Query().Get("offset"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"1"},{"id":"2"}]`))
	}))
	defer server.Close()

	adapter, err := NewAdapter(sampleSpec(server.URL), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	result, err := adapter.InvokeTool(context.Background(), "listUsers", map[string]interface{}{
		"limit":  10,
		"offset": 5,
	}, "")
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if result == "" {
		t.Error("InvokeTool() returned empty result")
	}
}

func TestInvokeTool_POST_JSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "Bob" {
			t.Errorf("body.name = %v, want Bob", body["name"])
		}
		if body["email"] != "bob@example.com" {
			t.Errorf("body.email = %v, want bob@example.com", body["email"])
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"456","name":"Bob"}`))
	}))
	defer server.Close()

	adapter, err := NewAdapter(sampleSpec(server.URL), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	result, err := adapter.InvokeTool(context.Background(), "createUser", map[string]interface{}{
		"name":  "Bob",
		"email": "bob@example.com",
	}, "")
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if !strings.Contains(result, "Bob") {
		t.Errorf("result = %q, expected to contain 'Bob'", result)
	}
}

func TestInvokeTool_OBOTokenInjection(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	adapter, err := NewAdapter(sampleSpec(server.URL), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	_, err = adapter.InvokeTool(context.Background(), "getUser", map[string]interface{}{
		"userId": "42",
	}, "obo-token-xyz")
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}

	if receivedAuth != "Bearer obo-token-xyz" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer obo-token-xyz")
	}
}

func TestInvokeTool_NoOBOToken(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	adapter, err := NewAdapter(sampleSpec(server.URL), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	_, err = adapter.InvokeTool(context.Background(), "getUser", map[string]interface{}{
		"userId": "1",
	}, "")
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}

	if receivedAuth != "" {
		t.Errorf("Authorization = %q, want empty (no OBO token)", receivedAuth)
	}
}

func TestInvokeTool_UnknownTool(t *testing.T) {
	adapter, err := NewAdapter(sampleSpec("http://localhost"), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	_, err = adapter.InvokeTool(context.Background(), "nonexistent", nil, "")
	if err == nil {
		t.Fatal("InvokeTool() expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error = %q, expected to contain 'unknown tool'", err.Error())
	}
}

func TestInvokeTool_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	adapter, err := NewAdapter(sampleSpec(server.URL), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	_, err = adapter.InvokeTool(context.Background(), "getUser", map[string]interface{}{
		"userId": "1",
	}, "")
	if err == nil {
		t.Fatal("InvokeTool() expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, expected to mention status 500", err.Error())
	}
}

func TestInvokeTool_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler should not be reached if context is cancelled.
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter, err := NewAdapter(sampleSpec(server.URL), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = adapter.InvokeTool(ctx, "getUser", map[string]interface{}{
		"userId": "1",
	}, "")
	if err == nil {
		t.Fatal("InvokeTool() expected error with cancelled context")
	}
}

func TestInvokeTool_POST_MixedParamsAndBody(t *testing.T) {
	// Test a spec where POST has both query params and a body.
	specData := []byte(`{
		"openapi": "3.0.3",
		"info": {"title": "Mixed API", "version": "1.0.0"},
		"paths": {
			"/items": {
				"post": {
					"operationId": "createItem",
					"summary": "Create an item",
					"parameters": [
						{"name": "dryRun", "in": "query", "schema": {"type": "boolean"}}
					],
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {
									"type": "object",
									"properties": {
										"title": {"type": "string"},
										"count": {"type": "integer"}
									},
									"required": ["title"]
								}
							}
						}
					}
				}
			}
		}
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dryRun") != "true" {
			t.Errorf("query dryRun = %q, want true", r.URL.Query().Get("dryRun"))
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["title"] != "Test Item" {
			t.Errorf("body.title = %v, want 'Test Item'", body["title"])
		}
		// dryRun should NOT be in the body since it's a query param.
		if _, exists := body["dryRun"]; exists {
			t.Error("body should not contain 'dryRun' (it's a query param)")
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"1","title":"Test Item"}`))
	}))
	defer server.Close()

	adapter, err := NewAdapter(specData, Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	result, err := adapter.InvokeTool(context.Background(), "createItem", map[string]interface{}{
		"dryRun": true,
		"title":  "Test Item",
		"count":  5,
	}, "")
	if err != nil {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if !strings.Contains(result, "Test Item") {
		t.Errorf("result = %q, expected to contain 'Test Item'", result)
	}
}

func TestListTools_InputSchemaRequired(t *testing.T) {
	adapter, err := NewAdapter(sampleSpec("http://localhost"), Config{})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	tools := adapter.ListTools()
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	// createUser should have "name" and "email" as required.
	createUser := tools[0]
	if createUser.Name != "createUser" {
		t.Fatalf("expected createUser, got %q", createUser.Name)
	}
	requiredSet := make(map[string]bool)
	for _, r := range createUser.InputSchema.Required {
		requiredSet[r] = true
	}
	if !requiredSet["name"] || !requiredSet["email"] {
		t.Errorf("createUser required = %v, want [name, email]", createUser.InputSchema.Required)
	}

	// getUser should have "userId" as required.
	getUser := tools[1]
	if getUser.Name != "getUser" {
		t.Fatalf("expected getUser, got %q", getUser.Name)
	}
	if len(getUser.InputSchema.Required) != 1 || getUser.InputSchema.Required[0] != "userId" {
		t.Errorf("getUser required = %v, want [userId]", getUser.InputSchema.Required)
	}
}

func TestNewAdapter_EmptyPaths(t *testing.T) {
	specData := []byte(`{"openapi":"3.0.3","info":{"title":"Empty","version":"1.0.0"},"paths":{}}`)
	adapter, err := NewAdapter(specData, Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	tools := adapter.ListTools()
	if len(tools) != 0 {
		t.Errorf("ListTools() returned %d tools, want 0", len(tools))
	}
}

func TestNewAdapter_OperationWithoutID(t *testing.T) {
	// Operations without operationId should be skipped.
	specData := []byte(`{
		"openapi": "3.0.3",
		"info": {"title": "NoID", "version": "1.0.0"},
		"paths": {
			"/health": {
				"get": {
					"summary": "Health check without operationId"
				}
			},
			"/status": {
				"get": {
					"operationId": "getStatus",
					"summary": "Get status"
				}
			}
		}
	}`)

	adapter, err := NewAdapter(specData, Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	tools := adapter.ListTools()
	if len(tools) != 1 {
		t.Fatalf("ListTools() returned %d tools, want 1 (skipping op without ID)", len(tools))
	}
	if tools[0].Name != "getStatus" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "getStatus")
	}
}
