package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// SkillValidator implements admission.CustomValidator for Skill.
type SkillValidator struct{}

// Compile-time interface check.
var _ admission.CustomValidator = (*SkillValidator)(nil)

// ValidateCreate is invoked on Skill CREATE.
func (v *SkillValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	skill, err := castSkill(obj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Skill", skill.Name, validateSkill(skill))
}

// ValidateUpdate is invoked on Skill UPDATE.
func (v *SkillValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	skill, err := castSkill(newObj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Skill", skill.Name, validateSkill(skill))
}

// ValidateDelete is invoked on Skill DELETE — no validation is needed
// at delete time (the controller drives the deletion finaliser).
func (v *SkillValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func castSkill(obj runtime.Object) (*skillv1alpha1.Skill, error) {
	skill, ok := obj.(*skillv1alpha1.Skill)
	if !ok {
		return nil, fmt.Errorf("expected *skill.ai-keeper.io/v1alpha1.Skill, got %T", obj)
	}
	return skill, nil
}

// validateSkill walks the Skill spec and accumulates field errors.
//
// Validates: Requirements A1.3, A2.1—A2.6.
func validateSkill(skill *skillv1alpha1.Skill) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, validateDNS1123Name(&skill.ObjectMeta)...)

	specPath := field.NewPath("spec")
	errs = append(errs, validateSemVer(specPath.Child("version"), skill.Spec.Version)...)
	errs = append(errs, validateStage(specPath.Child("stability"), skill.Spec.Stability)...)

	// Implementation requires.
	if req := skill.Spec.Implementation.Requires; req != nil {
		reqPath := specPath.Child("implementation", "requires")
		for i, m := range req.Models {
			mp := reqPath.Child("models").Index(i)
			errs = append(errs, validateResourceRef(mp.Child("ref"), m.Ref)...)
			errs = append(errs, validateResourceRefList(mp.Child("fallback"), m.Fallback)...)
		}
		for i, t := range req.Tools {
			errs = append(errs, validateResourceRef(reqPath.Child("tools").Index(i).Child("ref"), t.Ref)...)
		}
		for i, d := range req.DataSources {
			errs = append(errs, validateResourceRef(reqPath.Child("dataSources").Index(i).Child("ref"), d.Ref)...)
		}
		for i, s := range req.Skills {
			errs = append(errs, validateResourceRef(reqPath.Child("skills").Index(i).Child("ref"), s.Ref)...)
		}
	}

	// Prompt template ref.
	if pt := skill.Spec.Implementation.PromptTemplate; pt != nil {
		errs = append(errs, validateOptionalResourceRef(specPath.Child("implementation", "promptTemplate", "ref"), pt.Ref)...)
	}

	// Governance: classification + DLP patternsRef + retention.
	if gov := skill.Spec.Governance; gov != nil {
		errs = append(errs, validateGovernanceBlock(specPath.Child("governance"), gov)...)
	}

	// Reliability timeout + fallback refs.
	if rel := skill.Spec.Reliability; rel != nil {
		errs = append(errs, validateReliabilityBlock(specPath.Child("reliability"), rel)...)
	}

	// Evaluation refs.
	if ev := skill.Spec.Evaluation; ev != nil {
		errs = append(errs, validateOptionalResourceRef(specPath.Child("evaluation", "evalSet"), ev.EvalSet)...)
		errs = append(errs, validateOptionalResourceRef(specPath.Child("evaluation", "redTeamSet"), ev.RedTeamSet)...)
	}

	// Lifecycle.deprecation.successor / migrationGuide refs.
	if lc := skill.Spec.Lifecycle; lc != nil && lc.Deprecation != nil {
		errs = append(errs, validateOptionalResourceRef(specPath.Child("lifecycle", "deprecation", "successor"), lc.Deprecation.Successor)...)
		errs = append(errs, validateOptionalResourceRef(specPath.Child("lifecycle", "deprecation", "migrationGuide"), lc.Deprecation.MigrationGuide)...)
	}

	return errs
}

// validateGovernanceBlock validates a shared.GovernanceBlock reference.
func validateGovernanceBlock(path *field.Path, gov *shared.GovernanceBlock) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, validateOptionalClassification(path.Child("classification"), gov.Classification)...)
	if gov.PII != nil {
		errs = append(errs, validateOptionalResourceRef(path.Child("pii", "patternsRef"), gov.PII.PatternsRef)...)
	}
	if gov.Compliance != nil {
		errs = append(errs, validateOptionalResourceRef(path.Child("compliance", "reportTemplate"), gov.Compliance.ReportTemplate)...)
	}
	return errs
}

// validateReliabilityBlock walks the timeout + fallback chain.
func validateReliabilityBlock(path *field.Path, rel *shared.ReliabilityBlock) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, validateOptionalDuration(path.Child("timeout"), rel.Timeout)...)
	if rel.Retries == nil {
		// nothing
	}
	for i, fb := range rel.Fallback {
		errs = append(errs, validateOptionalResourceRef(path.Child("fallback").Index(i).Child("ref"), fb.Ref)...)
	}
	if rel.CircuitBreaker != nil {
		errs = append(errs, validateOptionalDuration(path.Child("circuitBreaker", "window"), rel.CircuitBreaker.Window)...)
	}
	return errs
}
