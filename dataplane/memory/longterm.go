// Package memory provides long-term vector memory with multi-level isolation.
package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// IsolationLevel defines the scope of memory isolation.
type IsolationLevel string

const (
	IsolationPerUser    IsolationLevel = "per_user"
	IsolationPerSession IsolationLevel = "per_session"
	IsolationPerTenant  IsolationLevel = "per_tenant"
	IsolationShared     IsolationLevel = "shared"
)

// MemoryEntry represents a single long-term memory record stored in a vector store.
type MemoryEntry struct {
	ID           string
	Content      string
	Embedding    []float32
	Metadata     map[string]string
	Timestamp    time.Time
	IsolationKey string
}

// VectorSearchResult is returned by the VectorStore Query method.
type VectorSearchResult struct {
	ID       string
	Score    float32
	Metadata map[string]string
}

// VectorStore is the interface for vector database operations.
// Implementations may target Qdrant, Pinecone, or an in-memory stub.
type VectorStore interface {
	// Upsert inserts or updates a vector entry.
	Upsert(ctx context.Context, id string, embedding []float32, metadata map[string]string) error
	// Query performs a similarity search filtered by metadata.
	Query(ctx context.Context, embedding []float32, filter map[string]string, topK int) ([]VectorSearchResult, error)
	// Delete removes an entry by ID.
	Delete(ctx context.Context, id string) error
}

// EmbeddingFunc generates an embedding vector from text content.
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// LongTermMemory provides semantic memory storage with isolation.
type LongTermMemory struct {
	store    VectorStore
	embedFn  EmbeddingFunc
}

// NewLongTermMemory creates a new LongTermMemory backed by the given VectorStore.
func NewLongTermMemory(store VectorStore, embedFn EmbeddingFunc) *LongTermMemory {
	return &LongTermMemory{
		store:   store,
		embedFn: embedFn,
	}
}

// Store persists a memory entry into the vector store.
func (m *LongTermMemory) Store(ctx context.Context, entry MemoryEntry) error {
	metadata := make(map[string]string)
	for k, v := range entry.Metadata {
		metadata[k] = v
	}
	metadata["_isolation_key"] = entry.IsolationKey
	metadata["_content"] = entry.Content
	metadata["_timestamp"] = entry.Timestamp.Format(time.RFC3339Nano)

	return m.store.Upsert(ctx, entry.ID, entry.Embedding, metadata)
}

// Search performs semantic search within the given isolation scope.
func (m *LongTermMemory) Search(ctx context.Context, query string, isolationKey string, topK int) ([]MemoryEntry, error) {
	embedding, err := m.embedFn(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	filter := map[string]string{
		"_isolation_key": isolationKey,
	}

	results, err := m.store.Query(ctx, embedding, filter, topK)
	if err != nil {
		return nil, fmt.Errorf("vector query: %w", err)
	}

	entries := make([]MemoryEntry, 0, len(results))
	for _, r := range results {
		entry := MemoryEntry{
			ID:           r.ID,
			IsolationKey: isolationKey,
			Metadata:     make(map[string]string),
		}
		for k, v := range r.Metadata {
			switch k {
			case "_content":
				entry.Content = v
			case "_timestamp":
				if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
					entry.Timestamp = t
				}
			case "_isolation_key":
				// already set
			default:
				entry.Metadata[k] = v
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Delete removes a specific memory entry by ID.
func (m *LongTermMemory) Delete(ctx context.Context, id string) error {
	return m.store.Delete(ctx, id)
}

// ListByIsolation returns entries matching the given isolation key, up to limit.
func (m *LongTermMemory) ListByIsolation(ctx context.Context, isolationKey string, limit int) ([]MemoryEntry, error) {
	// Use a zero-vector query to list by filter only.
	zeroEmbed := make([]float32, 1) // minimal dimension
	filter := map[string]string{
		"_isolation_key": isolationKey,
	}

	results, err := m.store.Query(ctx, zeroEmbed, filter, limit)
	if err != nil {
		return nil, fmt.Errorf("list by isolation: %w", err)
	}

	entries := make([]MemoryEntry, 0, len(results))
	for _, r := range results {
		entry := MemoryEntry{
			ID:           r.ID,
			IsolationKey: isolationKey,
			Metadata:     make(map[string]string),
		}
		for k, v := range r.Metadata {
			switch k {
			case "_content":
				entry.Content = v
			case "_timestamp":
				if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
					entry.Timestamp = t
				}
			case "_isolation_key":
				// skip
			default:
				entry.Metadata[k] = v
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// IsolationKeyBuilder constructs isolation keys in the format <level>:<tenant>:<user>:<session>.
type IsolationKeyBuilder struct {
	Tenant  string
	User    string
	Session string
}

// Build returns the isolation key for the given level.
func (b IsolationKeyBuilder) Build(level IsolationLevel) string {
	switch level {
	case IsolationShared:
		return fmt.Sprintf("shared:%s::", b.Tenant)
	case IsolationPerTenant:
		return fmt.Sprintf("per_tenant:%s::", b.Tenant)
	case IsolationPerUser:
		return fmt.Sprintf("per_user:%s:%s:", b.Tenant, b.User)
	case IsolationPerSession:
		return fmt.Sprintf("per_session:%s:%s:%s", b.Tenant, b.User, b.Session)
	default:
		return strings.Join([]string{string(level), b.Tenant, b.User, b.Session}, ":")
	}
}
