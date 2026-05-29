package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyToken(t *testing.T) {
	// Valid Bearer token should pass.
	err := VerifyToken("Bearer eyJhbGciOiJSUzI1NiJ9.test.signature", "app-id", "app-password")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestVerifyTokenInvalid(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		appID      string
		appPass    string
	}{
		{"empty auth header", "", "app-id", "app-password"},
		{"missing Bearer prefix", "Basic token123", "app-id", "app-password"},
		{"empty token after Bearer", "Bearer ", "app-id", "app-password"},
		{"empty appID", "Bearer token123", "", "app-password"},
		{"empty appPassword", "Bearer token123", "app-id", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyToken(tc.authHeader, tc.appID, tc.appPass)
			if err == nil {
				t.Fatal("expected error for invalid token")
			}
		})
	}
}

func TestHandleActivity(t *testing.T) {
	activityJSON := []byte(`{
		"type": "message",
		"id": "activity-123",
		"from": {"id": "user-1", "name": "Alice"},
		"conversation": {"id": "conv-456"},
		"text": "Hello Teams bot",
		"serviceUrl": "https://smba.trafficmanager.net/teams/",
		"channelId": "msteams"
	}`)

	activity, err := HandleActivity(activityJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if activity.Type != "message" {
		t.Errorf("expected Type 'message', got %q", activity.Type)
	}
	if activity.ID != "activity-123" {
		t.Errorf("expected ID 'activity-123', got %q", activity.ID)
	}
	if activity.From.ID != "user-1" {
		t.Errorf("expected From.ID 'user-1', got %q", activity.From.ID)
	}
	if activity.From.Name != "Alice" {
		t.Errorf("expected From.Name 'Alice', got %q", activity.From.Name)
	}
	if activity.Conversation.ID != "conv-456" {
		t.Errorf("expected Conversation.ID 'conv-456', got %q", activity.Conversation.ID)
	}
	if activity.Text != "Hello Teams bot" {
		t.Errorf("expected Text 'Hello Teams bot', got %q", activity.Text)
	}
	if activity.ServiceURL != "https://smba.trafficmanager.net/teams/" {
		t.Errorf("expected ServiceURL 'https://smba.trafficmanager.net/teams/', got %q", activity.ServiceURL)
	}
	if activity.ChannelID != "msteams" {
		t.Errorf("expected ChannelID 'msteams', got %q", activity.ChannelID)
	}
}

func TestHandleActivityInvalidJSON(t *testing.T) {
	_, err := HandleActivity([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandleActivityEmpty(t *testing.T) {
	_, err := HandleActivity([]byte{})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestHandleActivityMissingType(t *testing.T) {
	_, err := HandleActivity([]byte(`{"id": "123", "text": "hello"}`))
	if err == nil {
		t.Fatal("expected error for missing type field")
	}
}

func TestSendTextReply(t *testing.T) {
	var receivedBody map[string]interface{}
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if r.Header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("expected Content-Type 'application/json; charset=utf-8', got %s", r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"resp-1"}`))
	}))
	defer server.Close()

	adapter := NewTeamsAdapter()

	err := adapter.SendTextReply(context.Background(), server.URL, "conv-456", "activity-123", "Hello from bot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := "/v3/conversations/conv-456/activities/activity-123"
	if receivedPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, receivedPath)
	}
	if receivedBody["type"] != "message" {
		t.Errorf("expected type 'message', got %v", receivedBody["type"])
	}
	if receivedBody["text"] != "Hello from bot" {
		t.Errorf("expected text 'Hello from bot', got %v", receivedBody["text"])
	}
}

func TestSendCardReply(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"resp-2"}`))
	}))
	defer server.Close()

	adapter := NewTeamsAdapter()

	card := AdaptiveCard{
		Body: []CardElement{
			{Type: "TextBlock", Text: "Hello World", Size: "Large"},
			{Type: "TextBlock", Text: "This is an adaptive card"},
		},
	}

	err := adapter.SendCardReply(context.Background(), server.URL, "conv-456", "activity-123", card)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["type"] != "message" {
		t.Errorf("expected type 'message', got %v", receivedBody["type"])
	}

	attachments, ok := receivedBody["attachments"].([]interface{})
	if !ok {
		t.Fatal("expected attachments field to be an array")
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}

	attachment, ok := attachments[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected attachment to be a map")
	}
	if attachment["contentType"] != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("expected contentType 'application/vnd.microsoft.card.adaptive', got %v", attachment["contentType"])
	}

	content, ok := attachment["content"].(map[string]interface{})
	if !ok {
		t.Fatal("expected content to be a map")
	}
	if content["type"] != "AdaptiveCard" {
		t.Errorf("expected content type 'AdaptiveCard', got %v", content["type"])
	}
	if content["version"] != "1.4" {
		t.Errorf("expected version '1.4', got %v", content["version"])
	}

	body, ok := content["body"].([]interface{})
	if !ok {
		t.Fatal("expected body to be an array")
	}
	if len(body) != 2 {
		t.Fatalf("expected 2 card elements, got %d", len(body))
	}

	firstElement, ok := body[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first element to be a map")
	}
	if firstElement["type"] != "TextBlock" {
		t.Errorf("expected first element type 'TextBlock', got %v", firstElement["type"])
	}
	if firstElement["text"] != "Hello World" {
		t.Errorf("expected first element text 'Hello World', got %v", firstElement["text"])
	}
	if firstElement["size"] != "Large" {
		t.Errorf("expected first element size 'Large', got %v", firstElement["size"])
	}
}

func TestSendTextReplyAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	adapter := NewTeamsAdapter()

	err := adapter.SendTextReply(context.Background(), server.URL, "conv-456", "activity-123", "test")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}
