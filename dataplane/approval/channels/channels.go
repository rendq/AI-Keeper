// Package channels provides messaging channel integrations for the approval
// workflow engine. It supports sending approval cards to 飞书 (Feishu),
// 企微 (WeCom), and 钉钉 (DingTalk) and receiving approval/deny callbacks.
//
// Requirements: B14, D5
package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ai-keeper/ai-keeper/dataplane/approval"
)

// ──────────────────────────────────────────────────────────────────────────────
// Types
// ──────────────────────────────────────────────────────────────────────────────

// ChannelType enumerates the supported messaging channels.
const (
	ChannelFeishu   = "feishu"
	ChannelWeCom    = "wecom"
	ChannelDingTalk = "dingtalk"
)

// ChannelConfig holds the configuration for a messaging channel.
type ChannelConfig struct {
	Type       string // feishu, wecom, dingtalk
	WebhookURL string
	Token      string
}

// ApprovalNotifier defines the interface for sending approval cards to a
// messaging channel.
type ApprovalNotifier interface {
	SendApprovalCard(ctx context.Context, request approval.ApprovalRequest, channelConfig ChannelConfig) error
}

// ──────────────────────────────────────────────────────────────────────────────
// Feishu
// ──────────────────────────────────────────────────────────────────────────────

// FeishuApprovalNotifier sends interactive cards with approve/deny buttons
// via the Feishu (飞书) Bot webhook API.
type FeishuApprovalNotifier struct {
	Client *http.Client
}

// feishuPayload represents the interactive card message for Feishu.
type feishuPayload struct {
	MsgType string          `json:"msg_type"`
	Card    feishuCard      `json:"card"`
}

type feishuCard struct {
	Header   feishuHeader    `json:"header"`
	Elements []feishuElement `json:"elements"`
}

type feishuHeader struct {
	Title feishuText `json:"title"`
}

type feishuText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feishuElement struct {
	Tag     string         `json:"tag"`
	Text    *feishuText    `json:"text,omitempty"`
	Actions []feishuAction `json:"actions,omitempty"`
}

type feishuAction struct {
	Tag   string     `json:"tag"`
	Text  feishuText `json:"text"`
	Type  string     `json:"type"`
	Value map[string]string `json:"value"`
}

// SendApprovalCard sends an interactive card to Feishu with approve/deny buttons.
func (f *FeishuApprovalNotifier) SendApprovalCard(ctx context.Context, request approval.ApprovalRequest, cfg ChannelConfig) error {
	payload := feishuPayload{
		MsgType: "interactive",
		Card: feishuCard{
			Header: feishuHeader{
				Title: feishuText{Tag: "plain_text", Content: fmt.Sprintf("审批请求: %s", request.Action)},
			},
			Elements: []feishuElement{
				{
					Tag:  "div",
					Text: &feishuText{Tag: "plain_text", Content: fmt.Sprintf("请求人: %s\n操作: %s\n资源: %s\n原因: %s", request.Requester, request.Action, request.Resource, request.Reason)},
				},
				{
					Tag: "action",
					Actions: []feishuAction{
						{Tag: "button", Text: feishuText{Tag: "plain_text", Content: "批准"}, Type: "primary", Value: map[string]string{"action": "approve", "request_id": request.ID}},
						{Tag: "button", Text: feishuText{Tag: "plain_text", Content: "拒绝"}, Type: "danger", Value: map[string]string{"action": "deny", "request_id": request.ID}},
					},
				},
			},
		},
	}

	return f.post(ctx, cfg, payload)
}

func (f *FeishuApprovalNotifier) post(ctx context.Context, cfg ChannelConfig, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("feishu: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("feishu: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("feishu: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("feishu: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// WeCom
// ──────────────────────────────────────────────────────────────────────────────

// WeComApprovalNotifier sends text cards with action URLs via the WeCom (企微)
// Bot webhook API.
type WeComApprovalNotifier struct {
	Client *http.Client
}

// wecomPayload represents a text card message for WeCom.
type wecomPayload struct {
	MsgType  string         `json:"msgtype"`
	TextCard wecomTextCard  `json:"textcard"`
}

type wecomTextCard struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	BtnTxt      string `json:"btntxt"`
}

// SendApprovalCard sends a text card to WeCom with an action URL.
func (w *WeComApprovalNotifier) SendApprovalCard(ctx context.Context, request approval.ApprovalRequest, cfg ChannelConfig) error {
	payload := wecomPayload{
		MsgType: "textcard",
		TextCard: wecomTextCard{
			Title:       fmt.Sprintf("审批请求: %s", request.Action),
			Description: fmt.Sprintf("请求人: %s\n操作: %s\n资源: %s\n原因: %s", request.Requester, request.Action, request.Resource, request.Reason),
			URL:         fmt.Sprintf("%s/approval/%s", cfg.WebhookURL, request.ID),
			BtnTxt:      "处理审批",
		},
	}

	return w.post(ctx, cfg, payload)
}

func (w *WeComApprovalNotifier) post(ctx context.Context, cfg ChannelConfig, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("wecom: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("wecom: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	client := w.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("wecom: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("wecom: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// DingTalk
// ──────────────────────────────────────────────────────────────────────────────

// DingTalkApprovalNotifier sends action cards via the DingTalk (钉钉) Bot
// webhook API.
type DingTalkApprovalNotifier struct {
	Client *http.Client
}

// dingtalkPayload represents an action card message for DingTalk.
type dingtalkPayload struct {
	MsgType    string             `json:"msgtype"`
	ActionCard dingtalkActionCard `json:"actionCard"`
}

type dingtalkActionCard struct {
	Title          string         `json:"title"`
	Text           string         `json:"text"`
	BtnOrientation string         `json:"btnOrientation"`
	Btns           []dingtalkBtn  `json:"btns"`
}

type dingtalkBtn struct {
	Title     string `json:"title"`
	ActionURL string `json:"actionURL"`
}

// SendApprovalCard sends an action card to DingTalk with approve/deny buttons.
func (d *DingTalkApprovalNotifier) SendApprovalCard(ctx context.Context, request approval.ApprovalRequest, cfg ChannelConfig) error {
	payload := dingtalkPayload{
		MsgType: "actionCard",
		ActionCard: dingtalkActionCard{
			Title:          fmt.Sprintf("审批请求: %s", request.Action),
			Text:           fmt.Sprintf("### 审批请求\n- 请求人: %s\n- 操作: %s\n- 资源: %s\n- 原因: %s", request.Requester, request.Action, request.Resource, request.Reason),
			BtnOrientation: "1",
			Btns: []dingtalkBtn{
				{Title: "批准", ActionURL: fmt.Sprintf("%s/approval/%s/approve", cfg.WebhookURL, request.ID)},
				{Title: "拒绝", ActionURL: fmt.Sprintf("%s/approval/%s/deny", cfg.WebhookURL, request.ID)},
			},
		},
	}

	return d.post(ctx, cfg, payload)
}

func (d *DingTalkApprovalNotifier) post(ctx context.Context, cfg ChannelConfig, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("dingtalk: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dingtalk: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Factory
// ──────────────────────────────────────────────────────────────────────────────

// NewNotifier returns an ApprovalNotifier for the given channel type.
// Supported types: "feishu", "wecom", "dingtalk".
// Returns nil for unsupported channel types.
func NewNotifier(channelType string) ApprovalNotifier {
	switch channelType {
	case ChannelFeishu:
		return &FeishuApprovalNotifier{}
	case ChannelWeCom:
		return &WeComApprovalNotifier{}
	case ChannelDingTalk:
		return &DingTalkApprovalNotifier{}
	default:
		return nil
	}
}
