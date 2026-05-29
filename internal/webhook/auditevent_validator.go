package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	auditv1alpha1 "github.com/ai-keeper/ai-keeper/api/audit/v1alpha1"
)

// AuditEventValidator implements admission.CustomValidator for
// AuditEvent. Its primary job is enforcing Requirement A1.5: only
// system ServiceAccounts annotated `ai-keeper.io/system=true` may
// CREATE/UPDATE/DELETE.
//
// Field-level validation is intentionally minimal because the AuditEvent
// is itself the audit record; rejecting "almost-correct" events would
// risk dropping legitimate forensic data. The CRD's OpenAPI schema and
// the eventHash check on the dataplane side cover field shape; this
// validator only adds the system-SA gate plus a metadata.name DNS-1123
// re-check.
type AuditEventValidator struct {
	// SystemSAChecker is used to determine whether the requester has
	// the `ai-keeper.io/system=true` annotation. Required.
	SystemSAChecker *SystemSAChecker
}

var _ admission.CustomValidator = (*AuditEventValidator)(nil)

// ValidateCreate handles AuditEvent CREATE.
func (v *AuditEventValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	if err := v.guardSystemSA(ctx); err != nil {
		return nil, err
	}
	ae, err := castAuditEvent(obj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("AuditEvent", ae.Name, validateAuditEvent(ae))
}

// ValidateUpdate handles AuditEvent UPDATE.
func (v *AuditEventValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	if err := v.guardSystemSA(ctx); err != nil {
		return nil, err
	}
	ae, err := castAuditEvent(newObj)
	if err != nil {
		return nil, err
	}
	return nil, errorListToError("AuditEvent", ae.Name, validateAuditEvent(ae))
}

// ValidateDelete handles AuditEvent DELETE — also gated by the
// system-SA check (Requirement A1.5).
func (v *AuditEventValidator) ValidateDelete(ctx context.Context, _ runtime.Object) (admission.Warnings, error) {
	if err := v.guardSystemSA(ctx); err != nil {
		return nil, err
	}
	return nil, nil
}

func castAuditEvent(obj runtime.Object) (*auditv1alpha1.AuditEvent, error) {
	ae, ok := obj.(*auditv1alpha1.AuditEvent)
	if !ok {
		return nil, fmt.Errorf("expected *audit.ai-keeper.io/v1alpha1.AuditEvent, got %T", obj)
	}
	return ae, nil
}

// guardSystemSA pulls UserInfo from the admission context and runs the
// SystemSAChecker. Returns a denial error when the request is not from
// a system writer.
func (v *AuditEventValidator) guardSystemSA(ctx context.Context) error {
	if v.SystemSAChecker == nil {
		return fmt.Errorf("AuditEventValidator: SystemSAChecker is not configured")
	}
	userInfo, ok := userInfoFromContext(ctx)
	if !ok {
		return fmt.Errorf("AuditEventValidator: no admission request in context")
	}
	return v.SystemSAChecker.Check(ctx, userInfo)
}

// validateAuditEvent runs the minimal field validation required at
// admission time. Most fields are validated by the OpenAPI schema; this
// function exists as a hook for future cross-field invariants.
func validateAuditEvent(ae *auditv1alpha1.AuditEvent) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, validateDNS1123Name(&ae.ObjectMeta)...)
	return errs
}
