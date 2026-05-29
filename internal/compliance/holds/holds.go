// Package holds provides compliance hold management for audit data retention.
// A hold prevents audit events and associated S3 objects from being garbage
// collected or deleted, even if their normal retention period has expired.
package holds

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HoldStatus represents the lifecycle state of a compliance hold.
type HoldStatus string

const (
	StatusActive         HoldStatus = "active"
	StatusPendingRelease HoldStatus = "pending_release"
	StatusReleased       HoldStatus = "released"
)

// HoldScope defines the scope of data a hold applies to.
type HoldScope struct {
	Tenants   []string  `json:"tenants,omitempty"`
	TimeRange TimeRange `json:"timeRange,omitempty"`
	Query     string    `json:"query,omitempty"`
}

// TimeRange represents a start/end time window.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Hold represents a compliance hold applied to audit data.
type Hold struct {
	ID        string     `json:"id"`
	Reason    string     `json:"reason"`
	AppliedBy string     `json:"appliedBy"`
	AppliedAt time.Time  `json:"appliedAt"`
	Scope     HoldScope  `json:"scope"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Status    HoldStatus `json:"status"`
}

// ListFilter defines optional filters for listing holds.
type ListFilter struct {
	Status *HoldStatus
	Tenant *string
}

// Common errors returned by the holds package.
var (
	ErrNotFound       = errors.New("hold not found")
	ErrMissingReason  = errors.New("reason is required")
	ErrMissingApplier = errors.New("appliedBy is required")
	ErrMissingScope   = errors.New("scope must include at least one tenant or a query")
	ErrExpired        = errors.New("hold has already expired")
	ErrOverlap        = errors.New("hold overlaps with an existing active hold for the same scope")
)

// HoldStore defines the storage interface for compliance holds.
type HoldStore interface {
	Create(h *Hold) error
	Get(id string) (*Hold, error)
	List(filter ListFilter) ([]*Hold, error)
	Update(h *Hold) error
	Delete(id string) error
}

// MemoryStore is an in-memory implementation of HoldStore for testing.
type MemoryStore struct {
	mu    sync.RWMutex
	holds map[string]*Hold
}

// NewMemoryStore creates a new in-memory hold store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{holds: make(map[string]*Hold)}
}

func (s *MemoryStore) Create(h *Hold) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.holds[h.ID]; exists {
		return fmt.Errorf("hold %s already exists", h.ID)
	}
	// Store a copy to avoid external mutation.
	cp := *h
	s.holds[h.ID] = &cp
	return nil
}

func (s *MemoryStore) Get(id string) (*Hold, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.holds[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *h
	return &cp, nil
}

func (s *MemoryStore) List(filter ListFilter) ([]*Hold, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Hold
	for _, h := range s.holds {
		if filter.Status != nil && h.Status != *filter.Status {
			continue
		}
		if filter.Tenant != nil {
			found := false
			for _, t := range h.Scope.Tenants {
				if t == *filter.Tenant {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		cp := *h
		result = append(result, &cp)
	}
	return result, nil
}

func (s *MemoryStore) Update(h *Hold) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.holds[h.ID]; !ok {
		return ErrNotFound
	}
	cp := *h
	s.holds[h.ID] = &cp
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.holds[id]; !ok {
		return ErrNotFound
	}
	delete(s.holds, id)
	return nil
}

// NewHoldID generates a new unique hold identifier.
func NewHoldID() string {
	return uuid.New().String()
}
