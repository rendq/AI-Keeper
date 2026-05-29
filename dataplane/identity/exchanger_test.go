package identity

import (
	"context"
	"testing"
	"time"
)

// --- Token Exchanger Tests (TestOBO*) ---

func TestOBO_ExchangeSuccess(t *testing.T) {
	// Setup: broker with OIDC provider and SA with OBO enabled.
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	// Register OIDC provider that can verify the user's ID token.
	provider := &mockOIDCVerifier{
		issuer: "https://idp.example.com",
		verifyFn: func(_ context.Context, rawToken string, _ string) (*TokenClaims, error) {
			return &TokenClaims{
				Subject:   "alice@acme.com",
				Issuer:    "https://idp.example.com",
				ExpiresAt: time.Now().Add(time.Hour),
				TokenID:   "user-token-1",
				TenantID:  "acme-corp",
			}, nil
		},
	}
	broker.RegisterProvider(provider)

	// Register SA with OBO enabled.
	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "prod/agent-sa",
		TenantID:        "acme-corp",
		TokenLifetime:   10 * time.Minute,
		AllowOnBehalfOf: true,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	// Perform RFC 8693 token exchange.
	resp, err := exchanger.Exchange(ctx, ExchangeRequest{
		GrantType:         GrantTypeTokenExchange,
		SubjectToken:      "user-id-token-jwt",
		SubjectTokenType:  TokenTypeIDToken,
		Resource:          "https://docusign.example.com/api",
		Audience:          "docusign-client",
		ServiceAccountFQN: "prod/agent-sa",
		AgentName:         "legal-copilot",
	})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	// Verify response structure.
	if resp.TokenType != "Bearer" {
		t.Errorf("TokenType: got %s, want Bearer", resp.TokenType)
	}
	if resp.IssuedTokenType != TokenTypeAccessToken {
		t.Errorf("IssuedTokenType: got %s, want %s", resp.IssuedTokenType, TokenTypeAccessToken)
	}
	if resp.ExpiresIn <= 0 {
		t.Errorf("ExpiresIn should be > 0, got %d", resp.ExpiresIn)
	}

	// Verify OBO claims (B3.6).
	claims := resp.AccessToken
	if claims.OnBehalfOf != "alice@acme.com" {
		t.Errorf("OnBehalfOf: got %s, want alice@acme.com", claims.OnBehalfOf)
	}
	if claims.TenantID != "acme-corp" {
		t.Errorf("TenantID: got %s, want acme-corp", claims.TenantID)
	}
	if claims.AgentName != "legal-copilot" {
		t.Errorf("AgentName: got %s, want legal-copilot", claims.AgentName)
	}
	if claims.Audience != "docusign-client" {
		t.Errorf("Audience: got %s, want docusign-client", claims.Audience)
	}
	if claims.ServiceAccount != "prod/agent-sa" {
		t.Errorf("ServiceAccount: got %s, want prod/agent-sa", claims.ServiceAccount)
	}
}

func TestOBO_MissingUserContext401(t *testing.T) {
	// B3.4: Missing user context must return 401.
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "ns/sa",
		TenantID:        "tenant-1",
		TokenLifetime:   10 * time.Minute,
		AllowOnBehalfOf: true,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	// Attempt exchange with empty subject token.
	_, err = exchanger.Exchange(ctx, ExchangeRequest{
		GrantType:         GrantTypeTokenExchange,
		SubjectToken:      "", // missing!
		ServiceAccountFQN: "ns/sa",
		AgentName:         "agent",
	})
	if err != ErrMissingUserContext {
		t.Errorf("expected ErrMissingUserContext, got %v", err)
	}
}

func TestOBO_InvalidGrantType(t *testing.T) {
	broker := NewBroker(BrokerConfig{Issuer: "https://ai-keeper.io"})
	exchanger := NewExchanger(broker)
	ctx := context.Background()

	_, err := exchanger.Exchange(ctx, ExchangeRequest{
		GrantType:    "authorization_code", // wrong grant type
		SubjectToken: "some-token",
	})
	if err != ErrInvalidGrantType {
		t.Errorf("expected ErrInvalidGrantType, got %v", err)
	}
}

func TestOBO_SANotAllowedOBO(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	// Register SA without OBO permission.
	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "ns/no-obo-sa",
		TenantID:        "tenant-1",
		TokenLifetime:   10 * time.Minute,
		AllowOnBehalfOf: false, // OBO not allowed
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	provider := &mockOIDCVerifier{
		issuer: "https://idp.example.com",
		verifyFn: func(_ context.Context, _ string, _ string) (*TokenClaims, error) {
			return &TokenClaims{Subject: "user@example.com"}, nil
		},
	}
	broker.RegisterProvider(provider)

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	_, err = exchanger.Exchange(ctx, ExchangeRequest{
		GrantType:         GrantTypeTokenExchange,
		SubjectToken:      "user-token",
		ServiceAccountFQN: "ns/no-obo-sa",
		AgentName:         "agent",
	})
	if err != ErrOBONotAllowed {
		t.Errorf("expected ErrOBONotAllowed, got %v", err)
	}
}

func TestOBO_GetTokenForTool_OBOMode(t *testing.T) {
	// B3.3: oauth2_obo mode must use OBO token, not SA token.
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	provider := &mockOIDCVerifier{
		issuer: "https://idp.example.com",
		verifyFn: func(_ context.Context, _ string, _ string) (*TokenClaims, error) {
			return &TokenClaims{
				Subject:   "bob@corp.com",
				Issuer:    "https://idp.example.com",
				ExpiresAt: time.Now().Add(time.Hour),
				TokenID:   "user-tok",
			}, nil
		},
	}
	broker.RegisterProvider(provider)

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "ns/obo-sa",
		TenantID:        "tenant-1",
		TokenLifetime:   10 * time.Minute,
		AllowOnBehalfOf: true,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	// Call with OBO mode and user token present.
	claims, err := exchanger.GetTokenForTool(ctx, ToolAuthConfig{
		Mode:     AuthModeOAuth2OBO,
		Resource: "https://tool.example.com",
		Audience: "tool-aud",
	}, "user-id-token", "ns/obo-sa", "legal-copilot")
	if err != nil {
		t.Fatalf("GetTokenForTool: %v", err)
	}

	// Must contain onBehalfOf (the user identity).
	if claims.OnBehalfOf != "bob@corp.com" {
		t.Errorf("OnBehalfOf: got %s, want bob@corp.com", claims.OnBehalfOf)
	}
	if claims.AgentName != "legal-copilot" {
		t.Errorf("AgentName: got %s, want legal-copilot", claims.AgentName)
	}
}

func TestOBO_GetTokenForTool_OBOMode_NoUserToken_Returns401(t *testing.T) {
	// B3.3 + B3.4: oauth2_obo with missing user token must fail.
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "ns/obo-sa",
		TenantID:        "tenant-1",
		TokenLifetime:   10 * time.Minute,
		AllowOnBehalfOf: true,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	// OBO mode but no user token — must return 401.
	_, err = exchanger.GetTokenForTool(ctx, ToolAuthConfig{
		Mode: AuthModeOAuth2OBO,
	}, "", "ns/obo-sa", "agent") // empty user token
	if err != ErrMissingUserContext {
		t.Errorf("expected ErrMissingUserContext, got %v", err)
	}
}

func TestOBO_GetTokenForTool_SAMode(t *testing.T) {
	// service_account mode: issues a standard SA token (no onBehalfOf).
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:           "ns/regular-sa",
		TenantID:      "tenant-1",
		TokenLifetime: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	claims, err := exchanger.GetTokenForTool(ctx, ToolAuthConfig{
		Mode: AuthModeServiceAccount,
	}, "", "ns/regular-sa", "agent")
	if err != nil {
		t.Fatalf("GetTokenForTool SA mode: %v", err)
	}

	// SA mode: onBehalfOf should be empty.
	if claims.OnBehalfOf != "" {
		t.Errorf("OnBehalfOf should be empty for SA mode, got %s", claims.OnBehalfOf)
	}
	if claims.ServiceAccount != "ns/regular-sa" {
		t.Errorf("ServiceAccount: got %s, want ns/regular-sa", claims.ServiceAccount)
	}
}

func TestOBO_ValidateOBOToken(t *testing.T) {
	broker := NewBroker(BrokerConfig{Issuer: "https://ai-keeper.io"})
	exchanger := NewExchanger(broker)

	// Valid OBO token has onBehalfOf.
	validOBO := &TokenClaims{
		Subject:    "ns/sa",
		OnBehalfOf: "user@example.com",
		TenantID:   "tenant-1",
	}
	if err := exchanger.ValidateOBOToken(validOBO); err != nil {
		t.Errorf("valid OBO token should pass: %v", err)
	}

	// SA token without onBehalfOf should fail validation.
	saToken := &TokenClaims{
		Subject:  "ns/sa",
		TenantID: "tenant-1",
	}
	if err := exchanger.ValidateOBOToken(saToken); err != ErrSATokenNotPermitted {
		t.Errorf("SA token should fail OBO validation, got %v", err)
	}

	// Nil token.
	if err := exchanger.ValidateOBOToken(nil); err != ErrInvalidToken {
		t.Errorf("nil token should return ErrInvalidToken, got %v", err)
	}
}

func TestOBO_SANotFound(t *testing.T) {
	broker := NewBroker(BrokerConfig{Issuer: "https://ai-keeper.io"})
	exchanger := NewExchanger(broker)
	ctx := context.Background()

	_, err := exchanger.Exchange(ctx, ExchangeRequest{
		GrantType:         GrantTypeTokenExchange,
		SubjectToken:      "token",
		ServiceAccountFQN: "ns/nonexistent",
		AgentName:         "agent",
	})
	if err != ErrSANotFound {
		t.Errorf("expected ErrSANotFound, got %v", err)
	}
}

func TestOBO_NoProviderConfigured(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	// SA with OBO but no OIDC providers registered.
	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "ns/sa",
		TenantID:        "tenant-1",
		TokenLifetime:   10 * time.Minute,
		AllowOnBehalfOf: true,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	_, err = exchanger.Exchange(ctx, ExchangeRequest{
		GrantType:         GrantTypeTokenExchange,
		SubjectToken:      "user-token",
		ServiceAccountFQN: "ns/sa",
		AgentName:         "agent",
	})
	if err == nil {
		t.Error("expected error when no providers configured")
	}
}

func TestOBO_PreservesUserIdentityInChain(t *testing.T) {
	// B3.6: OBO token must contain original user's identity.
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	userID := "employee-12345@acme-corp.com"
	provider := &mockOIDCVerifier{
		issuer: "https://corporate-idp.acme.com",
		verifyFn: func(_ context.Context, _ string, _ string) (*TokenClaims, error) {
			return &TokenClaims{
				Subject:   userID,
				Issuer:    "https://corporate-idp.acme.com",
				ExpiresAt: time.Now().Add(time.Hour),
				TokenID:   "corp-token-1",
				TenantID:  "acme-corp",
			}, nil
		},
	}
	broker.RegisterProvider(provider)

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "legal/copilot-sa",
		TenantID:        "acme-corp",
		TokenLifetime:   5 * time.Minute,
		AllowOnBehalfOf: true,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	// Exchange for multiple tools — each should preserve the same user identity.
	tools := []ToolAuthConfig{
		{Mode: AuthModeOAuth2OBO, Resource: "https://docusign.com/api", Audience: "docusign"},
		{Mode: AuthModeOAuth2OBO, Resource: "https://legal-search.com/api", Audience: "legal-search"},
	}

	for _, tool := range tools {
		claims, err := exchanger.GetTokenForTool(ctx, tool, "user-jwt", "legal/copilot-sa", "legal-copilot")
		if err != nil {
			t.Fatalf("GetTokenForTool for %s: %v", tool.Resource, err)
		}
		if claims.OnBehalfOf != userID {
			t.Errorf("tool %s: OnBehalfOf got %s, want %s", tool.Resource, claims.OnBehalfOf, userID)
		}
		if claims.TenantID != "acme-corp" {
			t.Errorf("tool %s: TenantID got %s, want acme-corp", tool.Resource, claims.TenantID)
		}
	}
}

func TestOBO_DisabledSACannotExchange(t *testing.T) {
	redis := newMockRedisClient()
	broker := NewBroker(BrokerConfig{
		Issuer:     "https://ai-keeper.io",
		Blacklist:  NewRedisTokenBlacklist(redis),
		TokenStore: NewRedisSATokenStore(redis),
	})

	provider := &mockOIDCVerifier{
		issuer: "https://idp.example.com",
		verifyFn: func(_ context.Context, _ string, _ string) (*TokenClaims, error) {
			return &TokenClaims{Subject: "user@example.com"}, nil
		},
	}
	broker.RegisterProvider(provider)

	err := broker.RegisterSA(ServiceAccountInfo{
		FQN:             "ns/disabled-sa",
		TenantID:        "tenant-1",
		TokenLifetime:   10 * time.Minute,
		AllowOnBehalfOf: true,
	})
	if err != nil {
		t.Fatalf("RegisterSA: %v", err)
	}

	// Disable the SA.
	_ = broker.DisableSA("ns/disabled-sa")

	exchanger := NewExchanger(broker)
	ctx := context.Background()

	_, err = exchanger.Exchange(ctx, ExchangeRequest{
		GrantType:         GrantTypeTokenExchange,
		SubjectToken:      "user-token",
		ServiceAccountFQN: "ns/disabled-sa",
		AgentName:         "agent",
	})
	if err == nil {
		t.Error("expected error for disabled SA")
	}
}
