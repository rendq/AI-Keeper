//go:build pbt

// Feature: ai-platform, Property 12: fail-closed 默认拒绝
//
// Generator: 随机注入 PDP / DLP / IdentityBroker / KB ACL / token 失败模式
// Oracle: 任一失败 ⇒ PEP deny + 写审计 decision=deny + reason=...
// Property: P12 / Validates: F6, A4.10, A5.13, B2.8, B9.7

package pep

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func pbtSeed() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// ---------------------------------------------------------------------------
// Failure mode types
// ---------------------------------------------------------------------------

// FailureComponent identifies which component is injected with a failure.
type FailureComponent int

const (
	FailPDP FailureComponent = iota
	FailDLP
	FailIdentityBroker
	FailKBACL
	FailToken
)

func (f FailureComponent) String() string {
	switch f {
	case FailPDP:
		return "PDP"
	case FailDLP:
		return "DLP"
	case FailIdentityBroker:
		return "IdentityBroker"
	case FailKBACL:
		return "KBACL"
	case FailToken:
		return "Token"
	default:
		return "Unknown"
	}
}

// FailureMode describes how a component fails.
type FailureMode int

const (
	// FailModeError means the component returns an error (e.g., timeout, network issue).
	FailModeError FailureMode = iota
	// FailModeDeny means the component returns a deny/blocked/invalid result.
	FailModeDeny
)

func (f FailureMode) String() string {
	switch f {
	case FailModeError:
		return "Error"
	case FailModeDeny:
		return "Deny"
	default:
		return "Unknown"
	}
}

// FailureScenario describes a single failure injection.
type FailureScenario struct {
	Component FailureComponent
	Mode      FailureMode
	ErrorMsg  string // for error mode
}

// ---------------------------------------------------------------------------
// Mock implementations that can be configured to fail
// ---------------------------------------------------------------------------

type mockPDP struct {
	shouldError bool
	shouldDeny  bool
	errorMsg    string
}

func (m *mockPDP) Decide(_ context.Context, _ *PDPRequest) (*PDPResponse, error) {
	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}
	if m.shouldDeny {
		return &PDPResponse{Decision: "deny", Reason: "policy_violation"}, nil
	}
	return &PDPResponse{Decision: "allow", Reason: ""}, nil
}

type mockDLP struct {
	shouldError bool
	shouldBlock bool
	errorMsg    string
}

func (m *mockDLP) Inspect(_ context.Context, _ string) (*DLPResult, error) {
	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}
	if m.shouldBlock {
		return &DLPResult{Blocked: true, Reason: "PII_detected"}, nil
	}
	return &DLPResult{Blocked: false}, nil
}

type mockIdentity struct {
	shouldError bool
	shouldDeny  bool
	errorMsg    string
}

func (m *mockIdentity) Verify(_ context.Context, _ string) (*IdentityResult, error) {
	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}
	if m.shouldDeny {
		return &IdentityResult{Valid: false}, nil
	}
	return &IdentityResult{Valid: true, Subject: "user-1", TenantID: "t1", AgentName: "agent-1"}, nil
}

type mockKBACL struct {
	shouldError bool
	shouldDeny  bool
	errorMsg    string
}

func (m *mockKBACL) CheckAccess(_ context.Context, _ string, _ string) (*KBACLResult, error) {
	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}
	if m.shouldDeny {
		return &KBACLResult{Allowed: false, Reason: "insufficient_permission"}, nil
	}
	return &KBACLResult{Allowed: true}, nil
}

type mockToken struct {
	shouldError bool
	shouldDeny  bool
	errorMsg    string
}

func (m *mockToken) Validate(_ context.Context, _ string) (*TokenResult, error) {
	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}
	if m.shouldDeny {
		return &TokenResult{Valid: false, Reason: "expired"}, nil
	}
	return &TokenResult{Valid: true}, nil
}

// recordingAuditSink captures audit records for verification.
type recordingAuditSink struct {
	records []AuditRecord
}

func (s *recordingAuditSink) Write(record AuditRecord) error {
	s.records = append(s.records, record)
	return nil
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func genFailureComponent() gopter.Gen {
	return gen.IntRange(0, 4).Map(func(i int) FailureComponent {
		return FailureComponent(i)
	})
}

func genFailureMode() gopter.Gen {
	return gen.IntRange(0, 1).Map(func(i int) FailureMode {
		return FailureMode(i)
	})
}

func genErrorMsg() gopter.Gen {
	msgs := []string{
		"connection refused",
		"context deadline exceeded",
		"service unavailable",
		"internal server error",
		"network unreachable",
		"timeout waiting for response",
		"TLS handshake failed",
		"DNS resolution failed",
		"circuit breaker open",
		"resource exhausted",
	}
	return gen.IntRange(0, len(msgs)-1).Map(func(i int) string {
		return msgs[i]
	})
}

func genFailureScenario() gopter.Gen {
	return gopter.CombineGens(
		genFailureComponent(),
		genFailureMode(),
		genErrorMsg(),
	).Map(func(values []interface{}) FailureScenario {
		return FailureScenario{
			Component: values[0].(FailureComponent),
			Mode:      values[1].(FailureMode),
			ErrorMsg:  values[2].(string),
		}
	})
}

func genPrincipal() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9]{2,8}").Map(func(s string) string {
		return "user-" + s
	})
}

func genInvocationID() gopter.Gen {
	return gen.UInt64().Map(func(v uint64) string {
		return fmt.Sprintf("inv-%016x", v)
	})
}

func genAction() gopter.Gen {
	actions := []string{"invoke", "query", "execute", "create", "read", "write", "delete"}
	return gen.IntRange(0, len(actions)-1).Map(func(i int) string {
		return actions[i]
	})
}

func genResource() gopter.Gen {
	resources := []string{
		"skill://ns/contract-review",
		"skill://ns/search",
		"tool://ns/docusign",
		"agent://ns/copilot",
		"kb://ns/legal",
		"model://ns/gpt-4o",
	}
	return gen.IntRange(0, len(resources)-1).Map(func(i int) string {
		return resources[i]
	})
}

func genInputText() gopter.Gen {
	texts := []string{
		"Please review this contract",
		"Find documents about compliance",
		"Execute the signing workflow",
		"What is the policy on data retention?",
		"Summarize the legal brief",
	}
	return gen.IntRange(0, len(texts)-1).Map(func(i int) string {
		return texts[i]
	})
}

func genKBResource() gopter.Gen {
	resources := []string{
		"kb://ns/legal/section-1",
		"kb://ns/legal/confidential",
		"kb://ns/finance/reports",
		"kb://ns/hr/policies",
		"",
	}
	return gen.IntRange(0, len(resources)-1).Map(func(i int) string {
		return resources[i]
	})
}

// ---------------------------------------------------------------------------
// TestProperty12 — fail-closed 默认拒绝
//
// **Validates: Requirements F6, A4.10, A5.13, B2.8, B9.7**
//
// When ANY security component (PDP, DLP, IdentityBroker, KB ACL, or token
// validation) fails — either by returning an error or by returning a deny/
// block/invalid result — the PEP MUST:
//   1. Return decision=deny
//   2. Write an audit record with decision=deny and a non-empty reason
//
// This tests the fail-closed invariant: no component failure can result in
// an allow decision.
// ---------------------------------------------------------------------------

func TestProperty12(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Property 12: Any component failure ⇒ PEP deny + audit with decision=deny and reason
	properties.Property("fail-closed: any component failure results in deny + audit", prop.ForAll(
		func(scenario FailureScenario, principal string, invID string, action string, resource string, inputText string, kbResource string) (bool, error) {
			// Set up mocks: all pass by default
			pdpMock := &mockPDP{}
			dlpMock := &mockDLP{}
			identityMock := &mockIdentity{}
			kbaclMock := &mockKBACL{}
			tokenMock := &mockToken{}
			auditSink := &recordingAuditSink{}

			// Inject failure into the target component
			switch scenario.Component {
			case FailPDP:
				if scenario.Mode == FailModeError {
					pdpMock.shouldError = true
					pdpMock.errorMsg = scenario.ErrorMsg
				} else {
					pdpMock.shouldDeny = true
				}
			case FailDLP:
				if scenario.Mode == FailModeError {
					dlpMock.shouldError = true
					dlpMock.errorMsg = scenario.ErrorMsg
				} else {
					dlpMock.shouldBlock = true
				}
			case FailIdentityBroker:
				if scenario.Mode == FailModeError {
					identityMock.shouldError = true
					identityMock.errorMsg = scenario.ErrorMsg
				} else {
					identityMock.shouldDeny = true
				}
			case FailKBACL:
				if scenario.Mode == FailModeError {
					kbaclMock.shouldError = true
					kbaclMock.errorMsg = scenario.ErrorMsg
				} else {
					kbaclMock.shouldDeny = true
				}
			case FailToken:
				if scenario.Mode == FailModeError {
					tokenMock.shouldError = true
					tokenMock.errorMsg = scenario.ErrorMsg
				} else {
					tokenMock.shouldDeny = true
				}
			}

			// For DLP to be checked, inputText must be non-empty
			if scenario.Component == FailDLP && inputText == "" {
				inputText = "some text to trigger DLP"
			}

			// For KB ACL to be checked, kbResource must be non-empty
			if scenario.Component == FailKBACL && kbResource == "" {
				kbResource = "kb://ns/legal/secret-doc"
			}

			// Create enforcer and execute
			enforcer := NewEnforcer(pdpMock, dlpMock, identityMock, kbaclMock, tokenMock, auditSink)
			req := Request{
				InvocationID: invID,
				Principal:    principal,
				TenantID:     "tenant-test",
				AgentName:    "agent-test",
				Action:       action,
				Resource:     resource,
				Token:        "test-token-123",
				InputText:    inputText,
				KBResource:   kbResource,
			}

			result := enforcer.Enforce(context.Background(), req)

			// Oracle 1: Decision MUST be deny
			if result.Decision != DecisionDeny {
				return false, fmt.Errorf(
					"fail-closed violated: component=%s mode=%s but decision=%s (expected deny)",
					scenario.Component, scenario.Mode, result.Decision,
				)
			}

			// Oracle 2: Reason MUST be non-empty
			if result.Reason == "" {
				return false, fmt.Errorf(
					"fail-closed audit: component=%s mode=%s decision=deny but reason is empty",
					scenario.Component, scenario.Mode,
				)
			}

			// Oracle 3: An audit record MUST have been written
			if len(auditSink.records) == 0 {
				return false, fmt.Errorf(
					"fail-closed audit: component=%s mode=%s no audit record written",
					scenario.Component, scenario.Mode,
				)
			}

			// Oracle 4: Audit record must have decision=deny and non-empty reason
			auditRecord := auditSink.records[len(auditSink.records)-1]
			if auditRecord.Decision != DecisionDeny {
				return false, fmt.Errorf(
					"fail-closed audit: component=%s mode=%s audit.decision=%s (expected deny)",
					scenario.Component, scenario.Mode, auditRecord.Decision,
				)
			}
			if auditRecord.Reason == "" {
				return false, fmt.Errorf(
					"fail-closed audit: component=%s mode=%s audit.reason is empty",
					scenario.Component, scenario.Mode,
				)
			}

			// Oracle 5: Reason should reference the failing component
			componentKeywords := map[FailureComponent][]string{
				FailPDP:            {"pdp"},
				FailDLP:            {"dlp"},
				FailIdentityBroker: {"identity"},
				FailKBACL:         {"kb_acl"},
				FailToken:          {"token"},
			}
			keywords := componentKeywords[scenario.Component]
			found := false
			reasonLower := strings.ToLower(result.Reason)
			for _, kw := range keywords {
				if strings.Contains(reasonLower, kw) {
					found = true
					break
				}
			}
			if !found {
				return false, fmt.Errorf(
					"fail-closed reason: component=%s mode=%s reason=%q does not reference component keywords %v",
					scenario.Component, scenario.Mode, result.Reason, keywords,
				)
			}

			return true, nil
		},
		genFailureScenario(),
		genPrincipal(),
		genInvocationID(),
		genAction(),
		genResource(),
		genInputText(),
		genKBResource(),
	))

	properties.TestingRun(t)
}
