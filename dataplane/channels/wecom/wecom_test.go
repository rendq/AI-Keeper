package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	token := "test-token"
	timestamp := "1409659813"
	nonce := "nonce123"
	echostr := "echo-string-value"

	// Compute expected signature: SHA1(sort(token, timestamp, nonce))
	expected := computeSignature(token, timestamp, nonce)

	// Valid signature should return echostr.
	result, err := VerifySignature(token, timestamp, nonce, echostr, expected)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != echostr {
		t.Errorf("expected echostr %q, got %q", echostr, result)
	}
}

func TestVerifySignatureInvalid(t *testing.T) {
	token := "test-token"
	timestamp := "1409659813"
	nonce := "nonce123"
	echostr := "echo-string-value"

	// Invalid signature should fail.
	_, err := VerifySignature(token, timestamp, nonce, echostr, "invalid-signature")
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestHandleEvent(t *testing.T) {
	xmlBody := []byte(`<xml>
		<ToUserName><![CDATA[corp123]]></ToUserName>
		<FromUserName><![CDATA[user001]]></FromUserName>
		<CreateTime>1348831860</CreateTime>
		<MsgType><![CDATA[text]]></MsgType>
		<Content><![CDATA[Hello WeCom]]></Content>
		<MsgId>1234567890123456</MsgId>
		<AgentID>1000001</AgentID>
	</xml>`)

	msg, err := HandleEvent(xmlBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.FromUser != "user001" {
		t.Errorf("expected FromUser 'user001', got %q", msg.FromUser)
	}
	if msg.MsgType != "text" {
		t.Errorf("expected MsgType 'text', got %q", msg.MsgType)
	}
	if msg.Content != "Hello WeCom" {
		t.Errorf("expected Content 'Hello WeCom', got %q", msg.Content)
	}
	if msg.CreateTime != 1348831860 {
		t.Errorf("expected CreateTime 1348831860, got %d", msg.CreateTime)
	}
	if msg.AgentID != 1000001 {
		t.Errorf("expected AgentID 1000001, got %d", msg.AgentID)
	}
}

func TestHandleEventInvalidXML(t *testing.T) {
	_, err := HandleEvent([]byte("not xml"))
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestHandleEventEmptyFields(t *testing.T) {
	xmlBody := []byte(`<xml></xml>`)
	_, err := HandleEvent(xmlBody)
	if err == nil {
		t.Fatal("expected error for empty event")
	}
}

func TestSendTextMessage(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi-bin/message/send" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	adapter := NewWeComAdapter(
		WithAPIBase(server.URL),
	)

	err := adapter.SendTextMessage(context.Background(), "user001", "1000001", "Hello from bot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["touser"] != "user001" {
		t.Errorf("expected touser 'user001', got %v", receivedBody["touser"])
	}
	if receivedBody["msgtype"] != "text" {
		t.Errorf("expected msgtype 'text', got %v", receivedBody["msgtype"])
	}
	if receivedBody["agentid"] != "1000001" {
		t.Errorf("expected agentid '1000001', got %v", receivedBody["agentid"])
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

	adapter := NewWeComAdapter(
		WithAPIBase(server.URL),
	)

	card := CardMessage{
		Title:       "Task Notification",
		Description: "You have a new task assigned",
		URL:         "https://example.com/task/123",
		ButtonText:  "View Details",
	}

	err := adapter.SendCardMessage(context.Background(), "user002", "1000002", card)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["touser"] != "user002" {
		t.Errorf("expected touser 'user002', got %v", receivedBody["touser"])
	}
	if receivedBody["msgtype"] != "textcard" {
		t.Errorf("expected msgtype 'textcard', got %v", receivedBody["msgtype"])
	}
	cardMap, ok := receivedBody["textcard"].(map[string]interface{})
	if !ok {
		t.Fatal("expected textcard field to be a map")
	}
	if cardMap["title"] != "Task Notification" {
		t.Errorf("expected title 'Task Notification', got %v", cardMap["title"])
	}
	if cardMap["description"] != "You have a new task assigned" {
		t.Errorf("expected description, got %v", cardMap["description"])
	}
	if cardMap["url"] != "https://example.com/task/123" {
		t.Errorf("expected url, got %v", cardMap["url"])
	}
	if cardMap["btntxt"] != "View Details" {
		t.Errorf("expected btntxt 'View Details', got %v", cardMap["btntxt"])
	}
}

func TestSendTextMessageAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode":40014,"errmsg":"invalid access_token"}`))
	}))
	defer server.Close()

	adapter := NewWeComAdapter(
		WithAPIBase(server.URL),
	)

	err := adapter.SendTextMessage(context.Background(), "user001", "1000001", "test")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}
