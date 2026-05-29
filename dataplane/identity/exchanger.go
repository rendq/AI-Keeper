// Package identity — Token Exchanger implements RFC 8693 OAuth 2.0 Token Exchange
// for On-Behalf-Of (OBO) flows.
//
// When Tool.spec.authentication.mode=oauth2_obo, the caller MUST use the OBO token
// obtained through this exchanger. Fallback to ServiceAccount tokens is NOT permitted.
// If the end-user context is missing, the exchanger returns 401.
//
// Requirements: B3.1, B3.3, B3.4
package identity

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RFC 8693 grant type for token exchange.
const GrantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"

// RFC 8693 token type identifiers.
const (
	TokenTypeIDToken     = "urn:ietf:params:oauth:token-type:id_token"
	TokenTypeAccessToken = "urn:ietf:params:oauth:token-type:access_token"
)

// Tool authentication modes.
const (
	AuthModeOAuth2OBO      = "oauth2_obo"
	AuthModeServiceAccount = "service_account"
)

// Errors returned by the Token Exchanger.
var (
	ErrMissingUserContext    = errors.New("identity: missing end-user context (401)")
	ErrOBONotAllowed        = errors.New("identity: service account does not allow OBO")
	ErrMissingSubjectToken  = errors.New("identity: subject_token is required for token exchange")
	ErrInvalidGrantType     = errors.New("identity: invalid grant_type for token exchange")
	ErrSATokenNotPermitted  = errors.New("identity: oauth2_obo mode requires OBO token, SA token fallback not permitted")
	ErrExchangeFailed       = errors.New("identity: token exchange failed")
)

// ExchangeRequest represents an RFC 8693 token exchange request.
type ExchangeRequest struct {
	// GrantType must be GrantTypeTokenExchange.
	GrantType string

	// SubjectToken is the end-user's ID token to exchange.
	SubjectToken string

	// SubjectTokenType is the type of the subject token (e.g., TokenTypeIDToken).
	SubjectTokenType string

	// Resource is the target resource (tool endpoint) for which the OBO token is requested.
	Resource string

	// Audience is the intended audience for the exchanged token.
	Audience string

	// Scope is the requested scope for the exchanged token.
	Scope string

	// ServiceAccountFQN is the service account performing the exchange.
	ServiceAccountFQN string

	// AgentName is the agent initiating the exchange.
	AgentName string
}

// ExchangeResponse represents an RFC 8693 token exchange response.
type ExchangeResponse struct {
	// AccessToken is the exchanged OBO token (represented as claims).
	AccessToken *TokenClaims

	// IssuedTokenType is the type of the issued token.
	IssuedTokenType string

	// TokenType is the token type (e.g., "Bearer").
	TokenType string

	// ExpiresIn is the token lifetime in seconds.
	ExpiresIn int
}

// ToolAuthConfig describes the authentication configuration for a Tool.
type ToolAuthConfig struct {
	// Mode is the authentication mode (e.g., "oauth2_obo", "service_account").
	Mode string

	// TokenExchangeRef references the token exchange configuration.
	TokenExchangeRef string

	// Resource is the tool's resource URI.
	Resource string

	// Audience is the expected audience for tokens presented to this tool.
	Audience string
}

// Exchanger implements RFC 8693 On-Behalf-Of token exchange.
// It works with the Identity Broker to exchange end-user tokens
// for downstream OBO tokens that preserve the original user's identity.
type Exchanger struct {
	broker *Broker
}

// NewExchanger creates a new Token Exchanger backed by the given Broker.
func NewExchanger(broker *Broker) *Exchanger {
	return &Exchanger{broker: broker}
}

// Exchange performs an RFC 8693 token exchange.
//
// It takes the end-user's ID token (subject_token) and exchanges it for
// a downstream OBO token that contains the original user's identity in
// the onBehalfOf claim.
//
// Requirements:
//   - B3.1: Uses RFC 8693 token exchange protocol
//   - B3.4: Returns 401 if user context is missing
//   - B3.6: OBO token contains tenantId, agentName, onBehalfOf
func (e *Exchanger) Exchange(ctx context.Context, req ExchangeRequest) (*ExchangeResponse, error) {
	// Validate grant type.
	if req.GrantType != GrantTypeTokenExchange {
		return nil, ErrInvalidGrantType
	}

	// Validate subject token is present (B3.4: missing user context → 401).
	if req.SubjectToken == "" {
		return nil, ErrMissingUserContext
	}

	// Validate service account FQN.
	if req.ServiceAccountFQN == "" {
		return nil, ErrSANotFound
	}

	// Look up the service account and verify OBO is allowed.
	sa, err := e.broker.GetSA(req.ServiceAccountFQN)
	if err != nil {
		return nil, err
	}
	if !sa.AllowOnBehalfOf {
		return nil, ErrOBONotAllowed
	}

	// Extract user identity from the subject token.
	// We need to verify the subject token against configured OIDC providers.
	userIdentity, err := e.extractUserIdentity(ctx, req.SubjectToken)
	if err != nil {
		return nil, fmt.Errorf("identity: extract user identity: %w", err)
	}

	// Issue an OBO token via the broker with the user's identity in onBehalfOf.
	claims, err := e.broker.IssueToken(ctx, req.ServiceAccountFQN, req.AgentName, userIdentity)
	if err != nil {
		return nil, fmt.Errorf("identity: issue OBO token: %w", err)
	}

	// Set audience if specified.
	if req.Audience != "" {
		claims.Audience = req.Audience
	}

	expiresIn := int(time.Until(claims.ExpiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}

	return &ExchangeResponse{
		AccessToken:     claims,
		IssuedTokenType: TokenTypeAccessToken,
		TokenType:       "Bearer",
		ExpiresIn:       expiresIn,
	}, nil
}

// GetTokenForTool obtains the appropriate token for calling a tool based on its auth config.
//
// Requirements:
//   - B3.3: When Tool.spec.authentication.mode=oauth2_obo, MUST use OBO token,
//     SA token fallback is NOT permitted.
//   - B3.4: If requireUserContext=true and user context is missing, return 401.
func (e *Exchanger) GetTokenForTool(ctx context.Context, toolAuth ToolAuthConfig, userToken string, saFQN string, agentName string) (*TokenClaims, error) {
	switch toolAuth.Mode {
	case AuthModeOAuth2OBO:
		// OBO mode: MUST exchange for OBO token, no SA token fallback (B3.3).
		if userToken == "" {
			return nil, ErrMissingUserContext
		}

		resp, err := e.Exchange(ctx, ExchangeRequest{
			GrantType:         GrantTypeTokenExchange,
			SubjectToken:      userToken,
			SubjectTokenType:  TokenTypeIDToken,
			Resource:          toolAuth.Resource,
			Audience:          toolAuth.Audience,
			ServiceAccountFQN: saFQN,
			AgentName:         agentName,
		})
		if err != nil {
			return nil, err
		}
		return resp.AccessToken, nil

	case AuthModeServiceAccount, "":
		// Standard SA token mode — issue a regular SA token.
		claims, err := e.broker.IssueToken(ctx, saFQN, agentName, "")
		if err != nil {
			return nil, err
		}
		return claims, nil

	default:
		return nil, fmt.Errorf("identity: unsupported auth mode: %s", toolAuth.Mode)
	}
}

// ValidateOBOToken checks that a token is a valid OBO token (has onBehalfOf claim).
// This is used to enforce that oauth2_obo tools receive OBO tokens only.
func (e *Exchanger) ValidateOBOToken(claims *TokenClaims) error {
	if claims == nil {
		return ErrInvalidToken
	}
	if claims.OnBehalfOf == "" {
		return ErrSATokenNotPermitted
	}
	return nil
}

// extractUserIdentity verifies the subject token and extracts the user identity.
// It tries all registered OIDC providers in the broker to verify the token.
func (e *Exchanger) extractUserIdentity(ctx context.Context, subjectToken string) (string, error) {
	// Try to decode the token to find the issuer.
	e.broker.mu.RLock()
	providers := make([]OIDCVerifier, 0, len(e.broker.providers))
	for _, v := range e.broker.providers {
		providers = append(providers, v)
	}
	e.broker.mu.RUnlock()

	if len(providers) == 0 {
		return "", ErrNoProvider
	}

	// Try each provider to verify the subject token.
	var lastErr error
	for _, provider := range providers {
		claims, err := provider.Verify(ctx, subjectToken, "")
		if err != nil {
			lastErr = err
			continue
		}
		// Use the subject claim as the user identity for onBehalfOf.
		if claims.Subject != "" {
			return claims.Subject, nil
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("identity: no provider could verify subject token: %w", lastErr)
	}
	return "", ErrInvalidToken
}
