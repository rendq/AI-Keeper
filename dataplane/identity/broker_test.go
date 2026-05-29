package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- Mock implementations ---

// mockRedisClient implements RedisClient for testing.
type mockRedisClient struct {
	mu   sync.Mutex
	sets map[string]map[string]struct{}
	keys map[string]string
	ttls map[string]time.Duration
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{
		sets: make(map[string]map[string]struct{}),
		keys: make(map[string]string),
		ttls: make(map[string]time.Duration),
	}
}

func (m *mockRedisClient) SAdd(_ context.Context, key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sets[key] == nil {
		m.sets[key] = make(map[string]struct{})
	}
	for _, member := range members {
		m.sets[key][member] = struct{}{}
	}
	return nil
}

func (m *mockRedisClient) SIsMember(_ context.Context, key string, member string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sets[key]; ok {
		_, exists := s[member]
		return exists, nil
	}
	return false, nil
}

func (m *mockRedisClient) SMembers(_ context.Context, key string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sets[key]
	if !ok {
		return nil, nil
	}
	members := make([]string, 0, len(s))
	for k := range s {
		members = append(members, k)
	}
	return members, nil
}

func (m *mockRedisClient) Del(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.sets, key)
		delete(m.keys, key)
		delete(m.ttls, key)
	}
	return nil
}

func (m *mockRedisClient) Expire(_ context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttls[key] = ttl
	return nil
}

func (m *mockRedisClient) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[key] = value
	m.ttls[key] = ttl
	return nil
}

func (m *mockRedisClient) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.keys[key]
	return ok, nil
}

// mockOIDCVerifier implements OIDCVerifier for testing.
type mockOIDCVerifier struct {
	issuer   string
	verifyFn func(ctx context.Context, rawToken string, audience string) (*TokenClaims, error)
}

func (m *mockOIDCVerifier) Issuer() string { return m.issuer }
func (m *mockOIDCVerifier) Verify(ctx context.Context, rawToken string, audience string) (*TokenClaims, error) {
	if m.verifyFn != nil {
		return m.verifyFn(ctx, rawToken, audience)
	}
	return &TokenClaims{
		Subject:   "user-123",
		Issuer:    m.issuer,
		Audience:  audience,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		TokenID:   "token-abc",
		TenantID:  "tenant-1",
	}, nil
}

// --- Tests ---

func TestBroker_RegisterAndVerifyMultipleProviders(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	// Register two different providers (B3.2: multiple IdPs simultaneously).
	provider1 := &mockOIDCVerifier{
		issuer: "https://idp1.example.com",
		verifyFn: func(_ context.Context, _ string, _ string) (*TokenClaims, error) {
			return &TokenClaims{
				Subject:   "user-from-idp1",
				Issuer:    "https://idp1.example.com",
				ExpiresAt: time.Now().Add(time.Hour),
				TokenID:   "tok-1",
				TenantID:  "tenant-a",
			}, nil
		},
	}
	provider2 := &mockOIDCVerifier{
		issuer: "https://idp2.example.com",
		verifyFn: func(_ context.Context, _ string, _ string) (*TokenClaims, error) {
			return &TokenClaims{
				Subject:   "user-from-idp2",
				Issuer:    "https://idp2.example.com",
				ExpiresAt: time.Now().Add(time.Hour),
				TokenID:   "tok-2",
				TenantID:  "tenant-b",
			}, nil
		},
	}

	broker.RegisterProvider(provider1)
	broker.RegisterProvider(provider2)

	ctx := context.Background()

	// Verify with provider 1.
	claims, err := broker.VerifyToken(ctx, "fake-token-1", "https://idp1.example.com", "aik-gateway")
	if err != nil {
		t.Fatalf("VerifyToken with idp1: %v", err)
	}
	if claims.Subject != "user-from-idp1" {
		t.Errorf("expected subject user-from-idp1, got %s", claims.Subject)
	}
	if claims.TenantID != "tenant-a" {
		t.Errorf("expected tenantID tenant-a, got %s", claims.TenantID)
	}

	// Verify with provider 2.
	claims, err = broker.VerifyToken(ctx, "fake-token-2", "https://idp2.example.com", "aik-gateway")
	if err != nil {
		t.Fatalf("VerifyToken with idp2: %v", err)
	}
	if claims.Subject != "user-from-idp2" {
		t.Errorf("expected subject user-from-idp2, got %s", claims.Subject)
	}

	// Unknown provider should fail.
	_, err = broker.VerifyToken(ctx, "token", "https://unknown.example.com", "aud")
	if err != ErrNoProvider {
		t.Errorf("expected ErrNoProvider, got %v", err)
	}
}

func TestBroker_SATokenLifetimeEnforced(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	// Register SA with default lifetime.
	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:           "default/my-sa",
		TenantID:      "tenant-1",
		TokenLifetime: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	// Issue token.
	ctx := context.Background()
	claims, err := broker.IssueToken(ctx, "default/my-sa", "legal-copilot", "user@example.com")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Verify token lifetime is enforced.
	expectedDuration := 10 * time.Minute
	actualDuration := claims.ExpiresAt.Sub(claims.IssuedAt)
	if actualDuration != expectedDuration {
		t.Errorf("token lifetime: got %v, want %v", actualDuration, expectedDuration)
	}

	// Verify claims embedding (B3.6).
	if claims.TenantID != "tenant-1" {
		t.Errorf("tenantId: got %s, want tenant-1", claims.TenantID)
	}
	if claims.AgentName != "legal-copilot" {
		t.Errorf("agentName: got %s, want legal-copilot", claims.AgentName)
	}
	if claims.OnBehalfOf != "user@example.com" {
		t.Errorf("onBehalfOf: got %s, want user@example.com", claims.OnBehalfOf)
	}
	if claims.ServiceAccount != "default/my-sa" {
		t.Errorf("serviceAccount: got %s, want default/my-sa", claims.ServiceAccount)
	}
	if claims.TokenID == "" {
		t.Error("tokenID should not be empty")
	}
}

func TestBroker_SAMaxLifetimeRejected(t *testing.T) {
	broker := NewBroker(BrokerConfig{Issuer: "https://ai-keeper.io"})

	// Attempt to register SA with lifetime exceeding maximum (C8: short-lived).
	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:           "default/long-lived-sa",
		TenantID:      "tenant-1",
		TokenLifetime: 2 * time.Hour, // exceeds MaxTokenLifetime (1h)
	})
	if err != ErrLifetimeExceed {
		t.Errorf("expected ErrLifetimeExceed, got %v", err)
	}
}

func TestBroker_SADeleteRevokesWithin30s(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	// Register SA and issue multiple tokens.
	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:           "ns/test-sa",
		TenantID:      "tenant-1",
		TokenLifetime: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	ctx := context.Background()
	var tokenIDs []string
	for i := 0; i < 3; i++ {
		claims, err := broker.IssueToken(ctx, "ns/test-sa", "agent-1", fmt.Sprintf("user-%d", i))
		if err != nil {
			t.Fatalf("IssueToken %d: %v", i, err)
		}
		tokenIDs = append(tokenIDs, claims.TokenID)
	}

	// Delete SA — must revoke all tokens (B3.5).
	revoked, err := broker.DeleteSA(ctx, "ns/test-sa")
	if err != nil {
		t.Fatalf("DeleteSA: %v", err)
	}
	if revoked != 3 {
		t.Errorf("expected 3 revoked tokens, got %d", revoked)
	}

	// Verify all tokens are now blacklisted.
	blacklist := NewRedisTokenBlacklist(redis)
	for _, tid := range tokenIDs {
		isRevoked, err := blacklist.IsRevoked(ctx, tid)
		if err != nil {
			t.Fatalf("IsRevoked(%s): %v", tid, err)
		}
		if !isRevoked {
			t.Errorf("token %s should be revoked after SA deletion", tid)
		}
	}

	// SA should no longer be found.
	_, err = broker.GetSA("ns/test-sa")
	if err != ErrSANotFound {
		t.Errorf("expected ErrSANotFound after deletion, got %v", err)
	}
}

func TestBroker_RevokedTokenRejected(t *testing.T) {
	redis := newMockRedisClient()
	blacklist := NewRedisTokenBlacklist(redis)
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  blacklist,
		TokenStore: NewRedisSATokenStore(redis),
	})

	// Register a provider that returns a specific token ID.
	provider := &mockOIDCVerifier{
		issuer: "https://idp.example.com",
		verifyFn: func(_ context.Context, _ string, _ string) (*TokenClaims, error) {
			return &TokenClaims{
				Subject:   "user-1",
				Issuer:    "https://idp.example.com",
				ExpiresAt: time.Now().Add(time.Hour),
				TokenID:   "revoked-token-id",
				TenantID:  "tenant-1",
			}, nil
		},
	}
	broker.RegisterProvider(provider)

	ctx := context.Background()

	// Token should verify fine initially.
	_, err := broker.VerifyToken(ctx, "token", "https://idp.example.com", "aud")
	if err != nil {
		t.Fatalf("initial verify should pass: %v", err)
	}

	// Revoke the token.
	err = blacklist.Add(ctx, "revoked-token-id", time.Hour)
	if err != nil {
		t.Fatalf("Add to blacklist: %v", err)
	}

	// Token should now be rejected.
	_, err = broker.VerifyToken(ctx, "token", "https://idp.example.com", "aud")
	if err != ErrTokenRevoked {
		t.Errorf("expected ErrTokenRevoked, got %v", err)
	}
}

func TestBroker_DisabledSACannotIssueTokens(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:      "ns/disabled-sa",
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	// Disable the SA.
	err = broker.DisableSA("ns/disabled-sa")
	if err != nil {
		t.Fatalf("DisableSA: %v", err)
	}

	// Attempt to issue token should fail.
	ctx := context.Background()
	_, err = broker.IssueToken(ctx, "ns/disabled-sa", "agent", "user")
	if err != ErrSADisabled {
		t.Errorf("expected ErrSADisabled, got %v", err)
	}
}

func TestBroker_IssueTokenEmbedsClaims(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:           "prod/agent-sa",
		TenantID:      "acme-corp",
		TokenLifetime: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	ctx := context.Background()
	claims, err := broker.IssueToken(ctx, "prod/agent-sa", "legal-copilot", "alice@acme.com")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Verify all required claims are embedded (B3.6).
	if claims.TenantID != "acme-corp" {
		t.Errorf("tenantId mismatch: got %s", claims.TenantID)
	}
	if claims.AgentName != "legal-copilot" {
		t.Errorf("agentName mismatch: got %s", claims.AgentName)
	}
	if claims.OnBehalfOf != "alice@acme.com" {
		t.Errorf("onBehalfOf mismatch: got %s", claims.OnBehalfOf)
	}
	if claims.Issuer != "https://ai-keeper.io" {
		t.Errorf("issuer mismatch: got %s", claims.Issuer)
	}
	if claims.Subject != "prod/agent-sa" {
		t.Errorf("subject mismatch: got %s", claims.Subject)
	}
}

func TestBroker_DefaultTokenLifetime(t *testing.T) {
	broker := NewBroker(BrokerConfig{Issuer: "https://ai-keeper.io"})

	// Register SA with zero lifetime — should use default.
	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:      "ns/default-lifetime-sa",
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	sa, _ := broker.GetSA("ns/default-lifetime-sa")
	if sa.TokenLifetime != DefaultTokenLifetime {
		t.Errorf("expected default lifetime %v, got %v", DefaultTokenLifetime, sa.TokenLifetime)
	}
}

func TestBroker_RemoveProvider(t *testing.T) {
	broker := NewBroker(BrokerConfig{Issuer: "https://ai-keeper.io"})
	provider := &mockOIDCVerifier{issuer: "https://removable.example.com"}
	broker.RegisterProvider(provider)

	// Should work initially.
	ctx := context.Background()
	_, err := broker.VerifyToken(ctx, "token", "https://removable.example.com", "aud")
	if err != nil {
		t.Fatalf("expected success before removal: %v", err)
	}

	// Remove and verify it fails.
	broker.RemoveProvider("https://removable.example.com")
	_, err = broker.VerifyToken(ctx, "token", "https://removable.example.com", "aud")
	if err != ErrNoProvider {
		t.Errorf("expected ErrNoProvider after removal, got %v", err)
	}
}

func TestOIDCVerifier_VerifyJWT(t *testing.T) {
	issuer := "https://test-idp.example.com"
	verifier := NewOIDCVerifier(OIDCProviderConfig{
		Issuer:   issuer,
		ClientID: "my-client",
	}, nil)

	// Create a simple unsigned JWT for testing (header.payload.signature).
	header := base64URLEncode([]byte(`{"alg":"none","kid":"test-key"}`))
	payload := map[string]interface{}{
		"iss":            issuer,
		"sub":            "user-456",
		"aud":            "my-client",
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
		"jti":            "unique-token-id",
		"tenantId":       "tenant-x",
		"agentName":      "copilot",
		"onBehalfOf":     "bob@corp.com",
		"serviceAccount": "ns/my-sa",
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64URLEncode(payloadJSON)
	token := header + "." + payloadB64 + ".fakesig"

	ctx := context.Background()
	claims, err := verifier.Verify(ctx, token, "my-client")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if claims.Subject != "user-456" {
		t.Errorf("subject: got %s, want user-456", claims.Subject)
	}
	if claims.TenantID != "tenant-x" {
		t.Errorf("tenantId: got %s, want tenant-x", claims.TenantID)
	}
	if claims.AgentName != "copilot" {
		t.Errorf("agentName: got %s, want copilot", claims.AgentName)
	}
	if claims.OnBehalfOf != "bob@corp.com" {
		t.Errorf("onBehalfOf: got %s, want bob@corp.com", claims.OnBehalfOf)
	}
	if claims.TokenID != "unique-token-id" {
		t.Errorf("tokenID: got %s, want unique-token-id", claims.TokenID)
	}
}

func TestOIDCVerifier_RejectExpiredToken(t *testing.T) {
	issuer := "https://test-idp.example.com"
	verifier := NewOIDCVerifier(OIDCProviderConfig{
		Issuer:   issuer,
		ClientID: "my-client",
	}, nil)

	header := base64URLEncode([]byte(`{"alg":"none"}`))
	payload := map[string]interface{}{
		"iss": issuer,
		"sub": "user-expired",
		"aud": "my-client",
		"exp": time.Now().Add(-time.Hour).Unix(), // expired
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64URLEncode(payloadJSON)
	token := header + "." + payloadB64 + ".sig"

	ctx := context.Background()
	_, err := verifier.Verify(ctx, token, "my-client")
	if err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestOIDCVerifier_RejectWrongIssuer(t *testing.T) {
	verifier := NewOIDCVerifier(OIDCProviderConfig{
		Issuer:   "https://expected-issuer.com",
		ClientID: "client",
	}, nil)

	header := base64URLEncode([]byte(`{"alg":"none"}`))
	payload := map[string]interface{}{
		"iss": "https://wrong-issuer.com",
		"sub": "user",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64URLEncode(payloadJSON)
	token := header + "." + payloadB64 + ".sig"

	ctx := context.Background()
	_, err := verifier.Verify(ctx, token, "")
	if err == nil {
		t.Error("expected error for wrong issuer")
	}
}

func TestOIDCVerifier_RejectWrongAudience(t *testing.T) {
	issuer := "https://idp.example.com"
	verifier := NewOIDCVerifier(OIDCProviderConfig{
		Issuer:   issuer,
		ClientID: "client",
	}, nil)

	header := base64URLEncode([]byte(`{"alg":"none"}`))
	payload := map[string]interface{}{
		"iss": issuer,
		"sub": "user",
		"aud": "wrong-audience",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64URLEncode(payloadJSON)
	token := header + "." + payloadB64 + ".sig"

	ctx := context.Background()
	_, err := verifier.Verify(ctx, token, "expected-audience")
	if err == nil {
		t.Error("expected error for wrong audience")
	}
}

func TestOIDCVerifier_InvalidToken(t *testing.T) {
	verifier := NewOIDCVerifier(OIDCProviderConfig{
		Issuer: "https://idp.example.com",
	}, nil)

	ctx := context.Background()
	_, err := verifier.Verify(ctx, "not-a-jwt", "aud")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRedisBlacklist_AddAndCheck(t *testing.T) {
	redis := newMockRedisClient()
	bl := NewRedisTokenBlacklist(redis)
	ctx := context.Background()

	// Token not in blacklist.
	revoked, err := bl.IsRevoked(ctx, "token-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Error("token-1 should not be revoked initially")
	}

	// Add to blacklist.
	err = bl.Add(ctx, "token-1", time.Hour)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Now it should be revoked.
	revoked, err = bl.IsRevoked(ctx, "token-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Error("token-1 should be revoked after Add")
	}
}

func TestRedisSATokenStore_StoreAndList(t *testing.T) {
	redis := newMockRedisClient()
	store := NewRedisSATokenStore(redis)
	ctx := context.Background()

	expires := time.Now().Add(15 * time.Minute)
	err := store.Store(ctx, "ns/sa-1", "tok-a", expires)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	err = store.Store(ctx, "ns/sa-1", "tok-b", expires)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	tokens, err := store.ListActive(ctx, "ns/sa-1")
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}

	// RemoveAll.
	err = store.RemoveAll(ctx, "ns/sa-1")
	if err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	tokens, err = store.ListActive(ctx, "ns/sa-1")
	if err != nil {
		t.Fatalf("ListActive after removal: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after removal, got %d", len(tokens))
	}
}

// base64URLEncode encodes data without padding for JWT format.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
