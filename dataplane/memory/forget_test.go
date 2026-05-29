package memory

import (
	"context"
	"testing"
	"time"
)

func TestForget_SpecificIDs(t *testing.T) {
	store := NewInMemoryVectorStore()
	mem := NewLongTermMemory(store, fakeEmbedFn)
	ctx := context.Background()

	// Store 3 entries.
	for _, id := range []string{"m1", "m2", "m3"} {
		embedding, _ := fakeEmbedFn(ctx, id)
		err := mem.Store(ctx, MemoryEntry{
			ID:           id,
			Content:      id,
			Embedding:    embedding,
			Timestamp:    time.Now(),
			IsolationKey: "per_user:t:u1:",
		})
		if err != nil {
			t.Fatalf("Store %s: %v", id, err)
		}
	}

	// Forget only m1 and m3.
	result, err := mem.Forget(ctx, ForgetRequest{
		UserID:    "u1",
		MemoryIDs: []string{"m1", "m3"},
		Reason:    "GDPR erasure request",
	})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if result.DeletedCount != 2 {
		t.Errorf("expected DeletedCount=2, got %d", result.DeletedCount)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// m2 should still exist.
	entries, err := mem.ListByIsolation(ctx, "per_user:t:u1:", 100)
	if err != nil {
		t.Fatalf("ListByIsolation: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(entries))
	}
	if entries[0].ID != "m2" {
		t.Errorf("expected remaining entry m2, got %s", entries[0].ID)
	}
}

func TestForget_AllByIsolation(t *testing.T) {
	store := NewInMemoryVectorStore()
	mem := NewLongTermMemory(store, fakeEmbedFn)
	ctx := context.Background()

	// Store entries under two isolation keys.
	for _, id := range []string{"a1", "a2"} {
		embedding, _ := fakeEmbedFn(ctx, id)
		_ = mem.Store(ctx, MemoryEntry{
			ID:           id,
			Content:      id,
			Embedding:    embedding,
			Timestamp:    time.Now(),
			IsolationKey: "per_user:t:userA:",
		})
	}
	embedding, _ := fakeEmbedFn(ctx, "b1")
	_ = mem.Store(ctx, MemoryEntry{
		ID:           "b1",
		Content:      "b1",
		Embedding:    embedding,
		Timestamp:    time.Now(),
		IsolationKey: "per_user:t:userB:",
	})

	// Forget all for userA's isolation key.
	result, err := mem.Forget(ctx, ForgetRequest{
		UserID:       "userA",
		IsolationKey: "per_user:t:userA:",
		Reason:       "user account deletion",
	})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if result.DeletedCount != 2 {
		t.Errorf("expected DeletedCount=2, got %d", result.DeletedCount)
	}

	// userA entries should be gone.
	entries, _ := mem.ListByIsolation(ctx, "per_user:t:userA:", 100)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for userA, got %d", len(entries))
	}

	// userB entry should remain.
	entries, _ = mem.ListByIsolation(ctx, "per_user:t:userB:", 100)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry for userB, got %d", len(entries))
	}
}

func TestForget_EmptyRequest(t *testing.T) {
	store := NewInMemoryVectorStore()
	mem := NewLongTermMemory(store, fakeEmbedFn)
	ctx := context.Background()

	result, err := mem.Forget(ctx, ForgetRequest{})
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if result.DeletedCount != 0 {
		t.Errorf("expected DeletedCount=0, got %d", result.DeletedCount)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
}

func TestRetentionEnforcer_DeletesExpired(t *testing.T) {
	store := NewInMemoryVectorStore()
	mem := NewLongTermMemory(store, fakeEmbedFn)
	ctx := context.Background()

	// Store an entry with a timestamp 48 hours ago (expired).
	embedding, _ := fakeEmbedFn(ctx, "old")
	_ = mem.Store(ctx, MemoryEntry{
		ID:           "old-entry",
		Content:      "old",
		Embedding:    embedding,
		Timestamp:    time.Now().Add(-48 * time.Hour),
		IsolationKey: "per_user:t:u1:",
	})

	// Store a recent entry.
	embedding, _ = fakeEmbedFn(ctx, "new")
	_ = mem.Store(ctx, MemoryEntry{
		ID:           "new-entry",
		Content:      "new",
		Embedding:    embedding,
		Timestamp:    time.Now(),
		IsolationKey: "per_user:t:u1:",
	})

	enforcer := NewRetentionEnforcer(store, RetentionPolicy{
		MaxAge:              24 * time.Hour,
		EnforcementInterval: time.Hour,
	})

	deleted, err := enforcer.Enforce(ctx)
	if err != nil {
		t.Fatalf("Enforce: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Only the new entry should remain.
	entries, _ := mem.ListByIsolation(ctx, "per_user:t:u1:", 100)
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(entries))
	}
	if entries[0].ID != "new-entry" {
		t.Errorf("expected new-entry to remain, got %s", entries[0].ID)
	}
}

func TestRetentionEnforcer_KeepsRecent(t *testing.T) {
	store := NewInMemoryVectorStore()
	mem := NewLongTermMemory(store, fakeEmbedFn)
	ctx := context.Background()

	// Store entries that are all within retention period.
	for _, id := range []string{"r1", "r2", "r3"} {
		embedding, _ := fakeEmbedFn(ctx, id)
		_ = mem.Store(ctx, MemoryEntry{
			ID:           id,
			Content:      id,
			Embedding:    embedding,
			Timestamp:    time.Now().Add(-1 * time.Hour), // 1 hour ago, well within 24h
			IsolationKey: "per_user:t:u1:",
		})
	}

	enforcer := NewRetentionEnforcer(store, RetentionPolicy{
		MaxAge:              24 * time.Hour,
		EnforcementInterval: time.Hour,
	})

	deleted, err := enforcer.Enforce(ctx)
	if err != nil {
		t.Fatalf("Enforce: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}

	// All entries should remain.
	entries, _ := mem.ListByIsolation(ctx, "per_user:t:u1:", 100)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries to remain, got %d", len(entries))
	}
}
