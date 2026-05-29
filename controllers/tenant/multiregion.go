package tenant

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// RegionConfig describes a target region for multi-region deployment.
type RegionConfig struct {
	// Name is the canonical region identifier (e.g. "cn-north", "eu-west").
	Name string

	// Endpoint is the API endpoint for the region's control plane.
	Endpoint string

	// Zones lists the availability zones within the region.
	Zones []string
}

// RegionStatus captures the reconciliation state of a single region.
type RegionStatus struct {
	// Region is the name matching RegionConfig.Name.
	Region string

	// Phase is the convergence phase for this region (e.g. "Provisioning", "Ready", "Failed").
	Phase string

	// Namespace is the provisioned namespace for the tenant in this region.
	Namespace string

	// ReadyAt records when the region first reached the Ready phase.
	ReadyAt time.Time
}

// Multi-region phase constants.
const (
	RegionPhaseProvisioning = "Provisioning"
	RegionPhaseReady        = "Ready"
	RegionPhaseFailed       = "Failed"
)

// MultiRegionReconciler handles multi-region namespace provisioning
// and cross-border validation for a Tenant. It extends the base
// TenantReconciler without replacing it — the base reconciler handles
// the primary namespace; this reconciler handles additional regions.
type MultiRegionReconciler struct {
	Client client.Client
}

// ReconcileRegions provisions a namespace and applies default quotas
// in each specified region. Each region converges independently — a
// failure in one region does not block others.
//
// The namespace follows the pattern: <tenant-name>-<region>.
func (m *MultiRegionReconciler) ReconcileRegions(ctx context.Context, tenant *corev1alpha1.Tenant, regions []RegionConfig) ([]RegionStatus, error) {
	if len(regions) == 0 {
		return nil, nil
	}

	statuses := make([]RegionStatus, 0, len(regions))

	for _, region := range regions {
		rs := RegionStatus{
			Region: region.Name,
			Phase:  RegionPhaseProvisioning,
		}

		nsName := regionNamespace(tenant.Name, region.Name)
		rs.Namespace = nsName

		// Provision namespace for this region.
		if err := m.ensureRegionNamespace(ctx, tenant, nsName, region.Name); err != nil {
			rs.Phase = RegionPhaseFailed
			statuses = append(statuses, rs)
			continue
		}

		// Apply default quota in the region namespace.
		if err := m.ensureRegionQuota(ctx, tenant, nsName); err != nil {
			rs.Phase = RegionPhaseFailed
			statuses = append(statuses, rs)
			continue
		}

		rs.Phase = RegionPhaseReady
		rs.ReadyAt = time.Now()
		statuses = append(statuses, rs)
	}

	return statuses, nil
}

// ValidateCrossBorder checks whether a cross-region call is allowed
// for the given tenant. When ForbidCrossBorder is true and the source
// and target regions differ, it returns an error.
func (m *MultiRegionReconciler) ValidateCrossBorder(tenant *corev1alpha1.Tenant, sourceRegion, targetRegion string) error {
	if tenant == nil {
		return fmt.Errorf("tenant must not be nil")
	}

	forbid := isCrossBorderForbidden(tenant)

	if forbid && sourceRegion != targetRegion {
		return fmt.Errorf("cross-border call from %q to %q is forbidden for tenant %q",
			sourceRegion, targetRegion, tenant.Name)
	}

	return nil
}

// isCrossBorderForbidden returns true when the tenant's compliance
// profile forbids cross-border data flow. Defaults to true when the
// field is nil (safe default per C2.2).
func isCrossBorderForbidden(tenant *corev1alpha1.Tenant) bool {
	if tenant.Spec.ComplianceProfile.DataResidency == nil {
		return true
	}
	if tenant.Spec.ComplianceProfile.DataResidency.ForbidCrossBorder == nil {
		return true
	}
	return *tenant.Spec.ComplianceProfile.DataResidency.ForbidCrossBorder
}

// regionNamespace returns the namespace name for a tenant in a given region.
func regionNamespace(tenantName, region string) string {
	return tenantName + "-" + region
}

// ensureRegionNamespace creates or updates the namespace for a tenant
// in a specific region, labeling it with tenant and region metadata.
func (m *MultiRegionReconciler) ensureRegionNamespace(ctx context.Context, tenant *corev1alpha1.Tenant, nsName, region string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, m.Client, ns, func() error {
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		ns.Labels[LabelTenant] = tenant.Name
		ns.Labels[LabelManagedBy] = ManagerName
		ns.Labels["ai-keeper.io/region"] = region
		return nil
	})
	return err
}

// ensureRegionQuota creates a default quota in the region namespace.
func (m *MultiRegionReconciler) ensureRegionQuota(ctx context.Context, tenant *corev1alpha1.Tenant, nsName string) error {
	quota := &policyv1alpha1.Quota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultQuotaName,
			Namespace: nsName,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, m.Client, quota, func() error {
		if quota.Labels == nil {
			quota.Labels = map[string]string{}
		}
		quota.Labels[LabelTenant] = tenant.Name
		quota.Labels[LabelManagedBy] = ManagerName

		quota.Spec.Scope = policyv1alpha1.QuotaScope{
			Kind: "Tenant",
			Name: tenant.Name,
		}
		if len(quota.Spec.Limits) == 0 {
			quota.Spec.Limits = cloneIntOrStringMap(DefaultQuotaLimits)
		}
		return nil
	})
	return err
}
