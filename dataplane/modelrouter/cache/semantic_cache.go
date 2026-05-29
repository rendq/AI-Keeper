// Package cache provides semantic caching for LLM responses using embeddings
// and approximate nearest neighbor search with TTL-based expiry.
package cache

import (
	"math"
	"sync"
	"time"
)

// CacheConfig holds configuration for the semantic cache.
type CacheConfig struct {
	Enabled             bool
	SimilarityThreshold float64
	TTL                 time.Duration
	MaxEntries          int
}

// CacheEntry represents a single cached response with its embedding.
type CacheEntry struct {
	Key       string
	Embedding []float32
	Response  string
	CreatedAt time.Time
	HitCount  int
}

// CacheStats holds cache performance statistics.
type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
}

// SemanticCache implements an in-memory semantic cache using cosine similarity.
type SemanticCache struct {
	mu      sync.RWMutex
	config  CacheConfig
	entries []*CacheEntry
	stats   CacheStats
}

// NewSemanticCache creates a new SemanticCache with the given configuration.
func NewSemanticCache(config CacheConfig) *SemanticCache {
	return &SemanticCache{
		config:  config,
		entries: make([]*CacheEntry, 0, config.MaxEntries),
	}
}

// Get finds a cached response by cosine similarity to the given embedding.
// Returns the response and true if a cache hit is found above the similarity threshold.
func (c *SemanticCache) Get(embedding []float32) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var bestEntry *CacheEntry
	bestSimilarity := -1.0

	for _, entry := range c.entries {
		// Skip expired entries
		if now.Sub(entry.CreatedAt) > c.config.TTL {
			continue
		}
		sim := cosineSimilarity(embedding, entry.Embedding)
		if sim >= c.config.SimilarityThreshold && sim > bestSimilarity {
			bestSimilarity = sim
			bestEntry = entry
		}
	}

	if bestEntry != nil {
		bestEntry.HitCount++
		c.stats.Hits++
		return bestEntry.Response, true
	}

	c.stats.Misses++
	return "", false
}

// Put stores a response with its embedding in the cache.
// If the cache is full, the oldest entry is evicted.
func (c *SemanticCache) Put(embedding []float32, response string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.config.MaxEntries {
		c.evictOldest()
	}

	entry := &CacheEntry{
		Embedding: embedding,
		Response:  response,
		CreatedAt: time.Now(),
	}
	c.entries = append(c.entries, entry)
	c.stats.Size = len(c.entries)
}

// Evict removes all expired entries from the cache.
func (c *SemanticCache) Evict() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	alive := make([]*CacheEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		if now.Sub(entry.CreatedAt) <= c.config.TTL {
			alive = append(alive, entry)
		} else {
			c.stats.Evictions++
		}
	}
	c.entries = alive
	c.stats.Size = len(c.entries)
}

// Stats returns current cache performance statistics.
func (c *SemanticCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	stats := c.stats
	stats.Size = len(c.entries)
	return stats
}

// evictOldest removes the oldest entry from the cache. Must be called with lock held.
func (c *SemanticCache) evictOldest() {
	if len(c.entries) == 0 {
		return
	}
	oldestIdx := 0
	for i, entry := range c.entries {
		if entry.CreatedAt.Before(c.entries[oldestIdx].CreatedAt) {
			oldestIdx = i
		}
	}
	c.entries = append(c.entries[:oldestIdx], c.entries[oldestIdx+1:]...)
	c.stats.Evictions++
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
