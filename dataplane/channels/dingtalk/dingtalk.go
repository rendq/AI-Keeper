// Package dingtalk implements the DingTalk (钉钉) channel adapter for AIP Gateway.
// It handles DingTalk webhook event subscriptions, signature verification,
// JSON message parsing, and outbound message sending via DingTalk webhook API.
package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DingTalkAdapter handles DingTalk webhook events and sends messages via webhook.
type DingTalkAdapter struct {
	secret string

	// httpClient is used for outbound API calls.
	httpClient *http.Client
}

// InboundMessage represents a parsed incoming message/event from DingTalk.
type InboundMessage struct {
	SenderID       string `json:"senderId"`
	SenderNick     string `json:"senderNick"`
	MsgType        string `json:"msgtype"`
	Text           string `json:"text"`
	ConversationID string `json:"conversationId"`
	IsInAtList     bool   `json:"isInAtList"`
}

// CardMessage represents an action card message for DingTalk.
type CardMessage struct {
	Title       string
	Text        string
	SingleTitle string
	SingleURL   string
}

// Option configures a DingTalkAdapter.
type Option func(*DingTalkAdapter)

// WithSecret sets the signing secret for signature verification.
func WithSecret(secret string) Option {
	return func(a *DingTalkAdapter) { a.secret = secret }
}

// WithHTTPClient sets the HTTP client for outbound API calls.
func WithHTTPClient(c *http.Client) Option {
	return func(a *DingTalkAdapter) { a.httpClient = c }
}

// NewDingTalkAdapter creates a new DingTalk channel adapter.
func NewDingTalkAdapter(opts ...Option) *DingTalkAdapter {
	a := &DingTalkAdapter{
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// VerifySignature verifies DingTalk callback signature.
// DingTalk signature = Base64(HMAC-SHA256(timestamp + "\n" + secret, secret)).
func VerifySignature(timestamp, sign, secret string) error {
	computed := ComputeSignature(timestamp, secret)
	if computed != sign {
		return fmt.Errorf("signature mismatch: expected %s, got %s", computed, sign)
	}
	return nil
}

// ComputeSignature computes DingTalk callback signature.
// Formula: Base64(HMAC-SHA256(timestamp + "\n" + secret))
func ComputeSignature(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// HandleEvent parses a DingTalk inbound JSON event/message body.
func HandleEvent(body []byte) (*InboundMessage, error) {
	// DingTalk sends JSON with nested structure:
	// { "msgtype": "text", "text": {"content": "..."}, "senderId": "...", ... }
	var raw struct {
		MsgType        string `json:"msgtype"`
		SenderID       string `json:"senderId"`
		SenderNick     string `json:"senderNick"`
		ConversationID string `json:"conversationId"`
		IsInAtList     bool   `json:"isInAtList"`
		Text           struct {
			Content string `json:"content"`
		} `json:"text"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse DingTalk JSON event: %w", err)
	}
	if raw.SenderID == "" && raw.MsgType == "" {
		return nil, fmt.Errorf("invalid DingTalk event: missing required fields")
	}

	msg := &InboundMessage{
		SenderID:       raw.SenderID,
		SenderNick:     raw.SenderNick,
		MsgType:        raw.MsgType,
		Text:           raw.Text.Content,
		ConversationID: raw.ConversationID,
		IsInAtList:     raw.IsInAtList,
	}
	return msg, nil
}

// SendTextMessage sends a text message via DingTalk webhook.
func (a *DingTalkAdapter) SendTextMessage(ctx context.Context, webhookURL, content string) error {
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}
	return a.sendWebhook(ctx, webhookURL, payload)
}

// SendCardMessage sends an action card message via DingTalk webhook.
func (a *DingTalkAdapter) SendCardMessage(ctx context.Context, webhookURL string, card CardMessage) error {
	payload := map[string]interface{}{
		"msgtype": "actionCard",
		"actionCard": map[string]string{
			"title":          card.Title,
			"text":           card.Text,
			"singleTitle":    card.SingleTitle,
			"singleURL":      card.SingleURL,
			"btnOrientation": "0",
		},
	}
	return a.sendWebhook(ctx, webhookURL, payload)
}

// sendWebhook posts a message payload to a DingTalk webhook URL.
func (a *DingTalkAdapter) sendWebhook(ctx context.Context, webhookURL string, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal message payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send DingTalk message: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("DingTalk API error: %d - %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}
