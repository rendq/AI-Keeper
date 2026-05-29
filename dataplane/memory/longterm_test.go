package memory

import (
	"context"
	"testing"
	"time"
)

// fakeEmbedFn returns a deterministic embedding based on content for testing.
func fakeEmbedFn(_ context.Context, text string) ([]float32, error) {
	// Simple hash-based embedding for deterministic test results.
	vec := make([]float32, 8)
	for i, ch := range text {
		vec[i%8] += float32(ch)
	}
	return vec, nil
}

func setupMemory() *LongTermMemory {
	store := NewInMemoryVectorStore()
	return NewLongTermMemory(store, fakeEmbedFn)
}

func TestStoreAndSearchSameIsolation(t *testing.T) {
	mem := setupMemory()
	ctx := context.Background()

	embedding, _ := fakeEmbedFn(ctx, "hello world")
	entry := MemoryEntry{
		ID:           "entry-1",
		Content:      "hello world",
		Embedding:    embedding,
		Metadata:     map[string]string{"source": "test"},
		Timestamp:    time.Now(),
		IsolationKey: "per_user:tenantA:user1:",
	}

	if err := mem.Store(ctx, entry); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	results, err := mem.Search(ctx, "hello world", "per_user:tenantA:user1:", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "entry-1" {
		t.Errorf("expected ID entry-1, got %s", results[0].ID)
	}
	if results[0].Content != "hello world" {
		t.Errorf("expected content 'hello world', got %s", results[0].Content)
	}
	if results[0].Metadata["source"] != "test" {
		t.Errorf("expected metadata source=test, got %s", results[0].Metadata["source"])
	}
}

func TestSearchDoesNotReturnDifferentIsolation(t *testing.T) {
	mem := setupMemory()
	ctx := context.Background()

	embedding, _ := fakeEmbedFn(ctx, "secret data")
	entry := MemoryEntry{
		ID:           "entry-secret",
		Content:      "secret data",
		Embedding:    embedding,
		Metadata:     map[string]string{},
		Timestamp:    time.Now(),
		IsolationKey: "per_user:tenantA:user1:",
	}
	if err := mem.Store(ctx, entry); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Search with a different isolation key should return nothing.
	results, err := mem.Search(ctx, "secret data", "per_user:tenantA:user2:", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results from different isolation, got %d", len(results))
	}
}

func TestIsolationKeyBuilder(t *testing.T) {
	builder := IsolationKeyBuilder{
		Tenant:  "acme",
		User:    "alice",
		Session: "sess-123",
	}

	tests := []struct {
		level    IsolationLevel
		expected string
	}{
		{IsolationShared, "shared:acme::"},
		{IsolationPerTenant, "per_tenant:acme::"},
		{IsolationPerUser, "per_user:acme:alice:"},
		{IsolationPerSession, "per_session:acme:alice:sess-123"},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			got := builder.Build(tt.level)
			if got != tt.expected {
				t.Errorf("Build(%s) = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestDeleteRemovesEntry(t *testing.T) {
	mem := setupMemory()
	ctx := context.Background()

	embedding, _ := fakeEmbedFn(ctx, "to delete")
	entry := MemoryEntry{
		ID:           "entry-del",
		Content:      "to delete",
		Embedding:    embedding,
		Metadata:     map[string]string{},
		Timestamp:    time.Now(),
		IsolationKey: "per_tenant:tenantB::",
	}
	if err := mem.Store(ctx, entry); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if err := mem.Delete(ctx, "entry-del"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	results, err := mem.Search(ctx, "to delete", "per_tenant:tenantB::", 10)
	if err != nil {
		t.Fatalf("Search after delete failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after delete, got %d", len(results))
	}
}

func TestEmptySearchReturnsEmpty(t *testing.T) {
	mem := setupMemory()
	ctx := context.Background()

	results, err := mem.Search(ctx, "anything", "per_user:tenantX:userX:", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results on empty store, got %d", len(results))
	}
}
