package sink

import (
	"context"
	"time"

	"github.com/ai-keeper/ai-keeper/dataplane/audit"
)

// NATSConsumer abstracts NATS JetStream subscription for testability.
type NATSConsumer interface {
	// Subscribe starts consuming messages. The handler is called for each
	// message. The consumer must not acknowledge until the handler returns nil.
	Subscribe(ctx context.Context, handler func(ctx context.Context, event *audit.Event) error) error
	// Close gracefully shuts down the consumer.
	Close() error
}

// ClickHouseWriter abstracts ClickHouse async batch insert.
type ClickHouseWriter interface {
	// Write inserts a batch of events. Implementations should use async insert
	// for efficiency (flush every ~1s or N events).
	Write(ctx context.Context, events []*audit.Event) error
	// Close flushes pending writes and closes connections.
	Close() error
}

// S3Writer abstracts S3 PutObject with Object Lock Compliance Mode.
type S3Writer interface {
	// Write stores a single audit event with Object Lock (Compliance Mode).
	// retention specifies how long the object must be retained.
	Write(ctx context.Context, event *audit.Event, retention time.Duration) error
	// Close releases any resources.
	Close() error
}

// SIEMForwarder is a placeholder interface for SIEM integration (HEC/CEF).
// P0 implementation only logs; real Splunk/QRadar integration is P1.
type SIEMForwarder interface {
	// Forward sends an event to the SIEM system.
	Forward(ctx context.Context, event *audit.Event) error
	// Close releases resources.
	Close() error
}
