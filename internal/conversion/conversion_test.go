package conversion

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	agentv1beta1 "github.com/ai-keeper/ai-keeper/api/agent/v1beta1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	skillv1beta1 "github.com/ai-keeper/ai-keeper/api/skill/v1beta1"
)

// --- Skill conversion tests ---

// TestSkillAlphaToBeta verifies that shared fields map correctly from
// v1alpha1 to v1beta1.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func TestSkillAlphaToBeta(t *testing.T) {
	t.Parallel()
	src := &skillv1alpha1.Skill{
		TypeMeta: metav1.TypeMeta{APIVersion: "skill.ai-keeper.io/v1alpha1", Kind: "Skill"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "contract-review",
			Namespace: "default",
			Labels:    map[string]string{"team": "legal"},
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   shared.SemVer("2.1.0"),
			Stability: shared.StageStable,
			Interface: skillv1alpha1.SkillInterface{
				Input:  skillv1alpha1.SkillIO{Schema: nil},
				Output: skillv1alpha1.SkillIO{Schema: nil},
			},
			Implementation: skillv1alpha1.SkillImplementation{
				Type: "function",
				Runtime: &skillv1alpha1.SkillRuntime{
					Engine:     "aip-runtime/v2",
					Entrypoint: "review",
					Image:      "registry.io/contract:v2",
				},
			},
			Evaluation: &skillv1alpha1.SkillEvaluation{
				Schedule: "0 */6 * * *",
				Gates: map[string]map[string]string{
					"stable": {"accuracy": "value > 0.95"},
				},
			},
		},
	}

	dst, lossy := ConvertSkillAlphaToBeta(src)

	if dst == nil {
		t.Fatal("expected non-nil result")
	}
	if dst.APIVersion != "skill.ai-keeper.io/v1beta1" {
		t.Errorf("APIVersion = %q, want skill.ai-keeper.io/v1beta1", dst.APIVersion)
	}
	if dst.Name != "contract-review" {
		t.Errorf("Name = %q, want contract-review", dst.Name)
	}
	if dst.Spec.Version != "2.1.0" {
		t.Errorf("Version = %q, want 2.1.0", dst.Spec.Version)
	}
	if dst.Spec.Stability != shared.StageStable {
		t.Errorf("Stability = %q, want stable", dst.Spec.Stability)
	}
	if dst.Spec.Implementation.Type != "function" {
		t.Errorf("Implementation.Type = %q, want function", dst.Spec.Implementation.Type)
	}
	if dst.Spec.Implementation.Runtime == nil {
		t.Fatal("expected Runtime to be set")
	}
	if dst.Spec.Implementation.Runtime.Engine != "aip-runtime/v2" {
		t.Errorf("Runtime.Engine = %q, want aip-runtime/v2", dst.Spec.Implementation.Runtime.Engine)
	}
	if dst.Spec.Evaluation == nil || dst.Spec.Evaluation.Schedule != "0 */6 * * *" {
		t.Errorf("Evaluation.Schedule not preserved")
	}
	if dst.Labels["team"] != "legal" {
		t.Errorf("Labels not preserved")
	}
	// alpha→beta produces no lossy entries (beta is superset).
	if len(lossy) != 0 {
		t.Errorf("expected no lossy entries, got %v", lossy)
	}
	// Compliance should be nil (no source in alpha).
	if dst.Spec.Compliance != nil {
		t.Errorf("expected Compliance=nil, got %+v", dst.Spec.Compliance)
	}
}

// TestSkillBetaToAlpha verifies that v1beta1-only fields produce lossy annotations.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func TestSkillBetaToAlpha(t *testing.T) {
	t.Parallel()
	auditRequired := true
	continuousEval := true
	weight := 0.8

	src := &skillv1beta1.Skill{
		TypeMeta:   metav1.TypeMeta{APIVersion: "skill.ai-keeper.io/v1beta1", Kind: "Skill"},
		ObjectMeta: metav1.ObjectMeta{Name: "summarizer", Namespace: "ml"},
		Spec: skillv1beta1.SkillSpec{
			Version:   shared.SemVer("1.0.0"),
			Stability: shared.StageBeta,
			Interface: skillv1beta1.SkillInterface{
				Input:  skillv1beta1.SkillIO{Schema: nil},
				Output: skillv1beta1.SkillIO{Schema: nil},
			},
			Implementation: skillv1beta1.SkillImplementation{Type: "workflow"},
			Compliance: &skillv1beta1.SkillCompliance{
				Standards: []skillv1beta1.SkillComplianceStandard{
					{Name: "SOC2", Controls: []string{"CC6.1"}},
				},
				DataClassification: "confidential",
				AuditRequired:      &auditRequired,
			},
			Evaluation: &skillv1beta1.SkillEvaluation{
				Schedule:       "0 0 * * *",
				ContinuousEval: &continuousEval,
				Metrics: []skillv1beta1.SkillEvalMetric{
					{Name: "accuracy", Threshold: "value > 0.9", Weight: &weight},
				},
			},
		},
	}

	dst, lossy := ConvertSkillBetaToAlpha(src)

	if dst == nil {
		t.Fatal("expected non-nil result")
	}
	if dst.APIVersion != "skill.ai-keeper.io/v1alpha1" {
		t.Errorf("APIVersion = %q, want skill.ai-keeper.io/v1alpha1", dst.APIVersion)
	}
	if dst.Name != "summarizer" {
		t.Errorf("Name = %q, want summarizer", dst.Name)
	}
	if dst.Spec.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", dst.Spec.Version)
	}
	if dst.Spec.Implementation.Type != "workflow" {
		t.Errorf("Implementation.Type = %q, want workflow", dst.Spec.Implementation.Type)
	}

	// Must report lossy for compliance, metrics, continuousEval.
	if len(lossy) < 3 {
		t.Errorf("expected at least 3 lossy entries, got %d: %v", len(lossy), lossy)
	}
	foundCompliance, foundMetrics, foundContinuous := false, false, false
	for _, l := range lossy {
		if contains(l, "compliance") {
			foundCompliance = true
		}
		if contains(l, "metrics") {
			foundMetrics = true
		}
		if contains(l, "continuousEval") {
			foundContinuous = true
		}
	}
	if !foundCompliance {
		t.Error("lossy should mention compliance")
	}
	if !foundMetrics {
		t.Error("lossy should mention metrics")
	}
	if !foundContinuous {
		t.Error("lossy should mention continuousEval")
	}
}

// TestSkillRoundTrip verifies alpha→beta→alpha preserves essential fields.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func TestSkillRoundTrip(t *testing.T) {
	t.Parallel()
	original := &skillv1alpha1.Skill{
		TypeMeta:   metav1.TypeMeta{APIVersion: "skill.ai-keeper.io/v1alpha1", Kind: "Skill"},
		ObjectMeta: metav1.ObjectMeta{Name: "roundtrip-skill", Namespace: "test"},
		Spec: skillv1alpha1.SkillSpec{
			Version:   shared.SemVer("3.0.0"),
			Stability: shared.StageExperimental,
			Interface: skillv1alpha1.SkillInterface{
				Input:  skillv1alpha1.SkillIO{Schema: nil},
				Output: skillv1alpha1.SkillIO{Schema: nil},
			},
			Implementation: skillv1alpha1.SkillImplementation{
				Type: "agentic",
				Runtime: &skillv1alpha1.SkillRuntime{
					Engine: "langgraph",
					Image:  "img:latest",
				},
				Requires: &skillv1alpha1.SkillRequires{
					Models: []skillv1alpha1.SkillModelDep{
						{Alias: "reasoner", Ref: shared.ResourceRef("model://gpt4"), Purpose: "reasoning"},
					},
					Tools: []skillv1alpha1.SkillToolDep{
						{Ref: shared.ResourceRef("tool://search")},
					},
				},
			},
			Evaluation: &skillv1alpha1.SkillEvaluation{
				Schedule: "0 0 * * 0",
			},
		},
	}

	beta, _ := ConvertSkillAlphaToBeta(original)
	roundtripped, _ := ConvertSkillBetaToAlpha(beta)

	// Essential fields must be preserved.
	if roundtripped.Name != original.Name {
		t.Errorf("Name: got %q, want %q", roundtripped.Name, original.Name)
	}
	if roundtripped.Namespace != original.Namespace {
		t.Errorf("Namespace: got %q, want %q", roundtripped.Namespace, original.Namespace)
	}
	if roundtripped.Spec.Version != original.Spec.Version {
		t.Errorf("Version: got %q, want %q", roundtripped.Spec.Version, original.Spec.Version)
	}
	if roundtripped.Spec.Stability != original.Spec.Stability {
		t.Errorf("Stability: got %q, want %q", roundtripped.Spec.Stability, original.Spec.Stability)
	}
	if roundtripped.Spec.Implementation.Type != original.Spec.Implementation.Type {
		t.Errorf("Implementation.Type: got %q, want %q", roundtripped.Spec.Implementation.Type, original.Spec.Implementation.Type)
	}
	if roundtripped.Spec.Implementation.Runtime == nil {
		t.Fatal("Runtime lost in round-trip")
	}
	if roundtripped.Spec.Implementation.Runtime.Engine != "langgraph" {
		t.Errorf("Runtime.Engine: got %q, want langgraph", roundtripped.Spec.Implementation.Runtime.Engine)
	}
	if roundtripped.Spec.Implementation.Requires == nil {
		t.Fatal("Requires lost in round-trip")
	}
	if len(roundtripped.Spec.Implementation.Requires.Models) != 1 {
		t.Fatalf("Models count: got %d, want 1", len(roundtripped.Spec.Implementation.Requires.Models))
	}
	if roundtripped.Spec.Implementation.Requires.Models[0].Alias != "reasoner" {
		t.Errorf("Model alias: got %q, want reasoner", roundtripped.Spec.Implementation.Requires.Models[0].Alias)
	}
	if roundtripped.Spec.Evaluation == nil || roundtripped.Spec.Evaluation.Schedule != "0 0 * * 0" {
		t.Error("Evaluation.Schedule lost in round-trip")
	}
}

// --- Agent conversion tests ---

// TestAgentAlphaToBeta verifies shared fields map correctly.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func TestAgentAlphaToBeta(t *testing.T) {
	t.Parallel()
	src := minimalAgent()
	src.Spec.Description = "Legal AI assistant"
	src.Spec.Runtime.MaxSteps = int32Ptr(50)

	dst, lossy := ConvertAgentAlphaToBeta(src)

	if dst == nil {
		t.Fatal("expected non-nil result")
	}
	if dst.APIVersion != "agent.ai-keeper.io/v1beta1" {
		t.Errorf("APIVersion = %q, want agent.ai-keeper.io/v1beta1", dst.APIVersion)
	}
	if dst.Name != "legal-copilot" {
		t.Errorf("Name = %q", dst.Name)
	}
	if dst.Spec.DisplayName != "Legal Copilot" {
		t.Errorf("DisplayName = %q", dst.Spec.DisplayName)
	}
	if dst.Spec.Description != "Legal AI assistant" {
		t.Errorf("Description = %q", dst.Spec.Description)
	}
	if dst.Spec.Identity.ServiceAccount != "legal-bot" {
		t.Errorf("ServiceAccount = %q", dst.Spec.Identity.ServiceAccount)
	}
	if len(dst.Spec.Skills) != 1 || dst.Spec.Skills[0].Ref != shared.ResourceRef("skill://contract-review") {
		t.Errorf("Skills not preserved")
	}
	if dst.Spec.Runtime.Pattern != "tool_calling" {
		t.Errorf("Runtime.Pattern = %q", dst.Spec.Runtime.Pattern)
	}
	if dst.Spec.Runtime.MaxSteps == nil || *dst.Spec.Runtime.MaxSteps != 50 {
		t.Errorf("Runtime.MaxSteps not preserved")
	}
	if len(lossy) != 0 {
		t.Errorf("expected no lossy entries, got %v", lossy)
	}
}

// TestAgentBetaToAlpha verifies v1beta1-only fields produce lossy annotations.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func TestAgentBetaToAlpha(t *testing.T) {
	t.Parallel()
	autoRollback := true
	failureLimit := int32(3)
	src := &agentv1beta1.Agent{
		TypeMeta:   metav1.TypeMeta{APIVersion: "agent.ai-keeper.io/v1beta1", Kind: "Agent"},
		ObjectMeta: metav1.ObjectMeta{Name: "analytics-bot", Namespace: "prod"},
		Spec: agentv1beta1.AgentSpec{
			DisplayName: "Analytics Bot",
			Identity:    agentv1beta1.AgentIdentity{ServiceAccount: "analytics-sa"},
			Skills: []agentv1beta1.AgentSkillBinding{
				{Ref: shared.ResourceRef("skill://analytics")},
			},
			Runtime: agentv1beta1.AgentRuntime{Pattern: "react"},
			Deployment: &agentv1beta1.AgentDeployment{
				Replicas: int32Ptr(3),
				Rollout: &agentv1beta1.AgentRollout{
					Strategy: "canary",
					Steps:    []string{"10%", "50%", "100%"},
					Analysis: &agentv1beta1.RolloutAnalysis{
						AutoRollback: &autoRollback,
						Metrics: []agentv1beta1.RolloutAnalysisMetric{
							{Name: "error_rate", Threshold: "value < 0.01", FailureLimit: &failureLimit},
						},
					},
				},
			},
		},
	}

	dst, lossy := ConvertAgentBetaToAlpha(src)

	if dst == nil {
		t.Fatal("expected non-nil result")
	}
	if dst.APIVersion != "agent.ai-keeper.io/v1alpha1" {
		t.Errorf("APIVersion = %q", dst.APIVersion)
	}
	if dst.Spec.DisplayName != "Analytics Bot" {
		t.Errorf("DisplayName = %q", dst.Spec.DisplayName)
	}
	if dst.Spec.Deployment == nil || dst.Spec.Deployment.Rollout == nil {
		t.Fatal("Deployment.Rollout should be set")
	}
	if dst.Spec.Deployment.Rollout.Strategy != "canary" {
		t.Errorf("Rollout.Strategy = %q", dst.Spec.Deployment.Rollout.Strategy)
	}
	// Must report lossy for analysis.
	if len(lossy) == 0 {
		t.Fatal("expected lossy entries for rollout.analysis")
	}
	found := false
	for _, l := range lossy {
		if contains(l, "analysis") {
			found = true
		}
	}
	if !found {
		t.Errorf("lossy should mention analysis, got %v", lossy)
	}
}

// TestAgentRoundTrip verifies alpha→beta→alpha preserves essential fields.
//
// Validates: Requirements A11.2, A11.3, A11.4.
func TestAgentRoundTrip(t *testing.T) {
	t.Parallel()
	original := minimalAgent()
	original.Spec.Description = "Full round-trip test"
	original.Spec.Runtime.MaxSteps = int32Ptr(25)
	original.Spec.Runtime.Determinism = &agentv1alpha1.AgentDeterminism{
		Temperature: float64Ptr(0.7),
	}

	beta, _ := ConvertAgentAlphaToBeta(original)
	roundtripped, _ := ConvertAgentBetaToAlpha(beta)

	if roundtripped.Name != original.Name {
		t.Errorf("Name: got %q, want %q", roundtripped.Name, original.Name)
	}
	if roundtripped.Spec.DisplayName != original.Spec.DisplayName {
		t.Errorf("DisplayName: got %q, want %q", roundtripped.Spec.DisplayName, original.Spec.DisplayName)
	}
	if roundtripped.Spec.Description != original.Spec.Description {
		t.Errorf("Description: got %q, want %q", roundtripped.Spec.Description, original.Spec.Description)
	}
	if roundtripped.Spec.Identity.ServiceAccount != original.Spec.Identity.ServiceAccount {
		t.Errorf("ServiceAccount: got %q, want %q", roundtripped.Spec.Identity.ServiceAccount, original.Spec.Identity.ServiceAccount)
	}
	if roundtripped.Spec.Runtime.Pattern != original.Spec.Runtime.Pattern {
		t.Errorf("Pattern: got %q, want %q", roundtripped.Spec.Runtime.Pattern, original.Spec.Runtime.Pattern)
	}
	if roundtripped.Spec.Runtime.MaxSteps == nil || *roundtripped.Spec.Runtime.MaxSteps != 25 {
		t.Error("MaxSteps lost in round-trip")
	}
	if roundtripped.Spec.Runtime.Determinism == nil || roundtripped.Spec.Runtime.Determinism.Temperature == nil {
		t.Fatal("Determinism.Temperature lost in round-trip")
	}
	if *roundtripped.Spec.Runtime.Determinism.Temperature != 0.7 {
		t.Errorf("Temperature: got %f, want 0.7", *roundtripped.Spec.Runtime.Determinism.Temperature)
	}
	if len(roundtripped.Spec.Skills) != len(original.Spec.Skills) {
		t.Errorf("Skills count: got %d, want %d", len(roundtripped.Spec.Skills), len(original.Spec.Skills))
	}
}

// TestConvertNilInputs ensures nil inputs produce nil outputs without panic.
func TestConvertNilInputs(t *testing.T) {
	t.Parallel()
	if dst, _ := ConvertSkillAlphaToBeta(nil); dst != nil {
		t.Error("expected nil for nil skill alpha input")
	}
	if dst, _ := ConvertSkillBetaToAlpha(nil); dst != nil {
		t.Error("expected nil for nil skill beta input")
	}
	if dst, _ := ConvertAgentAlphaToBeta(nil); dst != nil {
		t.Error("expected nil for nil agent alpha input")
	}
	if dst, _ := ConvertAgentBetaToAlpha(nil); dst != nil {
		t.Error("expected nil for nil agent beta input")
	}
}

// --- Helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func int32Ptr(v int32) *int32       { return &v }
func float64Ptr(v float64) *float64 { return &v }
