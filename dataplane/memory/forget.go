package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ForgetRequest describes a GDPR-compliant memory deletion request.
type ForgetRequest struct {
	UserID       string
	IsolationKey string
	MemoryIDs    []string
	Reason       string
}

// ForgetResult reports outcomes of a Forget operation.
type ForgetResult struct {
	DeletedCount int
	Errors       []string
}

// Forget deletes specified memories or all memories for a given isolation key.
// If MemoryIDs is non-empty, only those specific IDs are deleted.
// If MemoryIDs is empty and IsolationKey is set, all memories for that isolation key are deleted.
func (m *LongTermMemory) Forget(ctx context.Context, req ForgetRequest) (*ForgetResult, error) {
	result := &ForgetResult{}

	if len(req.MemoryIDs) == 0 && req.IsolationKey == "" {
		// Nothing to delete.
		return result, nil
	}

	if len(req.MemoryIDs) > 0 {
		// Delete specific IDs.
		for _, id := range req.MemoryIDs {
			if err := m.store.Delete(ctx, id); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", id, err))
			} else {
				result.DeletedCount++
			}
		}
		return result, nil
	}

	// Delete all memories for the given isolation key.
	// Use a large limit to retrieve all entries.
	entries, err := m.ListByIsolation(ctx, req.IsolationKey, 10000)
	if err != nil {
		return nil, fmt.Errorf("listing memories for forget: %w", err)
	}

	for _, entry := range entries {
		if err := m.store.Delete(ctx, entry.ID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", entry.ID, err))
		} else {
			result.DeletedCount++
		}
	}

	return result, nil
}

// RetentionPolicy defines how long memories are kept before automatic expiry.
type RetentionPolicy struct {
	MaxAge              time.Duration
	EnforcementInterval time.Duration
}

// ScannableStore extends VectorStore with the ability to scan all entries.
// Required by RetentionEnforcer to find expired records.
type ScannableStore interface {
	VectorStore
	// ScanAll returns all stored record IDs and their metadata.
	ScanAll(ctx context.Context) ([]VectorSearchResult, error)
}

// RetentionEnforcer periodically scans and deletes expired memories.
type RetentionEnforcer struct {
	store  ScannableStore
	policy RetentionPolicy
	mu     sync.Mutex
}

// NewRetentionEnforcer creates a new enforcer that deletes memories older than MaxAge.
func NewRetentionEnforcer(store ScannableStore, policy RetentionPolicy) *RetentionEnforcer {
	return &RetentionEnforcer{
		store:  store,
		policy: policy,
	}
}

// Enforce performs a single pass to find and delete expired memories.
func (e *RetentionEnforcer) Enforce(ctx context.Context) (deletedCount int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	records, err := e.store.ScanAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("scanning for expired memories: %w", err)
	}

	now := time.Now()
	for _, rec := range records {
		tsStr, ok := rec.Metadata["_timestamp"]
		if !ok {
			continue
		}
		ts, parseErr := time.Parse(time.RFC3339Nano, tsStr)
		if parseErr != nil {
			continue
		}
		if now.Sub(ts) > e.policy.MaxAge {
			if delErr := e.store.Delete(ctx, rec.ID); delErr != nil {
				err = fmt.Errorf("deleting expired entry %s: %w", rec.ID, delErr)
			} else {
				deletedCount++
			}
		}
	}

	return deletedCount, err
}
