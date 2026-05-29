package tenant

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Reason constants surfaced on Tenant conditions and Events.
const (
	// ReasonProvisioned marks `NamespacesReady=True`.
	ReasonProvisioned = "Provisioned"
	// ReasonProvisioning marks `NamespacesReady=False` while the
	// namespace / default Budget / Quota are still being created.
	ReasonProvisioning = "Provisioning"
	// ReasonNamespaceFailed marks `NamespacesReady=False` on a hard
	// API server error.
	ReasonNamespaceFailed = "NamespaceFailed"
	// ReasonBudgetFailed marks `NamespacesReady=False` when the default
	// Budget could not be seeded.
	ReasonBudgetFailed = "BudgetFailed"
	// ReasonQuotaFailed marks `NamespacesReady=False` when the default
	// Quota could not be seeded.
	ReasonQuotaFailed = "QuotaFailed"

	// ReasonConnectorsDeferred reports that connector templates are
	// not wired in this build (P0 placeholder).
	ReasonConnectorsDeferred = "ConnectorsDeferred"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// FinalizerTenantCleanup is the finalizer added to every reconciled
// Tenant CR so the controller can deregister `status.namespaces` on
// deletion (Requirement A7.1).
const FinalizerTenantCleanup = "ai-keeper.io/tenant-cleanup"

// LabelTenant is the Kubernetes label stamped on every namespace this
// controller provisions. The value is the Tenant CR name.
const LabelTenant = "ai-keeper.io/tenant"

// LabelManagedBy mirrors the standard `app.kubernetes.io/managed-by`
// label so kubectl filtering surfaces controller-owned resources.
const LabelManagedBy = "app.kubernetes.io/managed-by"

// ManagerName identifies this controller in `managed-by` labels.
const ManagerName = "aip-tenant-controller"

// DefaultBudgetName is the canonical name of the Budget seeded into
// every tenant namespace.
const DefaultBudgetName = "default"

// DefaultQuotaName is the canonical name of the Quota seeded into every
// tenant namespace.
const DefaultQuotaName = "default"

// DefaultBudgetPeriod is the seed period applied when
// `spec.defaultBudget` does not specify one. Monthly aligns with the
// per-tenant USD/Tokens caps that operators care about during onboarding.
const DefaultBudgetPeriod = "monthly"

// SteadyStateRequeue mirrors the long-tail requeue used by the other
// AIP controllers so steady-state reconciles eventually re-evaluate
// drift.
const SteadyStateRequeue = 10 * time.Minute

// DefaultQuotaLimits is the placeholder cap applied to every freshly
// provisioned tenant. The values are intentionally loose so the P0
// onboarding flow does not block on quota; production operators
// override them via `aikctl tenant set-quota` or Helm values once the
// tenant has been triaged.
//
// The string keys map to AIP CRD kinds; the `intstr.IntOrString`
// values follow the Quota CRD wire shape.
var DefaultQuotaLimits = map[string]intstr.IntOrString{
	"agents":         intstr.FromInt32(100),
	"skills":         intstr.FromInt32(100),
	"tools":          intstr.FromInt32(100),
	"modelEndpoints": intstr.FromInt32(50),
	"knowledgeBases": intstr.FromInt32(50),
	"dataSources":    intstr.FromInt32(50),
}

// namespaceFor returns the Kubernetes namespace name that backs the
// supplied Tenant CR. P0 uses the `tenant-<name>` prefix; future
// revisions may consult `spec.deployment.namespacePrefix` (not yet in
// the spec).
func namespaceFor(tenant *corev1alpha1.Tenant) string {
	if tenant == nil || tenant.Name == "" {
		return ""
	}
	return "tenant-" + tenant.Name
}

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. `NamespacesReady=False` → Provisioning
//  3. Aggregate Ready=True → Active
//  4. Otherwise → Pending
func derivePhase(tenant *corev1alpha1.Tenant) sharedv1alpha1.Phase {
	if tenant == nil {
		return sharedv1alpha1.PhasePending
	}
	if !tenant.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	if isTrue(tenant.Status.Conditions, corev1alpha1.TenantReady) {
		return sharedv1alpha1.PhaseActive
	}
	if isFalse(tenant.Status.Conditions, corev1alpha1.TenantNamespacesReady) {
		return sharedv1alpha1.PhaseProvisioning
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic from
// design §6.5: NamespacesReady ∧ ConnectorsReady.
func readyFromConditions(tenant *corev1alpha1.Tenant) (status, reason, message string) {
	conds := tenant.Status.Conditions
	gates := []string{
		corev1alpha1.TenantNamespacesReady,
		corev1alpha1.TenantConnectorsReady,
	}
	for _, t := range gates {
		if !isTrue(conds, t) {
			return string(metav1.ConditionFalse), ReasonNotReady, t + " not satisfied"
		}
	}
	return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
}

// isTrue reports whether the named condition is present and True.
func isTrue(conds []metav1.Condition, t string) bool {
	for i := range conds {
		if conds[i].Type == t {
			return conds[i].Status == metav1.ConditionTrue
		}
	}
	return false
}

// isFalse reports whether the named condition is present and False.
func isFalse(conds []metav1.Condition, t string) bool {
	for i := range conds {
		if conds[i].Type == t {
			return conds[i].Status == metav1.ConditionFalse
		}
	}
	return false
}
