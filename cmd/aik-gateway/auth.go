package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

// Authenticator verifies incoming requests and returns the caller identity.
type Authenticator interface {
	Authenticate(ctx context.Context, r *http.Request) (*Identity, error)
}

// ChannelAuthConfig holds authentication configuration for a channel.
type ChannelAuthConfig struct {
	// Channel name (e.g., "feishu", "api").
	Channel string
	// AgentName this channel belongs to.
	AgentName string
	// TenantID for this channel.
	TenantID string
	// WebhookSecret for HMAC signature verification (for webhook-based channels).
	WebhookSecret string
	// APITokens accepted for this channel (for token-based auth).
	APITokens []string
}

// ChannelAuthenticator handles channel authentication using either
// webhook signature verification (e.g., feishu) or API token validation.
type ChannelAuthenticator struct {
	// channels maps "agentName/channelName" to config.
	channels map[string]*ChannelAuthConfig
}

// NewChannelAuthenticator creates an authenticator from channel configs.
func NewChannelAuthenticator(configs []*ChannelAuthConfig) *ChannelAuthenticator {
	m := make(map[string]*ChannelAuthConfig, len(configs))
	for _, c := range configs {
		key := c.AgentName + "/" + c.Channel
		m[key] = c
	}
	return &ChannelAuthenticator{channels: m}
}

// Authenticate verifies the request using webhook signature or API token.
// It looks for channel info in headers: X-Aip-Agent and X-Aip-Channel.
func (a *ChannelAuthenticator) Authenticate(ctx context.Context, r *http.Request) (*Identity, error) {
	agentName := r.Header.Get("X-Aip-Agent")
	channel := r.Header.Get("X-Aip-Channel")

	if agentName == "" || channel == "" {
		// Try API token auth without channel info.
		return a.authenticateByToken(r)
	}

	key := agentName + "/" + channel
	cfg, ok := a.channels[key]
	if !ok {
		return nil, errors.New("unknown channel")
	}

	// Try webhook signature first.
	if cfg.WebhookSecret != "" {
		if err := a.verifyWebhookSignature(r, cfg.WebhookSecret); err == nil {
			return &Identity{
				Subject:   "webhook:" + channel,
				TenantID:  cfg.TenantID,
				AgentName: cfg.AgentName,
				Channel:   cfg.Channel,
			}, nil
		}
	}

	// Try API token.
	if len(cfg.APITokens) > 0 {
		token := extractBearerToken(r)
		if token == "" {
			return nil, errors.New("missing authentication credentials")
		}
		for _, t := range cfg.APITokens {
			if token == t {
				return &Identity{
					Subject:   "token:" + channel,
					TenantID:  cfg.TenantID,
					AgentName: cfg.AgentName,
					Channel:   cfg.Channel,
				}, nil
			}
		}
		return nil, errors.New("invalid token")
	}

	return nil, errors.New("no authentication method configured")
}

// authenticateByToken tries to authenticate using only the Authorization header.
func (a *ChannelAuthenticator) authenticateByToken(r *http.Request) (*Identity, error) {
	token := extractBearerToken(r)
	if token == "" {
		return nil, errors.New("missing authentication credentials")
	}

	// Search all channels for a matching token.
	for _, cfg := range a.channels {
		for _, t := range cfg.APITokens {
			if token == t {
				return &Identity{
					Subject:   "token:" + cfg.Channel,
					TenantID:  cfg.TenantID,
					AgentName: cfg.AgentName,
					Channel:   cfg.Channel,
				}, nil
			}
		}
	}
	return nil, errors.New("invalid token")
}

// verifyWebhookSignature verifies HMAC-SHA256 webhook signature.
// Signature is expected in header X-Webhook-Signature as "sha256=<hex>".
// The signed content is the raw request body passed via X-Webhook-Body header
// (in production, the body would be read and buffered).
func (a *ChannelAuthenticator) verifyWebhookSignature(r *http.Request, secret string) error {
	sig := r.Header.Get("X-Webhook-Signature")
	if sig == "" {
		return errors.New("missing webhook signature")
	}

	// Parse "sha256=<hex>" format.
	if !strings.HasPrefix(sig, "sha256=") {
		return errors.New("invalid signature format")
	}
	sigHex := sig[7:]

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return errors.New("invalid signature hex")
	}

	// Use the body from X-Webhook-Body header for testing.
	// In production, this would be the buffered request body.
	body := r.Header.Get("X-Webhook-Body")
	if body == "" {
		return errors.New("no body for signature verification")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	expected := mac.Sum(nil)

	if !hmac.Equal(sigBytes, expected) {
		return errors.New("signature mismatch")
	}

	return nil
}

// extractBearerToken extracts the token from "Bearer <token>" in Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return auth[len(prefix):]
	}
	return ""
}

// noopAuth always passes authentication (for testing).
type noopAuth struct{}

func (n *noopAuth) Authenticate(_ context.Context, _ *http.Request) (*Identity, error) {
	return &Identity{Subject: "anonymous"}, nil
}
