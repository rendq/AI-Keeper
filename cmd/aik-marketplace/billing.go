package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ai-keeper/ai-keeper/dataplane/cost"
	"github.com/google/uuid"
)

// PricingModel defines how a listing is billed.
type PricingModel string

const (
	PricingFree     PricingModel = "free"
	PricingPerCall  PricingModel = "per_call"
	PricingPerMonth PricingModel = "per_month"
	PricingPerToken PricingModel = "per_token"
)

// PricingSpec defines the pricing configuration for a listing.
type PricingSpec struct {
	Model  PricingModel `json:"model"`
	Amount float64      `json:"amount"` // USD amount per unit (per call, per month, or per 1M tokens)
}

// UsageRecord represents a single usage event against a listing.
type UsageRecord struct {
	ID        string       `json:"id"`
	ListingID string       `json:"listingId"`
	TenantID  string       `json:"tenantId"`
	Model     PricingModel `json:"model"`
	Tokens    int64        `json:"tokens,omitempty"` // for per_token pricing
	Charge    float64      `json:"charge"`           // computed charge in USD
	CreatedAt time.Time    `json:"createdAt"`
}

// Invoice represents a monthly settlement invoice.
type Invoice struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenantId"`
	ListingID  string    `json:"listingId"`
	PeriodStart time.Time `json:"periodStart"`
	PeriodEnd   time.Time `json:"periodEnd"`
	TotalCharge float64  `json:"totalCharge"`
	LineItems   int      `json:"lineItems"`
	CreatedAt  time.Time `json:"createdAt"`
}

// InvoiceEvent is emitted when a monthly invoice is generated.
type InvoiceEvent struct {
	Type    string  `json:"type"`
	Invoice Invoice `json:"invoice"`
}

// BillingService tracks usage charges and integrates with the Cost Tracker.
type BillingService struct {
	mu       sync.RWMutex
	records  []*UsageRecord
	invoices []*Invoice
	tracker  *cost.Tracker // integration with Cost Tracker (13.1)
}

// NewBillingService creates a new BillingService.
// If tracker is nil, cost recording to the budget is skipped.
func NewBillingService(tracker *cost.Tracker) *BillingService {
	return &BillingService{
		records:  make([]*UsageRecord, 0),
		invoices: make([]*Invoice, 0),
		tracker:  tracker,
	}
}

// RecordUsage calculates a charge based on the pricing model and records it.
// For free listings, no charge is generated (returns 0).
// For per_call, the charge is PricingSpec.Amount per invocation.
// For per_token, the charge is (tokens / 1_000_000) * PricingSpec.Amount.
// For per_month, the charge is PricingSpec.Amount (recorded once per billing event).
func (bs *BillingService) RecordUsage(ctx context.Context, listingID, tenantID string, pricing PricingSpec, tokens int64) (*UsageRecord, error) {
	if pricing.Model == PricingFree {
		return &UsageRecord{
			ID:        uuid.New().String(),
			ListingID: listingID,
			TenantID:  tenantID,
			Model:     PricingFree,
			Charge:    0,
			CreatedAt: time.Now().UTC(),
		}, nil
	}

	charge := computeCharge(pricing, tokens)

	record := &UsageRecord{
		ID:        uuid.New().String(),
		ListingID: listingID,
		TenantID:  tenantID,
		Model:     pricing.Model,
		Tokens:    tokens,
		Charge:    charge,
		CreatedAt: time.Now().UTC(),
	}

	bs.mu.Lock()
	bs.records = append(bs.records, record)
	bs.mu.Unlock()

	// Integrate with Cost Tracker: write charge to budget
	if bs.tracker != nil && charge > 0 {
		dim := cost.Dimension{
			TenantID:  tenantID,
			SkillName: listingID,
		}
		// Use a synthetic EndpointCost to pass the pre-computed charge through the tracker.
		// We encode the charge as InputPerMillion and pass 1M tokens so ComputeCost returns charge exactly.
		usage := cost.Usage{Input: 1_000_000}
		endpointCost := cost.EndpointCost{InputPerMillion: charge}
		if _, err := bs.tracker.Record(ctx, dim, usage, endpointCost); err != nil {
			return record, fmt.Errorf("billing: cost tracker record: %w", err)
		}
	}

	return record, nil
}

// computeCharge calculates the USD charge for a given pricing model.
func computeCharge(pricing PricingSpec, tokens int64) float64 {
	switch pricing.Model {
	case PricingPerCall:
		return pricing.Amount
	case PricingPerToken:
		return (float64(tokens) / 1_000_000) * pricing.Amount
	case PricingPerMonth:
		return pricing.Amount
	default:
		return 0
	}
}

// MonthlySettle aggregates all usage records for the given period and tenant/listing,
// produces an Invoice, and emits an InvoiceEvent.
func (bs *BillingService) MonthlySettle(ctx context.Context, tenantID, listingID string, periodStart, periodEnd time.Time) (*InvoiceEvent, error) {
	bs.mu.RLock()
	var totalCharge float64
	var lineItems int
	for _, r := range bs.records {
		if r.TenantID != tenantID || r.ListingID != listingID {
			continue
		}
		if r.CreatedAt.Before(periodStart) || r.CreatedAt.After(periodEnd) {
			continue
		}
		totalCharge += r.Charge
		lineItems++
	}
	bs.mu.RUnlock()

	invoice := Invoice{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		ListingID:   listingID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCharge: totalCharge,
		LineItems:   lineItems,
		CreatedAt:   time.Now().UTC(),
	}

	bs.mu.Lock()
	bs.invoices = append(bs.invoices, &invoice)
	bs.mu.Unlock()

	event := &InvoiceEvent{
		Type:    "marketplace.invoice.created",
		Invoice: invoice,
	}

	return event, nil
}

// GetInvoices returns all invoices for a given tenant.
func (bs *BillingService) GetInvoices(_ context.Context, tenantID string) []*Invoice {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var result []*Invoice
	for _, inv := range bs.invoices {
		if inv.TenantID == tenantID {
			result = append(result, inv)
		}
	}
	return result
}
