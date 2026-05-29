package rollout

import (
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	rc := New()
	config := RolloutConfig{
		Steps:            []int{10, 30, 100},
		AnalysisInterval: 30 * time.Second,
		MaxFailures:      3,
	}

	state := rc.Start(config)

	if state.CurrentStep != 0 {
		t.Errorf("expected CurrentStep=0, got %d", state.CurrentStep)
	}
	if state.Weight != 10 {
		t.Errorf("expected Weight=10, got %d", state.Weight)
	}
	if state.Phase != PhaseProgressing {
		t.Errorf("expected Phase=Progressing, got %s", state.Phase)
	}
	if state.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestAdvanceStep(t *testing.T) {
	rc := New()
	config := RolloutConfig{
		Steps:            []int{10, 30, 100},
		AnalysisInterval: 30 * time.Second,
		MaxFailures:      3,
	}

	state := rc.Start(config)

	// Advance from step 0 (10%) to step 1 (30%)
	advanced, err := rc.AdvanceStep(state, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !advanced {
		t.Error("expected advance to return true")
	}
	if state.CurrentStep != 1 {
		t.Errorf("expected CurrentStep=1, got %d", state.CurrentStep)
	}
	if state.Weight != 30 {
		t.Errorf("expected Weight=30, got %d", state.Weight)
	}

	// Advance from step 1 (30%) to step 2 (100%)
	advanced, err = rc.AdvanceStep(state, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !advanced {
		t.Error("expected advance to return true")
	}
	if state.CurrentStep != 2 {
		t.Errorf("expected CurrentStep=2, got %d", state.CurrentStep)
	}
	if state.Weight != 100 {
		t.Errorf("expected Weight=100, got %d", state.Weight)
	}
}

func TestAdvanceAtFinalStep(t *testing.T) {
	rc := New()
	config := RolloutConfig{
		Steps:            []int{10, 30, 100},
		AnalysisInterval: 30 * time.Second,
		MaxFailures:      3,
	}

	state := rc.Start(config)

	// Advance to final step
	rc.AdvanceStep(state, config) // -> 30%
	rc.AdvanceStep(state, config) // -> 100%

	// Try to advance past final step — should complete
	advanced, err := rc.AdvanceStep(state, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if advanced {
		t.Error("expected advance to return false at final step")
	}
	if state.Phase != PhaseCompleted {
		t.Errorf("expected Phase=Completed, got %s", state.Phase)
	}
}

func TestPauseResume(t *testing.T) {
	rc := New()
	config := RolloutConfig{
		Steps:            []int{10, 30, 100},
		AnalysisInterval: 30 * time.Second,
		MaxFailures:      3,
	}

	state := rc.Start(config)

	rc.Pause(state)
	if state.Phase != PhasePaused {
		t.Errorf("expected Phase=Paused, got %s", state.Phase)
	}

	// AdvanceStep should fail when paused
	_, err := rc.AdvanceStep(state, config)
	if err == nil {
		t.Error("expected error when advancing paused rollout")
	}

	rc.Resume(state)
	if state.Phase != PhaseProgressing {
		t.Errorf("expected Phase=Progressing, got %s", state.Phase)
	}

	// Should be able to advance after resume
	advanced, err := rc.AdvanceStep(state, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !advanced {
		t.Error("expected advance to succeed after resume")
	}
}

func TestAbort(t *testing.T) {
	rc := New()
	config := RolloutConfig{
		Steps:            []int{10, 30, 100},
		AnalysisInterval: 30 * time.Second,
		MaxFailures:      3,
	}

	state := rc.Start(config)

	rc.Abort(state, "high error rate")
	if state.Phase != PhaseAborted {
		t.Errorf("expected Phase=Aborted, got %s", state.Phase)
	}
	if state.AbortReason != "high error rate" {
		t.Errorf("expected AbortReason='high error rate', got %q", state.AbortReason)
	}

	// AdvanceStep should fail when aborted
	_, err := rc.AdvanceStep(state, config)
	if err == nil {
		t.Error("expected error when advancing aborted rollout")
	}
}

func TestShouldAnalyze(t *testing.T) {
	rc := New()
	config := RolloutConfig{
		Steps:            []int{10, 30, 100},
		AnalysisInterval: 50 * time.Millisecond,
		MaxFailures:      3,
	}

	state := rc.Start(config)

	// Immediately after start, should not need analysis
	if rc.ShouldAnalyze(state, config) {
		t.Error("expected ShouldAnalyze=false immediately after start")
	}

	// Wait for the analysis interval to pass
	time.Sleep(60 * time.Millisecond)

	if !rc.ShouldAnalyze(state, config) {
		t.Error("expected ShouldAnalyze=true after interval passed")
	}

	// Paused state should not trigger analysis
	rc.Pause(state)
	if rc.ShouldAnalyze(state, config) {
		t.Error("expected ShouldAnalyze=false when paused")
	}
}
