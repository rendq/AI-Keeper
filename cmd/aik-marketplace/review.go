package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Review represents a user review for a marketplace listing.
type Review struct {
	ID        string     `json:"id"`
	ListingID string     `json:"listingId"`
	TenantID  string     `json:"tenantId"`
	Star      int        `json:"star"`
	Text      string     `json:"text,omitempty"`
	Usage     UsageStats `json:"usage,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// UsageStats captures how much the reviewer has used the skill.
type UsageStats struct {
	TotalCalls   int64   `json:"totalCalls,omitempty"`
	TotalTokens  int64   `json:"totalTokens,omitempty"`
	DurationDays int     `json:"durationDays,omitempty"`
	SuccessRate  float64 `json:"successRate,omitempty"`
}

// CreateReviewRequest is the request body for POST /listings/:id/reviews.
type CreateReviewRequest struct {
	TenantID string     `json:"tenantId"`
	Star     int        `json:"star"`
	Text     string     `json:"text,omitempty"`
	Usage    UsageStats `json:"usage,omitempty"`
}

// ReviewStore defines the persistence interface for reviews.
type ReviewStore interface {
	CreateReview(ctx context.Context, r *Review) error
	ListReviews(ctx context.Context, listingID string) ([]*Review, error)
	// HasRecentReview checks if a tenant has submitted a review for the listing within the given window.
	HasRecentReview(ctx context.Context, listingID, tenantID string, window time.Duration) (bool, error)
}

// MemoryReviewStore is an in-memory implementation of ReviewStore.
type MemoryReviewStore struct {
	mu      sync.RWMutex
	reviews map[string][]*Review // listingID -> reviews
}

// NewMemoryReviewStore returns a new in-memory review store.
func NewMemoryReviewStore() *MemoryReviewStore {
	return &MemoryReviewStore{reviews: make(map[string][]*Review)}
}

func (s *MemoryReviewStore) CreateReview(_ context.Context, r *Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	r.CreatedAt = time.Now().UTC()
	s.reviews[r.ListingID] = append(s.reviews[r.ListingID], r)
	return nil
}

func (s *MemoryReviewStore) ListReviews(_ context.Context, listingID string) ([]*Review, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.reviews[listingID], nil
}

func (s *MemoryReviewStore) HasRecentReview(_ context.Context, listingID, tenantID string, window time.Duration) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-window)
	for _, r := range s.reviews[listingID] {
		if r.TenantID == tenantID && r.CreatedAt.After(cutoff) {
			return true, nil
		}
	}
	return false, nil
}

// computeWeightedAverage calculates a weighted moving average for ratings.
// It uses usage-based weighting: reviewers with more usage get higher weight.
func computeWeightedAverage(reviews []*Review) float64 {
	if len(reviews) == 0 {
		return 0
	}

	var totalWeight float64
	var weightedSum float64

	for _, r := range reviews {
		weight := reviewWeight(r)
		weightedSum += float64(r.Star) * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 0
	}
	return weightedSum / totalWeight
}

// reviewWeight returns a weight for a review based on usage stats.
// Reviews from users with more usage carry more weight.
func reviewWeight(r *Review) float64 {
	base := 1.0
	if r.Usage.TotalCalls > 100 {
		base += 0.5
	}
	if r.Usage.TotalCalls > 1000 {
		base += 0.5
	}
	if r.Usage.DurationDays > 30 {
		base += 0.5
	}
	return base
}

// ErrRateLimited is returned when a tenant tries to review too frequently.
var ErrRateLimited = fmt.Errorf("rate limited: same tenant can only review once every 24 hours")
