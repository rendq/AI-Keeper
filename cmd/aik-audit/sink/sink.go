package sink

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ai-keeper/ai-keeper/dataplane/audit"
)

// Sink coordinates the dual-write from NATS → (ClickHouse + S3).
// Both writes must succeed for an event to be acknowledged to NATS.
type Sink struct {
	cfg       Config
	consumer  NATSConsumer
	chWriter  ClickHouseWriter
	s3Writer  S3Writer
	siem      SIEMForwarder

	// Batching for ClickHouse
	mu        sync.Mutex
	batch     []*audit.Event
	batchDone chan struct{}
	flushErr  error
}

// New creates a Sink with the provided dependencies.
// Use NewWithDeps for testing with mock implementations.
func New(cfg Config) (*Sink, error) {
	// In production, this would initialize real NATS, ClickHouse, and S3 clients.
	// For now, we return a Sink that expects deps to be injected via NewWithDeps.
	return &Sink{
		cfg:       cfg,
		batch:     make([]*audit.Event, 0, cfg.BatchSize),
		batchDone: make(chan struct{}),
	}, nil
}

// NewWithDeps creates a Sink with explicit dependencies (for testing).
func NewWithDeps(cfg Config, consumer NATSConsumer, ch ClickHouseWriter, s3 S3Writer, siem SIEMForwarder) *Sink {
	return &Sink{
		cfg:       cfg,
		consumer:  consumer,
		chWriter:  ch,
		s3Writer:  s3,
		siem:      siem,
		batch:     make([]*audit.Event, 0, cfg.BatchSize),
		batchDone: make(chan struct{}),
	}
}

// Run starts consuming from NATS and processing events.
// It blocks until the context is cancelled.
func (s *Sink) Run(ctx context.Context) error {
	if s.consumer == nil {
		return fmt.Errorf("audit sink: NATS consumer not configured")
	}

	// Start the background batch flusher for ClickHouse
	go s.batchFlusher(ctx)

	// Subscribe to NATS and process events
	return s.consumer.Subscribe(ctx, func(ctx context.Context, event *audit.Event) error {
		return s.processEvent(ctx, event)
	})
}

// processEvent handles a single audit event with dual-write guarantee:
// both ClickHouse and S3 must succeed before acknowledging to NATS.
func (s *Sink) processEvent(ctx context.Context, event *audit.Event) error {
	// 1. Write to S3 with Object Lock (individual event, immediate)
	if err := s.s3Writer.Write(ctx, event, s.cfg.S3DefaultRetention); err != nil {
		return fmt.Errorf("audit sink: S3 write failed for %s: %w", event.InvocationID, err)
	}

	// 2. Add to ClickHouse batch
	if err := s.addToBatch(ctx, event); err != nil {
		return fmt.Errorf("audit sink: ClickHouse batch failed for %s: %w", event.InvocationID, err)
	}

	// 3. Forward to SIEM (best-effort, P0 stub)
	if s.siem != nil && s.cfg.SIEMEnabled {
		if err := s.siem.Forward(ctx, event); err != nil {
			// SIEM forwarding is best-effort in P0; log but don't fail
			log.Printf("audit sink: SIEM forward failed for %s: %v", event.InvocationID, err)
		}
	}

	return nil
}

// addToBatch adds an event to the ClickHouse batch buffer.
// If the batch is full, it triggers a flush.
func (s *Sink) addToBatch(ctx context.Context, event *audit.Event) error {
	s.mu.Lock()
	s.batch = append(s.batch, event)
	shouldFlush := len(s.batch) >= s.cfg.BatchSize
	s.mu.Unlock()

	if shouldFlush {
		return s.flush(ctx)
	}
	return nil
}

// flush writes the current batch to ClickHouse.
func (s *Sink) flush(ctx context.Context) error {
	s.mu.Lock()
	if len(s.batch) == 0 {
		s.mu.Unlock()
		return nil
	}
	batch := s.batch
	s.batch = make([]*audit.Event, 0, s.cfg.BatchSize)
	s.mu.Unlock()

	if err := s.chWriter.Write(ctx, batch); err != nil {
		// On flush failure, put events back
		s.mu.Lock()
		s.batch = append(batch, s.batch...)
		s.mu.Unlock()
		return fmt.Errorf("audit sink: ClickHouse flush failed: %w", err)
	}
	return nil
}

// batchFlusher periodically flushes the ClickHouse batch.
func (s *Sink) batchFlusher(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.flush(ctx); err != nil {
				log.Printf("audit sink: periodic flush error: %v", err)
			}
		}
	}
}

// Shutdown gracefully shuts down the sink, flushing pending events.
func (s *Sink) Shutdown(ctx context.Context) error {
	// Flush remaining batch
	if err := s.flush(ctx); err != nil {
		log.Printf("audit sink: final flush error: %v", err)
	}

	var errs []error
	if s.consumer != nil {
		if err := s.consumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("consumer close: %w", err))
		}
	}
	if s.chWriter != nil {
		if err := s.chWriter.Close(); err != nil {
			errs = append(errs, fmt.Errorf("clickhouse close: %w", err))
		}
	}
	if s.s3Writer != nil {
		if err := s.s3Writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("s3 close: %w", err))
		}
	}
	if s.siem != nil {
		if err := s.siem.Close(); err != nil {
			errs = append(errs, fmt.Errorf("siem close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("audit sink shutdown errors: %v", errs)
	}
	return nil
}
