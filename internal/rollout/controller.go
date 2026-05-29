// Package rollout implements canary deployment step orchestration.
package rollout

import (
	"errors"
	"time"
)

// Phase represents the current phase of a rollout.
type Phase string

const (
	PhaseProgressing Phase = "Progressing"
	PhasePaused      Phase = "Paused"
	PhaseAborted     Phase = "Aborted"
	PhaseCompleted   Phase = "Completed"
)

// RolloutConfig defines the configuration for a canary rollout.
type RolloutConfig struct {
	// Steps defines the traffic weight progression (e.g. [10, 30, 100]).
	Steps []int
	// AnalysisInterval is the minimum time between analysis checks.
	AnalysisInterval time.Duration
	// MaxFailures is the maximum number of failures before aborting.
	MaxFailures int
}

// RolloutState holds the current state of an in-progress rollout.
type RolloutState struct {
	CurrentStep    int
	Weight         int
	Phase          Phase
	AbortReason    string
	StartedAt      time.Time
	LastAnalysisAt time.Time
}

// RolloutController orchestrates canary deployment steps.
type RolloutController struct{}

// New creates a new RolloutController.
func New() *RolloutController {
	return &RolloutController{}
}

// Start begins a rollout at step 0 with the given config.
func (rc *RolloutController) Start(config RolloutConfig) *RolloutState {
	now := time.Now()
	weight := 0
	if len(config.Steps) > 0 {
		weight = config.Steps[0]
	}
	return &RolloutState{
		CurrentStep:    0,
		Weight:         weight,
		Phase:          PhaseProgressing,
		StartedAt:      now,
		LastAnalysisAt: now,
	}
}

// AdvanceStep advances the rollout to the next step. Returns true if advanced,
// false if already at the final step (and marks complete). Returns an error if
// the rollout is not in Progressing phase.
func (rc *RolloutController) AdvanceStep(state *RolloutState, config RolloutConfig) (bool, error) {
	if state.Phase != PhaseProgressing {
		return false, errors.New("rollout is not in Progressing phase")
	}
	nextStep := state.CurrentStep + 1
	if nextStep >= len(config.Steps) {
		// Already at final step — mark complete.
		state.Phase = PhaseCompleted
		return false, nil
	}
	state.CurrentStep = nextStep
	state.Weight = config.Steps[nextStep]
	state.LastAnalysisAt = time.Now()
	return true, nil
}

// Pause pauses the rollout at the current step.
func (rc *RolloutController) Pause(state *RolloutState) {
	if state.Phase == PhaseProgressing {
		state.Phase = PhasePaused
	}
}

// Resume resumes a paused rollout.
func (rc *RolloutController) Resume(state *RolloutState) {
	if state.Phase == PhasePaused {
		state.Phase = PhaseProgressing
	}
}

// Abort aborts the rollout with a reason.
func (rc *RolloutController) Abort(state *RolloutState, reason string) {
	state.Phase = PhaseAborted
	state.AbortReason = reason
}

// Complete marks the rollout as completed.
func (rc *RolloutController) Complete(state *RolloutState) {
	state.Phase = PhaseCompleted
}

// ShouldAnalyze returns true if enough time has passed since the last analysis.
func (rc *RolloutController) ShouldAnalyze(state *RolloutState, config RolloutConfig) bool {
	if state.Phase != PhaseProgressing {
		return false
	}
	return time.Since(state.LastAnalysisAt) >= config.AnalysisInterval
}
