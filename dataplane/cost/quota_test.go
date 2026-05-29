package cost

import "testing"

func TestQuota_AllowsBelowLimit(t *testing.T) {
	qs := NewQuotaService([]QuotaConfig{
		{Scope: "tenant-a", ResourceType: "agents", Limit: 5},
	})

	allowed, reason := qs.CheckAdmission("agents", "tenant-a", 3)
	if !allowed {
		t.Fatalf("expected admission allowed below limit, got rejected: %s", reason)
	}
	if reason != "" {
		t.Fatalf("expected empty reason, got: %s", reason)
	}
}

func TestQuota_RejectsAtLimit(t *testing.T) {
	qs := NewQuotaService([]QuotaConfig{
		{Scope: "tenant-a", ResourceType: "agents", Limit: 5},
	})

	allowed, reason := qs.CheckAdmission("agents", "tenant-a", 5)
	if allowed {
		t.Fatal("expected admission rejected at limit, got allowed")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason when rejected")
	}

	// Also reject when over limit
	allowed, reason = qs.CheckAdmission("agents", "tenant-a", 7)
	if allowed {
		t.Fatal("expected admission rejected over limit, got allowed")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason when rejected")
	}
}

func TestQuota_DifferentScopesIndependent(t *testing.T) {
	qs := NewQuotaService([]QuotaConfig{
		{Scope: "tenant-a", ResourceType: "agents", Limit: 3},
		{Scope: "tenant-b", ResourceType: "agents", Limit: 3},
	})

	// tenant-a at limit
	allowed, _ := qs.CheckAdmission("agents", "tenant-a", 3)
	if allowed {
		t.Fatal("tenant-a should be rejected at limit")
	}

	// tenant-b still below limit
	allowed, _ = qs.CheckAdmission("agents", "tenant-b", 1)
	if !allowed {
		t.Fatal("tenant-b should be allowed below limit")
	}
}

func TestQuota_DifferentResourceTypesIndependent(t *testing.T) {
	qs := NewQuotaService([]QuotaConfig{
		{Scope: "tenant-a", ResourceType: "agents", Limit: 2},
		{Scope: "tenant-a", ResourceType: "skills", Limit: 10},
	})

	// agents at limit
	allowed, _ := qs.CheckAdmission("agents", "tenant-a", 2)
	if allowed {
		t.Fatal("agents should be rejected at limit")
	}

	// skills still below limit
	allowed, _ = qs.CheckAdmission("skills", "tenant-a", 5)
	if !allowed {
		t.Fatal("skills should be allowed below limit")
	}
}

func TestQuota_UpdateUsage(t *testing.T) {
	qs := NewQuotaService([]QuotaConfig{
		{Scope: "tenant-a", ResourceType: "agents", Limit: 5},
	})

	qs.UpdateUsage("agents", "tenant-a", 3)
	used, limit := qs.GetUsage("agents", "tenant-a")
	if used != 3 {
		t.Fatalf("expected used=3, got %d", used)
	}
	if limit != 5 {
		t.Fatalf("expected limit=5, got %d", limit)
	}

	// Update again
	qs.UpdateUsage("agents", "tenant-a", 5)
	used, limit = qs.GetUsage("agents", "tenant-a")
	if used != 5 {
		t.Fatalf("expected used=5, got %d", used)
	}
	if limit != 5 {
		t.Fatalf("expected limit=5, got %d", limit)
	}
}

func TestQuota_NoConfigMeansNoLimit(t *testing.T) {
	qs := NewQuotaService([]QuotaConfig{})

	// No config for any resource type — should always allow
	allowed, reason := qs.CheckAdmission("agents", "tenant-a", 1000)
	if !allowed {
		t.Fatalf("expected no limit when no config, got rejected: %s", reason)
	}

	// GetUsage returns -1 for limit when not configured
	used, limit := qs.GetUsage("agents", "tenant-a")
	if used != 0 {
		t.Fatalf("expected used=0, got %d", used)
	}
	if limit != -1 {
		t.Fatalf("expected limit=-1 for unconfigured, got %d", limit)
	}
}
