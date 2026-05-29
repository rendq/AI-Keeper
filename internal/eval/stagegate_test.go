package eval

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

func makeSkill(stability shared.Stage, gates map[string]map[string]string) *skillv1alpha1.Skill {
	sk := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-skill",
			Namespace: "default",
		},
		Spec: skillv1alpha1.SkillSpec{
			Stability: stability,
		},
	}
	if gates != nil {
		sk.Spec.Evaluation = &skillv1alpha1.SkillEvaluation{
			Gates: gates,
		}
	}
	return sk
}

func TestStageGate_AllPass(t *testing.T) {
	gates := map[string]map[string]string{
		"promoteToStable": {
			"answer_relevancy": ">= 0.8",
			"faithfulness":     ">= 0.7",
		},
	}
	skill := makeSkill(shared.StageBeta, gates)
	sg := NewStageGate(skill)

	result := EvalResult{
		Metrics: []MetricResult{
			{Name: "answer_relevancy", Score: 0.9, Passed: true},
			{Name: "faithfulness", Score: 0.85, Passed: true},
		},
		Passed: true,
	}

	shouldPromote, reason := sg.Evaluate(skill, result)
	if !shouldPromote {
		t.Errorf("expected promotion, got shouldPromote=false, reason=%s", reason)
	}
	if reason != "all gate thresholds passed" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestStageGate_OneFail(t *testing.T) {
	gates := map[string]map[string]string{
		"promoteToStable": {
			"answer_relevancy": ">= 0.8",
			"faithfulness":     ">= 0.7",
		},
	}
	skill := makeSkill(shared.StageBeta, gates)
	sg := NewStageGate(skill)

	result := EvalResult{
		Metrics: []MetricResult{
			{Name: "answer_relevancy", Score: 0.9, Passed: true},
			{Name: "faithfulness", Score: 0.5, Passed: false},
		},
		Passed: false,
	}

	shouldPromote, reason := sg.Evaluate(skill, result)
	if shouldPromote {
		t.Errorf("expected no promotion, got shouldPromote=true")
	}
	if reason == "" {
		t.Error("expected a failure reason")
	}
	// Verify the reason mentions the failing metric.
	if !contains(reason, "faithfulness") {
		t.Errorf("reason should mention 'faithfulness', got: %s", reason)
	}
}

func TestStageGate_EmptyResult(t *testing.T) {
	gates := map[string]map[string]string{
		"promoteToStable": {
			"answer_relevancy": ">= 0.8",
		},
	}
	skill := makeSkill(shared.StageBeta, gates)
	sg := NewStageGate(skill)

	result := EvalResult{
		Metrics: []MetricResult{}, // empty
	}

	shouldPromote, reason := sg.Evaluate(skill, result)
	if shouldPromote {
		t.Errorf("expected no promotion with empty metrics, got shouldPromote=true")
	}
	if reason == "" {
		t.Error("expected a reason for no promotion")
	}
}

func TestStageGate_NoGates(t *testing.T) {
	// No gates configured → auto-promote (experimental can always promote).
	skill := makeSkill(shared.StageBeta, nil)
	sg := NewStageGate(skill)

	result := EvalResult{
		Metrics: []MetricResult{
			{Name: "answer_relevancy", Score: 0.6, Passed: true},
		},
		Passed: true,
	}

	shouldPromote, reason := sg.Evaluate(skill, result)
	if !shouldPromote {
		t.Errorf("expected auto-promotion when no gates configured, got shouldPromote=false, reason=%s", reason)
	}
}

func TestStageGate_AlreadyStable(t *testing.T) {
	gates := map[string]map[string]string{
		"promoteToStable": {
			"answer_relevancy": ">= 0.8",
		},
	}
	skill := makeSkill(shared.StageStable, gates)
	sg := NewStageGate(skill)

	result := EvalResult{
		Metrics: []MetricResult{
			{Name: "answer_relevancy", Score: 0.9, Passed: true},
		},
		Passed: true,
	}

	shouldPromote, reason := sg.Evaluate(skill, result)
	if shouldPromote {
		t.Errorf("expected no promotion for already-stable skill, got shouldPromote=true")
	}
	if !contains(reason, "already stable") {
		t.Errorf("reason should mention 'already stable', got: %s", reason)
	}
}

func TestPromoteSkill_BetaToStable(t *testing.T) {
	skill := makeSkill(shared.StageBeta, nil)
	err := PromoteSkill(skill)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Spec.Stability != shared.StageStable {
		t.Errorf("expected stability=stable, got %s", skill.Spec.Stability)
	}
}

func TestPromoteSkill_AlreadyStable(t *testing.T) {
	skill := makeSkill(shared.StageStable, nil)
	err := PromoteSkill(skill)
	if err == nil {
		t.Error("expected error promoting already-stable skill")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
