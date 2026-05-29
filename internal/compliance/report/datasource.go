package report

import (
	"context"
	"time"
)

// AuditStats contains aggregated audit statistics from ClickHouse.
type AuditStats struct {
	TotalEvents     int64
	SuccessCount    int64
	FailureCount    int64
	UniqueUsers     int
	TopActions      []ActionCount
	PeriodStart     time.Time
	PeriodEnd       time.Time
}

// ActionCount represents an action and its occurrence count.
type ActionCount struct {
	Action string
	Count  int64
}

// ResourceScanResult contains results from a Kubernetes resource scan.
type ResourceScanResult struct {
	TotalResources    int
	CompliantCount    int
	NonCompliantCount int
	Resources         []ScannedResource
	ScanTime          time.Time
}

// ScannedResource represents a single scanned Kubernetes resource.
type ScannedResource struct {
	Kind       string
	Name       string
	Namespace  string
	Compliant  bool
	Issues     []string
}

// AuditDataSource defines the interface for querying audit statistics.
type AuditDataSource interface {
	QueryAuditStats(ctx context.Context, tenantID string, period Period) (*AuditStats, error)
}

// ResourceScanner defines the interface for scanning Kubernetes resources.
type ResourceScanner interface {
	ScanResources(ctx context.Context, tenantID string) (*ResourceScanResult, error)
}

// MockAuditDataSource is a mock implementation of AuditDataSource for testing.
type MockAuditDataSource struct {
	Stats *AuditStats
	Err   error
}

// QueryAuditStats returns the preconfigured mock stats or error.
func (m *MockAuditDataSource) QueryAuditStats(_ context.Context, _ string, _ Period) (*AuditStats, error) {
	return m.Stats, m.Err
}

// MockResourceScanner is a mock implementation of ResourceScanner for testing.
type MockResourceScanner struct {
	Result *ResourceScanResult
	Err    error
}

// ScanResources returns the preconfigured mock result or error.
func (m *MockResourceScanner) ScanResources(_ context.Context, _ string) (*ResourceScanResult, error) {
	return m.Result, m.Err
}
