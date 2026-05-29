package tenant

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the Tenant CR.
const (
	// EventReasonNamespaceProvisioned is published when the per-tenant
	// namespace is created for the first time.
	EventReasonNamespaceProvisioned = "NamespaceProvisioned"
	// EventReasonTenantReady is published when the aggregate Ready
	// condition first flips True.
	EventReasonTenantReady = "TenantReady"
)

// TenantReconciler implements the Tenant state machine documented in
// design.md §6.5 and Requirement A7.1. The reconciler is namespace
// agnostic — Tenant is a cluster-scoped Kind — and provisions a
// dedicated `tenant-<name>` Namespace for every CR.
type TenantReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager. Used
	// to set OwnerReferences on the per-tenant Namespace and on the
	// seeded default Budget / Quota.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("tenant: nil reconciler")
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("tenant-controller").
		For(&corev1alpha1.Tenant{}).
		Owns(&corev1.Namespace{}).
		Owns(&policyv1alpha1.Budget{}).
		Owns(&policyv1alpha1.Quota{}).
		Complete(r)
}

// Reconcile runs one reconciliation pass for a Tenant CR.
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("tenant", req.Name)

	tenant := &corev1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("tenant: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path. Operators own the Namespace lifecycle; the
	//    finalizer just clears `status.namespaces` and lets the API
	//    server garbage-collect the CR.
	if !tenant.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, tenant)
	}

	// 2) Ensure the cleanup finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, tenant, FinalizerTenantCleanup); err != nil {
		return ctrl.Result{}, err
	} else if added {
		// Re-read on the next pass so the patched generation drives
		// the rest of the reconcile.
		return ctrl.Result{Requeue: true}, nil
	}

	// 3) Provision the per-tenant Namespace.
	nsName := namespaceFor(tenant)
	if nsName == "" {
		return ctrl.Result{}, fmt.Errorf("tenant: cannot derive namespace for %q", tenant.Name)
	}
	created, err := r.ensureNamespace(ctx, tenant, nsName)
	if err != nil {
		common.SetCondition(tenant, corev1alpha1.TenantNamespacesReady,
			string(metav1.ConditionFalse), ReasonNamespaceFailed, truncateErr(err))
		r.aggregate(tenant)
		return r.writeStatus(ctx, tenant, common.RequeueWithBackoff(0))
	}
	if created {
		r.eventf(tenant, corev1.EventTypeNormal, EventReasonNamespaceProvisioned,
			"Namespace %q created for tenant", nsName)
	}

	// 4) Seed default Budget (skip when spec.defaultBudget is nil).
	if err := r.ensureDefaultBudget(ctx, tenant, nsName); err != nil {
		common.SetCondition(tenant, corev1alpha1.TenantNamespacesReady,
			string(metav1.ConditionFalse), ReasonBudgetFailed, truncateErr(err))
		r.aggregate(tenant)
		return r.writeStatus(ctx, tenant, common.RequeueWithBackoff(0))
	}

	// 5) Seed default Quota with placeholder limits.
	if err := r.ensureDefaultQuota(ctx, tenant, nsName); err != nil {
		common.SetCondition(tenant, corev1alpha1.TenantNamespacesReady,
			string(metav1.ConditionFalse), ReasonQuotaFailed, truncateErr(err))
		r.aggregate(tenant)
		return r.writeStatus(ctx, tenant, common.RequeueWithBackoff(0))
	}

	// 6) Track the provisioned namespace on `status.namespaces`.
	if !containsString(tenant.Status.Namespaces, nsName) {
		tenant.Status.Namespaces = append(tenant.Status.Namespaces, nsName)
		sort.Strings(tenant.Status.Namespaces)
	}

	common.SetCondition(tenant, corev1alpha1.TenantNamespacesReady,
		string(metav1.ConditionTrue), ReasonProvisioned,
		fmt.Sprintf("namespace %q + default Budget/Quota provisioned", nsName))

	// 7) Connectors — defaulted True for P0.
	common.SetCondition(tenant, corev1alpha1.TenantConnectorsReady,
		string(metav1.ConditionTrue), ReasonConnectorsDeferred,
		"connector templates not yet wired in this build")

	// 8) Aggregate Ready + derive phase.
	wasReady := common.IsReady(tenant)
	r.aggregate(tenant)
	if !wasReady && common.IsReady(tenant) {
		r.eventf(tenant, corev1.EventTypeNormal, EventReasonTenantReady,
			"Tenant %q is Ready", tenant.Name)
	}

	return r.writeStatus(ctx, tenant, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete clears `status.namespaces` and removes the
// finalizer. The Namespace itself is intentionally left in place —
// operators delete it through `kubectl delete ns` after reviewing the
// dependent CRs.
func (r *TenantReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, tenant *corev1alpha1.Tenant) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(tenant, FinalizerTenantCleanup) {
		return ctrl.Result{}, nil
	}
	tenant.Status.Phase = sharedv1alpha1.PhaseTerminating
	if len(tenant.Status.Namespaces) > 0 {
		tenant.Status.Namespaces = nil
	}
	if err := r.Status().Update(ctx, tenant); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("tenant: status update on terminating: %w", err)
	}
	if _, err := common.RemoveFinalizer(ctx, r.Client, tenant, FinalizerTenantCleanup); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("tenant: finalizer removed", "tenant", tenant.Name)
	return ctrl.Result{}, nil
}

// ensureNamespace creates (or updates labels on) the per-tenant
// Namespace. Returns (created=true, nil) when the Namespace was newly
// created, (created=false, nil) when the Namespace already existed and
// labels were re-applied, and (false, err) on a hard failure.
func (r *TenantReconciler) ensureNamespace(ctx context.Context, tenant *corev1alpha1.Tenant, nsName string) (bool, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		ns.Labels[LabelTenant] = tenant.Name
		ns.Labels[LabelManagedBy] = ManagerName
		// Owner reference so kubectl tree / cluster GC reflects the
		// link, even though we leave actual deletion to operators.
		if r.Scheme != nil {
			if err := controllerutil.SetControllerReference(tenant, ns, r.Scheme); err != nil {
				// Already owned by a different controller — surface it
				// rather than silently overwrite.
				return fmt.Errorf("tenant: set owner ref on namespace %q: %w", nsName, err)
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return op == controllerutil.OperationResultCreated, nil
}

// ensureDefaultBudget seeds the `default` Budget into the tenant
// namespace from `spec.defaultBudget`. Skips silently when the field
// is nil so operators can opt out of the default at creation time.
func (r *TenantReconciler) ensureDefaultBudget(ctx context.Context, tenant *corev1alpha1.Tenant, nsName string) error {
	defaults := tenant.Spec.DefaultBudget
	if defaults == nil {
		return nil
	}

	hardCap := true
	budget := &policyv1alpha1.Budget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultBudgetName,
			Namespace: nsName,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, budget, func() error {
		if budget.Labels == nil {
			budget.Labels = map[string]string{}
		}
		budget.Labels[LabelTenant] = tenant.Name
		budget.Labels[LabelManagedBy] = ManagerName

		// Spec body — preserve operator overrides on existing fields.
		budget.Spec.Scope = policyv1alpha1.BudgetScope{
			Kind: "Tenant",
			Name: tenant.Name,
		}
		if budget.Spec.Period == "" {
			budget.Spec.Period = DefaultBudgetPeriod
		}
		budget.Spec.Limits.Usd = defaults.UsdPerMonth
		budget.Spec.Limits.Tokens = defaults.TokensPerMonth
		if budget.Spec.HardCap == nil {
			budget.Spec.HardCap = &hardCap
		}
		if r.Scheme != nil {
			if err := controllerutil.SetControllerReference(tenant, budget, r.Scheme); err != nil {
				return fmt.Errorf("tenant: set owner ref on default Budget: %w", err)
			}
		}
		return nil
	})
	return err
}

// ensureDefaultQuota seeds the `default` Quota into the tenant
// namespace using [DefaultQuotaLimits]. Production deployments
// override these via `aikctl tenant set-quota` once the tenant has
// been triaged.
func (r *TenantReconciler) ensureDefaultQuota(ctx context.Context, tenant *corev1alpha1.Tenant, nsName string) error {
	quota := &policyv1alpha1.Quota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultQuotaName,
			Namespace: nsName,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, quota, func() error {
		if quota.Labels == nil {
			quota.Labels = map[string]string{}
		}
		quota.Labels[LabelTenant] = tenant.Name
		quota.Labels[LabelManagedBy] = ManagerName

		quota.Spec.Scope = policyv1alpha1.QuotaScope{
			Kind: "Tenant",
			Name: tenant.Name,
		}
		// Only seed limits the first time so aikctl overrides survive
		// subsequent reconciles.
		if len(quota.Spec.Limits) == 0 {
			quota.Spec.Limits = cloneIntOrStringMap(DefaultQuotaLimits)
		}
		if r.Scheme != nil {
			if err := controllerutil.SetControllerReference(tenant, quota, r.Scheme); err != nil {
				return fmt.Errorf("tenant: set owner ref on default Quota: %w", err)
			}
		}
		return nil
	})
	return err
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *TenantReconciler) aggregate(tenant *corev1alpha1.Tenant) {
	status, reason, message := readyFromConditions(tenant)
	common.SetCondition(tenant, corev1alpha1.TenantReady, status, reason, message)
	tenant.Status.Phase = derivePhase(tenant)
	tenant.Status.ObservedGeneration = tenant.Generation
}

// writeStatus persists the in-memory status block to the API server.
// Conflicts are non-fatal — controller-runtime retries on the next
// reconcile.
func (r *TenantReconciler) writeStatus(ctx context.Context, tenant *corev1alpha1.Tenant, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, tenant); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("tenant: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *TenantReconciler) eventf(tenant *corev1alpha1.Tenant, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(tenant, eventType, reason, msg, args...)
}

// containsString returns true iff `s` contains `target`.
func containsString(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

// cloneIntOrStringMap returns a deep copy of the supplied limits map
// so the package-level [DefaultQuotaLimits] cannot be mutated through
// a CR.
func cloneIntOrStringMap(in map[string]intstr.IntOrString) map[string]intstr.IntOrString {
	out := make(map[string]intstr.IntOrString, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// truncateErr clips long messages so they fit in a Condition message
// field without exceeding K8s API server payload limits.
func truncateErr(err error) string {
	const max = 240
	s := err.Error()
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Compile-time interface assertions.
var (
	_ reconcile.Reconciler = (*TenantReconciler)(nil)
)
