package lint

import (
	"testing"
	"time"
)

func TestAllRulesCount(t *testing.T) {
	rules := AllRules()
	if len(rules) != 14 {
		t.Fatalf("expected 14 rules, got %d", len(rules))
	}
}

// --- Error rule tests ---

func TestSkillHasEvalSet_StableWithoutEvalSet(t *testing.T) {
	res := &Resource{
		Kind: "Skill",
		Name: "my-skill",
		Spec: map[string]interface{}{
			"stability": "stable",
			"version":   "1.0.0",
		},
	}
	rule := &SkillHasEvalSet{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelError {
		t.Errorf("expected error level, got %s", violations[0].Level)
	}
	if violations[0].Rule != "skill/has-eval-set" {
		t.Errorf("expected rule skill/has-eval-set, got %s", violations[0].Rule)
	}
}

func TestSkillHasEvalSet_StableWithEvalSet(t *testing.T) {
	res := &Resource{
		Kind: "Skill",
		Name: "my-skill",
		Spec: map[string]interface{}{
			"stability": "stable",
			"version":   "1.0.0",
			"evaluation": map[string]interface{}{
				"evalSet": "ref://eval/my-eval",
			},
		},
	}
	rule := &SkillHasEvalSet{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestSkillHasEvalSet_ExperimentalSkipped(t *testing.T) {
	res := &Resource{
		Kind: "Skill",
		Name: "my-skill",
		Spec: map[string]interface{}{
			"stability": "experimental",
		},
	}
	rule := &SkillHasEvalSet{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations for experimental, got %d", len(violations))
	}
}

func TestToolDestructiveNeedsApproval_DestructiveWithoutApproval(t *testing.T) {
	res := &Resource{
		Kind: "Tool",
		Name: "delete-tool",
		Spec: map[string]interface{}{
			"governance": map[string]interface{}{
				"sideEffects": "destructive",
			},
		},
	}
	rule := &ToolDestructiveNeedsApproval{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelError {
		t.Errorf("expected error level, got %s", violations[0].Level)
	}
}

func TestToolDestructiveNeedsApproval_DestructiveWithApproval(t *testing.T) {
	res := &Resource{
		Kind: "Tool",
		Name: "delete-tool",
		Spec: map[string]interface{}{
			"governance": map[string]interface{}{
				"sideEffects":      "destructive",
				"requiresApproval": true,
			},
		},
	}
	rule := &ToolDestructiveNeedsApproval{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestAgentSkillsResolved_EmptyRef(t *testing.T) {
	res := &Resource{
		Kind: "Agent",
		Name: "my-agent",
		Spec: map[string]interface{}{
			"skills": []interface{}{
				map[string]interface{}{
					"ref": "",
				},
			},
		},
	}
	rule := &AgentSkillsResolved{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelError {
		t.Errorf("expected error, got %s", violations[0].Level)
	}
}

func TestKBACLNotOpen_ConfidentialWithOpen(t *testing.T) {
	res := &Resource{
		Kind: "KnowledgeBase",
		Name: "secret-kb",
		Spec: map[string]interface{}{
			"governance": map[string]interface{}{
				"classification": "confidential",
			},
			"acl": map[string]interface{}{
				"mode": "open",
			},
		},
	}
	rule := &KBACLNotOpen{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelError {
		t.Errorf("expected error, got %s", violations[0].Level)
	}
}

func TestKBACLNotOpen_PublicWithOpen(t *testing.T) {
	res := &Resource{
		Kind: "KnowledgeBase",
		Name: "public-kb",
		Spec: map[string]interface{}{
			"governance": map[string]interface{}{
				"classification": "public",
			},
			"acl": map[string]interface{}{
				"mode": "open",
			},
		},
	}
	rule := &KBACLNotOpen{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations for public, got %d", len(violations))
	}
}

// --- Warn rule tests ---

func TestSkillHasFallback_StableWithoutFallback(t *testing.T) {
	res := &Resource{
		Kind: "Skill",
		Name: "prod-skill",
		Spec: map[string]interface{}{
			"stability": "stable",
		},
	}
	rule := &SkillHasFallback{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelWarn {
		t.Errorf("expected warn level, got %s", violations[0].Level)
	}
}

func TestSkillBudgetSet_NoCost(t *testing.T) {
	res := &Resource{
		Kind: "Skill",
		Name: "my-skill",
		Spec: map[string]interface{}{
			"stability": "stable",
		},
	}
	rule := &SkillBudgetSet{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelWarn {
		t.Errorf("expected warn, got %s", violations[0].Level)
	}
}

func TestPolicySanePriority_ExtremePriority(t *testing.T) {
	for _, p := range []int{0, 1000} {
		res := &Resource{
			Kind: "Policy",
			Name: "extreme-policy",
			Spec: map[string]interface{}{
				"priority": p,
				"effect":   "allow",
			},
		}
		rule := &PolicySanePriority{}
		violations := rule.Check(res)
		if len(violations) != 1 {
			t.Fatalf("priority %d: expected 1 violation, got %d", p, len(violations))
		}
		if violations[0].Level != LevelWarn {
			t.Errorf("expected warn, got %s", violations[0].Level)
		}
	}
}

func TestPolicySanePriority_NormalPriority(t *testing.T) {
	res := &Resource{
		Kind: "Policy",
		Name: "normal-policy",
		Spec: map[string]interface{}{
			"priority": 500,
			"effect":   "allow",
		},
	}
	rule := &PolicySanePriority{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestPolicyEffectiveWindow_Exceeds5Years(t *testing.T) {
	future := time.Now().AddDate(6, 0, 0).Format(time.RFC3339)
	res := &Resource{
		Kind: "Policy",
		Name: "long-policy",
		Spec: map[string]interface{}{
			"effectiveWindow": map[string]interface{}{
				"notAfter": future,
			},
		},
	}
	rule := &PolicyEffectiveWindow{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelWarn {
		t.Errorf("expected warn, got %s", violations[0].Level)
	}
}

func TestPolicyEffectiveWindow_Within5Years(t *testing.T) {
	future := time.Now().AddDate(2, 0, 0).Format(time.RFC3339)
	res := &Resource{
		Kind: "Policy",
		Name: "short-policy",
		Spec: map[string]interface{}{
			"effectiveWindow": map[string]interface{}{
				"notAfter": future,
			},
		},
	}
	rule := &PolicyEffectiveWindow{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestModelDPARequired_GDPRWithoutDPA(t *testing.T) {
	res := &Resource{
		Kind: "ModelEndpoint",
		Name: "eu-model",
		Spec: map[string]interface{}{
			"compliance": []interface{}{"GDPR", "SOC2"},
			"privacy": map[string]interface{}{
				"dpaSigned": false,
			},
		},
	}
	rule := &ModelDPARequired{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelWarn {
		t.Errorf("expected warn, got %s", violations[0].Level)
	}
}

func TestModelDPARequired_GDPRWithDPA(t *testing.T) {
	res := &Resource{
		Kind: "ModelEndpoint",
		Name: "eu-model",
		Spec: map[string]interface{}{
			"compliance": []interface{}{"GDPR"},
			"privacy": map[string]interface{}{
				"dpaSigned": true,
			},
		},
	}
	rule := &ModelDPARequired{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestKBPostFilterWarn(t *testing.T) {
	res := &Resource{
		Kind: "KnowledgeBase",
		Name: "my-kb",
		Spec: map[string]interface{}{
			"acl": map[string]interface{}{
				"enforcement": "post_filter",
			},
		},
	}
	rule := &KBPostFilterWarn{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelWarn {
		t.Errorf("expected warn, got %s", violations[0].Level)
	}
}

func TestKBPostFilterWarn_PreFilter(t *testing.T) {
	res := &Resource{
		Kind: "KnowledgeBase",
		Name: "my-kb",
		Spec: map[string]interface{}{
			"acl": map[string]interface{}{
				"enforcement": "pre_filter",
			},
		},
	}
	rule := &KBPostFilterWarn{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestAgentSandboxRequired_ReactWithCodeNoSandbox(t *testing.T) {
	res := &Resource{
		Kind: "Agent",
		Name: "code-agent",
		Spec: map[string]interface{}{
			"runtime": map[string]interface{}{
				"pattern": "react",
			},
			"skills": []interface{}{
				map[string]interface{}{
					"ref": "skill://code-executor",
				},
			},
		},
	}
	rule := &AgentSandboxRequired{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelError {
		t.Errorf("expected error, got %s", violations[0].Level)
	}
}

func TestAgentSandboxRequired_ReactWithCodeAndSandbox(t *testing.T) {
	res := &Resource{
		Kind: "Agent",
		Name: "code-agent",
		Spec: map[string]interface{}{
			"runtime": map[string]interface{}{
				"pattern": "react",
				"sandbox": map[string]interface{}{
					"enabled": true,
				},
			},
			"skills": []interface{}{
				map[string]interface{}{
					"ref": "skill://code-executor",
				},
			},
		},
	}
	rule := &AgentSandboxRequired{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestAgentAuditMinLevel_ConfidentialWithBasic(t *testing.T) {
	res := &Resource{
		Kind: "Agent",
		Name: "my-agent",
		Spec: map[string]interface{}{
			"_classification": "confidential",
			"audit": map[string]interface{}{
				"level": "basic",
			},
		},
	}
	rule := &AgentAuditMinLevel{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelWarn {
		t.Errorf("expected warn, got %s", violations[0].Level)
	}
}

func TestAgentAuditMinLevel_ConfidentialWithHigh(t *testing.T) {
	res := &Resource{
		Kind: "Agent",
		Name: "my-agent",
		Spec: map[string]interface{}{
			"_classification": "confidential",
			"audit": map[string]interface{}{
				"level": "high",
			},
		},
	}
	rule := &AgentAuditMinLevel{}
	violations := rule.Check(res)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestSkillVersionBumped_SpecChangedWithoutBump(t *testing.T) {
	res := &Resource{
		Kind: "Skill",
		Name: "my-skill",
		Spec: map[string]interface{}{
			"version":          "1.0.0",
			"_lintSpecChanged": true,
		},
	}
	rule := &SkillVersionBumped{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelError {
		t.Errorf("expected error, got %s", violations[0].Level)
	}
}

func TestPolicyNoConflict_WithConflict(t *testing.T) {
	res := &Resource{
		Kind: "Policy",
		Name: "policy-a",
		Spec: map[string]interface{}{
			"effect":        "allow",
			"priority":      100,
			"_conflictWith": "policy-b",
		},
	}
	rule := &PolicyNoConflict{}
	violations := rule.Check(res)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Level != LevelError {
		t.Errorf("expected error, got %s", violations[0].Level)
	}
}

func TestRunAll_SkillStableNoEval(t *testing.T) {
	res := &Resource{
		Kind: "Skill",
		Name: "my-skill",
		Spec: map[string]interface{}{
			"stability": "stable",
			"version":   "1.0.0",
		},
	}
	violations := RunAll(res)
	// Should get violations from: has-eval-set (error), has-fallback (warn), budget-set (warn)
	if len(violations) < 3 {
		t.Fatalf("expected at least 3 violations for stable skill with no eval/fallback/budget, got %d", len(violations))
	}
	// Check that the error rule fired
	hasError := false
	for _, v := range violations {
		if v.Rule == "skill/has-eval-set" && v.Level == LevelError {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected skill/has-eval-set error violation")
	}
}

func TestClassificationAtLeast(t *testing.T) {
	tests := []struct {
		cls       string
		threshold string
		want      bool
	}{
		{"public", "confidential", false},
		{"internal", "confidential", false},
		{"confidential", "confidential", true},
		{"restricted", "confidential", true},
		{"secret", "confidential", true},
	}
	for _, tt := range tests {
		got := classificationAtLeast(tt.cls, tt.threshold)
		if got != tt.want {
			t.Errorf("classificationAtLeast(%q, %q) = %v, want %v", tt.cls, tt.threshold, got, tt.want)
		}
	}
}
