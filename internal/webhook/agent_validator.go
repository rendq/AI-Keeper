package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
)

// AgentValidator implements admission.CustomValidator for Agent.
type AgentValidator struct{}

var _ admission.CustomValidator = (*AgentValidator)(nil)

// ValidateCreate handles Agent CREATE.
func (v *AgentValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	a, err := castAgent(obj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Agent", a.Name, validateAgent(a))
}

// ValidateUpdate handles Agent UPDATE.
func (v *AgentValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	a, err := castAgent(newObj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("Agent", a.Name, validateAgent(a))
}

// ValidateDelete is a no-op for Agent.
func (v *AgentValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func castAgent(obj runtime.Object) (*agentv1alpha1.Agent, error) {
	a, ok := obj.(*agentv1alpha1.Agent)
	if !ok {
		return nil, fmt.Errorf("expected *agent.ai-keeper.io/v1alpha1.Agent, got %T", obj)
	}
	return a, nil
}

// validateAgent walks Agent spec for ref / duration / classification
// invariants. Most field validation is enforced at the OpenAPI layer
// (kubebuilder Enum/Pattern markers on the embedded blocks); this layer
// catches ResourceRef shape errors that the OpenAPI schema cannot
// express directly because the field type is `string` in JSON.
//
// Validates: Requirements A1.3, A2.1, A2.2, A2.5.
func validateAgent(a *agentv1alpha1.Agent) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, validateDNS1123Name(&a.ObjectMeta)...)

	specPath := field.NewPath("spec")

	// Skill bindings.
	for i, sb := range a.Spec.Skills {
		errs = append(errs, validateResourceRef(specPath.Child("skills").Index(i).Child("ref"), sb.Ref)...)
	}

	// Memory backends.
	if mem := a.Spec.Memory; mem != nil {
		if mem.ShortTerm != nil {
			errs = append(errs, validateOptionalDuration(specPath.Child("memory", "shortTerm", "ttl"), mem.ShortTerm.TTL)...)
			errs = append(errs, validateOptionalResourceRef(specPath.Child("memory", "shortTerm", "storage"), mem.ShortTerm.Storage)...)
		}
		if mem.LongTerm != nil {
			errs = append(errs, validateOptionalResourceRef(specPath.Child("memory", "longTerm", "ref"), mem.LongTerm.Ref)...)
			errs = append(errs, validateOptionalDuration(specPath.Child("memory", "longTerm", "retention"), mem.LongTerm.Retention)...)
		}
	}

	// Runtime durations.
	rt := a.Spec.Runtime
	errs = append(errs, validateOptionalDuration(specPath.Child("runtime", "timeout"), rt.Timeout)...)

	// Audit retention + forwarder refs.
	if au := a.Spec.Audit; au != nil {
		errs = append(errs, validateOptionalDuration(specPath.Child("audit", "retention"), au.Retention)...)
		for i, f := range au.Forwarders {
			errs = append(errs, validateResourceRef(specPath.Child("audit", "forwarders").Index(i).Child("ref"), f.Ref)...)
		}
	}

	// Deployment rollout.analysisInterval / analysisRef.
	if dep := a.Spec.Deployment; dep != nil && dep.Rollout != nil {
		errs = append(errs, validateOptionalDuration(specPath.Child("deployment", "rollout", "analysisInterval"), dep.Rollout.AnalysisInterval)...)
		errs = append(errs, validateOptionalResourceRef(specPath.Child("deployment", "rollout", "analysisRef"), dep.Rollout.AnalysisRef)...)
	}

	// Channel refs.
	for i, ch := range a.Spec.Channels {
		errs = append(errs, validateOptionalResourceRef(specPath.Child("channels").Index(i).Child("ref"), ch.Ref)...)
	}

	// Guardrail behavior.systemPrompt ref.
	if gr := a.Spec.Guardrails; gr != nil && gr.Behavior != nil {
		errs = append(errs, validateOptionalResourceRef(specPath.Child("guardrails", "behavior", "systemPrompt"), gr.Behavior.SystemPrompt)...)
	}

	return errs
}
