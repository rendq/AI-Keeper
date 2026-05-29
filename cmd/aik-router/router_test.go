package router

import (
	"errors"
	"net/http"
	"testing"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// --- Test helpers ---

// mockRegistry is a simple in-memory EndpointRegistry.
type mockRegistry struct {
	endpoints map[shared.ResourceRef]RouteEndpoint
}

func (m *mockRegistry) Get(ref shared.ResourceRef) (RouteEndpoint, bool) {
	ep, ok := m.endpoints[ref]
	return ep, ok
}

// mockCaller simulates endpoint calls; it fails for refs in failSet.
type mockCaller struct {
	failSet map[shared.ResourceRef]bool
}

func (m *mockCaller) Call(endpoint RouteEndpoint, request interface{}) (interface{}, error) {
	if m.failSet[endpoint.Ref] {
		return nil, errors.New("mock: endpoint unavailable")
	}
	return "ok:" + string(endpoint.Ref), nil
}

// helper to build a simple registry.
func newTestRegistry(entries ...RouteEndpoint) *mockRegistry {
	reg := &mockRegistry{endpoints: make(map[shared.ResourceRef]RouteEndpoint)}
	for _, e := range entries {
		reg.endpoints[e.Ref] = e
	}
	return reg
}

// --- CEL Evaluator Tests ---

func TestCELEvaluator_EmptyExpression(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator: %v", err)
	}
	result, err := eval.Evaluate("", RequestContext{})
	if err != nil {
		t.Fatalf("Evaluate empty: %v", err)
	}
	if !result {
		t.Error("empty expression should return true")
	}
}

func TestCELEvaluator_UserCountry(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator: %v", err)
	}

	ctx := RequestContext{UserCountry: "CN", Classification: "internal", CostSensitive: false}
	result, err := eval.Evaluate(`user.country == "CN"`, ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result {
		t.Error("expected true for user.country == CN")
	}

	result, err = eval.Evaluate(`user.country == "US"`, ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result {
		t.Error("expected false for user.country == US when country is CN")
	}
}

func TestCELEvaluator_CostSensitive(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator: %v", err)
	}

	ctx := RequestContext{CostSensitive: true, UserCountry: "US", Classification: "public"}
	result, err := eval.Evaluate(`costSensitive == true`, ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result {
		t.Error("expected true for costSensitive == true")
	}
}

func TestCELEvaluator_Classification(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator: %v", err)
	}

	ctx := RequestContext{Classification: "restricted", UserCountry: "EU", CostSensitive: false}
	result, err := eval.Evaluate(`classification == "restricted"`, ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result {
		t.Error("expected true for classification == restricted")
	}
}

func TestCELEvaluator_InvalidExpression(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator: %v", err)
	}

	_, err = eval.Evaluate(`invalid &&& syntax`, RequestContext{UserCountry: "US", Classification: "public"})
	if err == nil {
		t.Error("expected error for invalid CEL expression")
	}
}

func TestCELEvaluator_NonBoolReturn(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator: %v", err)
	}

	_, err = eval.Evaluate(`user.country`, RequestContext{UserCountry: "US", Classification: "public"})
	if err == nil {
		t.Error("expected error for non-bool return expression")
	}
}

func TestCELEvaluator_ComplexExpression(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator: %v", err)
	}

	ctx := RequestContext{
		UserCountry:    "CN",
		Classification: "confidential",
		CostSensitive:  true,
		ContextLength:  5000,
	}
	result, err := eval.Evaluate(`user.country == "CN" && classification == "confidential" && costSensitive`, ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !result {
		t.Error("expected true for complex expression")
	}
}

// --- Weighted Selection Tests ---

func TestWeightedSelect_SingleEndpoint(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://gpt-4o", Weight: 1}}}},
	}
	reg := newTestRegistry(RouteEndpoint{Ref: "model://gpt-4o", Region: "us-east-1"})
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	decision, err := r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if decision.Endpoint != "model://gpt-4o" {
		t.Errorf("expected model://gpt-4o, got %s", decision.Endpoint)
	}
}

func TestWeightedSelect_Distribution(t *testing.T) {
	// With deterministic seed, verify weighted selection produces expected distribution.
	endpoints := []WeightedEndpoint{
		{Ref: "model://a", Weight: 70},
		{Ref: "model://b", Weight: 30},
	}
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: endpoints}},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://a", Region: "us-east-1"},
		RouteEndpoint{Ref: "model://b", Region: "eu-west-1"},
	)
	r, err := NewRouter(table, reg, nil, 1)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	counts := map[shared.ResourceRef]int{}
	iterations := 1000
	for i := 0; i < iterations; i++ {
		decision, err := r.Route(RequestContext{UserCountry: "US", Classification: "public"})
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		counts[decision.Endpoint]++
	}

	// With 70/30 weights over 1000 iterations, we expect roughly 700/300.
	// Allow generous margin (±15%).
	aCount := counts["model://a"]
	bCount := counts["model://b"]
	if aCount < 550 || aCount > 850 {
		t.Errorf("model://a selected %d times (expected ~700)", aCount)
	}
	if bCount < 150 || bCount > 450 {
		t.Errorf("model://b selected %d times (expected ~300)", bCount)
	}
}

func TestWeightedSelect_ZeroWeight(t *testing.T) {
	// Zero or negative weights should be treated as 1.
	endpoints := []WeightedEndpoint{
		{Ref: "model://a", Weight: 0},
		{Ref: "model://b", Weight: 0},
	}
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: endpoints}},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://a", Region: "us-east-1"},
		RouteEndpoint{Ref: "model://b", Region: "eu-west-1"},
	)
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Should not panic and should select one of them.
	decision, err := r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if decision.Endpoint != "model://a" && decision.Endpoint != "model://b" {
		t.Errorf("unexpected endpoint: %s", decision.Endpoint)
	}
}

// --- CEL Rule Matching Tests ---

func TestRoute_FirstMatchWins(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{
			{CELExpression: `user.country == "CN"`, Endpoints: []WeightedEndpoint{{Ref: "model://cn-model", Weight: 1}}},
			{CELExpression: `user.country == "EU"`, Endpoints: []WeightedEndpoint{{Ref: "model://eu-model", Weight: 1}}},
			{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://default", Weight: 1}}},
		},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://cn-model", Region: "cn-north-1"},
		RouteEndpoint{Ref: "model://eu-model", Region: "eu-west-1"},
		RouteEndpoint{Ref: "model://default", Region: "us-east-1"},
	)
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// CN user should match first rule.
	decision, err := r.Route(RequestContext{UserCountry: "CN", Classification: "internal"})
	if err != nil {
		t.Fatalf("Route CN: %v", err)
	}
	if decision.Endpoint != "model://cn-model" {
		t.Errorf("expected cn-model, got %s", decision.Endpoint)
	}

	// US user should fall through to catch-all.
	decision, err = r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if err != nil {
		t.Fatalf("Route US: %v", err)
	}
	if decision.Endpoint != "model://default" {
		t.Errorf("expected default, got %s", decision.Endpoint)
	}
}

func TestRoute_NoMatchingRule(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{
			{CELExpression: `user.country == "CN"`, Endpoints: []WeightedEndpoint{{Ref: "model://cn-model", Weight: 1}}},
		},
	}
	reg := newTestRegistry(RouteEndpoint{Ref: "model://cn-model", Region: "cn-north-1"})
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	_, err = r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if !errors.Is(err, ErrNoMatchingRule) {
		t.Errorf("expected ErrNoMatchingRule, got %v", err)
	}
}

func TestRoute_DefaultEndpointFallback(t *testing.T) {
	defaultRef := shared.ResourceRef("model://fallback-default")
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{
			{CELExpression: `user.country == "CN"`, Endpoints: []WeightedEndpoint{{Ref: "model://cn-model", Weight: 1}}},
		},
		DefaultEndpoint: &defaultRef,
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://cn-model", Region: "cn-north-1"},
		RouteEndpoint{Ref: "model://fallback-default", Region: "us-east-1"},
	)
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	decision, err := r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if decision.Endpoint != "model://fallback-default" {
		t.Errorf("expected fallback-default, got %s", decision.Endpoint)
	}
}

// --- Allowlist Tests ---

func TestRoute_AllowlistEnforced(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://gpt-4o", Weight: 1}}}},
	}
	reg := newTestRegistry(RouteEndpoint{Ref: "model://gpt-4o", Region: "us-east-1"})

	// Allowlist does NOT contain gpt-4o.
	allowlist := []shared.ResourceRef{"model://claude-3"}
	r, err := NewRouter(table, reg, allowlist, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	_, err = r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if !errors.Is(err, ErrModelNotAllowed) {
		t.Errorf("expected ErrModelNotAllowed, got %v", err)
	}
}

func TestRoute_AllowlistPermits(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://gpt-4o", Weight: 1}}}},
	}
	reg := newTestRegistry(RouteEndpoint{Ref: "model://gpt-4o", Region: "us-east-1"})

	allowlist := []shared.ResourceRef{"model://gpt-4o", "model://claude-3"}
	r, err := NewRouter(table, reg, allowlist, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	decision, err := r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if decision.Endpoint != "model://gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", decision.Endpoint)
	}
}

func TestRoute_EmptyAllowlistPermitsAll(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://gpt-4o", Weight: 1}}}},
	}
	reg := newTestRegistry(RouteEndpoint{Ref: "model://gpt-4o", Region: "us-east-1"})

	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	decision, err := r.Route(RequestContext{UserCountry: "US", Classification: "public"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if decision.Endpoint != "model://gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", decision.Endpoint)
	}
}

// --- Fallback Chain Tests ---

func TestRouteWithFallback_PrimarySuccess(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://primary", Weight: 1}}}},
	}
	reg := newTestRegistry(RouteEndpoint{Ref: "model://primary", Region: "us-east-1", Fallback: []shared.ResourceRef{"model://secondary"}})
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	caller := &mockCaller{failSet: map[shared.ResourceRef]bool{}}
	resp, decision, err := r.RouteWithFallback(RequestContext{UserCountry: "US", Classification: "public"}, caller, nil)
	if err != nil {
		t.Fatalf("RouteWithFallback: %v", err)
	}
	if decision.FallbackUsed {
		t.Error("expected no fallback")
	}
	if resp != "ok:model://primary" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestRouteWithFallback_PrimaryFailsFallbackSucceeds(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://primary", Weight: 1}}}},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://primary", Region: "us-east-1", Fallback: []shared.ResourceRef{"model://secondary"}},
		RouteEndpoint{Ref: "model://secondary", Region: "eu-west-1"},
	)
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	caller := &mockCaller{failSet: map[shared.ResourceRef]bool{"model://primary": true}}
	resp, decision, err := r.RouteWithFallback(RequestContext{UserCountry: "US", Classification: "public"}, caller, nil)
	if err != nil {
		t.Fatalf("RouteWithFallback: %v", err)
	}
	if !decision.FallbackUsed {
		t.Error("expected fallback used")
	}
	if decision.Endpoint != "model://secondary" {
		t.Errorf("expected secondary, got %s", decision.Endpoint)
	}
	if resp != "ok:model://secondary" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestRouteWithFallback_AllFail502(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://primary", Weight: 1}}}},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://primary", Region: "us-east-1", Fallback: []shared.ResourceRef{"model://secondary", "model://tertiary"}},
		RouteEndpoint{Ref: "model://secondary", Region: "eu-west-1"},
		RouteEndpoint{Ref: "model://tertiary", Region: "ap-east-1"},
	)
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	caller := &mockCaller{failSet: map[shared.ResourceRef]bool{
		"model://primary":   true,
		"model://secondary": true,
		"model://tertiary":  true,
	}}
	_, decision, err := r.RouteWithFallback(RequestContext{UserCountry: "US", Classification: "public"}, caller, nil)
	if !errors.Is(err, ErrAllEndpointsFailed) {
		t.Errorf("expected ErrAllEndpointsFailed, got %v", err)
	}
	if !decision.FallbackUsed {
		t.Error("expected fallback used flag set")
	}
}

func TestRouteWithFallback_AllowlistBlocksFallback(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://primary", Weight: 1}}}},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://primary", Region: "us-east-1", Fallback: []shared.ResourceRef{"model://blocked-fallback"}},
		RouteEndpoint{Ref: "model://blocked-fallback", Region: "eu-west-1"},
	)
	// Allowlist permits primary but not blocked-fallback.
	allowlist := []shared.ResourceRef{"model://primary"}
	r, err := NewRouter(table, reg, allowlist, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	caller := &mockCaller{failSet: map[shared.ResourceRef]bool{"model://primary": true}}
	_, _, err = r.RouteWithFallback(RequestContext{UserCountry: "US", Classification: "public"}, caller, nil)
	if !errors.Is(err, ErrAllEndpointsFailed) {
		t.Errorf("expected ErrAllEndpointsFailed when fallback blocked by allowlist, got %v", err)
	}
}

// --- Audit Metadata Tests ---

func TestRoute_AuditMetadata(t *testing.T) {
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://gpt-4o-eu", Weight: 1}}}},
	}
	reg := newTestRegistry(RouteEndpoint{Ref: "model://gpt-4o-eu", Region: "eu-west-1"})
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	decision, err := r.Route(RequestContext{UserCountry: "EU", Classification: "confidential"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if decision.Endpoint != "model://gpt-4o-eu" {
		t.Errorf("expected model://gpt-4o-eu, got %s", decision.Endpoint)
	}
	if decision.Region != "eu-west-1" {
		t.Errorf("expected region eu-west-1, got %s", decision.Region)
	}
	if decision.FallbackUsed {
		t.Error("expected FallbackUsed=false for primary route")
	}
}

// --- Provider Adapter Tests ---

func TestOpenAIAdapter_BuildRequest(t *testing.T) {
	adapter := &OpenAIAdapter{}
	ep := RouteEndpoint{
		Ref:      "model://gpt-4o",
		Endpoint: "https://api.openai.com",
		Region:   "us",
	}

	req, err := adapter.BuildRequest(ep, []byte(`{"model":"gpt-4o"}`), "sk-test-key")
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if req.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URL.String() != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("unexpected URL: %s", req.URL.String())
	}
	if req.Header.Get("Authorization") != "Bearer sk-test-key" {
		t.Errorf("unexpected auth header: %s", req.Header.Get("Authorization"))
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("unexpected content-type: %s", req.Header.Get("Content-Type"))
	}
}

func TestOpenAIAdapter_NoAPIKey(t *testing.T) {
	adapter := &OpenAIAdapter{}
	ep := RouteEndpoint{Endpoint: "https://api.openai.com"}

	req, err := adapter.BuildRequest(ep, []byte(`{}`), "")
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("expected no Authorization header when apiKey is empty")
	}
}

func TestAzureOpenAIAdapter_BuildRequest(t *testing.T) {
	adapter := &AzureOpenAIAdapter{}
	ep := RouteEndpoint{
		Ref:      "model://azure-gpt4",
		Endpoint: "https://myresource.openai.azure.com/openai/deployments/gpt-4",
		Region:   "eastus",
	}

	req, err := adapter.BuildRequest(ep, []byte(`{"messages":[]}`), "azure-key-123")
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if req.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", req.Method)
	}
	expectedURL := "https://myresource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-01"
	if req.URL.String() != expectedURL {
		t.Errorf("unexpected URL: %s", req.URL.String())
	}
	if req.Header.Get("api-key") != "azure-key-123" {
		t.Errorf("unexpected api-key header: %s", req.Header.Get("api-key"))
	}
}

func TestAzureOpenAIAdapter_URLWithExistingPath(t *testing.T) {
	adapter := &AzureOpenAIAdapter{}
	ep := RouteEndpoint{
		Endpoint: "https://myresource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-06-01",
	}

	req, err := adapter.BuildRequest(ep, []byte(`{}`), "key")
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	// Should not double-append chat/completions.
	if req.URL.String() != ep.Endpoint {
		t.Errorf("unexpected URL: %s", req.URL.String())
	}
}

func TestNewProviderAdapter(t *testing.T) {
	tests := []struct {
		provider string
		expected ProviderType
	}{
		{"openai", ProviderOpenAI},
		{"azure_openai", ProviderAzureOpenAI},
		{"unknown_provider", ProviderOpenAI}, // fallback
	}
	for _, tt := range tests {
		adapter, err := NewProviderAdapter(tt.provider)
		if err != nil {
			t.Fatalf("NewProviderAdapter(%s): %v", tt.provider, err)
		}
		if adapter.ProviderName() != tt.expected {
			t.Errorf("NewProviderAdapter(%s): expected %s, got %s", tt.provider, tt.expected, adapter.ProviderName())
		}
	}
}

// --- Integration-like scenario tests ---

func TestRouter_EndToEndScenario_EUDataResidency(t *testing.T) {
	// Scenario: EU users with confidential data should be routed to EU endpoint.
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{
			{
				CELExpression: `user.country == "EU" && classification == "confidential"`,
				Endpoints:     []WeightedEndpoint{{Ref: "model://gpt-4o-eu", Weight: 1}},
			},
			{
				CELExpression: `costSensitive == true`,
				Endpoints:     []WeightedEndpoint{{Ref: "model://gpt-3.5", Weight: 1}},
			},
			{
				CELExpression: "",
				Endpoints:     []WeightedEndpoint{{Ref: "model://gpt-4o-us", Weight: 1}},
			},
		},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://gpt-4o-eu", Region: "eu-west-1"},
		RouteEndpoint{Ref: "model://gpt-3.5", Region: "us-east-1"},
		RouteEndpoint{Ref: "model://gpt-4o-us", Region: "us-east-1"},
	)
	allowlist := []shared.ResourceRef{"model://gpt-4o-eu", "model://gpt-3.5", "model://gpt-4o-us"}
	r, err := NewRouter(table, reg, allowlist, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// EU confidential → eu endpoint.
	decision, err := r.Route(RequestContext{UserCountry: "EU", Classification: "confidential", CostSensitive: false})
	if err != nil {
		t.Fatalf("Route EU: %v", err)
	}
	if decision.Endpoint != "model://gpt-4o-eu" {
		t.Errorf("EU confidential: expected gpt-4o-eu, got %s", decision.Endpoint)
	}
	if decision.Region != "eu-west-1" {
		t.Errorf("EU confidential: expected region eu-west-1, got %s", decision.Region)
	}

	// Cost sensitive user → gpt-3.5.
	decision, err = r.Route(RequestContext{UserCountry: "US", Classification: "public", CostSensitive: true})
	if err != nil {
		t.Fatalf("Route cost-sensitive: %v", err)
	}
	if decision.Endpoint != "model://gpt-3.5" {
		t.Errorf("cost-sensitive: expected gpt-3.5, got %s", decision.Endpoint)
	}

	// Default case → gpt-4o-us.
	decision, err = r.Route(RequestContext{UserCountry: "US", Classification: "public", CostSensitive: false})
	if err != nil {
		t.Fatalf("Route default: %v", err)
	}
	if decision.Endpoint != "model://gpt-4o-us" {
		t.Errorf("default: expected gpt-4o-us, got %s", decision.Endpoint)
	}
}

func TestRouter_EndToEndScenario_FallbackWithAudit(t *testing.T) {
	// Scenario: Primary endpoint fails, falls back to secondary, audit records fallback_used.
	table := &RouteTable{
		Alias: "reasoner",
		Rules: []RouteRule{{CELExpression: "", Endpoints: []WeightedEndpoint{{Ref: "model://primary", Weight: 1}}}},
	}
	reg := newTestRegistry(
		RouteEndpoint{Ref: "model://primary", Region: "us-east-1", Fallback: []shared.ResourceRef{"model://secondary", "model://tertiary"}},
		RouteEndpoint{Ref: "model://secondary", Region: "eu-west-1"},
		RouteEndpoint{Ref: "model://tertiary", Region: "ap-east-1"},
	)
	r, err := NewRouter(table, reg, nil, 42)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Primary and secondary fail, tertiary succeeds.
	caller := &mockCaller{failSet: map[shared.ResourceRef]bool{
		"model://primary":   true,
		"model://secondary": true,
	}}
	resp, decision, err := r.RouteWithFallback(RequestContext{UserCountry: "US", Classification: "public"}, caller, nil)
	if err != nil {
		t.Fatalf("RouteWithFallback: %v", err)
	}
	if !decision.FallbackUsed {
		t.Error("expected FallbackUsed=true")
	}
	if decision.Endpoint != "model://tertiary" {
		t.Errorf("expected tertiary, got %s", decision.Endpoint)
	}
	if decision.Region != "ap-east-1" {
		t.Errorf("expected region ap-east-1, got %s", decision.Region)
	}
	if resp != "ok:model://tertiary" {
		t.Errorf("unexpected response: %v", resp)
	}
}
