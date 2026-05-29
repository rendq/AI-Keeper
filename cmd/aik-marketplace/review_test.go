package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRatingSubmitReview(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID: "tenant-1", Name: "reviewed-skill",
		SkillRef: "skill://review@1.0.0", Publisher: "P", Category: "dev",
	})

	req := CreateReviewRequest{
		TenantID: "tenant-2",
		Star:     4,
		Text:     "Great skill, very useful!",
		Usage: UsageStats{
			TotalCalls:   500,
			DurationDays: 60,
		},
	}
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/listings/"+listing.ID+"/reviews", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var review Review
	json.NewDecoder(w.Body).Decode(&review)

	if review.ID == "" {
		t.Error("review should have an ID")
	}
	if review.Star != 4 {
		t.Errorf("expected star=4, got %d", review.Star)
	}
	if review.TenantID != "tenant-2" {
		t.Errorf("expected tenantId=tenant-2, got %s", review.TenantID)
	}
	if review.ListingID != listing.ID {
		t.Errorf("expected listingId=%s, got %s", listing.ID, review.ListingID)
	}
}

func TestRatingInvalidStar(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID: "tenant-1", Name: "skill",
		SkillRef: "skill://x@1.0.0", Publisher: "P", Category: "dev",
	})

	cases := []struct {
		name string
		star int
	}{
		{"too low", 0},
		{"too high", 6},
		{"negative", -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := CreateReviewRequest{TenantID: "tenant-2", Star: tc.star}
			body, _ := json.Marshal(req)
			r := httptest.NewRequest(http.MethodPost, "/listings/"+listing.ID+"/reviews", bytes.NewReader(body))
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for star=%d, got %d", tc.star, w.Code)
			}
		})
	}
}

func TestRatingMissingTenantID(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID: "tenant-1", Name: "skill",
		SkillRef: "skill://x@1.0.0", Publisher: "P", Category: "dev",
	})

	req := CreateReviewRequest{Star: 3}
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/listings/"+listing.ID+"/reviews", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRatingListingNotFound(t *testing.T) {
	srv := newTestServer()

	req := CreateReviewRequest{TenantID: "tenant-1", Star: 3, Text: "test"}
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/listings/nonexistent/reviews", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRatingAntiSpam(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID: "tenant-1", Name: "spam-test-skill",
		SkillRef: "skill://spam@1.0.0", Publisher: "P", Category: "dev",
	})

	// First review should succeed
	req := CreateReviewRequest{TenantID: "tenant-2", Star: 5, Text: "First review"}
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/listings/"+listing.ID+"/reviews", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("first review: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Second review from same tenant within 24h should be rejected
	req = CreateReviewRequest{TenantID: "tenant-2", Star: 1, Text: "Spam review"}
	body, _ = json.Marshal(req)
	r = httptest.NewRequest(http.MethodPost, "/listings/"+listing.ID+"/reviews", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second review: expected 429, got %d: %s", w.Code, w.Body.String())
	}

	// Different tenant should still be able to review
	req = CreateReviewRequest{TenantID: "tenant-3", Star: 3, Text: "Different tenant"}
	body, _ = json.Marshal(req)
	r = httptest.NewRequest(http.MethodPost, "/listings/"+listing.ID+"/reviews", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Errorf("different tenant: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRatingWeightedAverage(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID: "tenant-1", Name: "avg-test",
		SkillRef: "skill://avg@1.0.0", Publisher: "P", Category: "dev",
	})

	// Submit reviews from different tenants with different usage stats
	reviews := []CreateReviewRequest{
		{TenantID: "t-a", Star: 5, Usage: UsageStats{TotalCalls: 2000, DurationDays: 90}},
		{TenantID: "t-b", Star: 3, Usage: UsageStats{TotalCalls: 10}},
		{TenantID: "t-c", Star: 4, Usage: UsageStats{TotalCalls: 500, DurationDays: 45}},
	}

	for _, rev := range reviews {
		body, _ := json.Marshal(rev)
		r := httptest.NewRequest(http.MethodPost, "/listings/"+listing.ID+"/reviews", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("submit review: expected 201, got %d: %s", w.Code, w.Body.String())
		}
	}

	// Fetch listing and verify rating was updated
	r := httptest.NewRequest(http.MethodGet, "/listings/"+listing.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var got Listing
	json.NewDecoder(w.Body).Decode(&got)

	// Weighted average should not be a simple average (4.0)
	// High-usage reviewer (5 stars, weight 2.5) should pull the average up
	if got.Rating <= 0 {
		t.Error("rating should be > 0 after reviews")
	}
	if got.Rating == 4.0 {
		t.Error("weighted average should differ from simple average")
	}
	// With weights: t-a=2.5 (5*2.5=12.5), t-b=1.0 (3*1=3), t-c=2.0 (4*2=8)
	// Expected: (12.5 + 3 + 8) / (2.5 + 1 + 2) = 23.5 / 5.5 ≈ 4.27
	if got.Rating < 4.0 || got.Rating > 4.5 {
		t.Errorf("expected weighted average around 4.27, got %.2f", got.Rating)
	}
}

func TestRatingAggregation_Unit(t *testing.T) {
	reviews := []*Review{
		{Star: 5, Usage: UsageStats{TotalCalls: 2000, DurationDays: 90}},  // weight: 1 + 0.5 + 0.5 + 0.5 = 2.5
		{Star: 3, Usage: UsageStats{TotalCalls: 10}},                       // weight: 1.0
		{Star: 4, Usage: UsageStats{TotalCalls: 500, DurationDays: 45}},    // weight: 1 + 0.5 + 0.5 = 2.0
	}

	avg := computeWeightedAverage(reviews)
	// (5*2.5 + 3*1.0 + 4*2.0) / (2.5 + 1.0 + 2.0) = 23.5 / 5.5 ≈ 4.27
	expected := 23.5 / 5.5
	if diff := avg - expected; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected weighted avg ≈ %.4f, got %.4f", expected, avg)
	}
}

func TestRatingAggregation_Empty(t *testing.T) {
	avg := computeWeightedAverage(nil)
	if avg != 0 {
		t.Errorf("expected 0 for empty reviews, got %f", avg)
	}
}

func TestRatingAggregation_EqualWeight(t *testing.T) {
	// All reviews with same minimal usage → equal weight → simple average
	reviews := []*Review{
		{Star: 2, Usage: UsageStats{TotalCalls: 5}},
		{Star: 4, Usage: UsageStats{TotalCalls: 5}},
	}

	avg := computeWeightedAverage(reviews)
	// Both have weight 1.0, so average = (2+4)/2 = 3.0
	if avg != 3.0 {
		t.Errorf("expected 3.0, got %f", avg)
	}
}

func TestRatingReviewWeight(t *testing.T) {
	cases := []struct {
		name     string
		review   *Review
		expected float64
	}{
		{"minimal usage", &Review{Usage: UsageStats{TotalCalls: 10}}, 1.0},
		{"medium calls", &Review{Usage: UsageStats{TotalCalls: 500}}, 1.5},
		{"high calls", &Review{Usage: UsageStats{TotalCalls: 2000}}, 2.0},
		{"high calls + long duration", &Review{Usage: UsageStats{TotalCalls: 2000, DurationDays: 60}}, 2.5},
		{"medium calls + long duration", &Review{Usage: UsageStats{TotalCalls: 500, DurationDays: 60}}, 2.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := reviewWeight(tc.review)
			if w != tc.expected {
				t.Errorf("expected weight=%f, got %f", tc.expected, w)
			}
		})
	}
}

// TestRatingHasRecentReview tests the HasRecentReview logic directly.
func TestRatingHasRecentReview(t *testing.T) {
	store := NewMemoryReviewStore()
	ctx := context.Background()

	// No reviews yet
	has, err := store.HasRecentReview(ctx, "listing-1", "tenant-1", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected no recent review")
	}

	// Add a review
	store.CreateReview(ctx, &Review{
		ListingID: "listing-1",
		TenantID:  "tenant-1",
		Star:      4,
	})

	// Should now detect the recent review
	has, err = store.HasRecentReview(ctx, "listing-1", "tenant-1", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("expected recent review to be detected")
	}

	// Different tenant should not be affected
	has, err = store.HasRecentReview(ctx, "listing-1", "tenant-2", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected no recent review for different tenant")
	}

	// Different listing should not be affected
	has, err = store.HasRecentReview(ctx, "listing-2", "tenant-1", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected no recent review for different listing")
	}
}
