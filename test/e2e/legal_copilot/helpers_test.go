//go:build e2e

package legal_copilot_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// ---------- Feishu Helpers ----------

// feishuMessage represents a simulated inbound feishu message to the AIP Gateway.
type feishuMessage struct {
	Schema string             `json:"schema"`
	Header *feishuEventHeader `json:"header"`
	Event  json.RawMessage    `json:"event"`
}

type feishuEventHeader struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	CreateTime string `json:"create_time"`
	Token      string `json:"token"`
	AppID      string `json:"app_id"`
	TenantKey  string `json:"tenant_key"`
}

type feishuIMMessageEvent struct {
	Sender  feishuSender  `json:"sender"`
	Message feishuMsgBody `json:"message"`
}

type feishuSender struct {
	SenderID   feishuSenderID `json:"sender_id"`
	SenderType string         `json:"sender_type"`
	TenantKey  string         `json:"tenant_key"`
}

type feishuSenderID struct {
	UserID  string `json:"user_id"`
	OpenID  string `json:"open_id"`
	UnionID string `json:"union_id"`
}

type feishuMsgBody struct {
	MessageID   string `json:"message_id"`
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"`
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
}

// sendFeishuMessage simulates posting a feishu webhook event to the AIP Gateway.
func sendFeishuMessage(t *testing.T, ctx context.Context, userID, tenantKey, text string) (*http.Response, []byte) {
	t.Helper()

	event := feishuIMMessageEvent{
		Sender: feishuSender{
			SenderID:   feishuSenderID{UserID: userID, OpenID: "ou_" + userID, UnionID: "on_" + userID},
			SenderType: "user",
			TenantKey:  tenantKey,
		},
		Message: feishuMsgBody{
			MessageID:   fmt.Sprintf("om_%s_%d", userID, time.Now().UnixNano()),
			ChatID:      "oc_legal_copilot_chat",
			ChatType:    "p2p",
			Content:     fmt.Sprintf(`{"text":"%s"}`, text),
			MessageType: "text",
		},
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	msg := feishuMessage{
		Schema: "2.0",
		Header: &feishuEventHeader{
			EventID:    fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			EventType:  "im.message.receive_v1",
			CreateTime: fmt.Sprintf("%d", time.Now().Unix()),
			Token:      "test-verification-token",
			AppID:      "cli_legal_copilot",
			TenantKey:  tenantKey,
		},
		Event: eventBytes,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal feishu message: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.GatewayURL+"/channel/feishu/webhook", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lark-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Lark-Request-Nonce", "test-nonce")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send feishu message: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return resp, respBody
}

// ---------- Audit Helpers ----------

// auditEvent represents a simplified audit event for verification.
type auditEvent struct {
	InvocationID string            `json:"invocationId"`
	TenantID     string            `json:"tenantId"`
	AgentName    string            `json:"agentName"`
	UserID       string            `json:"userId"`
	Decision     string            `json:"decision"`
	ModelUsed    string            `json:"modelUsed"`
	TokensIn     int               `json:"tokensIn"`
	TokensOut    int               `json:"tokensOut"`
	CostUSD      float64           `json:"costUsd"`
	EventHash    string            `json:"eventHash"`
	Watermark    *watermarkInfo    `json:"watermark,omitempty"`
	Citations    []citationInfo    `json:"citations,omitempty"`
	Steps        []auditStep       `json:"steps,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type watermarkInfo struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
}

type citationInfo struct {
	Source string `json:"source"`
	Chunk  string `json:"chunk"`
}

type auditStep struct {
	Type     string `json:"type"`
	ToolName string `json:"toolName,omitempty"`
	TokensIn int    `json:"tokensIn"`
	TokensOut int   `json:"tokensOut"`
	LatencyMs int64 `json:"latencyMs"`
}

// auditQueryResponse is the response from the audit query endpoint.
type auditQueryResponse struct {
	Events []auditEvent `json:"events"`
	Total  int          `json:"total"`
}

// queryAuditEvents fetches audit events from the audit query service.
func queryAuditEvents(t *testing.T, ctx context.Context, params map[string]string) []auditEvent {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.AuditURL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("create audit request: %v", err)
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("audit query failed: status=%d body=%s", resp.StatusCode, body)
	}

	var result auditQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	return result.Events
}

// waitForAuditEvent polls the audit service until an event matching the filter is found.
func waitForAuditEvent(t *testing.T, ctx context.Context, params map[string]string) *auditEvent {
	t.Helper()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(pollTimeout)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for audit event (deadline=%v, params=%v)", deadline, params)
			return nil
		case <-ticker.C:
			events := queryAuditEvents(t, ctx, params)
			if len(events) > 0 {
				return &events[0]
			}
		}
	}
}

// ---------- ClickHouse / S3 Helpers ----------

// clickhouseEvent represents a row from the ClickHouse audit table.
type clickhouseEvent struct {
	InvocationID string  `json:"invocation_id"`
	EventHash    string  `json:"event_hash"`
	TenantID     string  `json:"tenant_id"`
	CostUSD      float64 `json:"cost_usd"`
	CreatedAt    string  `json:"created_at"`
}

// queryClickHouse sends a query to the ClickHouse HTTP interface.
func queryClickHouse(t *testing.T, ctx context.Context, query string) []clickhouseEvent {
	t.Helper()

	body := bytes.NewBufferString(query + " FORMAT JSON")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.ClickHouseURL+"/", body)
	if err != nil {
		t.Fatalf("create clickhouse request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("clickhouse query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("clickhouse query failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	var result struct {
		Data []clickhouseEvent `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode clickhouse response: %v", err)
	}
	return result.Data
}

// s3ObjectExists checks if an object exists in the S3-compatible audit bucket.
func s3ObjectExists(t *testing.T, ctx context.Context, bucket, key string) bool {
	t.Helper()

	url := fmt.Sprintf("%s/%s/%s", env.S3Endpoint, bucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		t.Fatalf("create s3 request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("s3 head object: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ---------- Cost / Budget Helpers ----------

// costRecord represents a cost tracking entry.
type costRecord struct {
	TenantID    string  `json:"tenantId"`
	AgentName   string  `json:"agentName"`
	TotalUSD    float64 `json:"totalUsd"`
	TotalTokens int     `json:"totalTokens"`
	BudgetLimit float64 `json:"budgetLimit"`
	Exceeded    bool    `json:"exceeded"`
}

// queryCostTracking fetches current cost data from the gateway's cost API.
func queryCostTracking(t *testing.T, ctx context.Context, tenantID, agentName string) *costRecord {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/cost?tenantId=%s&agentName=%s", env.GatewayURL, tenantID, agentName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create cost request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("query cost: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("cost query failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	var record costRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		t.Fatalf("decode cost response: %v", err)
	}
	return &record
}

// ---------- Crypto Helpers ----------

// verifyEventHash recomputes and verifies the SHA-256 hash of the event payload.
func verifyEventHash(t *testing.T, event *auditEvent) bool {
	t.Helper()

	// Canonical form: JSON encode without the eventHash field.
	canonical := struct {
		InvocationID string  `json:"invocationId"`
		TenantID     string  `json:"tenantId"`
		AgentName    string  `json:"agentName"`
		UserID       string  `json:"userId"`
		Decision     string  `json:"decision"`
		ModelUsed    string  `json:"modelUsed"`
		TokensIn     int     `json:"tokensIn"`
		TokensOut    int     `json:"tokensOut"`
		CostUSD      float64 `json:"costUsd"`
	}{
		InvocationID: event.InvocationID,
		TenantID:     event.TenantID,
		AgentName:    event.AgentName,
		UserID:       event.UserID,
		Decision:     event.Decision,
		ModelUsed:    event.ModelUsed,
		TokensIn:     event.TokensIn,
		TokensOut:    event.TokensOut,
		CostUSD:      event.CostUSD,
	}

	data, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("marshal canonical event: %v", err)
		return false
	}

	hash := sha256.Sum256(data)
	computed := hex.EncodeToString(hash[:])
	return computed == event.EventHash
}
