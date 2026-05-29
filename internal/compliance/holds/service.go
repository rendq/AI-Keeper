package holds

import "time"

// HoldService provides business logic for compliance hold operations.
type HoldService struct {
	store HoldStore
	now   func() time.Time // injectable clock for testing
}

// NewHoldService creates a new HoldService with the given store.
func NewHoldService(store HoldStore) *HoldService {
	return &HoldService{store: store, now: time.Now}
}

// ApplyHold creates and persists a new compliance hold after validation.
func (s *HoldService) ApplyHold(reason, appliedBy string, scope HoldScope, expiresAt *time.Time) (*Hold, error) {
	if err := s.validate(reason, appliedBy, scope, expiresAt); err != nil {
		return nil, err
	}

	if err := s.checkOverlap(scope); err != nil {
		return nil, err
	}

	h := &Hold{
		ID:        NewHoldID(),
		Reason:    reason,
		AppliedBy: appliedBy,
		AppliedAt: s.now(),
		Scope:     scope,
		ExpiresAt: expiresAt,
		Status:    StatusActive,
	}

	if err := s.store.Create(h); err != nil {
		return nil, err
	}
	return h, nil
}

// GetHold retrieves a hold by ID.
func (s *HoldService) GetHold(id string) (*Hold, error) {
	return s.store.Get(id)
}

// ListHolds returns holds matching the optional filter.
func (s *HoldService) ListHolds(filter ListFilter) ([]*Hold, error) {
	return s.store.List(filter)
}

// ReleaseHold marks a hold as pending_release. Actual release requires
// approval workflow (handled by task 43.2).
func (s *HoldService) ReleaseHold(id string) (*Hold, error) {
	h, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}

	h.Status = StatusPendingRelease
	if err := s.store.Update(h); err != nil {
		return nil, err
	}
	return h, nil
}

// validate checks required fields and expiration.
func (s *HoldService) validate(reason, appliedBy string, scope HoldScope, expiresAt *time.Time) error {
	if reason == "" {
		return ErrMissingReason
	}
	if appliedBy == "" {
		return ErrMissingApplier
	}
	if len(scope.Tenants) == 0 && scope.Query == "" {
		return ErrMissingScope
	}
	if expiresAt != nil && expiresAt.Before(s.now()) {
		return ErrExpired
	}
	return nil
}

// checkOverlap checks whether an active hold already exists for the same scope.
// Overlap is detected when both holds target at least one common tenant and
// their time ranges intersect (or neither specifies a time range).
func (s *HoldService) checkOverlap(scope HoldScope) error {
	active := StatusActive
	existing, err := s.store.List(ListFilter{Status: &active})
	if err != nil {
		return err
	}

	for _, h := range existing {
		if scopesOverlap(h.Scope, scope) {
			return ErrOverlap
		}
	}
	return nil
}

// scopesOverlap returns true if two hold scopes overlap.
func scopesOverlap(a, b HoldScope) bool {
	// Check tenant overlap.
	if len(a.Tenants) > 0 && len(b.Tenants) > 0 {
		if !tenantsIntersect(a.Tenants, b.Tenants) {
			return false
		}
	} else if len(a.Tenants) == 0 && len(b.Tenants) == 0 {
		// Both use query-only scope — check query equality.
		if a.Query != b.Query {
			return false
		}
	} else {
		// One has tenants, one doesn't — no overlap by tenant dimension.
		return false
	}

	// Check time range overlap.
	aHasRange := !a.TimeRange.Start.IsZero() || !a.TimeRange.End.IsZero()
	bHasRange := !b.TimeRange.Start.IsZero() || !b.TimeRange.End.IsZero()

	if aHasRange && bHasRange {
		return timeRangesOverlap(a.TimeRange, b.TimeRange)
	}
	// If one or both have no time range, they overlap on the time dimension.
	return true
}

func tenantsIntersect(a, b []string) bool {
	set := make(map[string]struct{}, len(a))
	for _, t := range a {
		set[t] = struct{}{}
	}
	for _, t := range b {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}

func timeRangesOverlap(a, b TimeRange) bool {
	// Two ranges [s1,e1] and [s2,e2] overlap if s1 < e2 && s2 < e1.
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}
