package channels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ai-keeper/ai-keeper/dataplane/approval"
)

func sampleRequest() approval.ApprovalRequest {
	return approval.ApprovalRequest{
		ID:        "req-001",
		Requester: "alice",
		Action:    "deploy-model",
		Resource:  "model/gpt-4o",
		Reason:    "production rollout",
	}
}

func TestFeishuNotifier_SendsCard(t *testing.T) {
	var received map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization Bearer test-token, got %s", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := &FeishuApprovalNotifier{Client: srv.Client()}
	cfg := ChannelConfig{Type: ChannelFeishu, WebhookURL: srv.URL, Token: "test-token"}

	err := notifier.SendApprovalCard(context.Background(), sampleRequest(), cfg)
	if err != nil {
		t.Fatalf("SendApprovalCard: %v", err)
	}

	if received["msg_type"] != "interactive" {
		t.Errorf("expected msg_type interactive, got %v", received["msg_type"])
	}

	card, ok := received["card"].(map[string]interface{})
	if !ok {
		t.Fatal("expected card field in payload")
	}
	header := card["header"].(map[string]interface{})
	title := header["title"].(map[string]interface{})
	if title["content"].(string) != "审批请求: deploy-model" {
		t.Errorf("unexpected title: %v", title["content"])
	}
}

func TestWeComNotifier_SendsCard(t *testing.T) {
	var received map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := &WeComApprovalNotifier{Client: srv.Client()}
	cfg := ChannelConfig{Type: ChannelWeCom, WebhookURL: srv.URL, Token: "wecom-token"}

	err := notifier.SendApprovalCard(context.Background(), sampleRequest(), cfg)
	if err != nil {
		t.Fatalf("SendApprovalCard: %v", err)
	}

	if received["msgtype"] != "textcard" {
		t.Errorf("expected msgtype textcard, got %v", received["msgtype"])
	}

	textCard, ok := received["textcard"].(map[string]interface{})
	if !ok {
		t.Fatal("expected textcard field in payload")
	}
	if textCard["title"].(string) != "审批请求: deploy-model" {
		t.Errorf("unexpected title: %v", textCard["title"])
	}
	if textCard["btntxt"].(string) != "处理审批" {
		t.Errorf("unexpected btntxt: %v", textCard["btntxt"])
	}
}

func TestDingTalkNotifier_SendsCard(t *testing.T) {
	var received map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := &DingTalkApprovalNotifier{Client: srv.Client()}
	cfg := ChannelConfig{Type: ChannelDingTalk, WebhookURL: srv.URL, Token: "ding-token"}

	err := notifier.SendApprovalCard(context.Background(), sampleRequest(), cfg)
	if err != nil {
		t.Fatalf("SendApprovalCard: %v", err)
	}

	if received["msgtype"] != "actionCard" {
		t.Errorf("expected msgtype actionCard, got %v", received["msgtype"])
	}

	actionCard, ok := received["actionCard"].(map[string]interface{})
	if !ok {
		t.Fatal("expected actionCard field in payload")
	}
	if actionCard["title"].(string) != "审批请求: deploy-model" {
		t.Errorf("unexpected title: %v", actionCard["title"])
	}

	btns, ok := actionCard["btns"].([]interface{})
	if !ok || len(btns) != 2 {
		t.Fatalf("expected 2 buttons, got %v", actionCard["btns"])
	}
}

func TestNewNotifier_Factory(t *testing.T) {
	tests := []struct {
		channelType string
		wantType    interface{}
		wantNil     bool
	}{
		{ChannelFeishu, &FeishuApprovalNotifier{}, false},
		{ChannelWeCom, &WeComApprovalNotifier{}, false},
		{ChannelDingTalk, &DingTalkApprovalNotifier{}, false},
		{"unknown", nil, true},
		{"", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.channelType, func(t *testing.T) {
			notifier := NewNotifier(tt.channelType)
			if tt.wantNil {
				if notifier != nil {
					t.Errorf("expected nil for channel type %q, got %T", tt.channelType, notifier)
				}
				return
			}
			if notifier == nil {
				t.Fatalf("expected non-nil notifier for channel type %q", tt.channelType)
			}
		})
	}
}
