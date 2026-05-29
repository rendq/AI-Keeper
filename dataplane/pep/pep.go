// Package pep implements the Policy Enforcement Point for the AIP data plane.
//
// The PEP is the central enforcement component that coordinates with PDP,
// DLP, IdentityBroker, KB ACL, and token verification. It implements
// fail-closed semantics: if ANY component fails, the request is denied
// and an audit event is written with decision=deny and a reason.
//
// Requirements: F6, A4.10, A5.13, B2.8, B9.7
package pep

import (
	"context"
	"fmt"
	"time"
)

// Decision represents a PEP enforcement decision.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

// EnforcementResult is the output of PEP enforcement.
type EnforcementResult struct {
	Decision Decision
	Reason   string
}

// AuditRecord captures the audit trail for a PEP decision.
type AuditRecord struct {
	InvocationID string
	Timestamp    time.Time
	Decision     Decision
	Reason       string
	Principal    string
	Action       string
	Resource     string
}

// Request represents an incoming request to be enforced by the PEP.
type Request struct {
	InvocationID string
	Principal    string
	TenantID     string
	AgentName    string
	Action       string
	Resource     string
	Token        string
	InputText    string // for DLP inspection
	KBResource   string // KB resource for ACL check
}

// PDPClient is the interface for calling the PDP.
type PDPClient interface {
	Decide(ctx context.Context, req *PDPRequest) (*PDPResponse, error)
}

// PDPRequest is the input to the PDP.
type PDPRequest struct {
	Principal string
	TenantID  string
	AgentName string
	Action    string
	Resource  string
}

// PDPResponse is the output of the PDP.
type PDPResponse struct {
	Decision string // "allow" or "deny"
	Reason   string
}

// DLPInspector is the interface for calling the DLP engine.
type DLPInspector interface {
	Inspect(ctx context.Context, text string) (*DLPResult, error)
}

// DLPResult is the output of DLP inspection.
type DLPResult struct {
	Blocked bool
	Reason  string
}

// IdentityVerifier is the interface for identity/token verification.
type IdentityVerifier interface {
	Verify(ctx context.Context, token string) (*IdentityResult, error)
}

// IdentityResult is the output of identity verification.
type IdentityResult struct {
	Valid     bool
	Subject   string
	TenantID  string
	AgentName string
}

// KBACLChecker is the interface for KB ACL pre-filter checks.
type KBACLChecker interface {
	CheckAccess(ctx context.Context, principal string, resource string) (*KBACLResult, error)
}

// KBACLResult is the output of KB ACL check.
type KBACLResult struct {
	Allowed bool
	Reason  string
}

// TokenValidator is the interface for token validation (expiry, revocation).
type TokenValidator interface {
	Validate(ctx context.Context, token string) (*TokenResult, error)
}

// TokenResult is the output of token validation.
type TokenResult struct {
	Valid  bool
	Reason string
}

// AuditSink is the interface for writing audit events.
type AuditSink interface {
	Write(record AuditRecord) error
}

// Enforcer is the PEP enforcement engine. It coordinates all security
// components and enforces fail-closed semantics.
type Enforcer struct {
	pdp      PDPClient
	dlp      DLPInspector
	identity IdentityVerifier
	kbACL    KBACLChecker
	token    TokenValidator
	audit    AuditSink
}

// NewEnforcer creates a PEP Enforcer with all required dependencies.
func NewEnforcer(pdp PDPClient, dlp DLPInspector, identity IdentityVerifier, kbACL KBACLChecker, token TokenValidator, audit AuditSink) *Enforcer {
	return &Enforcer{
		pdp:      pdp,
		dlp:      dlp,
		identity: identity,
		kbACL:    kbACL,
		token:    token,
		audit:    audit,
	}
}

// Enforce runs the full enforcement pipeline. If ANY component fails or denies,
// the result is deny and an audit record is written.
//
// Enforcement order:
// 1. Token validation
// 2. Identity verification
// 3. PDP policy decision
// 4. DLP inspection
// 5. KB ACL check (if applicable)
//
// Fail-closed: any error or denial at any stage → deny + audit.
func (e *Enforcer) Enforce(ctx context.Context, req Request) EnforcementResult {
	now := time.Now()

	// 1. Token validation
	if e.token != nil {
		tokenResult, err := e.token.Validate(ctx, req.Token)
		if err != nil {
			result := EnforcementResult{Decision: DecisionDeny, Reason: fmt.Sprintf("token_error: %v", err)}
			e.writeAudit(req, result, now)
			return result
		}
		if !tokenResult.Valid {
			reason := "token_invalid"
			if tokenResult.Reason != "" {
				reason = fmt.Sprintf("token_invalid: %s", tokenResult.Reason)
			}
			result := EnforcementResult{Decision: DecisionDeny, Reason: reason}
			e.writeAudit(req, result, now)
			return result
		}
	}

	// 2. Identity verification
	if e.identity != nil {
		identityResult, err := e.identity.Verify(ctx, req.Token)
		if err != nil {
			result := EnforcementResult{Decision: DecisionDeny, Reason: fmt.Sprintf("identity_error: %v", err)}
			e.writeAudit(req, result, now)
			return result
		}
		if !identityResult.Valid {
			result := EnforcementResult{Decision: DecisionDeny, Reason: "identity_invalid"}
			e.writeAudit(req, result, now)
			return result
		}
	}

	// 3. PDP policy decision
	if e.pdp != nil {
		pdpResp, err := e.pdp.Decide(ctx, &PDPRequest{
			Principal: req.Principal,
			TenantID:  req.TenantID,
			AgentName: req.AgentName,
			Action:    req.Action,
			Resource:  req.Resource,
		})
		if err != nil {
			result := EnforcementResult{Decision: DecisionDeny, Reason: fmt.Sprintf("pdp_error: %v", err)}
			e.writeAudit(req, result, now)
			return result
		}
		if pdpResp.Decision != "allow" {
			reason := "pdp_deny"
			if pdpResp.Reason != "" {
				reason = fmt.Sprintf("pdp_deny: %s", pdpResp.Reason)
			}
			result := EnforcementResult{Decision: DecisionDeny, Reason: reason}
			e.writeAudit(req, result, now)
			return result
		}
	}

	// 4. DLP inspection
	if e.dlp != nil && req.InputText != "" {
		dlpResult, err := e.dlp.Inspect(ctx, req.InputText)
		if err != nil {
			result := EnforcementResult{Decision: DecisionDeny, Reason: fmt.Sprintf("dlp_error: %v", err)}
			e.writeAudit(req, result, now)
			return result
		}
		if dlpResult.Blocked {
			reason := "dlp_blocked"
			if dlpResult.Reason != "" {
				reason = fmt.Sprintf("dlp_blocked: %s", dlpResult.Reason)
			}
			result := EnforcementResult{Decision: DecisionDeny, Reason: reason}
			e.writeAudit(req, result, now)
			return result
		}
	}

	// 5. KB ACL check
	if e.kbACL != nil && req.KBResource != "" {
		aclResult, err := e.kbACL.CheckAccess(ctx, req.Principal, req.KBResource)
		if err != nil {
			result := EnforcementResult{Decision: DecisionDeny, Reason: fmt.Sprintf("kb_acl_error: %v", err)}
			e.writeAudit(req, result, now)
			return result
		}
		if !aclResult.Allowed {
			reason := "kb_acl_denied"
			if aclResult.Reason != "" {
				reason = fmt.Sprintf("kb_acl_denied: %s", aclResult.Reason)
			}
			result := EnforcementResult{Decision: DecisionDeny, Reason: reason}
			e.writeAudit(req, result, now)
			return result
		}
	}

	// All checks passed → allow
	result := EnforcementResult{Decision: DecisionAllow, Reason: ""}
	e.writeAudit(req, result, now)
	return result
}

// writeAudit writes an audit record for the enforcement decision.
func (e *Enforcer) writeAudit(req Request, result EnforcementResult, ts time.Time) {
	if e.audit == nil {
		return
	}
	_ = e.audit.Write(AuditRecord{
		InvocationID: req.InvocationID,
		Timestamp:    ts,
		Decision:     result.Decision,
		Reason:       result.Reason,
		Principal:    req.Principal,
		Action:       req.Action,
		Resource:     req.Resource,
	})
}
