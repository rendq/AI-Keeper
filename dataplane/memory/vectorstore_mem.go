package memory

import (
	"context"
	"math"
	"sync"
)

// memRecord stores a vector entry in the in-memory store.
type memRecord struct {
	ID        string
	Embedding []float32
	Metadata  map[string]string
}

// InMemoryVectorStore is an in-memory implementation of VectorStore for testing.
type InMemoryVectorStore struct {
	mu      sync.RWMutex
	records map[string]memRecord
}

// NewInMemoryVectorStore creates a new in-memory vector store.
func NewInMemoryVectorStore() *InMemoryVectorStore {
	return &InMemoryVectorStore{
		records: make(map[string]memRecord),
	}
}

func (s *InMemoryVectorStore) Upsert(_ context.Context, id string, embedding []float32, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[id] = memRecord{
		ID:        id,
		Embedding: embedding,
		Metadata:  metadata,
	}
	return nil
}

func (s *InMemoryVectorStore) Query(_ context.Context, embedding []float32, filter map[string]string, topK int) ([]VectorSearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		id       string
		score    float32
		metadata map[string]string
	}

	var matches []scored
	for _, rec := range s.records {
		if !matchesFilter(rec.Metadata, filter) {
			continue
		}
		score := cosineSimilarity(embedding, rec.Embedding)
		matches = append(matches, scored{id: rec.ID, score: score, metadata: rec.Metadata})
	}

	// Sort by score descending.
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].score > matches[i].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	if topK < len(matches) {
		matches = matches[:topK]
	}

	results := make([]VectorSearchResult, len(matches))
	for i, m := range matches {
		results[i] = VectorSearchResult{
			ID:       m.id,
			Score:    m.score,
			Metadata: m.metadata,
		}
	}
	return results, nil
}

func (s *InMemoryVectorStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}

// ScanAll returns all stored records as VectorSearchResults (implements ScannableStore).
func (s *InMemoryVectorStore) ScanAll(_ context.Context) ([]VectorSearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]VectorSearchResult, 0, len(s.records))
	for _, rec := range s.records {
		results = append(results, VectorSearchResult{
			ID:       rec.ID,
			Score:    0,
			Metadata: rec.Metadata,
		})
	}
	return results, nil
}

// matchesFilter returns true if all filter key-value pairs are present in metadata.
func matchesFilter(metadata, filter map[string]string) bool {
	for k, v := range filter {
		if metadata[k] != v {
			return false
		}
	}
	return true
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return float32(dot / denom)
}
