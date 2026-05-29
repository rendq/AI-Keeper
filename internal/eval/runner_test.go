package eval

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestWorkflowName(t *testing.T) {
	tests := []struct {
		skillName string
		runID     string
		want      string
	}{
		{
			skillName: "sentiment-analysis",
			runID:     "abc123",
			want:      "aip-eval-skill-sentiment-analysis-abc123",
		},
		{
			skillName: "MySkill",
			runID:     "RUN-42",
			want:      "aip-eval-skill-myskill-run-42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.skillName, func(t *testing.T) {
			got := WorkflowName(tt.skillName, tt.runID)
			if got != tt.want {
				t.Errorf("WorkflowName(%q, %q) = %q, want %q", tt.skillName, tt.runID, got, tt.want)
			}
		})
	}
}

func TestWorkflowNameTruncation(t *testing.T) {
	longName := ""
	for i := 0; i < 300; i++ {
		longName += "a"
	}
	got := WorkflowName(longName, "run1")
	if len(got) > 253 {
		t.Errorf("WorkflowName should truncate to 253 chars, got %d", len(got))
	}
}

func TestEnqueue_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	runner := NewEvalRunner(client)

	req := EvalRequest{
		SkillNamespace: "team-alpha",
		SkillName:      "summarizer",
		EvalSetRef:     "ref://eval-sets/summarizer-v1",
		RedTeamSetRef:  "ref://red-team-sets/injection-v1",
		RunID:          "run-001",
		RequestedAt:    metav1.NewTime(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)),
	}

	err := runner.Enqueue(context.Background(), req)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Verify the create action was called.
	actions := client.Actions()
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}

	createAction, ok := actions[0].(clienttesting.CreateAction)
	if !ok {
		t.Fatalf("expected CreateAction, got %T", actions[0])
	}

	// Verify GVR.
	expectedGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "workflows",
	}
	if createAction.GetResource() != expectedGVR {
		t.Errorf("GVR = %v, want %v", createAction.GetResource(), expectedGVR)
	}

	// Verify namespace.
	if createAction.GetNamespace() != "team-alpha" {
		t.Errorf("namespace = %q, want %q", createAction.GetNamespace(), "team-alpha")
	}

	// Verify workflow content.
	obj := createAction.GetObject().(*unstructured.Unstructured)
	name, _, _ := unstructured.NestedString(obj.Object, "metadata", "name")
	if name != "aip-eval-skill-summarizer-run-001" {
		t.Errorf("workflow name = %q, want %q", name, "aip-eval-skill-summarizer-run-001")
	}

	// Verify labels.
	labels, _, _ := unstructured.NestedStringMap(obj.Object, "metadata", "labels")
	if labels["ai-keeper.io/skill-name"] != "summarizer" {
		t.Errorf("label ai-keeper.io/skill-name = %q, want %q", labels["ai-keeper.io/skill-name"], "summarizer")
	}
	if labels["ai-keeper.io/eval-run-id"] != "run-001" {
		t.Errorf("label ai-keeper.io/eval-run-id = %q, want %q", labels["ai-keeper.io/eval-run-id"], "run-001")
	}

	// Verify entrypoint.
	entrypoint, _, _ := unstructured.NestedString(obj.Object, "spec", "entrypoint")
	if entrypoint != "run-eval" {
		t.Errorf("entrypoint = %q, want %q", entrypoint, "run-eval")
	}

	// Verify service account.
	sa, _, _ := unstructured.NestedString(obj.Object, "spec", "serviceAccountName")
	if sa != DefaultServiceAccount {
		t.Errorf("serviceAccountName = %q, want %q", sa, DefaultServiceAccount)
	}
}

func TestEnqueue_CustomOptions(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	runner := NewEvalRunner(client,
		WithEvalImage("my-registry/eval:v2"),
		WithServiceAccount("custom-sa"),
	)

	req := EvalRequest{
		SkillNamespace: "default",
		SkillName:      "qa-bot",
		RunID:          "run-002",
		RequestedAt:    metav1.Now(),
	}

	err := runner.Enqueue(context.Background(), req)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	actions := client.Actions()
	createAction := actions[0].(clienttesting.CreateAction)
	obj := createAction.GetObject().(*unstructured.Unstructured)

	// Verify custom service account.
	sa, _, _ := unstructured.NestedString(obj.Object, "spec", "serviceAccountName")
	if sa != "custom-sa" {
		t.Errorf("serviceAccountName = %q, want %q", sa, "custom-sa")
	}

	// Verify custom image in template.
	templates, _, _ := unstructured.NestedSlice(obj.Object, "spec", "templates")
	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}
	tmpl := templates[0].(map[string]interface{})
	image, _, _ := unstructured.NestedString(tmpl, "container", "image")
	if image != "my-registry/eval:v2" {
		t.Errorf("image = %q, want %q", image, "my-registry/eval:v2")
	}
}

func TestEnqueue_ValidationErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)
	runner := NewEvalRunner(client)

	tests := []struct {
		name string
		req  EvalRequest
	}{
		{
			name: "missing skill name",
			req: EvalRequest{
				SkillNamespace: "ns",
				RunID:          "run-1",
				RequestedAt:    metav1.Now(),
			},
		},
		{
			name: "missing run ID",
			req: EvalRequest{
				SkillNamespace: "ns",
				SkillName:      "skill",
				RequestedAt:    metav1.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runner.Enqueue(context.Background(), tt.req)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestEnqueue_ParameterInjection(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)
	runner := NewEvalRunner(client)

	req := EvalRequest{
		SkillNamespace: "prod",
		SkillName:      "classifier",
		EvalSetRef:     "ref://eval-sets/class-v2",
		RedTeamSetRef:  "ref://red-team-sets/adversarial-v1",
		RunID:          "run-xyz",
		RequestedAt:    metav1.Now(),
	}

	err := runner.Enqueue(context.Background(), req)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	actions := client.Actions()
	createAction := actions[0].(clienttesting.CreateAction)
	obj := createAction.GetObject().(*unstructured.Unstructured)

	// Extract env vars from the first template's container.
	templates, _, _ := unstructured.NestedSlice(obj.Object, "spec", "templates")
	tmpl := templates[0].(map[string]interface{})
	envSlice, _, _ := unstructured.NestedSlice(tmpl, "container", "env")

	envMap := make(map[string]string)
	for _, e := range envSlice {
		entry := e.(map[string]interface{})
		envMap[entry["name"].(string)] = entry["value"].(string)
	}

	// Verify all expected env vars are injected.
	expectations := map[string]string{
		"AIP_SKILL_NAMESPACE":  "prod",
		"AIP_SKILL_NAME":       "classifier",
		"AIP_EVAL_RUN_ID":      "run-xyz",
		"AIP_EVAL_SET_REF":     "ref://eval-sets/class-v2",
		"AIP_RED_TEAM_SET_REF": "ref://red-team-sets/adversarial-v1",
	}

	for key, want := range expectations {
		got, ok := envMap[key]
		if !ok {
			t.Errorf("env var %q not found", key)
			continue
		}
		if got != want {
			t.Errorf("env[%q] = %q, want %q", key, got, want)
		}
	}
}
