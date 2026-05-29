//go:build e2e

package failclosed_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestFailClosedPDPTimeout verifies that when the PDP exceeds its 1s timeout,
// the PEP returns a deny decision with reason "PolicyTimeout".
// This injects a simulated PDP delay (>1s) and confirms the gateway responds with deny.
//
// Validates: Requirements F6, A5.13
func TestFailClosedPDPTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
	defer cancel()

	// Step 1: Inject PDP delay via the gateway's fault injection endpoint.
	// This tells the PDP sidecar to add 1.5s latency to all policy evaluations.
	injectPDPDelay(t, ctx, 1500*time.Millisecond)
	defer clearPDPDelay(t, context.Background())

	// Step 2: Send a decision request to the PEP; the PDP will timeout.
	resp := requestDecision(t, ctx, decisionRequest{
		TenantID:  "tenant-failclosed-test",
		AgentName: "legal-copilot",
		UserID:    "user-timeout-001",
		Action:    "invoke",
		Resource:  "agent/legal-copilot",
	})

	// Step 3: Verify the response is a deny with PolicyTimeout reason.
	if resp.Decision != "deny" {
		t.Errorf("expected decision=deny, got %q", resp.Decision)
	}
	if resp.Reason != "PolicyTimeout" {
		t.Errorf("expected reason=PolicyTimeout, got %q", resp.Reason)
	}

	// Step 4: Verify an audit event was emitted with the deny decision.
	auditCtx, auditCancel := context.WithTimeout(context.Background(), pollTimeout)
	defer auditCancel()

	event := waitForAuditEvent(t, auditCtx, map[string]string{
		"userId":   "user-timeout-001",
		"decision": "deny",
	})
	if event == nil {
		t.Fatal("expected audit event for denied request, got nil")
	}
	if event.Decision != "deny" {
		t.Errorf("audit event decision: expected deny, got %q", event.Decision)
	}
	if event.Reason != "PolicyTimeout" {
		t.Errorf("audit event reason: expected PolicyTimeout, got %q", event.Reason)
	}
}

// TestAuditImmutability verifies that S3 Object Lock prevents modification and
// deletion of audit events stored in the WORM-protected bucket.
//
// Validates: Requirements F5, B12.6
func TestAuditImmutability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
	defer cancel()

	// Step 1: Write a test audit event object to S3 (simulating normal audit write).
	testKey := fmt.Sprintf("audit-events/immutability-test/%d.json", time.Now().UnixNano())
	testEvent := map[string]string{
		"invocationId": "inv-immutability-test",
		"tenantId":     "tenant-immutability-test",
		"decision":     "allow",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}
	eventData, err := json.Marshal(testEvent)
	if err != nil {
		t.Fatalf("marshal test event: %v", err)
	}

	s3PutObject(t, ctx, testKey, eventData)

	// Step 2: Attempt to delete the object — should be rejected by Object Lock.
	deleteStatus := s3DeleteObject(t, ctx, testKey)
	if deleteStatus != http.StatusForbidden {
		t.Errorf("delete audit object: expected HTTP 403 (Object Lock), got %d", deleteStatus)
	}

	// Step 3: Attempt to overwrite the object — should also be rejected.
	modifiedEvent := map[string]string{
		"invocationId": "inv-immutability-test",
		"tenantId":     "tenant-immutability-test",
		"decision":     "TAMPERED",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}
	modifiedData, err := json.Marshal(modifiedEvent)
	if err != nil {
		t.Fatalf("marshal modified event: %v", err)
	}

	overwriteStatus := s3OverwriteObject(t, ctx, testKey, modifiedData)
	if overwriteStatus != http.StatusForbidden {
		t.Errorf("overwrite audit object: expected HTTP 403 (Object Lock), got %d", overwriteStatus)
	}
}

// ---------- Fault Injection Helpers ----------

// injectPDPDelay instructs the gateway to simulate PDP latency.
func injectPDPDelay(t *testing.T, ctx context.Context, delay time.Duration) {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/fault/pdp-delay", env.GatewayURL)
	body := fmt.Sprintf(`{"delayMs":%d}`, delay.Milliseconds())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("create fault inject request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = nopCloser([]byte(body))
	req.ContentLength = int64(len(body))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("inject pdp delay: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("inject pdp delay: unexpected status %d", resp.StatusCode)
	}
}

// clearPDPDelay removes the injected PDP delay.
func clearPDPDelay(t *testing.T, ctx context.Context) {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/fault/pdp-delay", env.GatewayURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		t.Logf("warning: could not clear pdp delay: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("warning: clear pdp delay request failed: %v", err)
		return
	}
	resp.Body.Close()
}

// nopCloser wraps a byte slice into an io.ReadCloser.
type nopReadCloser struct {
	*bytes.Reader
}

func (nopReadCloser) Close() error { return nil }

func nopCloser(data []byte) *nopReadCloser {
	return &nopReadCloser{Reader: bytes.NewReader(data)}
}
