package sync

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockConnector implements Connector for testing.
type mockConnector struct {
	docs       []Document
	checkpoint string
}

func (m *mockConnector) FetchIncremental(_ context.Context, checkpoint string, _ int) (<-chan Document, error) {
	ch := make(chan Document, len(m.docs))
	go func() {
		defer close(ch)
		for _, d := range m.docs {
			ch <- d
		}
	}()
	return ch, nil
}

func (m *mockConnector) Checkpoint() string {
	return m.checkpoint
}

func TestIncrementalSync_FromCheckpoint(t *testing.T) {
	connector := &mockConnector{
		docs: []Document{
			{ID: "doc-1", Content: []byte("hello"), Action: ActionAdd},
			{ID: "doc-2", Content: []byte("world"), Action: ActionUpdate},
			{ID: "doc-3", Content: []byte(""), Action: ActionDelete},
		},
		checkpoint: "cursor-abc-123",
	}

	syncer := NewIncrementalSyncer(SyncConfig{
		WorkerCount:        2,
		BatchSize:          50,
		CheckpointInterval: 10 * time.Second,
	})
	syncer.RegisterConnector("ds/legal-docs", connector)

	ctx := context.Background()
	result, err := syncer.Sync(ctx, "ds/legal-docs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DocumentsProcessed != 3 {
		t.Errorf("expected 3 documents processed, got %d", result.DocumentsProcessed)
	}
	if result.DocumentsAdded != 1 {
		t.Errorf("expected 1 added, got %d", result.DocumentsAdded)
	}
	if result.DocumentsUpdated != 1 {
		t.Errorf("expected 1 updated, got %d", result.DocumentsUpdated)
	}
	if result.DocumentsDeleted != 1 {
		t.Errorf("expected 1 deleted, got %d", result.DocumentsDeleted)
	}
	if result.NewCheckpoint != "cursor-abc-123" {
		t.Errorf("expected checkpoint 'cursor-abc-123', got %q", result.NewCheckpoint)
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}

	// Verify state is persisted
	state := syncer.GetState("ds/legal-docs")
	if state.Checkpoint != "cursor-abc-123" {
		t.Errorf("state checkpoint mismatch: %q", state.Checkpoint)
	}
	if state.DocumentsSynced != 3 {
		t.Errorf("expected 3 documents synced in state, got %d", state.DocumentsSynced)
	}
	if state.LastSyncAt.IsZero() {
		t.Error("expected LastSyncAt to be set")
	}
}

func TestIncrementalSync_NoConnector(t *testing.T) {
	syncer := NewIncrementalSyncer(DefaultSyncConfig())

	_, err := syncer.Sync(context.Background(), "unknown-source")
	if err == nil {
		t.Fatal("expected error for unregistered connector")
	}
}

func TestIncrementalSync_ContextCancelled(t *testing.T) {
	// Connector that produces docs with a cancelled context
	connector := &cancelledContextConnector{}

	syncer := NewIncrementalSyncer(SyncConfig{
		WorkerCount:        2,
		BatchSize:          10,
		CheckpointInterval: 5 * time.Second,
	})
	syncer.RegisterConnector("ds/cancel", connector)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately before sync processes docs
	cancel()

	result, err := syncer.Sync(ctx, "ds/cancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have context cancellation error recorded or no docs processed
	// Either errors are recorded or processing stopped early
	if result.DocumentsProcessed == int64(len(connector.docs)) && len(result.Errors) == 0 {
		t.Error("expected either fewer docs processed or errors when context cancelled")
	}
}

func TestIncrementalSync_DocumentErrors(t *testing.T) {
	connector := &mockConnector{
		docs: []Document{
			{ID: "doc-ok", Content: []byte("good"), Action: ActionAdd},
			{ID: "doc-fail", Err: context.DeadlineExceeded},
			{ID: "doc-ok2", Content: []byte("also good"), Action: ActionAdd},
		},
		checkpoint: "cp-2",
	}

	syncer := NewIncrementalSyncer(DefaultSyncConfig())
	syncer.RegisterConnector("ds/mixed", connector)

	result, err := syncer.Sync(context.Background(), "ds/mixed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DocumentsProcessed != 2 {
		t.Errorf("expected 2 processed (skipping error doc), got %d", result.DocumentsProcessed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestGetState_Uninitialized(t *testing.T) {
	syncer := NewIncrementalSyncer(DefaultSyncConfig())
	state := syncer.GetState("nonexistent")
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Checkpoint != "" {
		t.Errorf("expected empty checkpoint, got %q", state.Checkpoint)
	}
	if !state.LastSyncAt.IsZero() {
		t.Error("expected zero LastSyncAt")
	}
}

func TestDefaultSyncConfig(t *testing.T) {
	cfg := DefaultSyncConfig()
	if cfg.WorkerCount != 4 {
		t.Errorf("expected default worker count 4, got %d", cfg.WorkerCount)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("expected default batch size 100, got %d", cfg.BatchSize)
	}
	if cfg.CheckpointInterval != 30*time.Second {
		t.Errorf("expected default checkpoint interval 30s, got %v", cfg.CheckpointInterval)
	}
}

// cancelledContextConnector produces many documents but expects the context to be cancelled.
type cancelledContextConnector struct {
	docs []Document
}

func (c *cancelledContextConnector) FetchIncremental(ctx context.Context, _ string, _ int) (<-chan Document, error) {
	ch := make(chan Document)
	// Generate docs but respect context cancellation
	c.docs = make([]Document, 100)
	go func() {
		defer close(ch)
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				return
			case ch <- Document{ID: fmt.Sprintf("doc-%d", i), Content: []byte("data"), Action: ActionAdd}:
			}
		}
	}()
	return ch, nil
}

func (c *cancelledContextConnector) Checkpoint() string {
	return "cancelled-cp"
}
