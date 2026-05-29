//go:build e2e

package p1_test

import (
	"testing"
)

// TestE2E_GuardrailBlocksPromptInjection verifies that the guardrail engine
// detects and blocks prompt injection attacks before they reach the LLM.
//
// Scenario:
//  1. Deploy a Skill with a GuardrailPolicy attached (prompt-injection detector).
//  2. Send a request containing a known injection pattern.
//  3. Assert the request is rejected with reason "GuardrailBlocked".
//  4. Verify an audit event is emitted recording the block.
//
// Validates: Requirements related to Guardrail Engine (P1)
func TestE2E_GuardrailBlocksPromptInjection(t *testing.T) {
	t.Skip("requires kind cluster with full P1 stack deployed")

	// TODO: setup — create Skill + GuardrailPolicy via k8s client
	// TODO: send injection payload to skill endpoint
	// TODO: assert HTTP 403 / deny decision with reason "GuardrailBlocked"
	// TODO: poll audit sink for corresponding block event
}

// TestE2E_SandboxIsolation verifies that a sandboxed Skill execution environment
// prevents network escape (no egress beyond allowed destinations).
//
// Scenario:
//  1. Deploy a Skill configured with sandbox isolation (gVisor / network policy).
//  2. From within the sandbox, attempt to reach an external endpoint.
//  3. Assert the connection is refused or times out (NetworkPolicy blocks it).
//  4. Confirm the Skill can still reach its allowed upstream (e.g., model endpoint).
//
// Validates: Requirements related to Sandbox Isolation (P1)
func TestE2E_SandboxIsolation(t *testing.T) {
	t.Skip("requires kind cluster with full P1 stack deployed")

	// TODO: deploy Skill with sandbox enabled (gVisor runtime class + netpol)
	// TODO: exec into sandbox pod and attempt curl to disallowed external host
	// TODO: assert connection failure (timeout or reset)
	// TODO: verify allowed egress (model endpoint) still works
}

// TestE2E_EvalStageGate verifies that the eval runner gates Skill promotion
// from staging to production based on evaluation pass/fail criteria.
//
// Scenario:
//  1. Deploy a Skill in staging with an EvalRun referencing a test dataset.
//  2. Trigger the eval run; mock LLM returns results that PASS the threshold.
//  3. Assert the EvalRun status transitions to Passed and the Skill is promoted.
//  4. Repeat with a failing eval — assert Skill stays in staging.
//
// Validates: Requirements related to Eval Stage Gate (P1)
func TestE2E_EvalStageGate(t *testing.T) {
	t.Skip("requires kind cluster with full P1 stack deployed")

	// TODO: create EvalDataset + EvalRun CR targeting a staging Skill
	// TODO: configure mock-llm to return scores above threshold
	// TODO: wait for EvalRun status.phase == "Passed"
	// TODO: assert Skill condition shows promoted / ready

	// Negative case: eval fails
	// TODO: configure mock-llm to return scores below threshold
	// TODO: trigger new EvalRun
	// TODO: wait for EvalRun status.phase == "Failed"
	// TODO: assert Skill remains in staging (not promoted)
}
