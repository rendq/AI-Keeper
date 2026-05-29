// Package eval provides the Eval Runner that schedules offline evaluation
// workflows via Argo Workflows and collects results back into Skill status.
// Covers requirements A3.8 and C4.
package eval

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvalRequest represents a request to run an evaluation for a specific Skill.
type EvalRequest struct {
	// SkillRef is the namespace/name of the Skill CR to evaluate.
	SkillNamespace string `json:"skillNamespace"`
	SkillName      string `json:"skillName"`

	// EvalSetRef points to the evaluation dataset (e.g. ref://eval-sets/my-set).
	EvalSetRef string `json:"evalSetRef,omitempty"`

	// RedTeamSetRef points to the red team dataset.
	RedTeamSetRef string `json:"redTeamSetRef,omitempty"`

	// RunID is a unique identifier for this evaluation run.
	RunID string `json:"runId"`

	// RequestedAt is when the eval was enqueued.
	RequestedAt metav1.Time `json:"requestedAt"`
}

// MetricResult holds the result of a single evaluation metric.
type MetricResult struct {
	// Name of the metric (e.g. answer_relevancy, faithfulness).
	Name string `json:"name"`

	// Score is the numeric result (typically 0.0 - 1.0).
	Score float64 `json:"score"`

	// Threshold is the gate threshold for this metric.
	Threshold float64 `json:"threshold,omitempty"`

	// Passed indicates whether the metric meets its threshold.
	Passed bool `json:"passed"`
}

// EvalResult aggregates all metric results for a single evaluation run.
type EvalResult struct {
	// RunID matches the EvalRequest.RunID.
	RunID string `json:"runId"`

	// SkillNamespace and SkillName identify the evaluated Skill.
	SkillNamespace string `json:"skillNamespace"`
	SkillName      string `json:"skillName"`

	// Metrics is the list of individual metric results.
	Metrics []MetricResult `json:"metrics"`

	// Passed is true if ALL metrics passed their thresholds.
	Passed bool `json:"passed"`

	// StartedAt is when the evaluation workflow started.
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when the evaluation finished.
	CompletedAt time.Time `json:"completedAt"`

	// Error contains any error message if the eval failed.
	Error string `json:"error,omitempty"`
}
