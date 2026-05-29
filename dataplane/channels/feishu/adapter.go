// Package feishu implements the Feishu (飞书/Lark) channel adapter for AIP Gateway.
// It handles Feishu webhook event subscriptions, signature verification,
// URL verification challenges, message parsing, and card message responses.
package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Adapter handles Feishu webhook events and sends card message responses.
type Adapter struct {
	appID       string
	appSecret   string
	verifyToken string

	// rateLimit configuration aligned with Agent.spec.channels.
	rateLimit *RateLimitConfig

	mu          sync.RWMutex
	rateBuckets map[string]*tokenBucket

	// handler is called when a valid message event is received.
	handler MessageHandler
}

// RateLimitConfig holds rate limit settings aligned with Agent.spec.channels[].rateLimit.
type RateLimitConfig struct {
	RequestsPerMinute int
	ConcurrentSessions int
}

// MessageHandler is called when the adapter receives a valid user message.
type MessageHandler func(ctx context.Context, msg *IncomingMessage) (*CardMessage, error)

// IncomingMessage represents a parsed incoming message from Feishu.
type IncomingMessage struct {
	MessageID   string
	ChatID      string
	ChatType    string // "p2p" or "group"
	SenderID    string
	SenderType  string
	Content     string // extracted text content
	MentionIDs  []string
	Timestamp   time.Time
	EventID     string
	TenantKey   string
}

// Option configures an Adapter.
type Option func(*Adapter)

// WithAppID sets the app ID.
func WithAppID(id string) Option {
	return func(a *Adapter) { a.appID = id }
}

// WithAppSecret sets the app secret used for signature verification.
func WithAppSecret(secret string) Option {
	return func(a *Adapter) { a.appSecret = secret }
}

// WithVerifyToken sets the verification token for challenge validation.
func WithVerifyToken(token string) Option {
	return func(a *Adapter) { a.verifyToken = token }
}

// WithRateLimit sets the rate limit configuration.
func WithRateLimit(cfg *RateLimitConfig) Option {
	return func(a *Adapter) { a.rateLimit = cfg }
}

// WithHandler sets the message handler.
func WithHandler(h MessageHandler) Option {
	return func(a *Adapter) { a.handler = h }
}

// NewAdapter creates a new Feishu channel adapter.
func NewAdapter(opts ...Option) *Adapter {
	a := &Adapter{
		rateLimit: &RateLimitConfig{
			RequestsPerMinute:  60,
			ConcurrentSessions: 10,
		},
		rateBuckets: make(map[string]*tokenBucket),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// ServeHTTP handles incoming Feishu webhook requests.
func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the raw event envelope.
	var envelope eventEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "Bad Request: invalid JSON", http.StatusBadRequest)
		return
	}

	// Handle URL verification challenge.
	if envelope.Type == "url_verification" {
		a.handleChallenge(w, &envelope)
		return
	}

	// Verify signature if app secret is configured.
	if a.appSecret != "" {
		if err := a.verifySignature(r, body, &envelope); err != nil {
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	// Check rate limiting.
	tenantKey := ""
	if envelope.Header != nil {
		tenantKey = envelope.Header.TenantKey
	}
	if !a.allowRequest(tenantKey) {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Route by schema version and event type.
	if envelope.Schema == "2.0" {
		a.handleV2Event(w, r, body, &envelope)
		return
	}

	// V1 event format.
	a.handleV1Event(w, r, body)
}

// handleChallenge handles the Feishu URL verification challenge.
func (a *Adapter) handleChallenge(w http.ResponseWriter, env *eventEnvelope) {
	// Optionally verify the token.
	if a.verifyToken != "" && env.Token != a.verifyToken {
		http.Error(w, "Unauthorized: invalid verify token", http.StatusUnauthorized)
		return
	}

	resp := map[string]string{"challenge": env.Challenge}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleV2Event processes Feishu event subscription v2.0 format.
func (a *Adapter) handleV2Event(w http.ResponseWriter, r *http.Request, body []byte, env *eventEnvelope) {
	// Check event type.
	if env.Header.EventType != "im.message.receive_v1" {
		// Acknowledge unknown events.
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse the full event.
	var event messageEventV2
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Bad Request: invalid event body", http.StatusBadRequest)
		return
	}

	msg, err := a.parseV2Message(&event)
	if err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if a.handler == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	card, err := a.handler(r.Context(), msg)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if card != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card.ToFeishuResponse())
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// handleV1Event processes Feishu event subscription v1.0 format.
func (a *Adapter) handleV1Event(w http.ResponseWriter, r *http.Request, body []byte) {
	var event messageEventV1
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Bad Request: invalid event body", http.StatusBadRequest)
		return
	}

	if event.Event.Type != "message" {
		w.WriteHeader(http.StatusOK)
		return
	}

	msg := &IncomingMessage{
		MessageID:  event.Event.MsgType,
		ChatID:     event.Event.OpenChatID,
		ChatType:   event.Event.ChatType,
		SenderID:   event.Event.OpenID,
		Content:    extractTextContent(event.Event.Text),
		EventID:    event.UUID,
		TenantKey:  event.Event.TenantKey,
	}

	if a.handler == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	card, err := a.handler(r.Context(), msg)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if card != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card.ToFeishuResponse())
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// parseV2Message extracts IncomingMessage from a v2 event.
func (a *Adapter) parseV2Message(event *messageEventV2) (*IncomingMessage, error) {
	content, err := ParseMessageContent(event.Event.Message.MessageType, event.Event.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("parse content: %w", err)
	}

	var mentions []string
	for _, m := range event.Event.Message.Mentions {
		mentions = append(mentions, m.ID.OpenID)
	}

	var ts time.Time
	if event.Event.Message.CreateTime != "" {
		// Feishu timestamps are milliseconds since epoch.
		if t, err := parseTimestamp(event.Event.Message.CreateTime); err == nil {
			ts = t
		}
	}

	return &IncomingMessage{
		MessageID:  event.Event.Message.MessageID,
		ChatID:     event.Event.Message.ChatID,
		ChatType:   event.Event.Message.ChatType,
		SenderID:   event.Event.Sender.SenderID.OpenID,
		SenderType: event.Event.Sender.SenderType,
		Content:    content,
		MentionIDs: mentions,
		Timestamp:  ts,
		EventID:    event.Header.EventID,
		TenantKey:  event.Header.TenantKey,
	}, nil
}

// allowRequest implements rate limiting per tenant.
func (a *Adapter) allowRequest(tenantKey string) bool {
	if a.rateLimit == nil {
		return true
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	b, ok := a.rateBuckets[tenantKey]
	if !ok {
		b = &tokenBucket{
			tokens:     float64(a.rateLimit.RequestsPerMinute),
			maxTokens:  float64(a.rateLimit.RequestsPerMinute),
			refillRate: float64(a.rateLimit.RequestsPerMinute) / 60.0,
			lastTime:   time.Now(),
		}
		a.rateBuckets[tenantKey] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastTime = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// tokenBucket implements a simple token bucket for rate limiting.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastTime   time.Time
}

// --- Event envelope types ---

// eventEnvelope is the outer wrapper for all Feishu events.
type eventEnvelope struct {
	// V1 fields
	Type      string `json:"type,omitempty"`      // "url_verification" or ""
	Token     string `json:"token,omitempty"`     // verification token
	Challenge string `json:"challenge,omitempty"` // challenge value

	// V2 fields
	Schema string       `json:"schema,omitempty"` // "2.0"
	Header *eventHeader `json:"header,omitempty"`
}

type eventHeader struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	CreateTime string `json:"create_time"`
	Token      string `json:"token"`
	AppID      string `json:"app_id"`
	TenantKey  string `json:"tenant_key"`
}

// messageEventV2 is the full v2 event for im.message.receive_v1.
type messageEventV2 struct {
	Schema string       `json:"schema"`
	Header *eventHeader `json:"header"`
	Event  struct {
		Sender struct {
			SenderID struct {
				UnionID string `json:"union_id"`
				UserID  string `json:"user_id"`
				OpenID  string `json:"open_id"`
			} `json:"sender_id"`
			SenderType string `json:"sender_type"`
			TenantKey  string `json:"tenant_key"`
		} `json:"sender"`
		Message struct {
			MessageID   string `json:"message_id"`
			RootID      string `json:"root_id"`
			ParentID    string `json:"parent_id"`
			CreateTime  string `json:"create_time"`
			ChatID      string `json:"chat_id"`
			ChatType    string `json:"chat_type"`
			MessageType string `json:"message_type"`
			Content     string `json:"content"`
			Mentions    []struct {
				Key  string `json:"key"`
				ID   struct {
					UnionID string `json:"union_id"`
					UserID  string `json:"user_id"`
					OpenID  string `json:"open_id"`
				} `json:"id"`
				Name      string `json:"name"`
				TenantKey string `json:"tenant_key"`
			} `json:"mentions"`
		} `json:"message"`
	} `json:"event"`
}

// messageEventV1 is the v1 event format.
type messageEventV1 struct {
	UUID  string `json:"uuid"`
	Token string `json:"token"`
	Event struct {
		Type       string `json:"type"`
		MsgType    string `json:"msg_type"`
		Text       string `json:"text"`
		OpenID     string `json:"open_id"`
		OpenChatID string `json:"open_chat_id"`
		ChatType   string `json:"chat_type"`
		TenantKey  string `json:"tenant_key"`
	} `json:"event"`
}
