// Package wecom implements the WeCom (企业微信) channel adapter for AIP Gateway.
// It handles WeCom webhook event subscriptions, signature verification,
// XML message parsing, and outbound message sending via WeCom API.
package wecom

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// WeComAdapter handles WeCom webhook events and sends messages via WeCom API.
type WeComAdapter struct {
	token      string
	corpID     string
	corpSecret string
	apiBase    string

	// httpClient is used for outbound API calls.
	httpClient *http.Client
}

// InboundMessage represents a parsed incoming message/event from WeCom.
type InboundMessage struct {
	ToUserName string `xml:"ToUserName"`
	FromUser   string `xml:"FromUserName"`
	CreateTime int64  `xml:"CreateTime"`
	MsgType    string `xml:"MsgType"`
	Content    string `xml:"Content"`
	MsgID      int64  `xml:"MsgId"`
	AgentID    int64  `xml:"AgentID"`
	EventType  string `xml:"Event"`
	EventKey   string `xml:"EventKey"`
}

// CardMessage represents a text card message for WeCom.
type CardMessage struct {
	Title       string
	Description string
	URL         string
	ButtonText  string
}

// Option configures a WeComAdapter.
type Option func(*WeComAdapter)

// WithToken sets the callback verification token.
func WithToken(token string) Option {
	return func(a *WeComAdapter) { a.token = token }
}

// WithCorpID sets the corp ID.
func WithCorpID(id string) Option {
	return func(a *WeComAdapter) { a.corpID = id }
}

// WithCorpSecret sets the corp secret for API access.
func WithCorpSecret(secret string) Option {
	return func(a *WeComAdapter) { a.corpSecret = secret }
}

// WithAPIBase sets the WeCom API base URL (for testing).
func WithAPIBase(base string) Option {
	return func(a *WeComAdapter) { a.apiBase = base }
}

// WithHTTPClient sets the HTTP client for outbound API calls.
func WithHTTPClient(c *http.Client) Option {
	return func(a *WeComAdapter) { a.httpClient = c }
}

// NewWeComAdapter creates a new WeCom channel adapter.
func NewWeComAdapter(opts ...Option) *WeComAdapter {
	a := &WeComAdapter{
		apiBase:    "https://qyapi.weixin.qq.com",
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// VerifySignature verifies WeCom callback URL signature.
// WeCom signature = SHA1(sort(token, timestamp, nonce)).
// For URL verification callbacks, echostr is returned decrypted on success.
func VerifySignature(token, timestamp, nonce, echostr, msgSignature string) (string, error) {
	computed := computeSignature(token, timestamp, nonce)
	if computed != msgSignature {
		return "", fmt.Errorf("signature mismatch: expected %s, got %s", computed, msgSignature)
	}
	// In full implementation, echostr would be decrypted.
	// For non-encrypted mode, return echostr directly.
	return echostr, nil
}

// computeSignature computes WeCom callback signature.
// Formula: SHA1(sort([token, timestamp, nonce]))
func computeSignature(token, timestamp, nonce string) string {
	params := []string{token, timestamp, nonce}
	sort.Strings(params)
	joined := strings.Join(params, "")
	h := sha1.New()
	h.Write([]byte(joined))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// HandleEvent parses a WeCom inbound XML event/message body.
func HandleEvent(body []byte) (*InboundMessage, error) {
	var msg InboundMessage
	if err := xml.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("parse WeCom XML event: %w", err)
	}
	if msg.FromUser == "" && msg.MsgType == "" {
		return nil, fmt.Errorf("invalid WeCom event: missing required fields")
	}
	return &msg, nil
}

// SendTextMessage sends a text message to a user via WeCom API.
func (a *WeComAdapter) SendTextMessage(ctx context.Context, toUser, agentID, content string) error {
	payload := map[string]interface{}{
		"touser":  toUser,
		"msgtype": "text",
		"agentid": agentID,
		"text": map[string]string{
			"content": content,
		},
	}
	return a.sendMessage(ctx, payload)
}

// SendCardMessage sends a text card message to a user via WeCom API.
func (a *WeComAdapter) SendCardMessage(ctx context.Context, toUser, agentID string, card CardMessage) error {
	payload := map[string]interface{}{
		"touser":  toUser,
		"msgtype": "textcard",
		"agentid": agentID,
		"textcard": map[string]string{
			"title":       card.Title,
			"description": card.Description,
			"url":         card.URL,
			"btntxt":      card.ButtonText,
		},
	}
	return a.sendMessage(ctx, payload)
}

// sendMessage posts a message payload to the WeCom send message API.
func (a *WeComAdapter) sendMessage(ctx context.Context, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal message payload: %w", err)
	}

	url := fmt.Sprintf("%s/cgi-bin/message/send", a.apiBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send WeCom message: %w", err)
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
		return fmt.Errorf("WeCom API error: %d - %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}
