package main

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/ai-keeper/ai-keeper/dataplane/cost"
	"github.com/prometheus/client_golang/prometheus"
)

// mockRedis implements cost.RedisClient for testing.
type mockRedis struct {
	data map[string]float64
}

func newMockRedis() *mockRedis {
	return &mockRedis{data: make(map[string]float64)}
}

func (m *mockRedis) IncrByFloat(_ context.Context, key string, value float64) (float64, error) {
	m.data[key] += value
	return m.data[key], nil
}

func TestBillingRecordUsageFree(t *testing.T) {
	bs := NewBillingService(nil)
	ctx := context.Background()

	record, err := bs.RecordUsage(ctx, "listing-1", "tenant-1", PricingSpec{
		Model:  PricingFree,
		Amount: 0,
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Charge != 0 {
		t.Errorf("expected charge=0 for free model, got %f", record.Charge)
	}
	if record.Model != PricingFree {
		t.Errorf("expected model=free, got %s", record.Model)
	}
}

func TestBillingRecordUsagePerCall(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := cost.NewTracker(redis, reg)
	bs := NewBillingService(tracker)
	ctx := context.Background()

	pricing := PricingSpec{Model: PricingPerCall, Amount: 0.01}

	record, err := bs.RecordUsage(ctx, "listing-2", "tenant-1", pricing, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Charge != 0.01 {
		t.Errorf("expected charge=0.01, got %f", record.Charge)
	}

	// Verify cost tracker was called (Redis counter incremented)
	tenantKey := "aip:cost:tenant:tenant-1"
	if redis.data[tenantKey] == 0 {
		t.Error("expected cost tracker to record the charge in Redis")
	}
}

func TestBillingRecordUsagePerToken(t *testing.T) {
	bs := NewBillingService(nil)
	ctx := context.Background()

	pricing := PricingSpec{Model: PricingPerToken, Amount: 2.0} // $2 per 1M tokens

	record, err := bs.RecordUsage(ctx, "listing-3", "tenant-1", pricing, 500_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := 1.0 // 500k / 1M * $2
	if math.Abs(record.Charge-expected) > 0.0001 {
		t.Errorf("expected charge=%f, got %f", expected, record.Charge)
	}
}

func TestBillingRecordUsagePerMonth(t *testing.T) {
	bs := NewBillingService(nil)
	ctx := context.Background()

	pricing := PricingSpec{Model: PricingPerMonth, Amount: 9.99}

	record, err := bs.RecordUsage(ctx, "listing-4", "tenant-1", pricing, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.Charge != 9.99 {
		t.Errorf("expected charge=9.99, got %f", record.Charge)
	}
}

func TestBillingMonthlySettle(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := cost.NewTracker(redis, reg)
	bs := NewBillingService(tracker)
	ctx := context.Background()

	pricing := PricingSpec{Model: PricingPerCall, Amount: 0.05}

	// Record 3 usage events
	for i := 0; i < 3; i++ {
		_, err := bs.RecordUsage(ctx, "listing-5", "tenant-2", pricing, 0)
		if err != nil {
			t.Fatalf("unexpected error on record %d: %v", i, err)
		}
	}

	// Settle the month
	now := time.Now().UTC()
	periodStart := now.Add(-1 * time.Hour)
	periodEnd := now.Add(1 * time.Hour)

	event, err := bs.MonthlySettle(ctx, "tenant-2", "listing-5", periodStart, periodEnd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "marketplace.invoice.created" {
		t.Errorf("expected event type marketplace.invoice.created, got %s", event.Type)
	}
	if event.Invoice.LineItems != 3 {
		t.Errorf("expected 3 line items, got %d", event.Invoice.LineItems)
	}

	expectedTotal := 0.15 // 3 * $0.05
	if math.Abs(event.Invoice.TotalCharge-expectedTotal) > 0.0001 {
		t.Errorf("expected total charge=%f, got %f", expectedTotal, event.Invoice.TotalCharge)
	}
	if event.Invoice.TenantID != "tenant-2" {
		t.Errorf("expected tenantID=tenant-2, got %s", event.Invoice.TenantID)
	}
}

func TestBillingMonthlySettleNoRecords(t *testing.T) {
	bs := NewBillingService(nil)
	ctx := context.Background()

	now := time.Now().UTC()
	event, err := bs.MonthlySettle(ctx, "tenant-x", "listing-x", now.Add(-30*24*time.Hour), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Invoice.TotalCharge != 0 {
		t.Errorf("expected zero charge for no records, got %f", event.Invoice.TotalCharge)
	}
	if event.Invoice.LineItems != 0 {
		t.Errorf("expected 0 line items, got %d", event.Invoice.LineItems)
	}
}

func TestBillingGetInvoices(t *testing.T) {
	bs := NewBillingService(nil)
	ctx := context.Background()

	pricing := PricingSpec{Model: PricingPerCall, Amount: 0.10}
	bs.RecordUsage(ctx, "listing-6", "tenant-3", pricing, 0)

	now := time.Now().UTC()
	bs.MonthlySettle(ctx, "tenant-3", "listing-6", now.Add(-1*time.Hour), now.Add(1*time.Hour))

	invoices := bs.GetInvoices(ctx, "tenant-3")
	if len(invoices) != 1 {
		t.Fatalf("expected 1 invoice, got %d", len(invoices))
	}
	if invoices[0].TenantID != "tenant-3" {
		t.Errorf("expected tenantID=tenant-3, got %s", invoices[0].TenantID)
	}
}

func TestBillingCostTrackerIntegration(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := cost.NewTracker(redis, reg)
	bs := NewBillingService(tracker)
	ctx := context.Background()

	// Record per_token usage
	pricing := PricingSpec{Model: PricingPerToken, Amount: 3.0} // $3 per 1M tokens
	_, err := bs.RecordUsage(ctx, "listing-7", "tenant-4", pricing, 2_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected charge: 2M / 1M * $3 = $6
	tenantKey := "aip:cost:tenant:tenant-4"
	if math.Abs(redis.data[tenantKey]-6.0) > 0.0001 {
		t.Errorf("expected redis counter=6.0, got %f", redis.data[tenantKey])
	}

	skillKey := "aip:cost:skill:listing-7"
	if math.Abs(redis.data[skillKey]-6.0) > 0.0001 {
		t.Errorf("expected redis skill counter=6.0, got %f", redis.data[skillKey])
	}
}
