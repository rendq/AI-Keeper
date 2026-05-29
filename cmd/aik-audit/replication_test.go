package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ai-keeper/ai-keeper/dataplane/audit"
)

// mockReplicaWriter records all writes for assertions.
type mockReplicaWriter struct {
	mu      sync.Mutex
	batches [][]*audit.Event
}

func (m *mockReplicaWriter) Write(_ context.Context, events []*audit.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches = append(m.batches, events)
	return nil
}

func (m *mockReplicaWriter) Close() error { return nil }

func (m *mockReplicaWriter) totalEvents() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, b := range m.batches {
		total += len(b)
	}
	return total
}

func newTestEvent(id string) *audit.Event {
	return &audit.Event{
		InvocationID: id,
		Timestamp:    time.Now(),
		Principal:    audit.EventPrincipal{Agent: audit.EventPrincipalAgent{Name: "test"}},
		Action:       audit.EventAction{Verb: "invoke", Resource: "model"},
	}
}

func TestCrossRegionReplication(t *testing.T) {
	t.Run("replication writes events to target", func(t *testing.T) {
		w := &mockReplicaWriter{}
		r := NewReplicator(ReplicationConfig{
			TargetRegion:       "eu-west",
			ClickHouseEndpoint: "clickhouse://eu-west:9000",
			BatchSize:          10,
			FlushInterval:      time.Hour, // won't auto-flush in test
		}, w)

		ctx := context.Background()
		events := []*audit.Event{newTestEvent("e1"), newTestEvent("e2")}
		if err := r.Replicate(ctx, events); err != nil {
			t.Fatalf("Replicate: %v", err)
		}
		if err := r.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		if got := w.totalEvents(); got != 2 {
			t.Errorf("expected 2 events written, got %d", got)
		}
	})

	t.Run("batch accumulation and flush on size threshold", func(t *testing.T) {
		w := &mockReplicaWriter{}
		r := NewReplicator(ReplicationConfig{
			TargetRegion:       "eu-west",
			ClickHouseEndpoint: "clickhouse://eu-west:9000",
			BatchSize:          3,
			FlushInterval:      time.Hour,
		}, w)

		ctx := context.Background()
		// Send 3 events — should auto-flush because BatchSize=3
		events := []*audit.Event{newTestEvent("a"), newTestEvent("b"), newTestEvent("c")}
		if err := r.Replicate(ctx, events); err != nil {
			t.Fatalf("Replicate: %v", err)
		}

		if got := w.totalEvents(); got != 3 {
			t.Errorf("expected 3 events auto-flushed, got %d", got)
		}

		// Buffer should be empty
		status := r.Status()
		if status.PendingEvents != 0 {
			t.Errorf("expected 0 pending, got %d", status.PendingEvents)
		}
	})

	t.Run("status reports correct lag", func(t *testing.T) {
		w := &mockReplicaWriter{}
		r := NewReplicator(ReplicationConfig{
			TargetRegion:       "eu-west",
			ClickHouseEndpoint: "clickhouse://eu-west:9000",
			BatchSize:          100,
			FlushInterval:      time.Hour,
		}, w)

		// No events — lag should be 0
		status := r.Status()
		if status.LagSeconds != 0 {
			t.Errorf("expected 0 lag with no events, got %f", status.LagSeconds)
		}

		// Add events and wait a bit
		ctx := context.Background()
		_ = r.Replicate(ctx, []*audit.Event{newTestEvent("x")})
		time.Sleep(50 * time.Millisecond)

		status = r.Status()
		if status.LagSeconds < 0.04 {
			t.Errorf("expected lag >= 40ms, got %f s", status.LagSeconds)
		}
		if status.PendingEvents != 1 {
			t.Errorf("expected 1 pending, got %d", status.PendingEvents)
		}
		if status.Region != "eu-west" {
			t.Errorf("expected region eu-west, got %s", status.Region)
		}
	})

	t.Run("flush forces sync", func(t *testing.T) {
		w := &mockReplicaWriter{}
		r := NewReplicator(ReplicationConfig{
			TargetRegion:       "us-west",
			ClickHouseEndpoint: "clickhouse://us-west:9000",
			BatchSize:          100,
			FlushInterval:      time.Hour,
		}, w)

		ctx := context.Background()
		_ = r.Replicate(ctx, []*audit.Event{newTestEvent("f1"), newTestEvent("f2")})

		// Not yet flushed
		if got := w.totalEvents(); got != 0 {
			t.Fatalf("expected 0 before flush, got %d", got)
		}

		if err := r.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		if got := w.totalEvents(); got != 2 {
			t.Errorf("expected 2 after flush, got %d", got)
		}

		status := r.Status()
		if status.LastSyncAt.IsZero() {
			t.Error("expected LastSyncAt to be set after flush")
		}
		if status.PendingEvents != 0 {
			t.Errorf("expected 0 pending after flush, got %d", status.PendingEvents)
		}
	})

	t.Run("empty batch is no-op", func(t *testing.T) {
		w := &mockReplicaWriter{}
		r := NewReplicator(ReplicationConfig{
			TargetRegion:       "ap-east",
			ClickHouseEndpoint: "clickhouse://ap-east:9000",
			BatchSize:          10,
			FlushInterval:      time.Hour,
		}, w)

		ctx := context.Background()
		if err := r.Flush(ctx); err != nil {
			t.Fatalf("Flush empty: %v", err)
		}

		w.mu.Lock()
		batchCount := len(w.batches)
		w.mu.Unlock()

		if batchCount != 0 {
			t.Errorf("expected 0 write calls for empty flush, got %d", batchCount)
		}
	})
}
