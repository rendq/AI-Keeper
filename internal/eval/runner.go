package eval

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	// ArgoWorkflowGVR is the GVR for Argo Workflow resources.
	argoGroup    = "argoproj.io"
	argoVersion  = "v1alpha1"
	argoResource = "workflows"

	// DefaultEvalImage is the container image for the Python eval runner.
	DefaultEvalImage = "ghcr.io/aip-io/aip-eval:latest"

	// DefaultServiceAccount for Argo workflow pods.
	DefaultServiceAccount = "aip-eval-runner"
)

// EvalRunner schedules and manages eval workflows via Argo Workflows.
// It creates Argo Workflow CRs programmatically using unstructured objects.
type EvalRunner struct {
	// dynamicClient is used to create Argo Workflow CRs.
	dynamicClient dynamic.Interface

	// evalImage is the container image for the Python eval sidecar.
	evalImage string

	// serviceAccount for the workflow pods.
	serviceAccount string
}

// Option configures the EvalRunner.
type Option func(*EvalRunner)

// WithEvalImage sets a custom eval container image.
func WithEvalImage(image string) Option {
	return func(r *EvalRunner) {
		r.evalImage = image
	}
}

// WithServiceAccount sets a custom service account for workflow pods.
func WithServiceAccount(sa string) Option {
	return func(r *EvalRunner) {
		r.serviceAccount = sa
	}
}

// NewEvalRunner creates a new EvalRunner with the given dynamic client.
func NewEvalRunner(client dynamic.Interface, opts ...Option) *EvalRunner {
	r := &EvalRunner{
		dynamicClient:  client,
		evalImage:      DefaultEvalImage,
		serviceAccount: DefaultServiceAccount,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Enqueue creates an Argo Workflow CR to run the evaluation for the given Skill.
// The workflow name follows the pattern: aip-eval-skill-<name>-<runId>
func (r *EvalRunner) Enqueue(ctx context.Context, req EvalRequest) error {
	if req.SkillName == "" {
		return fmt.Errorf("eval: skillName is required")
	}
	if req.RunID == "" {
		return fmt.Errorf("eval: runID is required")
	}

	wf := r.buildWorkflow(req)

	gvr := schema.GroupVersionResource{
		Group:    argoGroup,
		Version:  argoVersion,
		Resource: argoResource,
	}

	_, err := r.dynamicClient.Resource(gvr).Namespace(req.SkillNamespace).Create(
		ctx, wf, metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("eval: failed to create workflow for skill %s/%s: %w",
			req.SkillNamespace, req.SkillName, err)
	}

	return nil
}

// WorkflowName generates the canonical workflow name for an eval run.
func WorkflowName(skillName, runID string) string {
	// Sanitize: K8s names must be DNS-1123 compliant (lowercase, max 253 chars).
	name := fmt.Sprintf("aip-eval-skill-%s-%s", strings.ToLower(skillName), strings.ToLower(runID))
	if len(name) > 253 {
		name = name[:253]
	}
	return name
}

// buildWorkflow constructs an unstructured Argo Workflow object.
func (r *EvalRunner) buildWorkflow(req EvalRequest) *unstructured.Unstructured {
	wfName := WorkflowName(req.SkillName, req.RunID)

	// Build environment variables for the Python eval container.
	envVars := []interface{}{
		map[string]interface{}{
			"name":  "AIP_SKILL_NAMESPACE",
			"value": req.SkillNamespace,
		},
		map[string]interface{}{
			"name":  "AIP_SKILL_NAME",
			"value": req.SkillName,
		},
		map[string]interface{}{
			"name":  "AIP_EVAL_RUN_ID",
			"value": req.RunID,
		},
	}
	if req.EvalSetRef != "" {
		envVars = append(envVars, map[string]interface{}{
			"name":  "AIP_EVAL_SET_REF",
			"value": req.EvalSetRef,
		})
	}
	if req.RedTeamSetRef != "" {
		envVars = append(envVars, map[string]interface{}{
			"name":  "AIP_RED_TEAM_SET_REF",
			"value": req.RedTeamSetRef,
		})
	}

	// Construct the Argo Workflow using unstructured map.
	wf := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", argoGroup, argoVersion),
			"kind":       "Workflow",
			"metadata": map[string]interface{}{
				"name":      wfName,
				"namespace": req.SkillNamespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "aip-eval-runner",
					"ai-keeper.io/skill-name":            req.SkillName,
					"ai-keeper.io/eval-run-id":           req.RunID,
				},
				"annotations": map[string]interface{}{
					"ai-keeper.io/requested-at": req.RequestedAt.Format(time.RFC3339),
				},
			},
			"spec": map[string]interface{}{
				"entrypoint":         "run-eval",
				"serviceAccountName": r.serviceAccount,
				"ttlStrategy": map[string]interface{}{
					"secondsAfterCompletion": int64(3600), // cleanup after 1h
				},
				"templates": []interface{}{
					map[string]interface{}{
						"name": "run-eval",
						"container": map[string]interface{}{
							"image":   r.evalImage,
							"command": []interface{}{"python", "-m", "aip_eval.runner"},
							"env":     envVars,
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"cpu":    "500m",
									"memory": "512Mi",
								},
								"limits": map[string]interface{}{
									"cpu":    "2",
									"memory": "2Gi",
								},
							},
						},
						"activeDeadlineSeconds": int64(1800), // 30min timeout
					},
				},
			},
		},
	}

	return wf
}
