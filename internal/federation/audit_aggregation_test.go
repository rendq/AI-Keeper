package federation

import (
	"testing"
	"time"
)

func TestAuditAggregation_IngestSuccess(t *testing.T) {
	store := NewInMemoryAuditStore()
	agg := NewAuditAggregator(store)

	events := []AuditEvent{
		{EventHash: "aaa", SourceCluster: "cluster-a", Timestamp: time.Now(), Payload: []byte("ev1")},
		{EventHash: "bbb", SourceCluster: "cluster-b", Timestamp: time.Now(), Payload: []byte("ev2")},
	}

	ingested, duplicates, err := agg.Ingest(events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ingested != 2 {
		t.Fatalf("expected 2 ingested, got %d", ingested)
	}
	if duplicates != 0 {
		t.Fatalf("expected 0 duplicates, got %d", duplicates)
	}
}

func TestAuditAggregation_Deduplication(t *testing.T) {
	store := NewInMemoryAuditStore()
	agg := NewAuditAggregator(store)

	events := []AuditEvent{
		{EventHash: "aaa", SourceCluster: "cluster-a", Timestamp: time.Now(), Payload: []byte("ev1")},
	}

	// First ingest
	agg.Ingest(events)

	// Second ingest with same hash
	ingested, duplicates, err := agg.Ingest(events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ingested != 0 {
		t.Fatalf("expected 0 ingested on duplicate, got %d", ingested)
	}
	if duplicates != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicates)
	}

	// Verify only one event stored
	all, _ := agg.Query(AuditFilter{})
	if len(all) != 1 {
		t.Fatalf("expected 1 event in store, got %d", len(all))
	}
}

func TestAuditAggregation_QueryByCluster(t *testing.T) {
	store := NewInMemoryAuditStore()
	agg := NewAuditAggregator(store)

	now := time.Now()
	events := []AuditEvent{
		{EventHash: "a1", SourceCluster: "cluster-a", Timestamp: now, Payload: []byte("a1")},
		{EventHash: "b1", SourceCluster: "cluster-b", Timestamp: now, Payload: []byte("b1")},
		{EventHash: "a2", SourceCluster: "cluster-a", Timestamp: now, Payload: []byte("a2")},
	}
	agg.Ingest(events)

	results, err := agg.Query(AuditFilter{SourceCluster: "cluster-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 events for cluster-a, got %d", len(results))
	}
	for _, r := range results {
		if r.SourceCluster != "cluster-a" {
			t.Fatalf("expected source_cluster cluster-a, got %s", r.SourceCluster)
		}
	}
}

func TestAuditAggregation_QueryByTimeRange(t *testing.T) {
	store := NewInMemoryAuditStore()
	agg := NewAuditAggregator(store)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []AuditEvent{
		{EventHash: "t1", SourceCluster: "c1", Timestamp: base, Payload: []byte("t1")},
		{EventHash: "t2", SourceCluster: "c1", Timestamp: base.Add(1 * time.Hour), Payload: []byte("t2")},
		{EventHash: "t3", SourceCluster: "c1", Timestamp: base.Add(2 * time.Hour), Payload: []byte("t3")},
		{EventHash: "t4", SourceCluster: "c1", Timestamp: base.Add(3 * time.Hour), Payload: []byte("t4")},
	}
	agg.Ingest(events)

	// Query events between hour 1 and hour 2 inclusive
	results, err := agg.Query(AuditFilter{
		Since: base.Add(1 * time.Hour),
		Until: base.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 events in time range, got %d", len(results))
	}
}
