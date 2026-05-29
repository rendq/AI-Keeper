package cedar

import (
	"context"
	"testing"
)

func TestCedarEvaluate_AllowPolicy(t *testing.T) {
	engine := NewCedarEngine()
	policy := `permit(
  principal == AIK::User::alice,
  action == AIK::Action::"invoke",
  resource == AIK::Skill::summarize
);`
	if err := engine.LoadPolicies(policy); err != nil {
		t.Fatalf("LoadPolicies: %v", err)
	}

	resp, err := engine.Evaluate(context.Background(), DecisionRequest{
		Principal: "AIK::User::alice",
		Action:    "AIK::Action::invoke",
		Resource:  "AIK::Skill::summarize",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if resp.Decision != "allow" {
		t.Errorf("expected allow, got %s (reason: %s)", resp.Decision, resp.Reason)
	}
	if len(resp.MatchedPolicies) == 0 {
		t.Error("expected at least one matched policy")
	}
}

func TestCedarEvaluate_DenyPolicy(t *testing.T) {
	engine := NewCedarEngine()
	policy := `forbid(
  principal == AIK::User::bob,
  action == AIK::Action::"delete",
  resource == AIK::Data::sensitive
);`
	if err := engine.LoadPolicies(policy); err != nil {
		t.Fatalf("LoadPolicies: %v", err)
	}

	resp, err := engine.Evaluate(context.Background(), DecisionRequest{
		Principal: "AIK::User::bob",
		Action:    "AIK::Action::delete",
		Resource:  "AIK::Data::sensitive",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if resp.Decision != "deny" {
		t.Errorf("expected deny, got %s (reason: %s)", resp.Decision, resp.Reason)
	}
	if resp.Reason != "explicit forbid policy matched" {
		t.Errorf("unexpected reason: %s", resp.Reason)
	}
}

func TestCedarEvaluate_DefaultDeny(t *testing.T) {
	engine := NewCedarEngine()
	policy := `permit(
  principal == AIK::User::alice,
  action == AIK::Action::"invoke",
  resource == AIK::Skill::summarize
);`
	if err := engine.LoadPolicies(policy); err != nil {
		t.Fatalf("LoadPolicies: %v", err)
	}

	// Request that does not match any policy
	resp, err := engine.Evaluate(context.Background(), DecisionRequest{
		Principal: "AIK::User::unknown",
		Action:    "AIK::Action::invoke",
		Resource:  "AIK::Skill::summarize",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if resp.Decision != "deny" {
		t.Errorf("expected deny (default), got %s", resp.Decision)
	}
	if resp.Reason != "no matching policy found (default deny)" {
		t.Errorf("unexpected reason: %s", resp.Reason)
	}
}

func TestCedarEvaluate_MultiplePolicy(t *testing.T) {
	engine := NewCedarEngine()
	// Both permit and forbid match the same request — forbid wins
	policy := `permit(
  principal == AIK::User::alice,
  action == AIK::Action::"invoke",
  resource == AIK::Skill::summarize
);
forbid(
  principal == AIK::User::alice,
  action == AIK::Action::"invoke",
  resource == AIK::Skill::summarize
);`
	if err := engine.LoadPolicies(policy); err != nil {
		t.Fatalf("LoadPolicies: %v", err)
	}

	resp, err := engine.Evaluate(context.Background(), DecisionRequest{
		Principal: "AIK::User::alice",
		Action:    "AIK::Action::invoke",
		Resource:  "AIK::Skill::summarize",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if resp.Decision != "deny" {
		t.Errorf("expected deny (forbid takes precedence), got %s", resp.Decision)
	}
	if resp.Reason != "explicit forbid policy matched" {
		t.Errorf("unexpected reason: %s", resp.Reason)
	}
}
