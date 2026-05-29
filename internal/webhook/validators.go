package webhook

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// The helpers below are intentionally narrow: each accepts the smallest
// piece of state it needs (a string, a metadata.ObjectMeta, ...) and
// returns a `field.ErrorList`. Validators compose them by walking the
// CR's spec tree explicitly — reflection-based walks are tempting but
// hide the canonical error path that gets surfaced to `kubectl apply`,
// and they make new fields silently un-validated.

// validateResourceRef validates that `value` matches the ResourceRef
// regex (Requirement A2.1). An empty value is treated as "not set" and
// produces no error — callers use `field.Required(...)` separately when
// the field is mandatory.
func validateResourceRef(path *field.Path, value shared.ResourceRef) field.ErrorList {
	if value == "" {
		return nil
	}
	if !value.IsValid() {
		return field.ErrorList{field.Invalid(path, string(value), "must match ResourceRef regex (scheme://path[@version])")}
	}
	return nil
}

// validateOptionalResourceRef applies validateResourceRef to a pointer.
func validateOptionalResourceRef(path *field.Path, value *shared.ResourceRef) field.ErrorList {
	if value == nil {
		return nil
	}
	return validateResourceRef(path, *value)
}

// validateResourceRefList validates each element of a slice.
func validateResourceRefList(path *field.Path, values []shared.ResourceRef) field.ErrorList {
	var errs field.ErrorList
	for i, v := range values {
		errs = append(errs, validateResourceRef(path.Index(i), v)...)
	}
	return errs
}

// validateDuration validates a Duration value (Requirement A2.2). An
// empty value is treated as "not set".
func validateDuration(path *field.Path, value shared.Duration) field.ErrorList {
	if value == "" {
		return nil
	}
	if !value.IsValid() {
		return field.ErrorList{field.Invalid(path, string(value), "must match Duration regex (^\\d+(ns|us|ms|s|m|h|d|w)$)")}
	}
	return nil
}

// validateOptionalDuration applies validateDuration to a pointer.
func validateOptionalDuration(path *field.Path, value *shared.Duration) field.ErrorList {
	if value == nil {
		return nil
	}
	return validateDuration(path, *value)
}

// validateSemVer validates a SemVer value (Requirement A2.3). An empty
// value is treated as "not set"; callers should additionally guard
// required fields.
func validateSemVer(path *field.Path, value shared.SemVer) field.ErrorList {
	if value == "" {
		return nil
	}
	if !value.IsValid() {
		return field.ErrorList{field.Invalid(path, string(value), "must be a strict semver string (e.g. 1.2.3 or 1.2.3-rc.1+build.7)")}
	}
	return nil
}

// validateClassification validates a Classification value (Requirement
// A2.5). An empty value is treated as "not set".
func validateClassification(path *field.Path, value shared.Classification) field.ErrorList {
	if value == "" {
		return nil
	}
	switch value {
	case shared.ClassificationPublic,
		shared.ClassificationInternal,
		shared.ClassificationConfidential,
		shared.ClassificationRestricted,
		shared.ClassificationSecret:
		return nil
	}
	return field.ErrorList{field.NotSupported(path, string(value), []string{
		string(shared.ClassificationPublic),
		string(shared.ClassificationInternal),
		string(shared.ClassificationConfidential),
		string(shared.ClassificationRestricted),
		string(shared.ClassificationSecret),
	})}
}

// validateOptionalClassification applies validateClassification to a
// pointer.
func validateOptionalClassification(path *field.Path, value *shared.Classification) field.ErrorList {
	if value == nil {
		return nil
	}
	return validateClassification(path, *value)
}

// validateStage validates a Stage value (Requirement A2.6).
func validateStage(path *field.Path, value shared.Stage) field.ErrorList {
	if value == "" {
		return nil
	}
	switch value {
	case shared.StageExperimental, shared.StageBeta, shared.StageStable, shared.StageDeprecated:
		return nil
	}
	return field.ErrorList{field.NotSupported(path, string(value), []string{
		string(shared.StageExperimental),
		string(shared.StageBeta),
		string(shared.StageStable),
		string(shared.StageDeprecated),
	})}
}

// validateDNS1123Name validates `metadata.name` (Requirement A2.4).
//
// metadata.namespace is similarly bounded by k8s; we re-check it for
// CRDs that rely on a non-default namespace (e.g. Tenant CRs which the
// API server forbids namespacing because they are cluster-scoped).
func validateDNS1123Name(meta *metav1.ObjectMeta) field.ErrorList {
	var errs field.ErrorList
	if meta == nil {
		return errs
	}
	path := field.NewPath("metadata", "name")
	if meta.Name == "" {
		// Generated names (`metadata.generateName`) are allowed: the
		// API server fills them in before our webhook sees the request.
		if meta.GenerateName == "" {
			errs = append(errs, field.Required(path, "metadata.name (or generateName) is required"))
		}
		return errs
	}
	if err := shared.IsValidDNS1123Subdomain(meta.Name); err != nil {
		errs = append(errs, field.Invalid(path, meta.Name, err.Error()))
	}
	return errs
}

// errorListToError converts a field.ErrorList into a single Go error
// suitable for return from a CustomValidator. A nil list returns nil.
//
// We keep the conversion here (rather than at every call site) so the
// formatting is identical across resources.
func errorListToError(kind, name string, errs field.ErrorList) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s %q is invalid: %s", kind, name, errs.ToAggregate().Error())
}
