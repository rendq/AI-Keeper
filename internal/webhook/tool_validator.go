package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// ToolValidator implements admission.CustomValidator for Tool.
type ToolValidator struct{}

var _ admission.CustomValidator = (*ToolValidator)(nil)

// ValidateCreate handles Tool CREATE.
func (v *ToolValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	tool, err := castTool(obj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Tool", tool.Name, validateTool(tool))
}

// ValidateUpdate handles Tool UPDATE.
func (v *ToolValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	tool, err := castTool(newObj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Tool", tool.Name, validateTool(tool))
}

// ValidateDelete is a no-op for Tool.
func (v *ToolValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func castTool(obj runtime.Object) (*skillv1alpha1.Tool, error) {
	tool, ok := obj.(*skillv1alpha1.Tool)
	if !ok {
		return nil, fmt.Errorf("expected *skill.ai-keeper.io/v1alpha1.Tool, got %T", obj)
	}
	return tool, nil
}

// validateTool walks Tool spec for cross-field invariants
// (Requirement A9.2 lint rule `tool/destructive-needs-approval` is the
// most important one — destructive tools MUST require approval).
//
// Validates: Requirements A1.3, A2.1, A2.5.
func validateTool(tool *skillv1alpha1.Tool) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, validateDNS1123Name(&tool.ObjectMeta)...)

	specPath := field.NewPath("spec")

	// Governance + classification (kubebuilder enum already enforces the
	// allowed values; we re-check defensively).
	gov := &tool.Spec.Governance.GovernanceBlock
	errs = append(errs, validateGovernanceBlock(specPath.Child("governance"), gov)...)

	// Cross-field: destructive side-effects MUST require approval. This
	// is Requirement A9.2 elevated to admission so destructive tools
	// can never reach the cluster without an approval flow.
	if tool.Spec.Governance.SideEffects == "destructive" {
		approval := tool.Spec.Governance.RequiresApproval
		if approval == nil || !*approval {
			errs = append(errs, field.Invalid(
				specPath.Child("governance", "requiresApproval"),
				approval,
				"sideEffects=destructive requires governance.requiresApproval=true",
			))
		}
	}

	// Authentication.tokenExchangeRef must be set when mode=oauth2_obo.
	if auth := tool.Spec.Authentication; auth != nil {
		if auth.Mode == "oauth2_obo" && auth.TokenExchangeRef == "" {
			errs = append(errs, field.Required(
				specPath.Child("authentication", "tokenExchangeRef"),
				"mode=oauth2_obo requires tokenExchangeRef",
			))
		}
	}

	// Reliability block refs.
	if rel := tool.Spec.Reliability; rel != nil {
		errs = append(errs, validateReliabilityBlock(specPath.Child("reliability"), rel)...)
	}
	return errs
}
