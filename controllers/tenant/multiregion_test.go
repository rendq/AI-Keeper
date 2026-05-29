package tenant_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/tenant"
)

// ---------------------------------------------------------------------------
// Multi-Region Tests — Validates: Requirements C2.1, C2.2
// ---------------------------------------------------------------------------

func TestMultiRegion_CreatesNamespacesForEachRegion(t *testing.T) {
	t.Parallel()

	tn := newTenant("acme")
	c, _ := newFakeClient(t, tn)
	reconciler := &tenant.MultiRegionReconciler{Client: c}

	regions := []tenant.RegionConfig{
		{Name: "cn-north", Endpoint: "https://cn-north.example.com", Zones: []string{"cn-north-1a", "cn-north-1b"}},
		{Name: "eu-west", Endpoint: "https://eu-west.example.com", Zones: []string{"eu-west-1a"}},
	}

	statuses, err := reconciler.ReconcileRegions(context.Background(), tn, regions)
	if err != nil {
		t.Fatalf("ReconcileRegions: %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("expected 2 region statuses, got %d", len(statuses))
	}

	// Verify namespaces were created with correct names.
	for _, region := range regions {
		wantNS := tn.Name + "-" + region.Name
		ns := &corev1.Namespace{}
		if err := c.Get(context.Background(), types.NamespacedName{Name: wantNS}, ns); err != nil {
			t.Fatalf("namespace %q not created: %v", wantNS, err)
		}
		if ns.Labels[tenant.LabelTenant] != tn.Name {
			t.Errorf("namespace %q: label %s = %q, want %q",
				wantNS, tenant.LabelTenant, ns.Labels[tenant.LabelTenant], tn.Name)
		}
		if ns.Labels["ai-keeper.io/region"] != region.Name {
			t.Errorf("namespace %q: label ai-keeper.io/region = %q, want %q",
				wantNS, ns.Labels["ai-keeper.io/region"], region.Name)
		}
	}

	// Verify quotas were created in each namespace.
	for _, region := range regions {
		wantNS := tn.Name + "-" + region.Name
		quota := &policyv1alpha1.Quota{}
		qKey := types.NamespacedName{Name: tenant.DefaultQuotaName, Namespace: wantNS}
		if err := c.Get(context.Background(), qKey, quota); err != nil {
			t.Fatalf("quota not created in namespace %q: %v", wantNS, err)
		}
		if quota.Spec.Scope.Kind != "Tenant" || quota.Spec.Scope.Name != tn.Name {
			t.Errorf("quota scope = %+v, want {Tenant, %s}", quota.Spec.Scope, tn.Name)
		}
	}
}

func TestMultiRegion_RegionStatusIndependentlyTracked(t *testing.T) {
	t.Parallel()

	tn := newTenant("acme")
	c, _ := newFakeClient(t, tn)
	reconciler := &tenant.MultiRegionReconciler{Client: c}

	regions := []tenant.RegionConfig{
		{Name: "us-east", Endpoint: "https://us-east.example.com"},
		{Name: "ap-south", Endpoint: "https://ap-south.example.com"},
		{Name: "eu-central", Endpoint: "https://eu-central.example.com"},
	}

	statuses, err := reconciler.ReconcileRegions(context.Background(), tn, regions)
	if err != nil {
		t.Fatalf("ReconcileRegions: %v", err)
	}

	if len(statuses) != 3 {
		t.Fatalf("expected 3 region statuses, got %d", len(statuses))
	}

	// Each status should have the correct region name and be independently set.
	seen := map[string]bool{}
	for _, s := range statuses {
		if seen[s.Region] {
			t.Fatalf("duplicate region status for %q", s.Region)
		}
		seen[s.Region] = true

		if s.Phase != "Ready" {
			t.Errorf("region %q: phase = %q, want Ready", s.Region, s.Phase)
		}
		if s.Namespace == "" {
			t.Errorf("region %q: namespace is empty", s.Region)
		}
		if s.ReadyAt.IsZero() {
			t.Errorf("region %q: ReadyAt is zero", s.Region)
		}
	}
}

func TestMultiRegion_CrossBorderRejectsWhenForbidden(t *testing.T) {
	t.Parallel()

	forbid := true
	tn := &corev1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "acme"},
		Spec: corev1alpha1.TenantSpec{
			DisplayName: "Acme",
			ComplianceProfile: corev1alpha1.TenantComplianceProfile{
				Tier: "regulated",
				DataResidency: &corev1alpha1.TenantDataResidency{
					PrimaryRegion:     "cn-north",
					ForbidCrossBorder: &forbid,
				},
			},
		},
	}

	reconciler := &tenant.MultiRegionReconciler{}

	err := reconciler.ValidateCrossBorder(tn, "cn-north", "eu-west")
	if err == nil {
		t.Fatal("expected error for cross-border call with forbidCrossBorder=true, got nil")
	}

	// Same-region should be allowed even when forbid is true.
	err = reconciler.ValidateCrossBorder(tn, "cn-north", "cn-north")
	if err != nil {
		t.Fatalf("same-region call should be allowed: %v", err)
	}
}

func TestMultiRegion_CrossBorderAllowsWhenNotForbidden(t *testing.T) {
	t.Parallel()

	forbid := false
	tn := &corev1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "acme"},
		Spec: corev1alpha1.TenantSpec{
			DisplayName: "Acme",
			ComplianceProfile: corev1alpha1.TenantComplianceProfile{
				Tier: "basic",
				DataResidency: &corev1alpha1.TenantDataResidency{
					PrimaryRegion:     "us-east",
					ForbidCrossBorder: &forbid,
				},
			},
		},
	}

	reconciler := &tenant.MultiRegionReconciler{}

	err := reconciler.ValidateCrossBorder(tn, "us-east", "eu-west")
	if err != nil {
		t.Fatalf("cross-border call should be allowed when forbidCrossBorder=false: %v", err)
	}
}

func TestMultiRegion_SingleRegionWorksNormally(t *testing.T) {
	t.Parallel()

	tn := newTenant("solo-tenant")
	c, _ := newFakeClient(t, tn)
	reconciler := &tenant.MultiRegionReconciler{Client: c}

	regions := []tenant.RegionConfig{
		{Name: "us-east", Endpoint: "https://us-east.example.com", Zones: []string{"us-east-1a"}},
	}

	statuses, err := reconciler.ReconcileRegions(context.Background(), tn, regions)
	if err != nil {
		t.Fatalf("ReconcileRegions: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 region status, got %d", len(statuses))
	}

	s := statuses[0]
	if s.Region != "us-east" {
		t.Errorf("region = %q, want us-east", s.Region)
	}
	if s.Phase != "Ready" {
		t.Errorf("phase = %q, want Ready", s.Phase)
	}
	if s.Namespace != "solo-tenant-us-east" {
		t.Errorf("namespace = %q, want solo-tenant-us-east", s.Namespace)
	}

	// Verify the namespace exists.
	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "solo-tenant-us-east"}, ns); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
}

func TestMultiRegion_EmptyRegionsReturnsNil(t *testing.T) {
	t.Parallel()

	tn := newTenant("empty")
	c, _ := newFakeClient(t, tn)
	reconciler := &tenant.MultiRegionReconciler{Client: c}

	statuses, err := reconciler.ReconcileRegions(context.Background(), tn, nil)
	if err != nil {
		t.Fatalf("ReconcileRegions with nil regions: %v", err)
	}
	if statuses != nil {
		t.Fatalf("expected nil statuses for empty regions, got %v", statuses)
	}
}
