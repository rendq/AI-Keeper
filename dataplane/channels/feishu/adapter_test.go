package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestURLVerificationChallenge(t *testing.T) {
	adapter := NewAdapter(
		WithVerifyToken("test-token"),
	)

	body := `{"type":"url_verification","token":"test-token","challenge":"challenge-value-123"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if resp["challenge"] != "challenge-value-123" {
		t.Errorf("expected challenge 'challenge-value-123', got %q", resp["challenge"])
	}
}

func TestURLVerificationInvalidToken(t *testing.T) {
	adapter := NewAdapter(
		WithVerifyToken("correct-token"),
	)

	body := `{"type":"url_verification","token":"wrong-token","challenge":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSignatureVerification(t *testing.T) {
	appSecret := "my-app-secret"
	timestamp := "1234567890"
	nonce := "random-nonce"
	body := `{"schema":"2.0","header":{"event_id":"ev1","event_type":"im.message.receive_v1","token":"tok","app_id":"app1","tenant_key":"t1"},"event":{"sender":{"sender_id":{"open_id":"ou_123"},"sender_type":"user"},"message":{"message_id":"msg1","chat_id":"oc_123","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}","create_time":"1700000000000"}}}`

	// Compute expected signature.
	sig := computeSignature(timestamp, nonce, appSecret, []byte(body))

	adapter := NewAdapter(
		WithAppSecret(appSecret),
		WithVerifyToken("tok"),
		WithHandler(func(ctx context.Context, msg *IncomingMessage) (*CardMessage, error) {
			return nil, nil
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	req.Header.Set("X-Lark-Request-Timestamp", timestamp)
	req.Header.Set("X-Lark-Request-Nonce", nonce)
	req.Header.Set("X-Lark-Signature", sig)
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSignatureVerificationFailure(t *testing.T) {
	adapter := NewAdapter(
		WithAppSecret("my-secret"),
		WithVerifyToken(""),
	)

	body := `{"schema":"2.0","header":{"event_id":"ev1","event_type":"im.message.receive_v1","token":"","app_id":"app1","tenant_key":"t1"},"event":{}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	req.Header.Set("X-Lark-Request-Timestamp", "123")
	req.Header.Set("X-Lark-Request-Nonce", "nonce")
	req.Header.Set("X-Lark-Signature", "invalid-signature")
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestV2MessageEvent(t *testing.T) {
	var receivedMsg *IncomingMessage
	adapter := NewAdapter(
		WithHandler(func(ctx context.Context, msg *IncomingMessage) (*CardMessage, error) {
			receivedMsg = msg
			card := NewCardMessage("Reply")
			card.AddMarkdown("Hello back!")
			return card, nil
		}),
	)

	body := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_001",
			"event_type": "im.message.receive_v1",
			"create_time": "1700000000000",
			"token": "test",
			"app_id": "app1",
			"tenant_key": "tenant1"
		},
		"event": {
			"sender": {
				"sender_id": {
					"union_id": "union1",
					"user_id": "user1",
					"open_id": "ou_abc"
				},
				"sender_type": "user",
				"tenant_key": "tenant1"
			},
			"message": {
				"message_id": "msg_001",
				"chat_id": "oc_chat1",
				"chat_type": "p2p",
				"message_type": "text",
				"content": "{\"text\":\"Hello AI\"}",
				"create_time": "1700000000000"
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if receivedMsg == nil {
		t.Fatal("handler was not called")
	}
	if receivedMsg.Content != "Hello AI" {
		t.Errorf("expected content 'Hello AI', got %q", receivedMsg.Content)
	}
	if receivedMsg.SenderID != "ou_abc" {
		t.Errorf("expected sender 'ou_abc', got %q", receivedMsg.SenderID)
	}
	if receivedMsg.ChatID != "oc_chat1" {
		t.Errorf("expected chatID 'oc_chat1', got %q", receivedMsg.ChatID)
	}
	if receivedMsg.EventID != "ev_001" {
		t.Errorf("expected eventID 'ev_001', got %q", receivedMsg.EventID)
	}

	// Check card response.
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if resp["msg_type"] != "interactive" {
		t.Errorf("expected msg_type 'interactive', got %v", resp["msg_type"])
	}
}

func TestV2PostMessage(t *testing.T) {
	var receivedMsg *IncomingMessage
	adapter := NewAdapter(
		WithHandler(func(ctx context.Context, msg *IncomingMessage) (*CardMessage, error) {
			receivedMsg = msg
			return nil, nil
		}),
	)

	postContent := `{"title":"Test Title","content":[[{"tag":"text","text":"paragraph one"}],[{"tag":"text","text":"paragraph two"}]]}`
	body := `{
		"schema": "2.0",
		"header": {"event_id":"ev2","event_type":"im.message.receive_v1","token":"t","app_id":"a","tenant_key":"tk"},
		"event": {
			"sender": {"sender_id":{"open_id":"ou_x"},"sender_type":"user"},
			"message": {"message_id":"m2","chat_id":"c2","chat_type":"group","message_type":"post","content":"` + strings.ReplaceAll(postContent, `"`, `\"`) + `","create_time":"1700000000000"}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if receivedMsg == nil {
		t.Fatal("handler not called")
	}
	if !strings.Contains(receivedMsg.Content, "Test Title") {
		t.Errorf("expected content to contain 'Test Title', got %q", receivedMsg.Content)
	}
	if !strings.Contains(receivedMsg.Content, "paragraph one") {
		t.Errorf("expected content to contain 'paragraph one', got %q", receivedMsg.Content)
	}
}

func TestRateLimiting(t *testing.T) {
	adapter := NewAdapter(
		WithRateLimit(&RateLimitConfig{
			RequestsPerMinute:  2,
			ConcurrentSessions: 1,
		}),
		WithHandler(func(ctx context.Context, msg *IncomingMessage) (*CardMessage, error) {
			return nil, nil
		}),
	)

	makeReq := func() int {
		body := `{"schema":"2.0","header":{"event_id":"ev","event_type":"im.message.receive_v1","token":"t","app_id":"a","tenant_key":"rate-test"},"event":{"sender":{"sender_id":{"open_id":"ou"},"sender_type":"user"},"message":{"message_id":"m","chat_id":"c","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hi\"}","create_time":"1700000000000"}}}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
		w := httptest.NewRecorder()
		adapter.ServeHTTP(w, req)
		return w.Code
	}

	// First 2 should succeed (bucket starts full).
	if code := makeReq(); code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", code)
	}
	if code := makeReq(); code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d", code)
	}
	// Third should be rate limited.
	if code := makeReq(); code != http.StatusTooManyRequests {
		t.Fatalf("third request: expected 429, got %d", code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	adapter := NewAdapter()
	req := httptest.NewRequest(http.MethodGet, "/webhook/feishu", nil)
	w := httptest.NewRecorder()
	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestUnknownEventType(t *testing.T) {
	adapter := NewAdapter()

	body := `{"schema":"2.0","header":{"event_id":"ev","event_type":"unknown.event","token":"t","app_id":"a","tenant_key":"tk"},"event":{}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	w := httptest.NewRecorder()
	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown event, got %d", w.Code)
	}
}

func TestCardMessageBuilder(t *testing.T) {
	card := NewCardMessage("Test Card")
	card.SetTemplate("blue")
	card.AddMarkdown("**Bold** text")
	card.AddHR()
	card.AddPlainText("Footer")
	card.AddButton("Click me", "primary", map[string]string{"action": "confirm"})

	resp := card.ToFeishuResponse()
	if resp["msg_type"] != "interactive" {
		t.Errorf("expected msg_type 'interactive', got %v", resp["msg_type"])
	}

	cardData, ok := resp["card"].(map[string]interface{})
	if !ok {
		t.Fatal("expected card field in response")
	}

	header, ok := cardData["header"].(*CardHeader)
	if !ok {
		t.Fatal("expected header in card")
	}
	if header.Title.Content != "Test Card" {
		t.Errorf("expected title 'Test Card', got %q", header.Title.Content)
	}
	if header.Template != "blue" {
		t.Errorf("expected template 'blue', got %q", header.Template)
	}

	elements, ok := cardData["elements"].([]CardElement)
	if !ok {
		t.Fatal("expected elements in card")
	}
	if len(elements) != 4 {
		t.Errorf("expected 4 elements, got %d", len(elements))
	}

	// Verify JSON serialization works.
	data, err := card.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("empty JSON output")
	}
}

func TestVerifySignatureExported(t *testing.T) {
	timestamp := "1609459200"
	nonce := "abc123"
	secret := "secret-key"
	body := []byte(`{"hello":"world"}`)

	sig := computeSignature(timestamp, nonce, secret, body)

	// Valid signature should pass.
	if err := VerifySignature(timestamp, nonce, secret, body, sig); err != nil {
		t.Fatalf("expected valid signature, got error: %v", err)
	}

	// Invalid signature should fail.
	if err := VerifySignature(timestamp, nonce, secret, body, "wrong"); err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestParseMessageContent(t *testing.T) {
	tests := []struct {
		name     string
		msgType  string
		content  string
		expected string
		hasErr   bool
	}{
		{
			name:     "text message",
			msgType:  "text",
			content:  `{"text":"Hello world"}`,
			expected: "Hello world",
		},
		{
			name:     "image message",
			msgType:  "image",
			content:  `{"image_key":"img_123"}`,
			expected: "[image]",
		},
		{
			name:     "file message with name",
			msgType:  "file",
			content:  `{"file_name":"report.pdf","file_key":"key123"}`,
			expected: "[file: report.pdf]",
		},
		{
			name:     "audio message",
			msgType:  "audio",
			content:  `{"file_key":"key"}`,
			expected: "[audio]",
		},
		{
			name:     "sticker message",
			msgType:  "sticker",
			content:  `{"file_key":"key"}`,
			expected: "[sticker]",
		},
		{
			name:     "unknown type",
			msgType:  "share_chat",
			content:  `{}`,
			expected: "[share_chat]",
		},
		{
			name:     "empty content",
			msgType:  "text",
			content:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMessageContent(tt.msgType, tt.content)
			if tt.hasErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.hasErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello world"},
		{"@_user_1 hello", "hello"},
		{"@_user_abc hey @_user_def there", "hey  there"},
		{"no mentions", "no mentions"},
	}

	for _, tt := range tests {
		result := extractTextContent(tt.input)
		if result != tt.expected {
			t.Errorf("extractTextContent(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestV1MessageEvent(t *testing.T) {
	var receivedMsg *IncomingMessage
	adapter := NewAdapter(
		WithHandler(func(ctx context.Context, msg *IncomingMessage) (*CardMessage, error) {
			receivedMsg = msg
			return nil, nil
		}),
	)

	body := `{
		"uuid": "uuid-001",
		"token": "verify-token",
		"event": {
			"type": "message",
			"msg_type": "text",
			"text": "Hello from v1",
			"open_id": "ou_v1user",
			"open_chat_id": "oc_chat_v1",
			"chat_type": "p2p",
			"tenant_key": "tk1"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if receivedMsg == nil {
		t.Fatal("handler not called")
	}
	if receivedMsg.Content != "Hello from v1" {
		t.Errorf("expected 'Hello from v1', got %q", receivedMsg.Content)
	}
	if receivedMsg.SenderID != "ou_v1user" {
		t.Errorf("expected sender 'ou_v1user', got %q", receivedMsg.SenderID)
	}
}

func TestNoHandlerReturnsOK(t *testing.T) {
	adapter := NewAdapter()

	body := `{"schema":"2.0","header":{"event_id":"ev","event_type":"im.message.receive_v1","token":"t","app_id":"a","tenant_key":"tk"},"event":{"sender":{"sender_id":{"open_id":"ou"},"sender_type":"user"},"message":{"message_id":"m","chat_id":"c","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hi\"}","create_time":"1700000000000"}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	w := httptest.NewRecorder()

	adapter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with no handler, got %d", w.Code)
	}
}
