package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	signingSecret := "test-signing-secret"
	timestamp := "1531420618"
	body := `{"token":"Jhj5dZrVaK7ZwHHjRyZWjbDl","challenge":"3eZbrw1aBm2rZgRNFdxV2595E9CY3gmdALWMmHkvFXO7tYXAYM8P"}`

	// Compute expected signature: v0=HMAC-SHA256("v0:{timestamp}:{body}", signingSecret)
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(baseString))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	// Valid signature should not return error.
	err := VerifySignature(signingSecret, timestamp, body, expected)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestVerifySignatureInvalid(t *testing.T) {
	signingSecret := "test-signing-secret"
	timestamp := "1531420618"
	body := `{"token":"test"}`

	// Invalid signature should fail.
	err := VerifySignature(signingSecret, timestamp, body, "v0=invalidsignature")
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestHandleEvent(t *testing.T) {
	eventJSON := []byte(`{
		"type": "event_callback",
		"event": {
			"type": "message",
			"user": "U061F7AUR",
			"channel": "C024BE91L",
			"text": "Hello Slack",
			"thread_ts": "1482960137.003543",
			"bot_id": ""
		}
	}`)

	msg, err := HandleEvent(eventJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.UserID != "U061F7AUR" {
		t.Errorf("expected UserID 'U061F7AUR', got %q", msg.UserID)
	}
	if msg.ChannelID != "C024BE91L" {
		t.Errorf("expected ChannelID 'C024BE91L', got %q", msg.ChannelID)
	}
	if msg.Text != "Hello Slack" {
		t.Errorf("expected Text 'Hello Slack', got %q", msg.Text)
	}
	if msg.EventType != "message" {
		t.Errorf("expected EventType 'message', got %q", msg.EventType)
	}
	if msg.ThreadTS != "1482960137.003543" {
		t.Errorf("expected ThreadTS '1482960137.003543', got %q", msg.ThreadTS)
	}
}

func TestHandleEventInvalidJSON(t *testing.T) {
	_, err := HandleEvent([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandleEventWrongType(t *testing.T) {
	eventJSON := []byte(`{"type": "url_verification", "challenge": "abc"}`)
	_, err := HandleEvent(eventJSON)
	if err == nil {
		t.Fatal("expected error for non-event_callback type")
	}
}

func TestSendMessage(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("expected Content-Type 'application/json; charset=utf-8', got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer xoxb-test-token" {
			t.Errorf("expected Authorization 'Bearer xoxb-test-token', got %s", r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	adapter := NewSlackAdapter(
		WithAPIBase(server.URL),
		WithBotToken("xoxb-test-token"),
	)

	err := adapter.SendMessage(context.Background(), "C024BE91L", "Hello from bot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["channel"] != "C024BE91L" {
		t.Errorf("expected channel 'C024BE91L', got %v", receivedBody["channel"])
	}
	if receivedBody["text"] != "Hello from bot" {
		t.Errorf("expected text 'Hello from bot', got %v", receivedBody["text"])
	}
}

func TestSendBlockMessage(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	adapter := NewSlackAdapter(
		WithAPIBase(server.URL),
		WithBotToken("xoxb-test-token"),
	)

	blocks := []Block{
		{
			Type: "section",
			Text: &Text{Type: "mrkdwn", Text: "Hello *world*"},
		},
		{
			Type: "divider",
		},
	}

	err := adapter.SendBlockMessage(context.Background(), "C024BE91L", blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["channel"] != "C024BE91L" {
		t.Errorf("expected channel 'C024BE91L', got %v", receivedBody["channel"])
	}

	blocksRaw, ok := receivedBody["blocks"].([]interface{})
	if !ok {
		t.Fatal("expected blocks field to be an array")
	}
	if len(blocksRaw) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocksRaw))
	}

	firstBlock, ok := blocksRaw[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first block to be a map")
	}
	if firstBlock["type"] != "section" {
		t.Errorf("expected first block type 'section', got %v", firstBlock["type"])
	}
}

func TestSendMessageAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer server.Close()

	adapter := NewSlackAdapter(
		WithAPIBase(server.URL),
		WithBotToken("xoxb-test-token"),
	)

	err := adapter.SendMessage(context.Background(), "C_INVALID", "test")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}
