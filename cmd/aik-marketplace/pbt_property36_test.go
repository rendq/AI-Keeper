// Feature: ai-platform, Property 36: Marketplace rating monotonicity

//go:build pbt

package main

import (
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements C5**

// genStar generates a valid star rating between 1 and 5.
func genStar() gopter.Gen {
	return gen.IntRange(1, 5)
}

// genUsageStats generates random UsageStats for a review.
func genUsageStats() gopter.Gen {
	return gopter.CombineGens(
		gen.Int64Range(0, 5000),
		gen.Int64Range(0, 1_000_000),
		gen.IntRange(0, 365),
	).Map(func(vals []interface{}) UsageStats {
		return UsageStats{
			TotalCalls:   vals[0].(int64),
			TotalTokens:  vals[1].(int64),
			DurationDays: vals[2].(int),
			SuccessRate:  0.5,
		}
	})
}

// genReview generates a single random Review.
func genReview() gopter.Gen {
	return gopter.CombineGens(
		genStar(),
		genUsageStats(),
	).Map(func(vals []interface{}) *Review {
		return &Review{
			ID:        "review-pbt",
			ListingID: "listing-pbt",
			TenantID:  "tenant-pbt",
			Star:      vals[0].(int),
			Usage:     vals[1].(UsageStats),
			CreatedAt: time.Now(),
		}
	})
}

// genReviewSlice generates a non-empty slice of random reviews (1 to 50 reviews).
func genReviewSlice() gopter.Gen {
	return gen.SliceOfN(50, genReview()).SuchThat(func(v interface{}) bool {
		s := v.([]*Review)
		return len(s) >= 1
	})
}

func TestProperty36(t *testing.T) {
	seed := time.Now().UnixNano()
	if s := os.Getenv("AIP_PBT_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Property: Adding a new review changes averageRating by at most ±1 from previous value.
	// This ensures the weighted average cannot jump more than one full star per additional review.
	properties.Property("adding one review changes averageRating by at most ±1", prop.ForAll(
		func(existingReviews []*Review, newReview *Review) bool {
			// Compute average before adding the new review
			avgBefore := computeWeightedAverage(existingReviews)

			// Compute average after adding the new review
			allReviews := append(existingReviews, newReview)
			avgAfter := computeWeightedAverage(allReviews)

			// The change should not exceed ±1
			delta := math.Abs(avgAfter - avgBefore)
			if delta > 1.0+1e-9 {
				t.Logf("VIOLATION: avgBefore=%.6f, avgAfter=%.6f, delta=%.6f, existing=%d reviews, newStar=%d",
					avgBefore, avgAfter, delta, len(existingReviews), newReview.Star)
				return false
			}

			// The average should always be in [1, 5] range (since all stars are 1-5)
			if avgAfter < 1.0-1e-9 || avgAfter > 5.0+1e-9 {
				t.Logf("VIOLATION: avgAfter=%.6f out of [1,5] range", avgAfter)
				return false
			}

			return true
		},
		genReviewSlice(),
		genReview(),
	))

	properties.TestingRun(t)
}
