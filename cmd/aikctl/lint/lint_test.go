package lint

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "testdata", "lint")
}

func TestSkillVersionBumped(t *testing.T) {
	files := []string{filepath.Join(testdataDir(), "skill_version_not_bumped.yaml")}
	rs, err := LoadResources(files)
	if err != nil {
		t.Fatal(err)
	}

	rule := &SkillVersionBumped{}
	results := rule.Run(rs)

	if len(results) == 0 {
		t.Fatal("expected at least one skill/version-bumped violation")
	}

	found := false
	for _, r := range results {
		if r.Rule == "skill/version-bumped" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error-level skill/version-bumped finding")
	}
}

func TestAgentSkillsResolved(t *testing.T) {
	files := []string{filepath.Join(testdataDir(), "agent_unresolved_skill.yaml")}
	rs, err := LoadResources(files)
	if err != nil {
		t.Fatal(err)
	}

	rule := &AgentSkillsResolved{}
	results := rule.Run(rs)

	if len(results) == 0 {
		t.Fatal("expected at least one agent/skills-resolved violation")
	}

	found := false
	for _, r := range results {
		if r.Rule == "agent/skills-resolved" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error-level agent/skills-resolved finding")
	}
}

func TestAgentSandboxRequired(t *testing.T) {
	files := []string{filepath.Join(testdataDir(), "agent_sandbox_missing.yaml")}
	rs, err := LoadResources(files)
	if err != nil {
		t.Fatal(err)
	}

	rule := &AgentSandboxRequired{}
	results := rule.Run(rs)

	if len(results) == 0 {
		t.Fatal("expected at least one agent/sandbox-required violation")
	}

	found := false
	for _, r := range results {
		if r.Rule == "agent/sandbox-required" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error-level agent/sandbox-required finding")
	}
}

func TestPolicyNoConflict(t *testing.T) {
	files := []string{filepath.Join(testdataDir(), "policy_conflict.yaml")}
	rs, err := LoadResources(files)
	if err != nil {
		t.Fatal(err)
	}

	rule := &PolicyNoConflict{}
	results := rule.Run(rs)

	if len(results) == 0 {
		t.Fatal("expected at least one policy/no-conflict violation")
	}

	found := false
	for _, r := range results {
		if r.Rule == "policy/no-conflict" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error-level policy/no-conflict finding")
	}
}

func TestToolDestructiveNeedsApproval(t *testing.T) {
	files := []string{filepath.Join(testdataDir(), "tool_destructive_no_approval.yaml")}
	rs, err := LoadResources(files)
	if err != nil {
		t.Fatal(err)
	}

	rule := &ToolDestructiveNeedsApproval{}
	results := rule.Run(rs)

	if len(results) == 0 {
		t.Fatal("expected at least one tool/destructive-needs-approval violation")
	}

	found := false
	for _, r := range results {
		if r.Rule == "tool/destructive-needs-approval" && r.Level == LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error-level tool/destructive-needs-approval finding")
	}
}

func TestRunAllRules_AllViolations(t *testing.T) {
	dir := testdataDir()
	files := []string{
		filepath.Join(dir, "skill_version_not_bumped.yaml"),
		filepath.Join(dir, "agent_unresolved_skill.yaml"),
		filepath.Join(dir, "agent_sandbox_missing.yaml"),
		filepath.Join(dir, "policy_conflict.yaml"),
		filepath.Join(dir, "tool_destructive_no_approval.yaml"),
	}

	rs, err := LoadResources(files)
	if err != nil {
		t.Fatal(err)
	}

	results := RunAllRules(rs)

	// Expect at least 5 errors (one per rule).
	errorCount := 0
	rulesSeen := make(map[string]bool)
	for _, r := range results {
		if r.Level == LevelError {
			errorCount++
			rulesSeen[r.Rule] = true
		}
	}

	expectedRules := []string{
		"skill/version-bumped",
		"agent/skills-resolved",
		"agent/sandbox-required",
		"policy/no-conflict",
		"tool/destructive-needs-approval",
	}

	for _, rule := range expectedRules {
		if !rulesSeen[rule] {
			t.Errorf("expected error from rule %q, but not found", rule)
		}
	}

	if errorCount < 5 {
		t.Errorf("expected at least 5 errors, got %d", errorCount)
	}
}

func TestNoViolations(t *testing.T) {
	// A valid skill should not produce any errors.
	files := []string{filepath.Join(testdataDir(), "..", "lint_valid", "valid_skill.yaml")}
	rs, err := LoadResources(files)
	if err != nil {
		t.Fatal(err)
	}

	results := RunAllRules(rs)

	for _, r := range results {
		if r.Level == LevelError {
			t.Errorf("unexpected error: [%s] %s", r.Rule, r.Message)
		}
	}
}
