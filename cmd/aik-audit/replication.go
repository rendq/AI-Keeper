// Package main provides cross-region audit replication.
//
// Audit events written to the primary region's ClickHouse + S3 are
// asynchronously replicated to backup region ClickHouse in batches,
// targeting < 60s replication lag.
//
// Requirements: C2.4, D1.2
package main

import (
	"context"
	"sync"
	"time"

	"github.com/ai-keeper/ai-keeper/dataplane/audit"
)

// ReplicationConfig configures the cross-region replicator.
type ReplicationConfig struct {
	TargetRegion       string
	ClickHouseEndpoint string
	BatchSize          int
	FlushInterval      time.Duration
	MaxLagSeconds      int
}

// ReplicationStatus reports the current state of replication to a target region.
type ReplicationStatus struct {
	Region        string
	LastSyncAt    time.Time
	LagSeconds    float64
	PendingEvents int
}

// ClickHouseReplicaWriter abstracts writes to the target region ClickHouse.
type ClickHouseReplicaWriter interface {
	Write(ctx context.Context, events []*audit.Event) error
	Close() error
}

// Replicator buffers audit events and asynchronously replicates them to a
// backup region ClickHouse instance.
type Replicator struct {
	config ReplicationConfig
	writer ClickHouseReplicaWriter

	mu         sync.Mutex
	buffer     []*audit.Event
	oldestAt   time.Time // timestamp of oldest unbatched event
	lastSyncAt time.Time

	stopCh chan struct{}
	done   chan struct{}
}

// NewReplicator creates a Replicator with the given config and writer.
func NewReplicator(config ReplicationConfig, writer ClickHouseReplicaWriter) *Replicator {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.MaxLagSeconds <= 0 {
		config.MaxLagSeconds = 60
	}
	return &Replicator{
		config: config,
		writer: writer,
		buffer: make([]*audit.Event, 0, config.BatchSize),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start begins the background flush loop. Call Stop to terminate.
func (r *Replicator) Start(ctx context.Context) {
	go r.flushLoop(ctx)
}

// Stop signals the flush loop to exit and waits for completion.
func (r *Replicator) Stop() {
	close(r.stopCh)
	<-r.done
}

// Replicate enqueues events for async batch replication. If the buffer reaches
// BatchSize, a flush is triggered immediately.
func (r *Replicator) Replicate(ctx context.Context, events []*audit.Event) error {
	r.mu.Lock()
	r.buffer = append(r.buffer, events...)
	if r.oldestAt.IsZero() && len(events) > 0 {
		r.oldestAt = time.Now()
	}
	shouldFlush := len(r.buffer) >= r.config.BatchSize
	r.mu.Unlock()

	if shouldFlush {
		return r.Flush(ctx)
	}
	return nil
}

// Flush forces all pending events to be written to the target region.
func (r *Replicator) Flush(ctx context.Context) error {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return nil
	}
	batch := r.buffer
	r.buffer = make([]*audit.Event, 0, r.config.BatchSize)
	r.oldestAt = time.Time{}
	r.mu.Unlock()

	if err := r.writer.Write(ctx, batch); err != nil {
		// On failure, put events back into buffer for retry.
		r.mu.Lock()
		r.buffer = append(batch, r.buffer...)
		if r.oldestAt.IsZero() {
			r.oldestAt = time.Now()
		}
		r.mu.Unlock()
		return err
	}

	r.mu.Lock()
	r.lastSyncAt = time.Now()
	r.mu.Unlock()
	return nil
}

// Status returns the current replication status for this target region.
func (r *Replicator) Status() ReplicationStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	var lag float64
	if !r.oldestAt.IsZero() {
		lag = time.Since(r.oldestAt).Seconds()
	}
	return ReplicationStatus{
		Region:        r.config.TargetRegion,
		LastSyncAt:    r.lastSyncAt,
		LagSeconds:    lag,
		PendingEvents: len(r.buffer),
	}
}

func (r *Replicator) flushLoop(ctx context.Context) {
	defer close(r.done)
	ticker := time.NewTicker(r.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush on shutdown
			_ = r.Flush(context.Background())
			return
		case <-r.stopCh:
			_ = r.Flush(context.Background())
			return
		case <-ticker.C:
			_ = r.Flush(ctx)
		}
	}
}
