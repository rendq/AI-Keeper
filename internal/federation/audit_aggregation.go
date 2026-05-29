package federation

import (
	"sync"
	"time"
)

// AuditEvent represents a simplified audit event from a linked cluster.
type AuditEvent struct {
	EventHash     string
	SourceCluster string
	Timestamp     time.Time
	Payload       []byte
}

// AuditFilter defines query criteria for audit events.
type AuditFilter struct {
	SourceCluster string
	Since         time.Time
	Until         time.Time
}

// AuditStore is the persistence interface for audit events.
type AuditStore interface {
	// Exists returns true if an event with the given hash already exists.
	Exists(eventHash string) bool
	// Write persists an audit event.
	Write(event AuditEvent) error
	// Query returns events matching the filter.
	Query(filter AuditFilter) ([]AuditEvent, error)
}

// InMemoryAuditStore is an in-memory implementation of AuditStore for testing.
type InMemoryAuditStore struct {
	mu     sync.RWMutex
	events []AuditEvent
	index  map[string]struct{}
}

// NewInMemoryAuditStore creates a new InMemoryAuditStore.
func NewInMemoryAuditStore() *InMemoryAuditStore {
	return &InMemoryAuditStore{
		events: make([]AuditEvent, 0),
		index:  make(map[string]struct{}),
	}
}

func (s *InMemoryAuditStore) Exists(eventHash string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.index[eventHash]
	return ok
}

func (s *InMemoryAuditStore) Write(event AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index[event.EventHash] = struct{}{}
	s.events = append(s.events, event)
	return nil
}

func (s *InMemoryAuditStore) Query(filter AuditFilter) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []AuditEvent
	for _, e := range s.events {
		if filter.SourceCluster != "" && e.SourceCluster != filter.SourceCluster {
			continue
		}
		if !filter.Since.IsZero() && e.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && e.Timestamp.After(filter.Until) {
			continue
		}
		results = append(results, e)
	}
	return results, nil
}

// AuditAggregator ingests and queries audit events with deduplication.
type AuditAggregator struct {
	store AuditStore
}

// NewAuditAggregator creates an AuditAggregator backed by the given store.
func NewAuditAggregator(store AuditStore) *AuditAggregator {
	return &AuditAggregator{store: store}
}

// Ingest writes events to the store, deduplicating by EventHash.
// Returns counts of newly ingested events and duplicates skipped.
func (a *AuditAggregator) Ingest(events []AuditEvent) (ingested int, duplicates int, err error) {
	for _, ev := range events {
		if a.store.Exists(ev.EventHash) {
			duplicates++
			continue
		}
		if err := a.store.Write(ev); err != nil {
			return ingested, duplicates, err
		}
		ingested++
	}
	return ingested, duplicates, nil
}

// Query retrieves events matching the given filter.
func (a *AuditAggregator) Query(filter AuditFilter) ([]AuditEvent, error) {
	return a.store.Query(filter)
}
