// Package eval — Stage Gate auto-promotion logic.
// Covers requirements C4 (eval gating) and A3.8 (eval-driven promotion).
package eval

import (
	"fmt"
	"strings"

	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// GateConfig defines threshold requirements for stage promotion.
type GateConfig struct {
	// MetricThresholds maps metric name → minimum score required.
	MetricThresholds map[string]float64

	// RequireAllPass means every metric must meet its threshold.
	// If false, only metrics listed in MetricThresholds are checked.
	RequireAllPass bool
}

// StageGate evaluates whether a Skill should be promoted based on eval results.
type StageGate struct {
	// Config holds the gate thresholds. Extracted from
	// Skill.spec.evaluation.gates["promoteToStable"].
	Config GateConfig
}

// NewStageGate creates a StageGate from the Skill's evaluation gates.
// If no gates are configured, an empty config is returned (will auto-promote).
func NewStageGate(skill *skillv1alpha1.Skill) *StageGate {
	sg := &StageGate{
		Config: GateConfig{
			MetricThresholds: make(map[string]float64),
			RequireAllPass:   true,
		},
	}

	if skill.Spec.Evaluation == nil || skill.Spec.Evaluation.Gates == nil {
		return sg
	}

	// Read "promoteToStable" gate thresholds.
	// Gates format: map[string]map[string]string where outer key is target stage,
	// inner key is metric name, value is a threshold expression (e.g. ">= 0.8").
	promoteGates, ok := skill.Spec.Evaluation.Gates["promoteToStable"]
	if !ok {
		return sg
	}

	for metric, expr := range promoteGates {
		threshold := parseThreshold(expr)
		sg.Config.MetricThresholds[metric] = threshold
	}

	return sg
}

// parseThreshold extracts a float64 from a gate expression like ">= 0.8" or "0.8".
func parseThreshold(expr string) float64 {
	// Strip common comparison operators.
	s := strings.TrimSpace(expr)
	s = strings.TrimPrefix(s, ">=")
	s = strings.TrimPrefix(s, ">")
	s = strings.TrimPrefix(s, "==")
	s = strings.TrimSpace(s)

	var val float64
	fmt.Sscanf(s, "%f", &val)
	return val
}

// Evaluate checks whether the EvalResult satisfies all gate thresholds.
// Returns shouldPromote=true if the skill should be promoted, along with a reason.
func (sg *StageGate) Evaluate(skill *skillv1alpha1.Skill, result EvalResult) (shouldPromote bool, reason string) {
	// If skill is already stable, no promotion needed.
	if skill.Spec.Stability == shared.StageStable {
		return false, "skill is already stable; no promotion needed"
	}

	// If no metrics in the result, cannot promote.
	if len(result.Metrics) == 0 {
		return false, "no metrics in eval result; cannot promote"
	}

	// If no thresholds configured, auto-promote (experimental can always promote).
	if len(sg.Config.MetricThresholds) == 0 {
		return true, "no gate thresholds configured; auto-promoting"
	}

	// Check each configured threshold against the eval result.
	var failures []string
	for metricName, threshold := range sg.Config.MetricThresholds {
		score, found := findMetricScore(result, metricName)
		if !found {
			failures = append(failures, fmt.Sprintf("metric %q not found in eval results", metricName))
			continue
		}
		if score < threshold {
			failures = append(failures, fmt.Sprintf("metric %q score %.4f < threshold %.4f", metricName, score, threshold))
		}
	}

	if len(failures) > 0 {
		return false, "gate check failed: " + strings.Join(failures, "; ")
	}

	return true, "all gate thresholds passed"
}

// findMetricScore looks up a metric score from an EvalResult.
func findMetricScore(result EvalResult, metricName string) (float64, bool) {
	for _, m := range result.Metrics {
		if m.Name == metricName {
			return m.Score, true
		}
	}
	return 0, false
}

// PromoteSkill updates the Skill's stability from beta to stable.
// Returns an error if the skill is not in a promotable state.
func PromoteSkill(skill *skillv1alpha1.Skill) error {
	switch skill.Spec.Stability {
	case shared.StageStable:
		return fmt.Errorf("skill %s/%s is already stable", skill.Namespace, skill.Name)
	case shared.StageDeprecated:
		return fmt.Errorf("skill %s/%s is deprecated; cannot promote", skill.Namespace, skill.Name)
	case shared.StageBeta:
		skill.Spec.Stability = shared.StageStable
		return nil
	case shared.StageExperimental:
		// experimental → beta first, then beta → stable in next cycle.
		// For now, allow direct promotion if gates pass.
		skill.Spec.Stability = shared.StageStable
		return nil
	default:
		return fmt.Errorf("skill %s/%s has unknown stability %q", skill.Namespace, skill.Name, skill.Spec.Stability)
	}
}
