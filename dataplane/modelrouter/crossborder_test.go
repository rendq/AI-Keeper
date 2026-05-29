package modelrouter

import "testing"

func TestCrossBorder_SameRegion_Allowed(t *testing.T) {
	allowed, reason := CheckCrossBorder("us-east-1", "us-east-1", true)
	if !allowed {
		t.Fatalf("expected allowed for same region, got rejected: %s", reason)
	}
	if reason != "" {
		t.Fatalf("expected empty reason, got: %s", reason)
	}
}

func TestCrossBorder_DifferentRegion_ForbidTrue_Rejected(t *testing.T) {
	allowed, reason := CheckCrossBorder("us-east-1", "eu-west-1", true)
	if allowed {
		t.Fatal("expected rejection for cross-border with forbid=true")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason for rejection")
	}
}

func TestCrossBorder_DifferentRegion_ForbidFalse_Allowed(t *testing.T) {
	allowed, reason := CheckCrossBorder("us-east-1", "eu-west-1", false)
	if !allowed {
		t.Fatalf("expected allowed when forbid=false, got rejected: %s", reason)
	}
	if reason != "" {
		t.Fatalf("expected empty reason, got: %s", reason)
	}
}

func TestCrossBorder_EmptyRegion_Allowed(t *testing.T) {
	// Empty tenant region
	allowed, reason := CheckCrossBorder("", "eu-west-1", true)
	if !allowed {
		t.Fatalf("expected allowed for empty tenant region, got rejected: %s", reason)
	}

	// Empty endpoint region
	allowed, reason = CheckCrossBorder("us-east-1", "", true)
	if !allowed {
		t.Fatalf("expected allowed for empty endpoint region, got rejected: %s", reason)
	}

	// Both empty
	allowed, reason = CheckCrossBorder("", "", true)
	if !allowed {
		t.Fatalf("expected allowed for both empty regions, got rejected: %s", reason)
	}
	_ = reason
}
