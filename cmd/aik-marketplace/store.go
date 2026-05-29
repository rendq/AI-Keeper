package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Scope represents the visibility scope of a listing.
type Scope string

const (
	// ScopeTenant means the listing is only visible within the publishing tenant.
	ScopeTenant Scope = "tenant"
	// ScopeGlobal means the listing is visible to all tenants.
	ScopeGlobal Scope = "global"
)

// Phase represents the publish lifecycle phase of a listing.
type Phase string

const (
	PhaseDraft         Phase = "Draft"
	PhasePendingReview Phase = "PendingReview"
	PhasePublished     Phase = "Published"
	PhaseRejected      Phase = "Rejected"
)

// Listing is the domain model for a marketplace skill listing.
type Listing struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId"`
	Scope     Scope     `json:"scope"`
	Phase     Phase     `json:"phase"`
	Name      string    `json:"name"`
	SkillRef  string    `json:"skillRef"`
	Publisher string    `json:"publisher"`
	Category  string    `json:"category"`
	Tags      []string  `json:"tags,omitempty"`
	Readme    string    `json:"readme,omitempty"`
	Rating    float64   `json:"rating"`
	Downloads int64     `json:"downloads"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SearchParams defines the filter/search criteria for listing queries.
type SearchParams struct {
	TenantID string // caller's tenant — used for scope filtering
	Category string
	Tags     []string
	MinRating float64
	Query    string // full-text search query
}

// Store defines the persistence interface for listings.
type Store interface {
	Create(ctx context.Context, l *Listing) error
	Get(ctx context.Context, id string) (*Listing, error)
	Update(ctx context.Context, l *Listing) error
	Delete(ctx context.Context, id string) error
	Search(ctx context.Context, params SearchParams) ([]*Listing, error)
}

// MemoryStore is an in-memory implementation of Store for development and testing.
type MemoryStore struct {
	mu       sync.RWMutex
	listings map[string]*Listing
}

// NewMemoryStore returns a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{listings: make(map[string]*Listing)}
}

func (s *MemoryStore) Create(_ context.Context, l *Listing) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	l.CreatedAt = now
	l.UpdatedAt = now
	s.listings[l.ID] = l
	return nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (*Listing, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	l, ok := s.listings[id]
	if !ok {
		return nil, fmt.Errorf("listing not found: %s", id)
	}
	return l, nil
}

func (s *MemoryStore) Update(_ context.Context, l *Listing) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.listings[l.ID]; !ok {
		return fmt.Errorf("listing not found: %s", l.ID)
	}
	l.UpdatedAt = time.Now().UTC()
	s.listings[l.ID] = l
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.listings[id]; !ok {
		return fmt.Errorf("listing not found: %s", id)
	}
	delete(s.listings, id)
	return nil
}

func (s *MemoryStore) Search(_ context.Context, params SearchParams) ([]*Listing, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Listing
	for _, l := range s.listings {
		if !matchesScope(l, params.TenantID) {
			continue
		}
		if params.Category != "" && !strings.EqualFold(l.Category, params.Category) {
			continue
		}
		if len(params.Tags) > 0 && !hasAnyTag(l.Tags, params.Tags) {
			continue
		}
		if params.MinRating > 0 && l.Rating < params.MinRating {
			continue
		}
		if params.Query != "" && !matchesFullText(l, params.Query) {
			continue
		}
		results = append(results, l)
	}
	return results, nil
}

// matchesScope checks listing visibility: global listings are always visible;
// tenant-scoped listings are only visible to the owning tenant.
func matchesScope(l *Listing, callerTenantID string) bool {
	if l.Scope == ScopeGlobal {
		return true
	}
	// Tenant-scoped: only visible to the same tenant.
	return l.TenantID == callerTenantID
}

func hasAnyTag(listingTags, filterTags []string) bool {
	tagSet := make(map[string]struct{}, len(listingTags))
	for _, t := range listingTags {
		tagSet[strings.ToLower(t)] = struct{}{}
	}
	for _, t := range filterTags {
		if _, ok := tagSet[strings.ToLower(t)]; ok {
			return true
		}
	}
	return false
}

func matchesFullText(l *Listing, query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(l.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(l.Category), q) {
		return true
	}
	if strings.Contains(strings.ToLower(l.Readme), q) {
		return true
	}
	if strings.Contains(strings.ToLower(l.Publisher), q) {
		return true
	}
	for _, tag := range l.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}
