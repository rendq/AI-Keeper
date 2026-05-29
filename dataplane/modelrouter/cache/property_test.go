//go:build pbt

package cache

import (
	"math"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// genEmbedding generates a random non-zero embedding of a given dimension.
func genEmbedding(dim int) gopter.Gen {
	return gen.SliceOfN(dim, gen.Float32Range(-1.0, 1.0)).SuchThat(func(v []float32) bool {
		// Ensure non-zero vector for valid cosine similarity
		var norm float64
		for _, x := range v {
			norm += float64(x) * float64(x)
		}
		return norm > 1e-6
	})
}

// genResponse generates a random non-empty response string.
func genResponse() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})
}

// genTenantID generates a random tenant identifier.
func genTenantID() gopter.Gen {
	return gen.OneConstOf("tenant-alpha", "tenant-beta", "tenant-gamma", "tenant-delta", "tenant-epsilon")
}

// **Validates: Requirements B8.4, F19**
// TestPropertyP18_TenantIsolation verifies that cache entries stored in one
// tenant's cache instance are never returned when querying another tenant's
// cache instance. This simulates per-tenant isolation using separate
// SemanticCache instances per tenant.
func TestPropertyP18_TenantIsolation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 1000
	parameters.MaxSize = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("P18: entries from tenant A never returned for tenant B queries", prop.ForAll(
		func(tenantA, tenantB string, embA, embB []float32, response string) bool {
			if tenantA == tenantB {
				// Skip when tenants are the same — isolation only matters across tenants
				return true
			}

			cfg := CacheConfig{
				Enabled:             true,
				SimilarityThreshold: 0.5, // Low threshold to maximize chance of accidental hits
				TTL:                 10 * time.Minute,
				MaxEntries:          100,
			}

			// Simulate per-tenant cache isolation with separate instances
			cacheA := NewSemanticCache(cfg)
			cacheB := NewSemanticCache(cfg)

			// Store entry in tenant A's cache
			cacheA.Put(embA, response)

			// Query tenant B's cache with both tenant A's embedding and tenant B's embedding
			// Neither should return tenant A's data
			_, hitWithA := cacheB.Get(embA)
			_, hitWithB := cacheB.Get(embB)

			// Tenant B's cache must never return tenant A's entry
			return !hitWithA && !hitWithB
		},
		genTenantID(),
		genTenantID().SuchThat(func(s string) bool { return true }), // will filter in body
		genEmbedding(8),
		genEmbedding(8),
		genResponse(),
	))

	properties.TestingRun(t)
}

// **Validates: Requirements B8.4, F19**
// TestPropertyP19_TTLEnforcement verifies that TTL-expired entries are never
// returned from the cache regardless of how similar the query embedding is.
func TestPropertyP19_TTLEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 1000
	parameters.MaxSize = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("P19: TTL expired entries never returned regardless of similarity", prop.ForAll(
		func(emb []float32, response string, pastSeconds uint16) bool {
			// Use a fixed TTL
			ttl := 1 * time.Second
			cfg := CacheConfig{
				Enabled:             true,
				SimilarityThreshold: 0.0, // Lowest possible threshold — any match would hit
				TTL:                 ttl,
				MaxEntries:          100,
			}

			c := NewSemanticCache(cfg)

			// Store entry then manually set CreatedAt to the past to simulate expiry
			c.Put(emb, response)

			// Set the entry's CreatedAt to be expired (at least TTL+1ms in the past)
			expiredTime := time.Now().Add(-ttl - time.Duration(pastSeconds+1)*time.Millisecond)
			c.mu.Lock()
			if len(c.entries) > 0 {
				c.entries[len(c.entries)-1].CreatedAt = expiredTime
			}
			c.mu.Unlock()

			// Query with exact same embedding — should NOT hit because entry is expired
			_, hit := c.Get(emb)
			return !hit
		},
		genEmbedding(8),
		genResponse(),
		gen.UInt16(), // additional past offset in milliseconds
	))

	properties.TestingRun(t)
}

// **Validates: Requirements B8.4**
// TestPropertyP20_SimilarityThreshold verifies that all cache hits have a
// cosine similarity score that is >= the configured threshold.
func TestPropertyP20_SimilarityThreshold(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 1000
	parameters.MaxSize = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("P20: cache hit similarity always >= configured threshold", prop.ForAll(
		func(storedEmb, queryEmb []float32, thresholdPct uint8) bool {
			// Map thresholdPct (0-100) to a similarity threshold in [0.5, 0.99]
			threshold := 0.5 + float64(thresholdPct%50)*0.01

			cfg := CacheConfig{
				Enabled:             true,
				SimilarityThreshold: threshold,
				TTL:                 10 * time.Minute,
				MaxEntries:          100,
			}

			c := NewSemanticCache(cfg)
			c.Put(storedEmb, "cached-response")

			_, hit := c.Get(queryEmb)
			if hit {
				// Verify the actual similarity is indeed >= threshold
				sim := cosineSimilarity(queryEmb, storedEmb)
				return sim >= threshold
			}
			// If no hit, verify that either no entry has similarity >= threshold,
			// or entries are expired. Since we just stored it with a long TTL,
			// a miss means similarity < threshold.
			sim := cosineSimilarity(queryEmb, storedEmb)
			// Allow floating point tolerance
			return sim < threshold+1e-9 || isZeroVector(storedEmb) || isZeroVector(queryEmb)
		},
		genEmbedding(8),
		genEmbedding(8),
		gen.UInt8(),
	))

	properties.TestingRun(t)
}

// isZeroVector checks if a vector has near-zero magnitude.
func isZeroVector(v []float32) bool {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	return math.Sqrt(norm) < 1e-6
}
