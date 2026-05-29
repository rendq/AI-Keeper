package holds

import (
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
}

func newTestService() *HoldService {
	svc := NewHoldService(NewMemoryStore())
	svc.now = fixedNow
	return svc
}

func validScope() HoldScope {
	return HoldScope{
		Tenants: []string{"tenant-a"},
		TimeRange: TimeRange{
			Start: fixedNow().Add(-24 * time.Hour),
			End:   fixedNow().Add(24 * time.Hour),
		},
	}
}

func TestApplyHold(t *testing.T) {
	svc := newTestService()
	expires := fixedNow().Add(72 * time.Hour)

	h, err := svc.ApplyHold("legal-investigation", "admin@corp.com", validScope(), &expires)
	if err != nil {
		t.Fatalf("ApplyHold failed: %v", err)
	}
	if h.ID == "" {
		t.Error("expected non-empty ID")
	}
	if h.Status != StatusActive {
		t.Errorf("expected status %q, got %q", StatusActive, h.Status)
	}
	if h.Reason != "legal-investigation" {
		t.Errorf("expected reason %q, got %q", "legal-investigation", h.Reason)
	}
	if h.AppliedBy != "admin@corp.com" {
		t.Errorf("expected appliedBy %q, got %q", "admin@corp.com", h.AppliedBy)
	}
	if !h.AppliedAt.Equal(fixedNow()) {
		t.Errorf("expected appliedAt %v, got %v", fixedNow(), h.AppliedAt)
	}
}

func TestGetHold(t *testing.T) {
	svc := newTestService()
	h, _ := svc.ApplyHold("audit", "officer@corp.com", validScope(), nil)

	got, err := svc.GetHold(h.ID)
	if err != nil {
		t.Fatalf("GetHold failed: %v", err)
	}
	if got.ID != h.ID {
		t.Errorf("expected ID %q, got %q", h.ID, got.ID)
	}
	if got.Reason != "audit" {
		t.Errorf("expected reason %q, got %q", "audit", got.Reason)
	}
}

func TestGetHoldNotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.GetHold("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListHolds(t *testing.T) {
	svc := newTestService()

	// Create holds for different tenants.
	scopeA := HoldScope{Tenants: []string{"tenant-a"}, Query: "q1"}
	scopeB := HoldScope{Tenants: []string{"tenant-b"}, Query: "q2"}

	svc.ApplyHold("reason-a", "user-a", scopeA, nil)
	svc.ApplyHold("reason-b", "user-b", scopeB, nil)

	// List all.
	all, err := svc.ListHolds(ListFilter{})
	if err != nil {
		t.Fatalf("ListHolds failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 holds, got %d", len(all))
	}

	// Filter by tenant.
	tenantB := "tenant-b"
	filtered, err := svc.ListHolds(ListFilter{Tenant: &tenantB})
	if err != nil {
		t.Fatalf("ListHolds with filter failed: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 hold for tenant-b, got %d", len(filtered))
	}

	// Filter by status.
	released := StatusReleased
	empty, err := svc.ListHolds(ListFilter{Status: &released})
	if err != nil {
		t.Fatalf("ListHolds with status filter failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 released holds, got %d", len(empty))
	}
}

func TestReleaseHold(t *testing.T) {
	svc := newTestService()
	h, _ := svc.ApplyHold("legal", "admin@corp.com", validScope(), nil)

	released, err := svc.ReleaseHold(h.ID)
	if err != nil {
		t.Fatalf("ReleaseHold failed: %v", err)
	}
	if released.Status != StatusPendingRelease {
		t.Errorf("expected status %q, got %q", StatusPendingRelease, released.Status)
	}

	// Verify persisted.
	got, _ := svc.GetHold(h.ID)
	if got.Status != StatusPendingRelease {
		t.Errorf("expected persisted status %q, got %q", StatusPendingRelease, got.Status)
	}
}

func TestReleaseHoldNotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.ReleaseHold("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestValidation_MissingReason(t *testing.T) {
	svc := newTestService()
	_, err := svc.ApplyHold("", "admin@corp.com", validScope(), nil)
	if err != ErrMissingReason {
		t.Errorf("expected ErrMissingReason, got %v", err)
	}
}

func TestValidation_MissingApplier(t *testing.T) {
	svc := newTestService()
	_, err := svc.ApplyHold("reason", "", validScope(), nil)
	if err != ErrMissingApplier {
		t.Errorf("expected ErrMissingApplier, got %v", err)
	}
}

func TestValidation_MissingScope(t *testing.T) {
	svc := newTestService()
	_, err := svc.ApplyHold("reason", "admin", HoldScope{}, nil)
	if err != ErrMissingScope {
		t.Errorf("expected ErrMissingScope, got %v", err)
	}
}

func TestValidation_ExpiredHold(t *testing.T) {
	svc := newTestService()
	past := fixedNow().Add(-1 * time.Hour)
	_, err := svc.ApplyHold("reason", "admin", validScope(), &past)
	if err != ErrExpired {
		t.Errorf("expected ErrExpired, got %v", err)
	}
}

func TestOverlapDetection(t *testing.T) {
	svc := newTestService()
	scope := HoldScope{
		Tenants: []string{"tenant-x"},
		TimeRange: TimeRange{
			Start: fixedNow(),
			End:   fixedNow().Add(48 * time.Hour),
		},
	}

	_, err := svc.ApplyHold("first", "admin", scope, nil)
	if err != nil {
		t.Fatalf("first ApplyHold failed: %v", err)
	}

	// Same scope should fail.
	_, err = svc.ApplyHold("second", "admin", scope, nil)
	if err != ErrOverlap {
		t.Errorf("expected ErrOverlap, got %v", err)
	}

	// Non-overlapping time range should succeed.
	nonOverlapping := HoldScope{
		Tenants: []string{"tenant-x"},
		TimeRange: TimeRange{
			Start: fixedNow().Add(49 * time.Hour),
			End:   fixedNow().Add(72 * time.Hour),
		},
	}
	_, err = svc.ApplyHold("third", "admin", nonOverlapping, nil)
	if err != nil {
		t.Errorf("non-overlapping hold should succeed, got %v", err)
	}

	// Different tenant should succeed.
	diffTenant := HoldScope{
		Tenants: []string{"tenant-y"},
		TimeRange: TimeRange{
			Start: fixedNow(),
			End:   fixedNow().Add(48 * time.Hour),
		},
	}
	_, err = svc.ApplyHold("fourth", "admin", diffTenant, nil)
	if err != nil {
		t.Errorf("different tenant hold should succeed, got %v", err)
	}
}
