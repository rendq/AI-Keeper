package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Auth Tests ---

func TestChannelAuth_WebhookSignature(t *testing.T) {
	secret := "my-webhook-secret"
	body := `{"event":"message"}`

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	auth := NewChannelAuthenticator([]*ChannelAuthConfig{
		{
			Channel:       "feishu",
			AgentName:     "legal-copilot",
			TenantID:      "tenant-1",
			WebhookSecret: secret,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "legal-copilot")
	req.Header.Set("X-Aip-Channel", "feishu")
	req.Header.Set("X-Webhook-Signature", sig)
	req.Header.Set("X-Webhook-Body", body)

	id, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if id.Channel != "feishu" {
		t.Errorf("expected channel=feishu, got %s", id.Channel)
	}
	if id.AgentName != "legal-copilot" {
		t.Errorf("expected agentName=legal-copilot, got %s", id.AgentName)
	}
	if id.TenantID != "tenant-1" {
		t.Errorf("expected tenantID=tenant-1, got %s", id.TenantID)
	}
}

func TestChannelAuth_WebhookSignatureInvalid(t *testing.T) {
	auth := NewChannelAuthenticator([]*ChannelAuthConfig{
		{
			Channel:       "feishu",
			AgentName:     "legal-copilot",
			TenantID:      "tenant-1",
			WebhookSecret: "correct-secret",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "legal-copilot")
	req.Header.Set("X-Aip-Channel", "feishu")
	req.Header.Set("X-Webhook-Signature", "sha256=deadbeef")
	req.Header.Set("X-Webhook-Body", "some body")

	_, err := auth.Authenticate(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth failure for invalid signature")
	}
}

func TestChannelAuth_APIToken(t *testing.T) {
	auth := NewChannelAuthenticator([]*ChannelAuthConfig{
		{
			Channel:   "api",
			AgentName: "legal-copilot",
			TenantID:  "tenant-1",
			APITokens: []string{"valid-token-123"},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "legal-copilot")
	req.Header.Set("X-Aip-Channel", "api")
	req.Header.Set("Authorization", "Bearer valid-token-123")

	id, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if id.AgentName != "legal-copilot" {
		t.Errorf("expected agentName=legal-copilot, got %s", id.AgentName)
	}
}

func TestChannelAuth_InvalidToken(t *testing.T) {
	auth := NewChannelAuthenticator([]*ChannelAuthConfig{
		{
			Channel:   "api",
			AgentName: "legal-copilot",
			TenantID:  "tenant-1",
			APITokens: []string{"valid-token-123"},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "legal-copilot")
	req.Header.Set("X-Aip-Channel", "api")
	req.Header.Set("Authorization", "Bearer wrong-token")

	_, err := auth.Authenticate(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth failure for invalid token")
	}
}

func TestChannelAuth_MissingCredentials(t *testing.T) {
	auth := NewChannelAuthenticator([]*ChannelAuthConfig{
		{
			Channel:   "api",
			AgentName: "legal-copilot",
			TenantID:  "tenant-1",
			APITokens: []string{"valid-token-123"},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "legal-copilot")
	req.Header.Set("X-Aip-Channel", "api")
	// No Authorization header

	_, err := auth.Authenticate(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth failure for missing credentials")
	}
}

func TestChannelAuth_UnknownChannel(t *testing.T) {
	auth := NewChannelAuthenticator([]*ChannelAuthConfig{
		{
			Channel:   "api",
			AgentName: "legal-copilot",
			TenantID:  "tenant-1",
			APITokens: []string{"token"},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "unknown-agent")
	req.Header.Set("X-Aip-Channel", "unknown")

	_, err := auth.Authenticate(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth failure for unknown channel")
	}
}

// --- Rate Limiter Tests ---

func TestTokenBucketLimiter_AllowsWithinLimit(t *testing.T) {
	limiter := NewTokenBucketLimiter(map[string]*RateLimitConfig{
		"agent/channel": {RequestsPerMinute: 60, BurstSize: 5},
	}, nil)

	ctx := context.Background()
	// Should allow burst of 5.
	for i := 0; i < 5; i++ {
		if !limiter.Allow(ctx, "agent/channel") {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
}

func TestTokenBucketLimiter_DeniesOverLimit(t *testing.T) {
	limiter := NewTokenBucketLimiter(map[string]*RateLimitConfig{
		"agent/channel": {RequestsPerMinute: 60, BurstSize: 2},
	}, nil)

	ctx := context.Background()
	// Exhaust burst.
	limiter.Allow(ctx, "agent/channel")
	limiter.Allow(ctx, "agent/channel")

	// Next request should be denied (no time to refill).
	if limiter.Allow(ctx, "agent/channel") {
		t.Fatal("request should be denied after burst exhausted")
	}
}

func TestTokenBucketLimiter_DefaultConfig(t *testing.T) {
	limiter := NewTokenBucketLimiter(nil, &RateLimitConfig{
		RequestsPerMinute: 60,
		BurstSize:         3,
	})

	ctx := context.Background()
	// Unknown key should use default config.
	for i := 0; i < 3; i++ {
		if !limiter.Allow(ctx, "unknown/key") {
			t.Fatalf("request %d should be allowed with default config", i+1)
		}
	}
	if limiter.Allow(ctx, "unknown/key") {
		t.Fatal("should deny after default burst exhausted")
	}
}

// --- Trace Injector Tests ---

func TestTraceInjector_InjectsTraceparent(t *testing.T) {
	injector := &DefaultTraceInjector{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	req = injector.Inject(req)

	tp := req.Header.Get("traceparent")
	if tp == "" {
		t.Fatal("traceparent header should be set")
	}

	// Validate format: 00-<32hex>-<16hex>-01
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		t.Fatalf("traceparent should have 4 parts, got %d: %s", len(parts), tp)
	}
	if parts[0] != "00" {
		t.Errorf("version should be 00, got %s", parts[0])
	}
	if len(parts[1]) != 32 {
		t.Errorf("trace_id should be 32 hex chars, got %d", len(parts[1]))
	}
	if len(parts[2]) != 16 {
		t.Errorf("span_id should be 16 hex chars, got %d", len(parts[2]))
	}
	if parts[3] != "01" {
		t.Errorf("flags should be 01, got %s", parts[3])
	}
}

func TestTraceInjector_PreservesExisting(t *testing.T) {
	injector := &DefaultTraceInjector{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	existing := "00-abcdef1234567890abcdef1234567890-1234567890abcdef-01"
	req.Header.Set("traceparent", existing)

	req = injector.Inject(req)

	if got := req.Header.Get("traceparent"); got != existing {
		t.Errorf("should preserve existing traceparent, got %s", got)
	}
}

// --- Secret Provider Tests ---

func TestStubSecretProvider_GetSecret(t *testing.T) {
	sp := NewStubSecretProvider(map[string]string{
		"model/openai-key": "sk-test-123",
	})

	val, err := sp.GetSecret(context.Background(), "model/openai-key")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if val != "sk-test-123" {
		t.Errorf("expected sk-test-123, got %s", val)
	}
}

func TestStubSecretProvider_NotFound(t *testing.T) {
	sp := NewStubSecretProvider(nil)

	_, err := sp.GetSecret(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

// --- Gateway Integration Tests ---

func TestGateway_AuthFailure_Returns401(t *testing.T) {
	gw := NewGateway(
		WithAuthenticator(NewChannelAuthenticator([]*ChannelAuthConfig{
			{Channel: "api", AgentName: "agent", TenantID: "t1", APITokens: []string{"valid"}},
		})),
	)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "agent")
	req.Header.Set("X-Aip-Channel", "api")
	req.Header.Set("Authorization", "Bearer invalid")

	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGateway_RateLimitExceeded_Returns429(t *testing.T) {
	gw := NewGateway(
		WithAuthenticator(NewChannelAuthenticator([]*ChannelAuthConfig{
			{Channel: "api", AgentName: "agent", TenantID: "t1", APITokens: []string{"token"}},
		})),
		WithRateLimiter(NewTokenBucketLimiter(map[string]*RateLimitConfig{
			"agent/api": {RequestsPerMinute: 60, BurstSize: 1},
		}, nil)),
	)

	makeReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Aip-Agent", "agent")
		req.Header.Set("X-Aip-Channel", "api")
		req.Header.Set("Authorization", "Bearer token")
		return req
	}

	// First request should pass.
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, makeReq())
	if w.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", w.Code)
	}

	// Second request should be rate limited.
	w = httptest.NewRecorder()
	gw.ServeHTTP(w, makeReq())
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestGateway_Success_InjectsHeaders(t *testing.T) {
	var capturedReq *http.Request
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
	})

	gw := NewGateway(
		WithAuthenticator(&noopAuth{}),
		WithDownstream(downstream),
	)

	req := httptest.NewRequest(http.MethodPost, "/invoke", nil)
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Check traceparent was injected.
	if capturedReq.Header.Get("traceparent") == "" {
		t.Error("traceparent header should be injected")
	}

	// Check invocation ID was generated.
	invID := capturedReq.Header.Get("X-Aip-Invocation-Id")
	if invID == "" {
		t.Error("X-Aip-Invocation-Id should be set")
	}
	if len(invID) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("invocation ID should be 32 hex chars, got %d", len(invID))
	}
}

func TestGateway_FullChain_Success(t *testing.T) {
	secret := "webhook-secret"
	body := `{"msg":"hello"}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	downstreamCalled := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	gw := NewGateway(
		WithAuthenticator(NewChannelAuthenticator([]*ChannelAuthConfig{
			{
				Channel:       "feishu",
				AgentName:     "legal-copilot",
				TenantID:      "tenant-1",
				WebhookSecret: secret,
			},
		})),
		WithRateLimiter(NewTokenBucketLimiter(map[string]*RateLimitConfig{
			"legal-copilot/feishu": {RequestsPerMinute: 120, BurstSize: 10},
		}, nil)),
		WithDownstream(downstream),
	)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Aip-Agent", "legal-copilot")
	req.Header.Set("X-Aip-Channel", "feishu")
	req.Header.Set("X-Webhook-Signature", sig)
	req.Header.Set("X-Webhook-Body", body)

	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !downstreamCalled {
		t.Error("downstream PEP should have been called")
	}
}

func TestGenerateInvocationID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateInvocationID()
		if ids[id] {
			t.Fatalf("duplicate invocation ID: %s", id)
		}
		ids[id] = true
	}
}

func TestIdentityFromContext(t *testing.T) {
	id := &Identity{Subject: "test", TenantID: "t1"}
	ctx := withIdentity(context.Background(), id)

	got := IdentityFromContext(ctx)
	if got == nil {
		t.Fatal("expected identity in context")
	}
	if got.Subject != "test" {
		t.Errorf("expected subject=test, got %s", got.Subject)
	}
}

func TestIdentityFromContext_Nil(t *testing.T) {
	got := IdentityFromContext(context.Background())
	if got != nil {
		t.Error("expected nil identity from empty context")
	}
}
