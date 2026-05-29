// Package sync provides incremental synchronization for knowledge base data sources.
// It manages checkpoints and tracks sync state to avoid re-processing unchanged documents.
package sync

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SyncConfig holds configuration for the incremental syncer.
type SyncConfig struct {
	// WorkerCount is the number of parallel connector workers.
	WorkerCount int
	// BatchSize is the maximum number of documents to process per batch.
	BatchSize int
	// CheckpointInterval is how often the sync state is persisted.
	CheckpointInterval time.Duration
}

// DefaultSyncConfig returns a SyncConfig with sensible defaults.
func DefaultSyncConfig() SyncConfig {
	return SyncConfig{
		WorkerCount:        4,
		BatchSize:          100,
		CheckpointInterval: 30 * time.Second,
	}
}

// SyncState tracks the state of a data source sync operation.
type SyncState struct {
	// LastSyncAt is the timestamp of the last successful sync.
	LastSyncAt time.Time
	// Checkpoint is an opaque cursor/token used to resume from where we left off.
	Checkpoint string
	// DocumentsSynced is the total number of documents synced in the last run.
	DocumentsSynced int64
	// Errors contains any errors encountered during the last sync.
	Errors []string
}

// SyncResult contains the outcome of an incremental sync operation.
type SyncResult struct {
	// DocumentsProcessed is the number of documents processed in this sync run.
	DocumentsProcessed int64
	// DocumentsAdded is the number of new documents added.
	DocumentsAdded int64
	// DocumentsUpdated is the number of existing documents updated.
	DocumentsUpdated int64
	// DocumentsDeleted is the number of documents deleted.
	DocumentsDeleted int64
	// NewCheckpoint is the checkpoint to resume from on the next sync.
	NewCheckpoint string
	// Duration is how long the sync took.
	Duration time.Duration
	// Errors contains any non-fatal errors encountered.
	Errors []string
}

// Connector defines the interface for a data source connector.
// Implementations handle protocol-specific details (S3, HTTP, database, etc.).
type Connector interface {
	// FetchIncremental fetches documents changed since the given checkpoint.
	// Returns a channel of documents and a new checkpoint.
	FetchIncremental(ctx context.Context, checkpoint string, batchSize int) (<-chan Document, error)
	// Checkpoint returns the current checkpoint value after fetching completes.
	Checkpoint() string
}

// Document represents a single document from a data source.
type Document struct {
	// ID uniquely identifies this document within the data source.
	ID string
	// Content is the raw document content.
	Content []byte
	// Metadata holds key-value pairs associated with the document.
	Metadata map[string]string
	// Action indicates whether this document was added, updated, or deleted.
	Action DocumentAction
	// Err is set if there was an error fetching this specific document.
	Err error
}

// DocumentAction represents the type of change for a document.
type DocumentAction string

const (
	ActionAdd    DocumentAction = "add"
	ActionUpdate DocumentAction = "update"
	ActionDelete DocumentAction = "delete"
)

// IncrementalSyncer orchestrates incremental sync operations for data sources.
type IncrementalSyncer struct {
	config     SyncConfig
	pool       *ConnectorWorkerPool
	states     map[string]*SyncState
	connectors map[string]Connector
	mu         sync.RWMutex
}

// NewIncrementalSyncer creates a new IncrementalSyncer with the given config.
func NewIncrementalSyncer(config SyncConfig) *IncrementalSyncer {
	if config.WorkerCount <= 0 {
		config.WorkerCount = DefaultSyncConfig().WorkerCount
	}
	if config.BatchSize <= 0 {
		config.BatchSize = DefaultSyncConfig().BatchSize
	}
	if config.CheckpointInterval <= 0 {
		config.CheckpointInterval = DefaultSyncConfig().CheckpointInterval
	}

	return &IncrementalSyncer{
		config:     config,
		pool:       NewConnectorWorkerPool(config.WorkerCount),
		states:     make(map[string]*SyncState),
		connectors: make(map[string]Connector),
	}
}

// RegisterConnector registers a connector for a data source reference.
func (s *IncrementalSyncer) RegisterConnector(dataSourceRef string, connector Connector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectors[dataSourceRef] = connector
}

// Sync performs an incremental sync for the specified data source.
// It resumes from the last known checkpoint and processes documents in batches.
func (s *IncrementalSyncer) Sync(ctx context.Context, dataSourceRef string) (*SyncResult, error) {
	s.mu.RLock()
	connector, ok := s.connectors[dataSourceRef]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no connector registered for data source %q", dataSourceRef)
	}

	// Get current state (or initialize)
	state := s.GetState(dataSourceRef)
	checkpoint := state.Checkpoint

	start := time.Now()
	result := &SyncResult{}

	// Fetch documents incrementally from the connector
	docChan, err := connector.FetchIncremental(ctx, checkpoint, s.config.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("fetch incremental for %q: %w", dataSourceRef, err)
	}

	// Process documents
	for doc := range docChan {
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("context cancelled: %v", ctx.Err()))
			break
		}

		if doc.Err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("document %s: %v", doc.ID, doc.Err))
			continue
		}

		result.DocumentsProcessed++
		switch doc.Action {
		case ActionAdd:
			result.DocumentsAdded++
		case ActionUpdate:
			result.DocumentsUpdated++
		case ActionDelete:
			result.DocumentsDeleted++
		}
	}

	// Update checkpoint from connector
	result.NewCheckpoint = connector.Checkpoint()
	result.Duration = time.Since(start)

	// Persist state
	s.mu.Lock()
	s.states[dataSourceRef] = &SyncState{
		LastSyncAt:      time.Now(),
		Checkpoint:      result.NewCheckpoint,
		DocumentsSynced: result.DocumentsProcessed,
		Errors:          result.Errors,
	}
	s.mu.Unlock()

	return result, nil
}

// GetState returns the current sync state for a data source.
// Returns a zero-value state if no sync has been performed yet.
func (s *IncrementalSyncer) GetState(dataSourceRef string) *SyncState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state, ok := s.states[dataSourceRef]; ok {
		return state
	}
	return &SyncState{}
}
