package webhook

import (
	"context"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// minimalSkill builds a structurally valid Skill so individual tests
// can mutate one field at a time and isolate the validator's response.
func minimalSkill() *skillv1alpha1.Skill {
	return &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contract-review",
			Namespace: "default",
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   shared.SemVer("1.0.0"),
			Stability: shared.StageBeta,
			Interface: skillv1alpha1.SkillInterface{
				Input:  skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)}},
				Output: skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)}},
			},
			Implementation: skillv1alpha1.SkillImplementation{
				Type: "function",
				Requires: &skillv1alpha1.SkillRequires{
					Tools: []skillv1alpha1.SkillToolDep{
						{Ref: shared.ResourceRef("tool://docusign/create-envelope@v3")},
					},
				},
			},
			Reliability: &shared.ReliabilityBlock{
				Timeout: ptrDuration("30s"),
			},
		},
	}
}

func ptrDuration(s string) *shared.Duration {
	d := shared.Duration(s)
	return &d
}

// TestSkillValidator covers the core invariants from Requirements A1.3
// and A2.1—A2.6.
//
// Validates: Requirements A1.3, A2.1, A2.2, A2.3, A2.4, A2.5, A2.6.
func TestSkillValidator(t *testing.T) {
	t.Parallel()
	v := &SkillValidator{}
	ctx := context.Background()

	tests := []struct {
		name      string
		mutate    func(*skillv1alpha1.Skill)
		wantError string
	}{
		{
			name:      "valid",
			mutate:    func(*skillv1alpha1.Skill) {},
			wantError: "",
		},
		{
			name: "bad ResourceRef in tools.ref",
			mutate: func(s *skillv1alpha1.Skill) {
				s.Spec.Implementation.Requires.Tools[0].Ref = shared.ResourceRef("not-a-ref")
			},
			wantError: "implementation.requires.tools[0].ref",
		},
		{
			name: "bad SemVer in spec.version",
			mutate: func(s *skillv1alpha1.Skill) {
				s.Spec.Version = shared.SemVer("v1.2.3")
			},
			wantError: "spec.version",
		},
		{
			name: "bad Duration in reliability.timeout",
			mutate: func(s *skillv1alpha1.Skill) {
				s.Spec.Reliability.Timeout = ptrDuration("5min")
			},
			wantError: "reliability.timeout",
		},
		{
			name: "bad Stage in spec.stability",
			mutate: func(s *skillv1alpha1.Skill) {
				s.Spec.Stability = shared.Stage("ga")
			},
			wantError: "spec.stability",
		},
		{
			name: "bad metadata.name",
			mutate: func(s *skillv1alpha1.Skill) {
				s.ObjectMeta.Name = "Invalid_Name"
			},
			wantError: "metadata.name",
		},
		{
			name: "bad classification in governance",
			mutate: func(s *skillv1alpha1.Skill) {
				cls := shared.Classification("topsecret")
				s.Spec.Governance = &shared.GovernanceBlock{Classification: &cls}
			},
			wantError: "governance.classification",
		},
		{
			name: "bad ResourceRef in models.fallback",
			mutate: func(s *skillv1alpha1.Skill) {
				s.Spec.Implementation.Requires.Models = []skillv1alpha1.SkillModelDep{{
					Alias: "reasoner",
					Ref:   shared.ResourceRef("model://gpt-4o-eu@2024-05-13"),
					Fallback: []shared.ResourceRef{
						"INVALID-fallback",
					},
				}}
			},
			wantError: "models[0].fallback[0]",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := minimalSkill()
			tc.mutate(s)
			_, err := v.ValidateCreate(ctx, s)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantError)
			}
		})
	}
}

// TestSkillValidator_ValidateUpdate sanity-checks UPDATE wiring.
func TestSkillValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()
	v := &SkillValidator{}
	old := minimalSkill()
	newer := minimalSkill()
	newer.Spec.Version = shared.SemVer("not-semver")
	if _, err := v.ValidateUpdate(context.Background(), old, newer); err == nil {
		t.Fatalf("expected error for bad SemVer on update, got nil")
	}
}

// TestSkillValidator_ValidateDelete confirms delete is a no-op for
// Skill (admission delete-time validation is handled by the controller
// finaliser, not the webhook).
func TestSkillValidator_ValidateDelete(t *testing.T) {
	t.Parallel()
	v := &SkillValidator{}
	if _, err := v.ValidateDelete(context.Background(), minimalSkill()); err != nil {
		t.Fatalf("expected nil error on delete, got %v", err)
	}
}

// minimalTool produces a Tool that passes every webhook check.
func minimalTool() *skillv1alpha1.Tool {
	return &skillv1alpha1.Tool{
		ObjectMeta: metav1.ObjectMeta{Name: "docusign", Namespace: "default"},
		Spec: skillv1alpha1.ToolSpec{
			Protocol: "mcp",
			Endpoint: "https://api.docusign.example.com",
			Schema: skillv1alpha1.ToolSchema{
				Input:  &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
				Output: &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
			},
			Governance: skillv1alpha1.ToolGovernance{
				SideEffects: "read_only",
			},
		},
	}
}

// TestToolValidator exercises the cross-field rule for destructive
// tools (Requirement A9.2 lint rule, elevated to admission to make it
// non-bypassable) plus oauth2_obo precondition.
//
// Validates: Requirements A1.3, A2.1, A2.5.
func TestToolValidator(t *testing.T) {
	t.Parallel()
	v := &ToolValidator{}
	ctx := context.Background()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		if _, err := v.ValidateCreate(ctx, minimalTool()); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("destructive without approval", func(t *testing.T) {
		t.Parallel()
		tl := minimalTool()
		tl.Spec.Governance.SideEffects = "destructive"
		_, err := v.ValidateCreate(ctx, tl)
		if err == nil || !strings.Contains(err.Error(), "requiresApproval") {
			t.Fatalf("expected requiresApproval error, got %v", err)
		}
	})

	t.Run("destructive with approval", func(t *testing.T) {
		t.Parallel()
		tl := minimalTool()
		tl.Spec.Governance.SideEffects = "destructive"
		approval := true
		tl.Spec.Governance.RequiresApproval = &approval
		if _, err := v.ValidateCreate(ctx, tl); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("oauth2_obo without tokenExchangeRef", func(t *testing.T) {
		t.Parallel()
		tl := minimalTool()
		tl.Spec.Authentication = &skillv1alpha1.ToolAuthentication{
			Mode: "oauth2_obo",
		}
		_, err := v.ValidateCreate(ctx, tl)
		if err == nil || !strings.Contains(err.Error(), "tokenExchangeRef") {
			t.Fatalf("expected tokenExchangeRef error, got %v", err)
		}
	})

	t.Run("bad metadata.name", func(t *testing.T) {
		t.Parallel()
		tl := minimalTool()
		tl.ObjectMeta.Name = strings.Repeat("a", 254)
		_, err := v.ValidateCreate(ctx, tl)
		if err == nil || !strings.Contains(err.Error(), "metadata.name") {
			t.Fatalf("expected metadata.name error, got %v", err)
		}
	})
}

// minimalAgent builds a structurally valid Agent.
func minimalAgent() *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "legal-copilot", Namespace: "default"},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: "Legal Copilot",
			Identity: agentv1alpha1.AgentIdentity{
				ServiceAccount: "legal-bot",
			},
			Skills: []agentv1alpha1.AgentSkillBinding{
				{Ref: shared.ResourceRef("skill://contract-review")},
			},
			Runtime: agentv1alpha1.AgentRuntime{
				Pattern: "tool_calling",
				Timeout: ptrDuration("5m"),
			},
		},
	}
}

// TestAgentValidator covers Agent-side ResourceRef and Duration checks.
//
// Validates: Requirements A1.3, A2.1, A2.2.
func TestAgentValidator(t *testing.T) {
	t.Parallel()
	v := &AgentValidator{}
	ctx := context.Background()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		if _, err := v.ValidateCreate(ctx, minimalAgent()); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("bad skill ref", func(t *testing.T) {
		t.Parallel()
		a := minimalAgent()
		a.Spec.Skills[0].Ref = shared.ResourceRef("not-a-skill-ref")
		_, err := v.ValidateCreate(ctx, a)
		if err == nil || !strings.Contains(err.Error(), "skills[0].ref") {
			t.Fatalf("expected skills[0].ref error, got %v", err)
		}
	})

	t.Run("bad timeout", func(t *testing.T) {
		t.Parallel()
		a := minimalAgent()
		a.Spec.Runtime.Timeout = ptrDuration("5min")
		_, err := v.ValidateCreate(ctx, a)
		if err == nil || !strings.Contains(err.Error(), "runtime.timeout") {
			t.Fatalf("expected runtime.timeout error, got %v", err)
		}
	})

	t.Run("bad channel ref", func(t *testing.T) {
		t.Parallel()
		a := minimalAgent()
		bad := shared.ResourceRef("BAD")
		a.Spec.Channels = []agentv1alpha1.AgentChannel{
			{Kind: "feishu", Ref: &bad},
		}
		_, err := v.ValidateCreate(ctx, a)
		if err == nil || !strings.Contains(err.Error(), "channels[0].ref") {
			t.Fatalf("expected channels[0].ref error, got %v", err)
		}
	})
}

// minimalPolicy builds a Policy that passes baseline validation.
func minimalPolicy() *policyv1alpha1.Policy {
	pri := int32(100)
	return &policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "legal-acl", Namespace: "default"},
		Spec: policyv1alpha1.PolicySpec{
			Effect:   "allow",
			Priority: &pri,
			Subject: policyv1alpha1.SubjectSelector{
				AnyOf: []policyv1alpha1.SubjectEntry{{Kind: "Agent"}},
			},
			Action: policyv1alpha1.PolicyAction{
				Verbs: []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{
					AnyOf: []policyv1alpha1.ResourceSelector{{Kind: "Skill"}},
				},
			},
		},
	}
}

// TestPolicyValidator exercises Policy ResourceRef/Duration paths.
//
// Validates: Requirements A1.3, A2.1, A2.2.
func TestPolicyValidator(t *testing.T) {
	t.Parallel()
	v := &PolicyValidator{}
	ctx := context.Background()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		if _, err := v.ValidateCreate(ctx, minimalPolicy()); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("bad obligations.redact.patternsRef", func(t *testing.T) {
		t.Parallel()
		p := minimalPolicy()
		bad := shared.ResourceRef("not-a-ref")
		p.Spec.Obligations = &policyv1alpha1.PolicyObligations{
			Redact: &policyv1alpha1.ObligationRedact{PatternsRef: &bad},
		}
		_, err := v.ValidateCreate(ctx, p)
		if err == nil || !strings.Contains(err.Error(), "obligations.redact.patternsRef") {
			t.Fatalf("expected patternsRef error, got %v", err)
		}
	})

	t.Run("bad approvals timeout", func(t *testing.T) {
		t.Parallel()
		p := minimalPolicy()
		p.Spec.Approvals = []policyv1alpha1.ApprovalSpec{{
			When:     policyv1alpha1.ApprovalWhen{},
			Approver: policyv1alpha1.ApprovalApprover{Kind: "User", Name: "alice"},
			Timeout:  ptrDuration("4hours"),
		}}
		_, err := v.ValidateCreate(ctx, p)
		if err == nil || !strings.Contains(err.Error(), "approvals[0].timeout") {
			t.Fatalf("expected approvals[0].timeout error, got %v", err)
		}
	})
}
