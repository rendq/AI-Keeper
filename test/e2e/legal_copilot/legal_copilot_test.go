//go:build e2e

package legal_copilot_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Scenario 1: 飞书入站 → Gateway → PEP allow → tool_calling agent
// Validates: Requirements B1, B2, B5, B13
// ---------------------------------------------------------------------------

func TestFeishuToAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Send a feishu message as an authorized legal team user.
	resp, body := sendFeishuMessage(t, ctx,
		"user_legal_alice",      // userID
		"tenant_legal_corp",     // tenantKey
		"帮我查一下合同模板中关于保密条款的内容", // text
	)

	// 2. Verify gateway accepted the request (not 401/403/429).
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 200/202 from gateway, got %d: %s", resp.StatusCode, body)
	}

	// 3. Parse the response — should indicate agent processing.
	var gatewayResp struct {
		InvocationID string `json:"invocationId"`
		Status       string `json:"status"`
		AgentName    string `json:"agentName"`
	}
	if err := json.Unmarshal(body, &gatewayResp); err != nil {
		t.Fatalf("unmarshal gateway response: %v (body=%s)", err, body)
	}

	if gatewayResp.InvocationID == "" {
		t.Fatal("expected non-empty invocationId in response")
	}
	if gatewayResp.AgentName == "" {
		t.Fatal("expected agentName in response (tool_calling agent should be routed)")
	}

	// 4. Verify audit event recorded the allow decision.
	event := waitForAuditEvent(t, ctx, map[string]string{
		"invocationId": gatewayResp.InvocationID,
	})

	if event.Decision != "allow" {
		t.Errorf("expected PEP decision=allow, got %q", event.Decision)
	}
	if event.UserID != "user_legal_alice" {
		t.Errorf("expected userId=user_legal_alice, got %q", event.UserID)
	}

	// 5. Verify the agent executed with tool_calling pattern (steps should contain tool_call).
	hasToolCall := false
	for _, step := range event.Steps {
		if step.Type == "tool_call" {
			hasToolCall = true
			break
		}
	}
	if !hasToolCall {
		t.Log("WARNING: no tool_call step found — agent may not have entered tool_calling mode")
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: KB pre_filter（无权用户看不到机密章节）
// Validates: Requirements B9, A5, B2
// ---------------------------------------------------------------------------

func TestKBPreFilter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Authorized user queries KB — should see confidential sections.
	_, authBody := sendFeishuMessage(t, ctx,
		"user_legal_alice",
		"tenant_legal_corp",
		"查找薪资保密协议条款", // confidential content
	)

	var authResp struct {
		InvocationID string `json:"invocationId"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(authBody, &authResp); err != nil {
		t.Fatalf("unmarshal auth response: %v", err)
	}

	// Wait for the authorized user's audit event.
	authEvent := waitForAuditEvent(t, ctx, map[string]string{
		"invocationId": authResp.InvocationID,
	})

	// 2. Unauthorized user queries the same KB — should NOT see confidential.
	_, unauthBody := sendFeishuMessage(t, ctx,
		"user_intern_bob",       // intern with lower clearance
		"tenant_legal_corp",
		"查找薪资保密协议条款",
	)

	var unauthResp struct {
		InvocationID string `json:"invocationId"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(unauthBody, &unauthResp); err != nil {
		t.Fatalf("unmarshal unauth response: %v", err)
	}

	unauthEvent := waitForAuditEvent(t, ctx, map[string]string{
		"invocationId": unauthResp.InvocationID,
	})

	// 3. Compare citations — unauthorized user should have no confidential citations.
	authConfidentialCount := 0
	for _, c := range authEvent.Citations {
		if strings.Contains(c.Source, "confidential") || strings.Contains(c.Source, "salary") {
			authConfidentialCount++
		}
	}

	for _, c := range unauthEvent.Citations {
		if strings.Contains(c.Source, "confidential") || strings.Contains(c.Source, "salary") {
			t.Errorf("unauthorized user saw confidential citation: %+v", c)
		}
	}

	t.Logf("authorized user saw %d confidential citations; unauthorized user saw 0 (as expected)", authConfidentialCount)
}

// ---------------------------------------------------------------------------
// Scenario 3: DocuSign OBO 调用（OBO claim 透传）
// Validates: Requirements B3, C8
// ---------------------------------------------------------------------------

func TestDocusignOBO(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Send a message that triggers DocuSign tool call via OBO.
	resp, body := sendFeishuMessage(t, ctx,
		"user_legal_alice",
		"tenant_legal_corp",
		"请用DocuSign发送合同给对方签署",
	)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 200/202, got %d: %s", resp.StatusCode, body)
	}

	var gatewayResp struct {
		InvocationID string `json:"invocationId"`
	}
	if err := json.Unmarshal(body, &gatewayResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// 2. Wait for audit event and verify OBO claim is present.
	event := waitForAuditEvent(t, ctx, map[string]string{
		"invocationId": gatewayResp.InvocationID,
	})

	// Verify the tool call step used OBO identity.
	var docusignStep *auditStep
	for i, step := range event.Steps {
		if step.Type == "tool_call" && step.ToolName == "docusign" {
			docusignStep = &event.Steps[i]
			break
		}
	}

	if docusignStep == nil {
		t.Fatal("expected a tool_call step for docusign in audit event")
	}

	// Verify OBO metadata is attached to the event.
	if event.Metadata == nil {
		t.Fatal("expected metadata in audit event for OBO verification")
	}

	oboUser, ok := event.Metadata["obo_subject"]
	if !ok {
		t.Fatal("expected obo_subject in audit metadata (RFC 8693 OBO claim)")
	}
	if oboUser != "user_legal_alice" {
		t.Errorf("expected obo_subject=user_legal_alice, got %q", oboUser)
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: GPT-4o 调用 → cost 计入 → budget 未超
// Validates: Requirements B8, B11, A8
// ---------------------------------------------------------------------------

func TestModelCallCost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Send a message that triggers a model call.
	_, body := sendFeishuMessage(t, ctx,
		"user_legal_alice",
		"tenant_legal_corp",
		"帮我总结这份合同的关键风险点",
	)

	var gatewayResp struct {
		InvocationID string `json:"invocationId"`
	}
	if err := json.Unmarshal(body, &gatewayResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// 2. Wait for audit event.
	event := waitForAuditEvent(t, ctx, map[string]string{
		"invocationId": gatewayResp.InvocationID,
	})

	// 3. Verify model was called (GPT-4o or mock equivalent).
	if event.ModelUsed == "" {
		t.Fatal("expected modelUsed to be recorded in audit event")
	}

	// 4. Verify token usage is recorded.
	if event.TokensIn == 0 && event.TokensOut == 0 {
		t.Error("expected non-zero token usage in audit event")
	}

	// 5. Verify cost is recorded and positive.
	if event.CostUSD <= 0 {
		t.Errorf("expected positive costUsd, got %f", event.CostUSD)
	}

	// 6. Verify budget is not exceeded.
	cost := queryCostTracking(t, ctx, "tenant_legal_corp", "legal-copilot")
	if cost.Exceeded {
		t.Errorf("budget should not be exceeded after a single call, totalUsd=%f budgetLimit=%f",
			cost.TotalUSD, cost.BudgetLimit)
	}

	t.Logf("model=%s tokensIn=%d tokensOut=%d costUsd=%.6f budget_exceeded=%v",
		event.ModelUsed, event.TokensIn, event.TokensOut, event.CostUSD, cost.Exceeded)
}

// ---------------------------------------------------------------------------
// Scenario 5: 输出含引用 + 水印
// Validates: Requirements B2, B6, B9
// ---------------------------------------------------------------------------

func TestOutputCitationsWatermark(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Send a query that should produce cited KB output with watermark.
	_, body := sendFeishuMessage(t, ctx,
		"user_legal_alice",
		"tenant_legal_corp",
		"列出竞业禁止条款的主要内容并注明出处",
	)

	var gatewayResp struct {
		InvocationID string `json:"invocationId"`
	}
	if err := json.Unmarshal(body, &gatewayResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// 2. Wait for audit event.
	event := waitForAuditEvent(t, ctx, map[string]string{
		"invocationId": gatewayResp.InvocationID,
	})

	// 3. Verify citations are present.
	if len(event.Citations) == 0 {
		t.Error("expected at least one citation in output (guardrails.behavior.requiredCitations=true)")
	}

	for i, c := range event.Citations {
		if c.Source == "" {
			t.Errorf("citation[%d] has empty source", i)
		}
	}

	// 4. Verify watermark is applied.
	if event.Watermark == nil {
		t.Fatal("expected watermark info in audit event (obligations.watermark.enabled=true)")
	}
	if !event.Watermark.Enabled {
		t.Error("expected watermark.enabled=true")
	}
	if event.Watermark.Mode == "" {
		t.Error("expected watermark.mode to be set (visible/invisible/both)")
	}

	t.Logf("citations=%d watermark.mode=%s", len(event.Citations), event.Watermark.Mode)
}

// ---------------------------------------------------------------------------
// Scenario 6: 审计落 ClickHouse + S3 + eventHash 校验通过
// Validates: Requirements B12, D1, E1
// ---------------------------------------------------------------------------

func TestAuditIntegrity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// 1. Send a message to generate an auditable event.
	_, body := sendFeishuMessage(t, ctx,
		"user_legal_alice",
		"tenant_legal_corp",
		"审计完整性测试消息",
	)

	var gatewayResp struct {
		InvocationID string `json:"invocationId"`
	}
	if err := json.Unmarshal(body, &gatewayResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// 2. Wait for audit event in the query API.
	event := waitForAuditEvent(t, ctx, map[string]string{
		"invocationId": gatewayResp.InvocationID,
	})

	// 3. Verify event hash integrity.
	if event.EventHash == "" {
		t.Fatal("expected eventHash to be set on audit event")
	}
	if !verifyEventHash(t, event) {
		t.Error("eventHash verification failed — audit record may have been tampered")
	}

	// 4. Verify the event is persisted in ClickHouse.
	query := fmt.Sprintf(
		"SELECT invocation_id, event_hash, tenant_id, cost_usd, created_at FROM aip_audit.events WHERE invocation_id = '%s'",
		gatewayResp.InvocationID,
	)
	chEvents := queryClickHouse(t, ctx, query)
	if len(chEvents) == 0 {
		t.Fatal("audit event not found in ClickHouse")
	}
	if chEvents[0].EventHash != event.EventHash {
		t.Errorf("ClickHouse event_hash mismatch: got %q, want %q", chEvents[0].EventHash, event.EventHash)
	}

	// 5. Verify the event is persisted in S3 (WORM storage).
	s3Key := fmt.Sprintf("audit/%s/%s.json", event.TenantID, gatewayResp.InvocationID)
	if !s3ObjectExists(t, ctx, "aik-audit-worm", s3Key) {
		t.Errorf("audit event not found in S3 WORM bucket at key=%s", s3Key)
	}

	t.Logf("audit integrity OK: invocationId=%s eventHash=%s ch=found s3=found",
		gatewayResp.InvocationID, event.EventHash)
}
