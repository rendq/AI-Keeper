package holds

import "time"

// RetentionPolicy defines how long data should be retained for a tenant.
type RetentionPolicy struct {
	RetentionDays int
	Tenant        string
}

// HoldEligibility represents the GC eligibility status of an object.
type HoldEligibility string

const (
	HoldStatusHeld     HoldEligibility = "held"
	HoldStatusEligible HoldEligibility = "eligible"
	HoldStatusDeleted  HoldEligibility = "deleted"
)

// GCCandidate represents an object being evaluated for garbage collection.
type GCCandidate struct {
	ObjectKey  string
	Tenant     string
	CreatedAt  time.Time
	HoldStatus HoldEligibility
}

// GCResult records the outcome of a GC evaluation for one object.
type GCResult struct {
	ObjectKey string
	Deleted   bool
	Reason    string
}

// RetentionGC integrates hold checks into the retention garbage collection process.
type RetentionGC struct {
	store HoldStore
	now   func() time.Time
}

// NewRetentionGC creates a RetentionGC with the given hold store.
func NewRetentionGC(store HoldStore) *RetentionGC {
	return &RetentionGC{store: store, now: time.Now}
}

// ShouldDelete determines whether a GC candidate can be deleted.
// It returns (true, "") if deletion is allowed, or (false, reason) if blocked.
func (gc *RetentionGC) ShouldDelete(candidate GCCandidate, policy RetentionPolicy) (bool, string) {
	// Check retention expiry first.
	expiry := candidate.CreatedAt.Add(time.Duration(policy.RetentionDays) * 24 * time.Hour)
	if gc.now().Before(expiry) {
		return false, "retention_not_expired"
	}

	// Check for active holds covering this tenant.
	if gc.hasActiveHold(candidate) {
		return false, "held"
	}

	return true, ""
}

// ProcessBatch evaluates a batch of GC candidates against the retention policy.
func (gc *RetentionGC) ProcessBatch(candidates []GCCandidate, policy RetentionPolicy) []GCResult {
	results := make([]GCResult, 0, len(candidates))
	for _, c := range candidates {
		canDelete, reason := gc.ShouldDelete(c, policy)
		if canDelete {
			results = append(results, GCResult{ObjectKey: c.ObjectKey, Deleted: true, Reason: "expired"})
		} else {
			results = append(results, GCResult{ObjectKey: c.ObjectKey, Deleted: false, Reason: reason})
		}
	}
	return results
}

// hasActiveHold checks whether any active hold covers the candidate's tenant and time.
func (gc *RetentionGC) hasActiveHold(candidate GCCandidate) bool {
	active := StatusActive
	tenant := candidate.Tenant
	holds, err := gc.store.List(ListFilter{Status: &active, Tenant: &tenant})
	if err != nil {
		return false
	}

	for _, h := range holds {
		// If the hold has a time range, check that the candidate falls within it.
		hasRange := !h.Scope.TimeRange.Start.IsZero() || !h.Scope.TimeRange.End.IsZero()
		if hasRange {
			if candidate.CreatedAt.Before(h.Scope.TimeRange.Start) || candidate.CreatedAt.After(h.Scope.TimeRange.End) {
				continue
			}
		}
		// Active hold covers this candidate.
		return true
	}
	return false
}
