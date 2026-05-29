package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// PolicyValidator implements admission.CustomValidator for Policy.
type PolicyValidator struct{}

var _ admission.CustomValidator = (*PolicyValidator)(nil)

// ValidateCreate handles Policy CREATE.
func (v *PolicyValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	p, err := castPolicy(obj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Policy", p.Name, validatePolicy(p))
}

// ValidateUpdate handles Policy UPDATE.
func (v *PolicyValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	p, err := castPolicy(newObj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Policy", p.Name, validatePolicy(p))
}

// ValidateDelete is a no-op for Policy.
func (v *PolicyValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func castPolicy(obj runtime.Object) (*policyv1alpha1.Policy, error) {
	p, ok := obj.(*policyv1alpha1.Policy)
	if !ok {
		return nil, fmt.Errorf("expected *policy.ai-keeper.io/v1alpha1.Policy, got %T", obj)
	}
	return p, nil
}

// validatePolicy walks Policy.spec for ResourceRef / Duration /
// Classification invariants. The Priority `0..1000` bound and Effect
// enum are already enforced by kubebuilder markers; we focus on cross-
// field rules.
//
// Validates: Requirements A1.3, A2.1, A2.2, A2.5.
func validatePolicy(p *policyv1alpha1.Policy) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, validateDNS1123Name(&p.ObjectMeta)...)

	specPath := field.NewPath("spec")

	// Action.resources.anyOf[*].match.ref.
	for i, sel := range p.Spec.Action.Resources.AnyOf {
		if sel.Match != nil {
			errs = append(errs, validateOptionalResourceRef(specPath.Child("action", "resources", "anyOf").Index(i).Child("match", "ref"), sel.Match.Ref)...)
		}
	}

	// Constraints (rateLimit + budget) refs.
	if c := p.Spec.Constraints; c != nil {
		errs = append(errs, validateOptionalResourceRef(specPath.Child("constraints", "quota"), c.Quota)...)
	}

	// Approvals.timeout.
	for i, ap := range p.Spec.Approvals {
		errs = append(errs, validateOptionalDuration(specPath.Child("approvals").Index(i).Child("timeout"), ap.Timeout)...)
	}

	// Obligations: redact.patternsRef + notify channel refs (they are
	// strings, not ResourceRef, so we only validate refs here).
	if ob := p.Spec.Obligations; ob != nil {
		if ob.Redact != nil {
			errs = append(errs, validateOptionalResourceRef(specPath.Child("obligations", "redact", "patternsRef"), ob.Redact.PatternsRef)...)
		}
		if ob.Audit != nil {
			errs = append(errs, validateResourceRefList(specPath.Child("obligations", "audit", "forwardTo"), ob.Audit.ForwardTo)...)
		}
	}

	return errs
}
