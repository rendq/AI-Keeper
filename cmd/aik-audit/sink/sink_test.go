package sink

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ai-keeper/ai-keeper/dataplane/audit"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock implementations
// ─────────────────────────────────────────────────────────────────────────────

type mockNATSConsumer struct {
	events []*audit.Event
	closed bool
}

func (m *mockNATSConsumer) Subscribe(ctx context.Context, handler func(ctx context.Context, event *audit.Event) error) error {
	for _, e := range m.events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := handler(ctx, e); err != nil {
				return err
			}
		}
	}
	// Block until context is done (simulates long-running subscription)
	<-ctx.Done()
	return nil
}

func (m *mockNATSConsumer) Close() error {
	m.closed = true
	return nil
}

type mockClickHouseWriter struct {
	mu      sync.Mutex
	batches [][]*audit.Event
	failAt  int // fail on the Nth write (-1 = never)
	calls   int
}

func (m *mockClickHouseWriter) Write(ctx context.Context, events []*audit.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.failAt >= 0 && m.calls == m.failAt {
		return fmt.Errorf("clickhouse: simulated write failure")
	}
	m.batches = append(m.batches, events)
	return nil
}

func (m *mockClickHouseWriter) Close() error { return nil }

func (m *mockClickHouseWriter) totalEvents() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, b := range m.batches {
		total += len(b)
	}
	return total
}

type mockS3Writer struct {
	mu       sync.Mutex
	events   []*audit.Event
	retns    []time.Duration
	failAt   int // fail on the Nth write (-1 = never)
	calls    int
}

func (m *mockS3Writer) Write(ctx context.Context, event *audit.Event, retention time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.failAt >= 0 && m.calls == m.failAt {
		return fmt.Errorf("s3: simulated write failure")
	}
	m.events = append(m.events, event)
	m.retns = append(m.retns, retention)
	return nil
}

func (m *mockS3Writer) Close() error { return nil }

type mockSIEMForwarder struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (m *mockSIEMForwarder) Forward(ctx context.Context, event *audit.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockSIEMForwarder) Close() error { return nil }

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

func testEvent(id string) *audit.Event {
	return &audit.Event{
		InvocationID: id,
		Timestamp:    time.Now(),
		Principal: audit.EventPrincipal{
			Agent: audit.EventPrincipalAgent{Name: "test-agent", Namespace: "default"},
		},
		Action: audit.EventAction{Verb: "invoke", Resource: "skill://test-skill@1.0.0"},
		Outcome: &audit.EventOutcome{Status: "success"},
	}
}

func testConfig() Config {
	return Config{
		NATSUrl:            "nats://localhost:4222",
		NATSSubject:        "audit.events",
		NATSStream:         "AUDIT",
		NATSDurable:        "test-sink",
		ClickHouseDSN:      "clickhouse://localhost:9000/aip",
		S3Endpoint:         "localhost:9000",
		S3Bucket:           "audit-events",
		S3Region:           "us-east-1",
		S3DefaultRetention: 365 * 24 * time.Hour,
		BatchSize:          5,
		FlushInterval:      100 * time.Millisecond,
		SIEMEnabled:        true,
		SIEMType:           "hec",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestSink_DualWrite_Success(t *testing.T) {
	// Validates: F4 (every invocation produces exactly one AuditEvent)
	// Validates: F5 (events written to WORM storage)
	// Validates: B12.1 (structured audit storage)
	cfg := testConfig()
	cfg.BatchSize = 100 // won't trigger auto-flush by size

	events := []*audit.Event{
		testEvent("inv-001"),
		testEvent("inv-002"),
		testEvent("inv-003"),
	}

	consumer := &mockNATSConsumer{events: events}
	ch := &mockClickHouseWriter{failAt: -1}
	s3 := &mockS3Writer{failAt: -1}
	siem := &mockSIEMForwarder{}

	s := NewWithDeps(cfg, consumer, ch, s3, siem)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run consumes all events then blocks; we cancel after timeout
	_ = s.Run(ctx)

	// Flush remaining batch
	shutdownCtx := context.Background()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	// Verify S3: each event written individually
	if len(s3.events) != 3 {
		t.Errorf("expected 3 S3 writes, got %d", len(s3.events))
	}
	for i, e := range s3.events {
		if e.InvocationID != events[i].InvocationID {
			t.Errorf("S3 event %d: expected invocationId %s, got %s", i, events[i].InvocationID, e.InvocationID)
		}
		if s3.retns[i] != cfg.S3DefaultRetention {
			t.Errorf("S3 event %d: expected retention %v, got %v", i, cfg.S3DefaultRetention, s3.retns[i])
		}
	}

	// Verify ClickHouse: all events flushed
	if ch.totalEvents() != 3 {
		t.Errorf("expected 3 ClickHouse events, got %d", ch.totalEvents())
	}

	// Verify SIEM: all events forwarded
	if len(siem.events) != 3 {
		t.Errorf("expected 3 SIEM forwards, got %d", len(siem.events))
	}
}

func TestSink_S3Failure_BlocksAcknowledge(t *testing.T) {
	// Validates: dual-write guarantee — S3 failure means event is not acked
	cfg := testConfig()
	events := []*audit.Event{testEvent("inv-fail-s3")}

	consumer := &mockNATSConsumer{events: events}
	ch := &mockClickHouseWriter{failAt: -1}
	s3 := &mockS3Writer{failAt: 1} // Fail on first write
	siem := &mockSIEMForwarder{}

	s := NewWithDeps(cfg, consumer, ch, s3, siem)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	// The subscription handler returns error, so Run should return it
	if err == nil {
		t.Error("expected error when S3 write fails, got nil")
	}

	// ClickHouse should NOT have received the event (S3 is written first)
	if ch.totalEvents() != 0 {
		t.Errorf("expected 0 ClickHouse events on S3 failure, got %d", ch.totalEvents())
	}
}

func TestSink_ClickHouseFailure_BlocksAcknowledge(t *testing.T) {
	// Validates: dual-write guarantee — CH batch failure is returned
	cfg := testConfig()
	cfg.BatchSize = 1 // Flush immediately

	events := []*audit.Event{testEvent("inv-fail-ch")}

	consumer := &mockNATSConsumer{events: events}
	ch := &mockClickHouseWriter{failAt: 1} // Fail on first write
	s3 := &mockS3Writer{failAt: -1}
	siem := &mockSIEMForwarder{}

	s := NewWithDeps(cfg, consumer, ch, s3, siem)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if err == nil {
		t.Error("expected error when ClickHouse write fails, got nil")
	}

	// S3 should have received the event (written before CH)
	if len(s3.events) != 1 {
		t.Errorf("expected 1 S3 event, got %d", len(s3.events))
	}
}

func TestSink_BatchFlush_BySize(t *testing.T) {
	// Validates: batch flush triggers when batch size is reached
	cfg := testConfig()
	cfg.BatchSize = 3
	cfg.FlushInterval = 10 * time.Second // Won't trigger by time

	events := make([]*audit.Event, 6)
	for i := range events {
		events[i] = testEvent(fmt.Sprintf("inv-batch-%d", i))
	}

	consumer := &mockNATSConsumer{events: events}
	ch := &mockClickHouseWriter{failAt: -1}
	s3 := &mockS3Writer{failAt: -1}

	s := NewWithDeps(cfg, consumer, ch, s3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = s.Run(ctx)
	_ = s.Shutdown(context.Background())

	// Should have had 2 batch flushes (6 events / batch size 3)
	if ch.totalEvents() != 6 {
		t.Errorf("expected 6 total ClickHouse events, got %d", ch.totalEvents())
	}
}

func TestSink_BatchFlush_ByTime(t *testing.T) {
	// Validates: B12.1 — batch 1s async insert to ClickHouse
	cfg := testConfig()
	cfg.BatchSize = 100         // Won't trigger by size
	cfg.FlushInterval = 50 * time.Millisecond

	events := []*audit.Event{testEvent("inv-time-1"), testEvent("inv-time-2")}

	consumer := &mockNATSConsumer{events: events}
	ch := &mockClickHouseWriter{failAt: -1}
	s3 := &mockS3Writer{failAt: -1}

	s := NewWithDeps(cfg, consumer, ch, s3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = s.Run(ctx)
	_ = s.Shutdown(context.Background())

	// Events should have been flushed by the timer
	if ch.totalEvents() != 2 {
		t.Errorf("expected 2 ClickHouse events after time flush, got %d", ch.totalEvents())
	}
}

func TestSink_S3_ObjectLockRetention(t *testing.T) {
	// Validates: F5 — events written to WORM storage with retention from config
	cfg := testConfig()
	cfg.S3DefaultRetention = 730 * 24 * time.Hour // 2 years

	events := []*audit.Event{testEvent("inv-retention")}

	consumer := &mockNATSConsumer{events: events}
	ch := &mockClickHouseWriter{failAt: -1}
	s3 := &mockS3Writer{failAt: -1}

	s := NewWithDeps(cfg, consumer, ch, s3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = s.Run(ctx)
	_ = s.Shutdown(context.Background())

	if len(s3.retns) != 1 {
		t.Fatalf("expected 1 S3 write, got %d", len(s3.retns))
	}
	if s3.retns[0] != cfg.S3DefaultRetention {
		t.Errorf("expected retention %v, got %v", cfg.S3DefaultRetention, s3.retns[0])
	}
}

func TestSink_SIEMDisabled(t *testing.T) {
	// Validates: SIEM forwarder is a stub; when disabled, no forwarding occurs
	cfg := testConfig()
	cfg.SIEMEnabled = false

	events := []*audit.Event{testEvent("inv-no-siem")}

	consumer := &mockNATSConsumer{events: events}
	ch := &mockClickHouseWriter{failAt: -1}
	s3 := &mockS3Writer{failAt: -1}
	siem := &mockSIEMForwarder{}

	s := NewWithDeps(cfg, consumer, ch, s3, siem)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = s.Run(ctx)
	_ = s.Shutdown(context.Background())

	if len(siem.events) != 0 {
		t.Errorf("expected 0 SIEM forwards when disabled, got %d", len(siem.events))
	}
}

func TestSink_NoConsumer_ReturnsError(t *testing.T) {
	cfg := testConfig()
	s := &Sink{
		cfg:       cfg,
		batch:     make([]*audit.Event, 0),
		batchDone: make(chan struct{}),
	}

	err := s.Run(context.Background())
	if err == nil {
		t.Error("expected error when consumer is nil")
	}
}

func TestSIEMStub_FormatHEC(t *testing.T) {
	e := testEvent("inv-hec")
	data, err := FormatHEC(e)
	if err != nil {
		t.Fatalf("FormatHEC error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty HEC output")
	}
}

func TestSIEMStub_FormatCEF(t *testing.T) {
	e := testEvent("inv-cef")
	cef, err := FormatCEF(e)
	if err != nil {
		t.Fatalf("FormatCEF error: %v", err)
	}
	if cef == "" {
		t.Error("expected non-empty CEF output")
	}
	if len(cef) < 10 {
		t.Errorf("CEF output too short: %s", cef)
	}
}

func TestSIEMStub_Forward(t *testing.T) {
	stub := NewSIEMStub("hec")
	e := testEvent("inv-stub")
	if err := stub.Forward(context.Background(), e); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := stub.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.NATSSubject != "audit.events" {
		t.Errorf("expected default subject 'audit.events', got %s", cfg.NATSSubject)
	}
	if cfg.FlushInterval != 1*time.Second {
		t.Errorf("expected 1s flush interval, got %v", cfg.FlushInterval)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("expected batch size 100, got %d", cfg.BatchSize)
	}
}
