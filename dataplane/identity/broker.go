// Package identity implements the Identity Broker for the AIP data plane.
//
// It provides:
//   - Multi-OIDC IdP verification (multiple IdPs can be configured simultaneously)
//   - ServiceAccount token registry with enforced lifetime
//   - Token revocation via Redis blacklist (SA deletion revokes within 30s)
//   - Token claims embedding: tenantId, agentName, onBehalfOf
//
// Requirements: B3.2, B3.5, B3.6, C8 (short-lived tokens)
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Errors returned by the Identity Broker.
var (
	ErrTokenExpired    = errors.New("identity: token expired")
	ErrTokenRevoked    = errors.New("identity: token revoked")
	ErrInvalidToken    = errors.New("identity: invalid token")
	ErrNoProvider      = errors.New("identity: no matching OIDC provider configured")
	ErrSANotFound      = errors.New("identity: service account not found")
	ErrSADisabled      = errors.New("identity: service account disabled")
	ErrLifetimeExceed  = errors.New("identity: requested lifetime exceeds maximum")
)

// MaxTokenLifetime is the hard cap on SA token lifetime (C8: short-lived).
const MaxTokenLifetime = 1 * time.Hour

// DefaultTokenLifetime is the default SA token lifetime if not specified.
const DefaultTokenLifetime = 15 * time.Minute

// TokenClaims are embedded in every token issued by the Identity Broker (B3.6).
type TokenClaims struct {
	TenantID   string `json:"tenantId"`
	AgentName  string `json:"agentName"`
	OnBehalfOf string `json:"onBehalfOf,omitempty"`

	// Standard claims
	Subject   string    `json:"sub"`
	Issuer    string    `json:"iss"`
	Audience  string    `json:"aud"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
	TokenID   string    `json:"jti"`

	// ServiceAccount FQN (namespace/name) if this is a SA token.
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

// OIDCProviderConfig configures a single OIDC identity provider.
type OIDCProviderConfig struct {
	// Issuer is the OIDC issuer URL (e.g. https://accounts.google.com).
	Issuer string
	// ClientID is the expected audience / client_id for this provider.
	ClientID string
	// JWKSURL overrides the auto-discovered JWKS endpoint.
	JWKSURL string
}

// OIDCVerifier validates OIDC JWT tokens from a specific IdP.
type OIDCVerifier interface {
	// Verify validates a raw JWT token and returns claims if valid.
	Verify(ctx context.Context, rawToken string, audience string) (*TokenClaims, error)
	// Issuer returns the issuer URL this verifier handles.
	Issuer() string
}

// ServiceAccountInfo holds metadata about a registered service account.
type ServiceAccountInfo struct {
	// FQN is the namespace/name identifier.
	FQN string
	// TenantID is the owning tenant.
	TenantID string
	// TokenLifetime is the maximum token duration for this SA.
	TokenLifetime time.Duration
	// AllowOnBehalfOf indicates whether OBO is enabled for this SA.
	AllowOnBehalfOf bool
	// Disabled indicates the SA has been disabled (tokens should be revoked).
	Disabled bool
	// CreatedAt is when the SA was registered.
	CreatedAt time.Time
}

// TokenBlacklist provides a mechanism to revoke tokens using Redis.
// Tokens are added to a blacklist set and checked on each verification.
type TokenBlacklist interface {
	// Add adds a token ID to the blacklist with the given TTL.
	// The TTL should match the remaining token lifetime.
	Add(ctx context.Context, tokenID string, ttl time.Duration) error
	// AddForSA adds all tokens for a service account to the blacklist.
	// Returns the number of tokens blacklisted.
	AddForSA(ctx context.Context, saFQN string, ttl time.Duration) (int, error)
	// IsRevoked checks if a token ID is in the blacklist.
	IsRevoked(ctx context.Context, tokenID string) (bool, error)
}

// SATokenStore tracks issued tokens for service accounts,
// enabling bulk revocation when an SA is deleted.
type SATokenStore interface {
	// Store records a token issued for a service account.
	Store(ctx context.Context, saFQN string, tokenID string, expiresAt time.Time) error
	// ListActive returns all non-expired token IDs for a service account.
	ListActive(ctx context.Context, saFQN string) ([]string, error)
	// Remove removes a specific token from the store.
	Remove(ctx context.Context, saFQN string, tokenID string) error
	// RemoveAll removes all tokens for a service account.
	RemoveAll(ctx context.Context, saFQN string) error
}

// Broker is the Identity Broker that coordinates OIDC verification,
// SA token issuance, and token revocation.
type Broker struct {
	mu        sync.RWMutex
	providers map[string]OIDCVerifier // issuer → verifier
	saStore   map[string]*ServiceAccountInfo
	blacklist TokenBlacklist
	tokens    SATokenStore
	issuer    string // our own issuer identifier
}

// BrokerConfig holds configuration for creating a new Broker.
type BrokerConfig struct {
	// Issuer is the issuer identifier for tokens we issue.
	Issuer string
	// Blacklist is the token blacklist implementation (Redis-backed).
	Blacklist TokenBlacklist
	// TokenStore tracks issued SA tokens for bulk revocation.
	TokenStore SATokenStore
}

// NewBroker creates a new Identity Broker instance.
func NewBroker(cfg BrokerConfig) *Broker {
	return &Broker{
		providers: make(map[string]OIDCVerifier),
		saStore:   make(map[string]*ServiceAccountInfo),
		blacklist: cfg.Blacklist,
		tokens:    cfg.TokenStore,
		issuer:    cfg.Issuer,
	}
}

// RegisterProvider adds an OIDC verifier for a given issuer.
// Multiple providers can be registered simultaneously (B3.2).
func (b *Broker) RegisterProvider(v OIDCVerifier) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.providers[v.Issuer()] = v
}

// RemoveProvider removes an OIDC verifier by issuer URL.
func (b *Broker) RemoveProvider(issuer string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.providers, issuer)
}

// RegisterSA registers a service account in the Identity Broker (A7.2).
func (b *Broker) RegisterSA(sa ServiceAccountInfo) error {
	if sa.TokenLifetime <= 0 {
		sa.TokenLifetime = DefaultTokenLifetime
	}
	if sa.TokenLifetime > MaxTokenLifetime {
		return ErrLifetimeExceed
	}
	if sa.CreatedAt.IsZero() {
		sa.CreatedAt = time.Now()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saStore[sa.FQN] = &sa
	return nil
}

// GetSA returns the service account info for a given FQN.
func (b *Broker) GetSA(fqn string) (*ServiceAccountInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	sa, ok := b.saStore[fqn]
	if !ok {
		return nil, ErrSANotFound
	}
	return sa, nil
}

// DisableSA marks a service account as disabled.
func (b *Broker) DisableSA(fqn string) error {
	b.mu.Lock()
	sa, ok := b.saStore[fqn]
	if !ok {
		b.mu.Unlock()
		return ErrSANotFound
	}
	sa.Disabled = true
	b.mu.Unlock()
	return nil
}

// DeleteSA removes a service account and revokes all its tokens within 30s (B3.5).
func (b *Broker) DeleteSA(ctx context.Context, fqn string) (int, error) {
	b.mu.Lock()
	sa, ok := b.saStore[fqn]
	if !ok {
		b.mu.Unlock()
		return 0, ErrSANotFound
	}
	sa.Disabled = true
	delete(b.saStore, fqn)
	b.mu.Unlock()

	// Revoke all outstanding tokens for this SA via blacklist.
	revoked, err := b.revokeAllTokens(ctx, fqn)
	if err != nil {
		return revoked, fmt.Errorf("identity: revoke tokens for SA %s: %w", fqn, err)
	}
	return revoked, nil
}

// revokeAllTokens blacklists all active tokens for a service account.
func (b *Broker) revokeAllTokens(ctx context.Context, saFQN string) (int, error) {
	if b.blacklist == nil {
		return 0, nil
	}
	count, err := b.blacklist.AddForSA(ctx, saFQN, MaxTokenLifetime)
	if err != nil {
		return 0, err
	}
	// Clean up token store.
	if b.tokens != nil {
		_ = b.tokens.RemoveAll(ctx, saFQN)
	}
	return count, nil
}

// VerifyToken verifies an inbound OIDC token against configured providers.
// It checks the token blacklist and returns parsed claims.
func (b *Broker) VerifyToken(ctx context.Context, rawToken string, issuer string, audience string) (*TokenClaims, error) {
	b.mu.RLock()
	verifier, ok := b.providers[issuer]
	b.mu.RUnlock()

	if !ok {
		return nil, ErrNoProvider
	}

	claims, err := verifier.Verify(ctx, rawToken, audience)
	if err != nil {
		return nil, fmt.Errorf("identity: verify token: %w", err)
	}

	// Check if the token has been revoked.
	if b.blacklist != nil && claims.TokenID != "" {
		revoked, err := b.blacklist.IsRevoked(ctx, claims.TokenID)
		if err != nil {
			// fail-closed: if we can't check the blacklist, deny the token
			return nil, fmt.Errorf("identity: blacklist check failed: %w", err)
		}
		if revoked {
			return nil, ErrTokenRevoked
		}
	}

	// Check expiration.
	if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return claims, nil
}

// IssueToken creates a new SA token with embedded claims (B3.6).
// The token includes tenantId, agentName, and onBehalfOf.
func (b *Broker) IssueToken(ctx context.Context, saFQN string, agentName string, onBehalfOf string) (*TokenClaims, error) {
	b.mu.RLock()
	sa, ok := b.saStore[saFQN]
	b.mu.RUnlock()
	if !ok {
		return nil, ErrSANotFound
	}
	if sa.Disabled {
		return nil, ErrSADisabled
	}

	now := time.Now()
	tokenID, err := generateTokenID()
	if err != nil {
		return nil, fmt.Errorf("identity: generate token ID: %w", err)
	}

	claims := &TokenClaims{
		TenantID:       sa.TenantID,
		AgentName:      agentName,
		OnBehalfOf:     onBehalfOf,
		Subject:        saFQN,
		Issuer:         b.issuer,
		IssuedAt:       now,
		ExpiresAt:      now.Add(sa.TokenLifetime),
		TokenID:        tokenID,
		ServiceAccount: saFQN,
	}

	// Track the issued token for later bulk revocation.
	if b.tokens != nil {
		if err := b.tokens.Store(ctx, saFQN, tokenID, claims.ExpiresAt); err != nil {
			return nil, fmt.Errorf("identity: store token: %w", err)
		}
	}

	return claims, nil
}

// RevokeToken revokes a single token by ID.
func (b *Broker) RevokeToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	if b.blacklist == nil {
		return nil
	}
	return b.blacklist.Add(ctx, tokenID, ttl)
}

// generateTokenID creates a cryptographically random token ID.
func generateTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
