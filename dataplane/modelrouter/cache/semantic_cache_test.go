package cache

import (
	"testing"
	"time"
)

func defaultConfig() CacheConfig {
	return CacheConfig{
		Enabled:             true,
		SimilarityThreshold: 0.9,
		TTL:                 5 * time.Minute,
		MaxEntries:          100,
	}
}

func TestCache_PutAndGet(t *testing.T) {
	c := NewSemanticCache(defaultConfig())

	emb := []float32{1.0, 0.0, 0.0}
	c.Put(emb, "hello world")

	// Query with the same embedding should hit
	resp, hit := c.Get([]float32{1.0, 0.0, 0.0})
	if !hit {
		t.Fatal("expected cache hit")
	}
	if resp != "hello world" {
		t.Fatalf("expected 'hello world', got %q", resp)
	}

	// Query with a very similar embedding should also hit
	resp, hit = c.Get([]float32{0.99, 0.01, 0.0})
	if !hit {
		t.Fatal("expected cache hit for similar embedding")
	}
	if resp != "hello world" {
		t.Fatalf("expected 'hello world', got %q", resp)
	}
}

func TestCache_Miss(t *testing.T) {
	c := NewSemanticCache(defaultConfig())

	c.Put([]float32{1.0, 0.0, 0.0}, "response A")

	// Orthogonal vector should miss
	_, hit := c.Get([]float32{0.0, 1.0, 0.0})
	if hit {
		t.Fatal("expected cache miss for dissimilar embedding")
	}

	stats := c.Stats()
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	cfg := defaultConfig()
	cfg.TTL = 50 * time.Millisecond
	c := NewSemanticCache(cfg)

	c.Put([]float32{1.0, 0.0, 0.0}, "ephemeral")

	// Should hit immediately
	_, hit := c.Get([]float32{1.0, 0.0, 0.0})
	if !hit {
		t.Fatal("expected cache hit before TTL")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	_, hit = c.Get([]float32{1.0, 0.0, 0.0})
	if hit {
		t.Fatal("expected cache miss after TTL expiry")
	}

	// Evict should remove expired entries
	c.Evict()
	stats := c.Stats()
	if stats.Size != 0 {
		t.Fatalf("expected 0 entries after evict, got %d", stats.Size)
	}
	if stats.Evictions != 1 {
		t.Fatalf("expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestCache_EvictOldest(t *testing.T) {
	cfg := defaultConfig()
	cfg.MaxEntries = 3
	c := NewSemanticCache(cfg)

	c.Put([]float32{1.0, 0.0, 0.0}, "first")
	time.Sleep(time.Millisecond)
	c.Put([]float32{0.0, 1.0, 0.0}, "second")
	time.Sleep(time.Millisecond)
	c.Put([]float32{0.0, 0.0, 1.0}, "third")

	// Cache is full, adding another should evict the oldest ("first")
	c.Put([]float32{0.5, 0.5, 0.0}, "fourth")

	stats := c.Stats()
	if stats.Size != 3 {
		t.Fatalf("expected 3 entries, got %d", stats.Size)
	}
	if stats.Evictions != 1 {
		t.Fatalf("expected 1 eviction, got %d", stats.Evictions)
	}

	// The first entry (1,0,0) should be evicted
	_, hit := c.Get([]float32{1.0, 0.0, 0.0})
	if hit {
		t.Fatal("expected evicted entry to miss")
	}
}

func TestCache_SimilarityThreshold(t *testing.T) {
	cfg := defaultConfig()
	cfg.SimilarityThreshold = 0.95
	c := NewSemanticCache(cfg)

	c.Put([]float32{1.0, 0.0, 0.0}, "strict")

	// Slightly different — cosine similarity ~0.995, should hit
	_, hit := c.Get([]float32{0.99, 0.1, 0.0})
	if !hit {
		t.Fatal("expected hit for high similarity embedding")
	}

	// More different — cosine similarity will be lower, should miss
	_, hit = c.Get([]float32{0.7, 0.7, 0.0})
	if hit {
		t.Fatal("expected miss for below-threshold similarity")
	}
}
