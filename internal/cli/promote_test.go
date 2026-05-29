package cli

import (
	"context"
	"fmt"
	"testing"
)

// mockSkillClient implements SkillClient for testing.
type mockSkillClient struct {
	skill *SkillInfo
	err   error
	// updated tracks if UpdateSkillStability was called.
	updated         bool
	updatedStability string
}

func (m *mockSkillClient) GetSkill(_ context.Context, _, _ string) (*SkillInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.skill, nil
}

func (m *mockSkillClient) UpdateSkillStability(_ context.Context, _, _, stability string) error {
	m.updated = true
	m.updatedStability = stability
	return nil
}

func TestRunPromote_Success(t *testing.T) {
	client := &mockSkillClient{
		skill: &SkillInfo{
			Name:        "my-skill",
			Namespace:   "default",
			Stability:   "beta",
			EvalPassing: true,
		},
	}

	opts := PromoteOptions{
		SkillName: "my-skill",
		Namespace: "default",
	}

	err := RunPromote(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !client.updated {
		t.Fatal("expected UpdateSkillStability to be called")
	}
	if client.updatedStability != "stable" {
		t.Fatalf("expected stability to be 'stable', got %q", client.updatedStability)
	}
}

func TestRunPromote_EvalNotPassed(t *testing.T) {
	client := &mockSkillClient{
		skill: &SkillInfo{
			Name:        "my-skill",
			Namespace:   "default",
			Stability:   "beta",
			EvalPassing: false,
		},
	}

	opts := PromoteOptions{
		SkillName: "my-skill",
		Namespace: "default",
	}

	err := RunPromote(context.Background(), client, opts)
	if err == nil {
		t.Fatal("expected error when eval not passed")
	}
	expected := "latest evaluation did not pass"
	if !containsSubstring(err.Error(), expected) {
		t.Fatalf("expected error to contain %q, got: %v", expected, err)
	}
	if client.updated {
		t.Fatal("UpdateSkillStability should not be called when eval fails")
	}
}

func TestRunPromote_NotBeta(t *testing.T) {
	client := &mockSkillClient{
		skill: &SkillInfo{
			Name:        "my-skill",
			Namespace:   "default",
			Stability:   "experimental",
			EvalPassing: true,
		},
	}

	opts := PromoteOptions{
		SkillName: "my-skill",
		Namespace: "default",
	}

	err := RunPromote(context.Background(), client, opts)
	if err == nil {
		t.Fatal("expected error when skill is not in beta")
	}
	expected := "only beta skills can be promoted"
	if !containsSubstring(err.Error(), expected) {
		t.Fatalf("expected error to contain %q, got: %v", expected, err)
	}
}

func TestRunPromote_DryRun(t *testing.T) {
	client := &mockSkillClient{
		skill: &SkillInfo{
			Name:        "my-skill",
			Namespace:   "default",
			Stability:   "beta",
			EvalPassing: true,
		},
	}

	opts := PromoteOptions{
		SkillName: "my-skill",
		Namespace: "default",
		DryRun:    true,
	}

	err := RunPromote(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client.updated {
		t.Fatal("UpdateSkillStability should not be called in dry-run mode")
	}
}

func TestRunPromote_GetSkillError(t *testing.T) {
	client := &mockSkillClient{
		err: fmt.Errorf("not found"),
	}

	opts := PromoteOptions{
		SkillName: "missing-skill",
		Namespace: "default",
	}

	err := RunPromote(context.Background(), client, opts)
	if err == nil {
		t.Fatal("expected error when skill cannot be fetched")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
