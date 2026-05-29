// Package slack implements the Slack channel adapter for AIP Gateway.
// It handles Slack Events API subscriptions, signature verification,
// JSON event parsing, and outbound message sending via Slack API.
package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SlackAdapter handles Slack Events API webhooks and sends messages via Slack API.
type SlackAdapter struct {
	signingSecret string
	botToken      string
	apiBase       string

	// httpClient is used for outbound API calls.
	httpClient *http.Client
}

// InboundMessage represents a parsed incoming message/event from Slack.
type InboundMessage struct {
	UserID    string `json:"user"`
	ChannelID string `json:"channel"`
	Text      string `json:"text"`
	EventType string `json:"type"`
	ThreadTS  string `json:"thread_ts"`
	BotID     string `json:"bot_id"`
}

// Block represents a Slack Block Kit block element.
type Block struct {
	Type string `json:"type"`
	Text *Text  `json:"text,omitempty"`
}

// Text represents a text object within a Block Kit block.
type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Option configures a SlackAdapter.
type Option func(*SlackAdapter)

// WithSigningSecret sets the Slack signing secret for request verification.
func WithSigningSecret(secret string) Option {
	return func(a *SlackAdapter) { a.signingSecret = secret }
}

// WithBotToken sets the bot token for Slack API calls.
func WithBotToken(token string) Option {
	return func(a *SlackAdapter) { a.botToken = token }
}

// WithAPIBase sets the Slack API base URL (for testing).
func WithAPIBase(base string) Option {
	return func(a *SlackAdapter) { a.apiBase = base }
}

// WithHTTPClient sets the HTTP client for outbound API calls.
func WithHTTPClient(c *http.Client) Option {
	return func(a *SlackAdapter) { a.httpClient = c }
}

// NewSlackAdapter creates a new Slack channel adapter.
func NewSlackAdapter(opts ...Option) *SlackAdapter {
	a := &SlackAdapter{
		apiBase:    "https://slack.com/api",
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// VerifySignature verifies the Slack request signature using HMAC-SHA256.
// Slack signature format: v0=HMAC-SHA256("v0:{timestamp}:{body}", signingSecret)
func VerifySignature(signingSecret, timestamp, body, signature string) error {
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, body)

	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(baseString))
	computed := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(computed), []byte(signature)) {
		return fmt.Errorf("signature mismatch: expected %s, got %s", computed, signature)
	}
	return nil
}

// slackEventPayload represents the outer Slack event callback envelope.
type slackEventPayload struct {
	Type  string          `json:"type"`
	Event json.RawMessage `json:"event"`
}

// HandleEvent parses a Slack Events API JSON body (event_callback type).
func HandleEvent(body []byte) (*InboundMessage, error) {
	var payload slackEventPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse Slack event JSON: %w", err)
	}
	if payload.Type != "event_callback" {
		return nil, fmt.Errorf("unsupported event type: %q, expected event_callback", payload.Type)
	}
	if payload.Event == nil {
		return nil, fmt.Errorf("invalid Slack event: missing event field")
	}

	var msg InboundMessage
	if err := json.Unmarshal(payload.Event, &msg); err != nil {
		return nil, fmt.Errorf("parse Slack inner event: %w", err)
	}
	return &msg, nil
}

// SendMessage sends a text message to a Slack channel via chat.postMessage API.
func (a *SlackAdapter) SendMessage(ctx context.Context, channel, text string) error {
	payload := map[string]string{
		"channel": channel,
		"text":    text,
	}
	return a.postMessage(ctx, payload)
}

// SendBlockMessage sends a Block Kit message to a Slack channel via chat.postMessage API.
func (a *SlackAdapter) SendBlockMessage(ctx context.Context, channel string, blocks []Block) error {
	blocksJSON, err := json.Marshal(blocks)
	if err != nil {
		return fmt.Errorf("marshal blocks: %w", err)
	}

	payload := map[string]interface{}{
		"channel": channel,
		"blocks":  json.RawMessage(blocksJSON),
	}
	return a.postMessageAny(ctx, payload)
}

// postMessage posts a string-valued message payload to the Slack chat.postMessage API.
func (a *SlackAdapter) postMessage(ctx context.Context, payload map[string]string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal message payload: %w", err)
	}
	return a.doPost(ctx, data)
}

// postMessageAny posts an arbitrary payload to the Slack chat.postMessage API.
func (a *SlackAdapter) postMessageAny(ctx context.Context, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal message payload: %w", err)
	}
	return a.doPost(ctx, data)
}

// doPost performs the HTTP POST to chat.postMessage and handles the response.
func (a *SlackAdapter) doPost(ctx context.Context, data []byte) error {
	url := fmt.Sprintf("%s/chat.postMessage", a.apiBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+a.botToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send Slack message: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("Slack API error: %s", result.Error)
	}
	return nil
}
