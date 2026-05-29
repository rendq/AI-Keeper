// Package teams implements the Microsoft Teams channel adapter for AIP Gateway.
// It handles Teams Bot Framework webhook activities, JWT token verification,
// JSON activity parsing, and outbound message replies via Bot Framework REST API.
package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TeamsAdapter handles Microsoft Teams Bot Framework webhooks and sends replies.
type TeamsAdapter struct {
	appID       string
	appPassword string

	// httpClient is used for outbound API calls.
	httpClient *http.Client
}

// Activity represents a Microsoft Teams Bot Framework Activity.
type Activity struct {
	Type         string       `json:"type"`
	ID           string       `json:"id"`
	From         ChannelActor `json:"from"`
	Conversation Conversation `json:"conversation"`
	Text         string       `json:"text"`
	ServiceURL   string       `json:"serviceUrl"`
	ChannelID    string       `json:"channelId"`
}

// ChannelActor represents the sender of an activity.
type ChannelActor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Conversation represents the conversation context for an activity.
type Conversation struct {
	ID string `json:"id"`
}

// AdaptiveCard represents a Microsoft Adaptive Card payload.
type AdaptiveCard struct {
	Body []CardElement `json:"body"`
}

// CardElement represents an element within an Adaptive Card body.
type CardElement struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Size string `json:"size,omitempty"`
}

// Option configures a TeamsAdapter.
type Option func(*TeamsAdapter)

// WithAppID sets the Microsoft App ID for token verification.
func WithAppID(appID string) Option {
	return func(a *TeamsAdapter) { a.appID = appID }
}

// WithAppPassword sets the Microsoft App Password.
func WithAppPassword(appPassword string) Option {
	return func(a *TeamsAdapter) { a.appPassword = appPassword }
}

// WithHTTPClient sets the HTTP client for outbound API calls.
func WithHTTPClient(c *http.Client) Option {
	return func(a *TeamsAdapter) { a.httpClient = c }
}

// NewTeamsAdapter creates a new Teams channel adapter.
func NewTeamsAdapter(opts ...Option) *TeamsAdapter {
	a := &TeamsAdapter{
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// VerifyToken verifies the Teams Bot Framework JWT Bearer token format.
// This is a stub implementation that validates the Bearer token format
// and checks that appID and appPassword are provided.
func VerifyToken(authHeader, appID, appPassword string) error {
	if appID == "" {
		return fmt.Errorf("appID is required for token verification")
	}
	if appPassword == "" {
		return fmt.Errorf("appPassword is required for token verification")
	}
	if authHeader == "" {
		return fmt.Errorf("authorization header is empty")
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return fmt.Errorf("authorization header must start with 'Bearer '")
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return fmt.Errorf("bearer token is empty")
	}
	// Stub: In production, validate the JWT signature against Microsoft's
	// OpenID metadata keys and verify claims (iss, aud, etc.).
	return nil
}

// HandleActivity parses a Teams Bot Framework Activity JSON body.
func HandleActivity(body []byte) (*Activity, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty activity body")
	}

	var activity Activity
	if err := json.Unmarshal(body, &activity); err != nil {
		return nil, fmt.Errorf("parse Teams activity JSON: %w", err)
	}
	if activity.Type == "" {
		return nil, fmt.Errorf("invalid activity: missing type field")
	}
	return &activity, nil
}

// SendTextReply sends a text reply to a Teams conversation via Bot Framework REST API.
func (a *TeamsAdapter) SendTextReply(ctx context.Context, serviceURL, conversationID, activityID, text string) error {
	payload := map[string]string{
		"type": "message",
		"text": text,
	}
	return a.postReply(ctx, serviceURL, conversationID, activityID, payload)
}

// SendCardReply sends an Adaptive Card reply to a Teams conversation via Bot Framework REST API.
func (a *TeamsAdapter) SendCardReply(ctx context.Context, serviceURL, conversationID, activityID string, card AdaptiveCard) error {
	attachment := map[string]interface{}{
		"contentType": "application/vnd.microsoft.card.adaptive",
		"content": map[string]interface{}{
			"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
			"type":    "AdaptiveCard",
			"version": "1.4",
			"body":    card.Body,
		},
	}
	payload := map[string]interface{}{
		"type":        "message",
		"attachments": []interface{}{attachment},
	}
	return a.postReplyAny(ctx, serviceURL, conversationID, activityID, payload)
}

// postReply posts a string-valued reply payload to Bot Framework REST API.
func (a *TeamsAdapter) postReply(ctx context.Context, serviceURL, conversationID, activityID string, payload map[string]string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal reply payload: %w", err)
	}
	return a.doPost(ctx, serviceURL, conversationID, activityID, data)
}

// postReplyAny posts an arbitrary reply payload to Bot Framework REST API.
func (a *TeamsAdapter) postReplyAny(ctx context.Context, serviceURL, conversationID, activityID string, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal reply payload: %w", err)
	}
	return a.doPost(ctx, serviceURL, conversationID, activityID, data)
}

// doPost performs the HTTP POST to the Bot Framework reply endpoint.
func (a *TeamsAdapter) doPost(ctx context.Context, serviceURL, conversationID, activityID string, data []byte) error {
	url := fmt.Sprintf("%s/v3/conversations/%s/activities/%s", strings.TrimRight(serviceURL, "/"), conversationID, activityID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send Teams reply: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Teams API error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
