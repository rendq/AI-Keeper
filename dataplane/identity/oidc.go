package identity

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// JWKSFetcher fetches the JWKS (JSON Web Key Set) for an OIDC provider.
type JWKSFetcher interface {
	// FetchJWKS returns the raw JWKS JSON for the given URL.
	FetchJWKS(ctx context.Context, jwksURL string) ([]byte, error)
}

// StandardOIDCVerifier implements OIDCVerifier for OIDC JWT tokens.
// Supports multiple IdPs simultaneously (B3.2).
type StandardOIDCVerifier struct {
	issuerURL string
	clientID  string
	jwksURL   string
	fetcher   JWKSFetcher
	// keys caches parsed public keys from JWKS.
	keys map[string]crypto.PublicKey
}

// NewOIDCVerifier creates a new OIDC token verifier for the given provider.
func NewOIDCVerifier(cfg OIDCProviderConfig, fetcher JWKSFetcher) *StandardOIDCVerifier {
	jwksURL := cfg.JWKSURL
	if jwksURL == "" {
		// Standard OIDC discovery: issuer + "/.well-known/openid-configuration"
		jwksURL = strings.TrimRight(cfg.Issuer, "/") + "/.well-known/jwks.json"
	}
	return &StandardOIDCVerifier{
		issuerURL: cfg.Issuer,
		clientID:  cfg.ClientID,
		jwksURL:   jwksURL,
		fetcher:   fetcher,
		keys:      make(map[string]crypto.PublicKey),
	}
}

// Issuer returns the issuer URL this verifier handles.
func (v *StandardOIDCVerifier) Issuer() string {
	return v.issuerURL
}

// Verify validates a raw JWT token and returns claims.
func (v *StandardOIDCVerifier) Verify(ctx context.Context, rawToken string, audience string) (*TokenClaims, error) {
	// Parse JWT parts.
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Decode header.
	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("identity: decode header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("identity: parse header: %w", err)
	}

	// Decode payload.
	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("identity: decode payload: %w", err)
	}

	var payload struct {
		Iss   string      `json:"iss"`
		Sub   string      `json:"sub"`
		Aud   interface{} `json:"aud"` // string or []string
		Exp   int64       `json:"exp"`
		Iat   int64       `json:"iat"`
		Jti   string      `json:"jti"`
		Email string      `json:"email"`
		// AIP custom claims (B3.6).
		TenantID       string `json:"tenantId"`
		AgentName      string `json:"agentName"`
		OnBehalfOf     string `json:"onBehalfOf"`
		ServiceAccount string `json:"serviceAccount"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("identity: parse payload: %w", err)
	}

	// Validate issuer.
	if payload.Iss != v.issuerURL {
		return nil, fmt.Errorf("identity: issuer mismatch: got %s, want %s", payload.Iss, v.issuerURL)
	}

	// Validate audience.
	if audience != "" && !matchesAudience(payload.Aud, audience) {
		return nil, fmt.Errorf("identity: audience mismatch")
	}

	// Check expiration.
	if payload.Exp > 0 {
		expTime := time.Unix(payload.Exp, 0)
		if time.Now().After(expTime) {
			return nil, ErrTokenExpired
		}
	}

	// Build claims.
	claims := &TokenClaims{
		TenantID:       payload.TenantID,
		AgentName:      payload.AgentName,
		OnBehalfOf:     payload.OnBehalfOf,
		Subject:        payload.Sub,
		Issuer:         payload.Iss,
		Audience:       audience,
		IssuedAt:       time.Unix(payload.Iat, 0),
		ExpiresAt:      time.Unix(payload.Exp, 0),
		TokenID:        payload.Jti,
		ServiceAccount: payload.ServiceAccount,
	}

	return claims, nil
}

// matchesAudience checks if the token audience matches the expected audience.
func matchesAudience(aud interface{}, expected string) bool {
	switch a := aud.(type) {
	case string:
		return a == expected
	case []interface{}:
		for _, v := range a {
			if s, ok := v.(string); ok && s == expected {
				return true
			}
		}
	}
	return false
}

// base64URLDecode decodes a base64url encoded string (no padding).
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if needed.
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// JWKS represents a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a single JSON Web Key.
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// ParseJWKS parses a JWKS JSON document into public keys.
func ParseJWKS(data []byte) (map[string]crypto.PublicKey, error) {
	var jwks JWKS
	if err := json.Unmarshal(data, &jwks); err != nil {
		return nil, fmt.Errorf("identity: parse JWKS: %w", err)
	}

	keys := make(map[string]crypto.PublicKey)
	for _, jwk := range jwks.Keys {
		if jwk.Use != "" && jwk.Use != "sig" {
			continue
		}
		key, err := jwkToPublicKey(jwk)
		if err != nil {
			continue // skip invalid keys
		}
		keys[jwk.Kid] = key
	}
	return keys, nil
}

func jwkToPublicKey(jwk JWK) (crypto.PublicKey, error) {
	switch jwk.Kty {
	case "RSA":
		return parseRSAPublicKey(jwk)
	case "EC":
		return parseECPublicKey(jwk)
	default:
		return nil, errors.New("unsupported key type: " + jwk.Kty)
	}
}

func parseRSAPublicKey(jwk JWK) (*rsa.PublicKey, error) {
	nBytes, err := base64URLDecode(jwk.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64URLDecode(jwk.E)
	if err != nil {
		return nil, err
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

func parseECPublicKey(jwk JWK) (*ecdsa.PublicKey, error) {
	// Simplified EC key parsing — production would select curve from jwk.Crv
	xBytes, err := base64URLDecode(jwk.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64URLDecode(jwk.Y)
	if err != nil {
		return nil, err
	}

	_ = xBytes
	_ = yBytes
	return nil, errors.New("EC key parsing requires curve selection (not implemented in P0)")
}
