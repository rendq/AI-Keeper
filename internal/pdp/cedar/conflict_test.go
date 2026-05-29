package cedar

import "testing"

func TestConflictDetection_HardConflict(t *testing.T) {
	cedarText := `
permit(principal == AIK::User::alice, action == AIK::Action::invoke, resource == AIK::Skill::summarize);
forbid(principal == AIK::User::alice, action == AIK::Action::invoke, resource == AIK::Skill::summarize);
`
	detector := NewConflictDetector()
	conflicts, err := detector.DetectConflicts(cedarText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Type != HardConflict {
		t.Errorf("expected hard_conflict, got %s", conflicts[0].Type)
	}
}

func TestConflictDetection_SoftConflict(t *testing.T) {
	// One policy uses wildcard principal, the other is specific → partial overlap
	cedarText := `
permit(principal, action == AIK::Action::invoke, resource == AIK::Skill::summarize);
forbid(principal == AIK::User::alice, action == AIK::Action::invoke, resource == AIK::Skill::summarize);
`
	detector := NewConflictDetector()
	conflicts, err := detector.DetectConflicts(cedarText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Type != SoftConflict {
		t.Errorf("expected soft_conflict, got %s", conflicts[0].Type)
	}
}

func TestConflictDetection_NoConflict(t *testing.T) {
	// Completely different scopes — no overlap
	cedarText := `
permit(principal == AIK::User::alice, action == AIK::Action::invoke, resource == AIK::Skill::summarize);
forbid(principal == AIK::User::bob, action == AIK::Action::delete, resource == AIK::Skill::translate);
`
	detector := NewConflictDetector()
	conflicts, err := detector.DetectConflicts(cedarText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestConflictDetection_SameEffect(t *testing.T) {
	// Same scope but same effect → no conflict
	cedarText := `
permit(principal == AIK::User::alice, action == AIK::Action::invoke, resource == AIK::Skill::summarize);
permit(principal == AIK::User::alice, action == AIK::Action::invoke, resource == AIK::Skill::summarize);
`
	detector := NewConflictDetector()
	conflicts, err := detector.DetectConflicts(cedarText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
}
