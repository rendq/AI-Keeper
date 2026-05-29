package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Publish workflow errors.
var (
	ErrInvalidTransition = errors.New("invalid phase transition")
	ErrSecurityScanFail  = errors.New("security scan failed")
)

// SecurityScanResult holds the outcome of an automated security scan.
type SecurityScanResult struct {
	Passed  bool     `json:"passed"`
	Issues  []string `json:"issues,omitempty"`
	ScannedAt time.Time `json:"scannedAt"`
}

// PublishWorkflow manages the publish state machine for marketplace listings.
type PublishWorkflow struct {
	store Store
}

// NewPublishWorkflow creates a new workflow bound to the given store.
func NewPublishWorkflow(store Store) *PublishWorkflow {
	return &PublishWorkflow{store: store}
}

// SubmitForReview transitions a listing from Draft to PendingReview.
// It runs an automatic security scan first; if the scan fails, the transition is rejected.
func (pw *PublishWorkflow) SubmitForReview(ctx context.Context, listingID string) (*SecurityScanResult, error) {
	listing, err := pw.store.Get(ctx, listingID)
	if err != nil {
		return nil, err
	}

	if listing.Phase != PhaseDraft {
		return nil, ErrInvalidTransition
	}

	// Run automated security scan.
	result := runSecurityScan(listing)
	if !result.Passed {
		return &result, ErrSecurityScanFail
	}

	// Transition to PendingReview.
	listing.Phase = PhasePendingReview
	if err := pw.store.Update(ctx, listing); err != nil {
		return &result, err
	}

	return &result, nil
}

// ApproveReview transitions a listing from PendingReview to Published.
func (pw *PublishWorkflow) ApproveReview(ctx context.Context, listingID string) error {
	listing, err := pw.store.Get(ctx, listingID)
	if err != nil {
		return err
	}

	if listing.Phase != PhasePendingReview {
		return ErrInvalidTransition
	}

	listing.Phase = PhasePublished
	return pw.store.Update(ctx, listing)
}

// RejectReview transitions a listing from PendingReview to Rejected.
func (pw *PublishWorkflow) RejectReview(ctx context.Context, listingID string, reason string) error {
	listing, err := pw.store.Get(ctx, listingID)
	if err != nil {
		return err
	}

	if listing.Phase != PhasePendingReview {
		return ErrInvalidTransition
	}

	listing.Phase = PhaseRejected
	return pw.store.Update(ctx, listing)
}

// badPatterns contains known dangerous patterns in skill references that the
// security scanner checks for.
var badPatterns = []string{
	"exec(",
	"eval(",
	"__import__",
	"os.system",
	"subprocess",
	"rm -rf",
}

// runSecurityScan performs an automated security scan on a listing.
// It checks the skill reference and readme for known bad patterns.
func runSecurityScan(l *Listing) SecurityScanResult {
	result := SecurityScanResult{
		ScannedAt: time.Now().UTC(),
		Passed:    true,
	}

	content := strings.ToLower(l.SkillRef + " " + l.Readme)
	for _, pattern := range badPatterns {
		if strings.Contains(content, strings.ToLower(pattern)) {
			result.Passed = false
			result.Issues = append(result.Issues, "found dangerous pattern: "+pattern)
		}
	}

	return result
}
