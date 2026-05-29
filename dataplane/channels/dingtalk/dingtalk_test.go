package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	secret := "test-secret-key"
	timestamp := "1609459200000"

	// Compute expected signature.
	expected := ComputeSignature(timestamp, secret)

	// Valid signature should succeed.
	err := VerifySignature(timestamp, expected, secret)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestVerifySignatureInvalid(t *testing.T) {
	secret := "test-secret-key"
	timestamp := "1609459200000"

	// Invalid signature should fail.
	err := VerifySignature(timestamp, "invalid-signature", secret)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestHandleEvent(t *testing.T) {
	body := []byte(`{
		"msgtype": "text",
		"text": {"content": "Hello DingTalk"},
		"senderId": "user123",
		"senderNick": "张三",
		"conversationId": "conv456",
		"isInAtList": true
	}`)

	msg, err := HandleEvent(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.SenderID != "user123" {
		t.Errorf("expected SenderID 'user123', got %q", msg.SenderID)
	}
	if msg.SenderNick != "张三" {
		t.Errorf("expected SenderNick '张三', got %q", msg.SenderNick)
	}
	if msg.MsgType != "text" {
		t.Errorf("expected MsgType 'text', got %q", msg.MsgType)
	}
	if msg.Text != "Hello DingTalk" {
		t.Errorf("expected Text 'Hello DingTalk', got %q", msg.Text)
	}
	if msg.ConversationID != "conv456" {
		t.Errorf("expected ConversationID 'conv456', got %q", msg.ConversationID)
	}
	if !msg.IsInAtList {
		t.Error("expected IsInAtList to be true")
	}
}

func TestHandleEventInvalidJSON(t *testing.T) {
	_, err := HandleEvent([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandleEventEmptyFields(t *testing.T) {
	_, err := HandleEvent([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error for empty event")
	}
}

func TestSendTextMessage(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	adapter := NewDingTalkAdapter()

	err := adapter.SendTextMessage(context.Background(), server.URL, "Hello from bot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["msgtype"] != "text" {
		t.Errorf("expected msgtype 'text', got %v", receivedBody["msgtype"])
	}
	textMap, ok := receivedBody["text"].(map[string]interface{})
	if !ok {
		t.Fatal("expected text field to be a map")
	}
	if textMap["content"] != "Hello from bot" {
		t.Errorf("expected content 'Hello from bot', got %v", textMap["content"])
	}
}

func TestSendCardMessage(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	adapter := NewDingTalkAdapter()

	card := CardMessage{
		Title:       "Task Alert",
		Text:        "You have a new task",
		SingleTitle: "View Task",
		SingleURL:   "https://example.com/task/123",
	}

	err := adapter.SendCardMessage(context.Background(), server.URL, card)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["msgtype"] != "actionCard" {
		t.Errorf("expected msgtype 'actionCard', got %v", receivedBody["msgtype"])
	}
	cardMap, ok := receivedBody["actionCard"].(map[string]interface{})
	if !ok {
		t.Fatal("expected actionCard field to be a map")
	}
	if cardMap["title"] != "Task Alert" {
		t.Errorf("expected title 'Task Alert', got %v", cardMap["title"])
	}
	if cardMap["text"] != "You have a new task" {
		t.Errorf("expected text, got %v", cardMap["text"])
	}
	if cardMap["singleTitle"] != "View Task" {
		t.Errorf("expected singleTitle 'View Task', got %v", cardMap["singleTitle"])
	}
	if cardMap["singleURL"] != "https://example.com/task/123" {
		t.Errorf("expected singleURL, got %v", cardMap["singleURL"])
	}
}

func TestSendTextMessageAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode":310000,"errmsg":"sign not match"}`))
	}))
	defer server.Close()

	adapter := NewDingTalkAdapter()

	err := adapter.SendTextMessage(context.Background(), server.URL, "test")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}
