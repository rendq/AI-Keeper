//go:build e2e

package failclosed_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// ---------- Gateway / PEP Helpers ----------

// decisionRequest represents a policy decision request sent to the gateway PEP endpoint.
type decisionRequest struct {
	TenantID  string `json:"tenantId"`
	AgentName string `json:"agentName"`
	UserID    string `json:"userId"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
}

// decisionResponse represents the PEP response.
type decisionResponse struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// requestDecision sends a policy decision request to the gateway PEP endpoint.
func requestDecision(t *testing.T, ctx context.Context, req decisionRequest) *decisionResponse {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal decision request: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, env.GatewayURL+"/api/v1/decide", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create decision request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("send decision request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read decision response: %v", err)
	}

	var result decisionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("decode decision response: status=%d body=%s err=%v", resp.StatusCode, respBody, err)
	}

	return &result
}

// ---------- Audit Helpers ----------

// auditEvent represents a simplified audit event.
type auditEvent struct {
	InvocationID string `json:"invocationId"`
	TenantID     string `json:"tenantId"`
	AgentName    string `json:"agentName"`
	UserID       string `json:"userId"`
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
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

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for audit event (params=%v)", params)
			return nil
		case <-ticker.C:
			events := queryAuditEvents(t, ctx, params)
			if len(events) > 0 {
				return &events[0]
			}
		}
	}
}

// ---------- S3 Helpers ----------

// s3PutObject uploads a test object to the S3 audit bucket.
func s3PutObject(t *testing.T, ctx context.Context, key string, data []byte) {
	t.Helper()

	url := fmt.Sprintf("%s/%s/%s", env.S3Endpoint, auditBucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create s3 put request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("s3 put object: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("s3 put object failed: status=%d", resp.StatusCode)
	}
}

// s3DeleteObject attempts to delete an object from the S3 audit bucket.
// Returns the HTTP status code (expected 403 when Object Lock is active).
func s3DeleteObject(t *testing.T, ctx context.Context, key string) int {
	t.Helper()

	url := fmt.Sprintf("%s/%s/%s", env.S3Endpoint, auditBucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("create s3 delete request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("s3 delete object: %v", err)
	}
	resp.Body.Close()

	return resp.StatusCode
}

// s3OverwriteObject attempts to overwrite an existing object in the S3 audit bucket.
// Returns the HTTP status code (expected 403 when Object Lock is active).
func s3OverwriteObject(t *testing.T, ctx context.Context, key string, data []byte) int {
	t.Helper()

	url := fmt.Sprintf("%s/%s/%s", env.S3Endpoint, auditBucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create s3 overwrite request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("s3 overwrite object: %v", err)
	}
	resp.Body.Close()

	return resp.StatusCode
}
