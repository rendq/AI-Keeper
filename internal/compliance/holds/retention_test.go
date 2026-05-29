package holds

import (
	"testing"
	"time"
)

func newRetentionGC(store HoldStore, now time.Time) *RetentionGC {
	gc := NewRetentionGC(store)
	gc.now = func() time.Time { return now }
	return gc
}

func TestRetentionActiveHoldBlocksDeletion(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Create an active hold for tenant-a.
	hold := &Hold{
		ID:        "hold-1",
		Reason:    "legal",
		AppliedBy: "admin",
		AppliedAt: now.Add(-48 * time.Hour),
		Scope:     HoldScope{Tenants: []string{"tenant-a"}},
		Status:    StatusActive,
	}
	if err := store.Create(hold); err != nil {
		t.Fatal(err)
	}

	gc := newRetentionGC(store, now)
	candidate := GCCandidate{
		ObjectKey: "obj-1",
		Tenant:    "tenant-a",
		CreatedAt: now.Add(-30 * 24 * time.Hour), // 30 days old
	}
	policy := RetentionPolicy{RetentionDays: 7, Tenant: "tenant-a"}

	canDelete, reason := gc.ShouldDelete(candidate, policy)
	if canDelete {
		t.Fatal("expected deletion to be blocked by active hold")
	}
	if reason != "held" {
		t.Fatalf("expected reason 'held', got %q", reason)
	}
}

func TestRetentionReleasedHoldAllowsDeletion(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Create a released hold — should not block GC.
	hold := &Hold{
		ID:        "hold-2",
		Reason:    "legal",
		AppliedBy: "admin",
		AppliedAt: now.Add(-48 * time.Hour),
		Scope:     HoldScope{Tenants: []string{"tenant-a"}},
		Status:    StatusReleased,
	}
	if err := store.Create(hold); err != nil {
		t.Fatal(err)
	}

	gc := newRetentionGC(store, now)
	candidate := GCCandidate{
		ObjectKey: "obj-2",
		Tenant:    "tenant-a",
		CreatedAt: now.Add(-30 * 24 * time.Hour),
	}
	policy := RetentionPolicy{RetentionDays: 7, Tenant: "tenant-a"}

	canDelete, reason := gc.ShouldDelete(candidate, policy)
	if !canDelete {
		t.Fatalf("expected deletion to proceed after hold released, reason: %s", reason)
	}
}

func TestRetentionNoHoldAllowsNormalGC(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	gc := newRetentionGC(store, now)
	candidate := GCCandidate{
		ObjectKey: "obj-3",
		Tenant:    "tenant-b",
		CreatedAt: now.Add(-30 * 24 * time.Hour),
	}
	policy := RetentionPolicy{RetentionDays: 7, Tenant: "tenant-b"}

	canDelete, reason := gc.ShouldDelete(candidate, policy)
	if !canDelete {
		t.Fatalf("expected normal GC when no hold exists, reason: %s", reason)
	}
	if reason != "" {
		t.Fatalf("expected empty reason, got %q", reason)
	}
}

func TestRetentionHoldScopeDoesNotMatch(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Hold covers tenant-a only.
	hold := &Hold{
		ID:        "hold-3",
		Reason:    "legal",
		AppliedBy: "admin",
		AppliedAt: now.Add(-48 * time.Hour),
		Scope:     HoldScope{Tenants: []string{"tenant-a"}},
		Status:    StatusActive,
	}
	if err := store.Create(hold); err != nil {
		t.Fatal(err)
	}

	gc := newRetentionGC(store, now)
	// Candidate is for tenant-b — hold scope doesn't match.
	candidate := GCCandidate{
		ObjectKey: "obj-4",
		Tenant:    "tenant-b",
		CreatedAt: now.Add(-30 * 24 * time.Hour),
	}
	policy := RetentionPolicy{RetentionDays: 7, Tenant: "tenant-b"}

	canDelete, reason := gc.ShouldDelete(candidate, policy)
	if !canDelete {
		t.Fatalf("expected GC to proceed when hold scope doesn't match, reason: %s", reason)
	}
}

func TestRetentionExpiredWithActiveHoldReportsHeld(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	hold := &Hold{
		ID:        "hold-4",
		Reason:    "investigation",
		AppliedBy: "compliance-officer",
		AppliedAt: now.Add(-10 * 24 * time.Hour),
		Scope:     HoldScope{Tenants: []string{"tenant-c"}},
		Status:    StatusActive,
	}
	if err := store.Create(hold); err != nil {
		t.Fatal(err)
	}

	gc := newRetentionGC(store, now)
	// Object retention has expired (created 60 days ago, policy is 30 days).
	candidate := GCCandidate{
		ObjectKey:  "obj-5",
		Tenant:     "tenant-c",
		CreatedAt:  now.Add(-60 * 24 * time.Hour),
		HoldStatus: HoldStatusHeld,
	}
	policy := RetentionPolicy{RetentionDays: 30, Tenant: "tenant-c"}

	canDelete, reason := gc.ShouldDelete(candidate, policy)
	if canDelete {
		t.Fatal("expected deletion blocked despite expired retention")
	}
	if reason != "held" {
		t.Fatalf("expected 'held' status, got %q", reason)
	}

	// Verify batch processing marks it correctly.
	results := gc.ProcessBatch([]GCCandidate{candidate}, policy)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Deleted {
		t.Fatal("expected Deleted=false in batch result")
	}
	if results[0].Reason != "held" {
		t.Fatalf("expected reason 'held' in batch, got %q", results[0].Reason)
	}
}
