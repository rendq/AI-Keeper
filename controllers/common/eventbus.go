package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// DomainEventKind is the controlled vocabulary of cross-controller
// domain events broadcast on the AIP event bus (Requirement A6.5,
// design.md §3 / §11.1).
type DomainEventKind string

// Canonical domain event kinds. Task 3.1 specifies five mandatory
// values; we ship a couple of nearby helpers (`SkillRegistered`,
// `AgentScaledOut`) that subsequent controller tasks (3.2 / 3.4) will
// already need — keeping them here avoids breaking import cycles when
// those controllers wire up.
const (
	EventSkillRegistered   DomainEventKind = "SkillRegistered"
	EventSkillPromoted     DomainEventKind = "SkillPromoted"
	EventSkillDeprecated   DomainEventKind = "SkillDeprecated"
	EventPolicyDistributed DomainEventKind = "PolicyDistributed"
	EventAgentDeployed     DomainEventKind = "AgentDeployed"
	EventAgentRolledBack   DomainEventKind = "AgentRolledBack"
	EventAgentScaledOut    DomainEventKind = "AgentScaledOut"
)

// AllDomainEventKinds is the exhaustive list emitted by [DomainEventKind]
// constants above, useful for stream subject configuration and tests.
var AllDomainEventKinds = []DomainEventKind{
	EventSkillRegistered,
	EventSkillPromoted,
	EventSkillDeprecated,
	EventPolicyDistributed,
	EventAgentDeployed,
	EventAgentRolledBack,
	EventAgentScaledOut,
}

// DomainEvent is the JSON-serialisable envelope shared by every cross-
// controller signal. The `Subject` slot uses the platform's canonical
// [sharedv1alpha1.ResourceRef] so consumers can route by URI without
// reimplementing parser logic. `Payload` is intentionally
// `map[string]string` (not `interface{}`) to keep the over-the-wire
// shape stable and avoid encoding/decoding ambiguity in mixed-language
// consumers (Go controllers, Python skill runtime, TS console).
type DomainEvent struct {
	// Kind identifies the event family. MUST be one of the
	// [DomainEventKind] constants exported by this package.
	Kind DomainEventKind `json:"kind"`

	// Subject is the canonical reference of the resource that produced
	// the event (`skill://...`, `agent://...`, `policy://...`).
	Subject sharedv1alpha1.ResourceRef `json:"subject"`

	// Payload carries event-specific scalars. Keep keys CamelCase to
	// align with the rest of the AIP API surface.
	Payload map[string]string `json:"payload,omitempty"`

	// Timestamp records the producer-side wall-clock time the event was
	// minted. Consumers MUST tolerate skew; for ordering, rely on the
	// JetStream sequence number rather than this field.
	Timestamp time.Time `json:"timestamp"`

	// TraceID is the OpenTelemetry trace identifier (when available)
	// that lets operators stitch a domain event back to the originating
	// reconcile / data-plane invocation.
	TraceID string `json:"traceId,omitempty"`
}

// SubjectFor returns the NATS / JetStream subject that an event with the
// given kind is published on, given a subject prefix (e.g. `aip.events`
// in production). The mapping is `<prefix>.<kind>` after lower-casing
// the kind for consistency with NATS naming conventions.
func SubjectFor(prefix string, kind DomainEventKind) string {
	if prefix == "" {
		prefix = DefaultEventSubjectPrefix
	}
	return prefix + "." + strings.ToLower(string(kind))
}

// Errors that callers may wish to inspect.
var (
	// ErrEventBusClosed is returned by every method on a closed bus.
	ErrEventBusClosed = errors.New("eventbus: closed")
	// ErrInvalidDomainEvent is returned when a [DomainEvent] fails
	// pre-publish validation (missing kind, malformed subject, ...).
	ErrInvalidDomainEvent = errors.New("eventbus: invalid domain event")
)

// validate is the single pre-publish check we apply across all bus
// implementations to keep the wire format honest.
func (e *DomainEvent) validate() error {
	if e == nil {
		return fmt.Errorf("%w: nil event", ErrInvalidDomainEvent)
	}
	if e.Kind == "" {
		return fmt.Errorf("%w: missing kind", ErrInvalidDomainEvent)
	}
	if e.Subject == "" {
		return fmt.Errorf("%w: missing subject", ErrInvalidDomainEvent)
	}
	if !e.Subject.IsValid() {
		return fmt.Errorf("%w: subject %q does not match ResourceRef regex", ErrInvalidDomainEvent, e.Subject)
	}
	return nil
}

// EventBus is the abstraction every AIP controller depends on for
// broadcasting domain events. Producers should only `Publish`; consumers
// (e.g. the Agent controller watching `SkillPromoted`) wire up via the
// underlying transport directly because watch semantics differ between
// NATS / Kafka / no-op tests.
type EventBus interface {
	// Publish hands the event to the bus. Implementations SHOULD
	// timestamp `e.Timestamp` to the current wall clock when the field
	// is zero, and SHOULD return an error fast (sub-second) on transport
	// failure so reconciles do not stall.
	Publish(ctx context.Context, e DomainEvent) error

	// Close releases any backing transport resources. Subsequent
	// Publish calls return [ErrEventBusClosed].
	Close() error
}

// DefaultEventSubjectPrefix is the topic prefix used by AIP controllers
// in production (design.md §11.1 — `aip.events.<kind>`).
const DefaultEventSubjectPrefix = "aip.events"

// natsStreamName is the JetStream stream that backs the
// [DefaultEventSubjectPrefix] hierarchy. Operators may override the
// stream layout via Helm; the constant is exported so tests and
// integration scaffolding can reuse the canonical name.
const natsStreamName = "AIP_EVENTS"

// NATSJetStreamBus publishes [DomainEvent] envelopes onto a NATS
// JetStream stream. It is the production transport selected in
// design.md §3 / §11.1.
type NATSJetStreamBus struct {
	prefix  string
	timeout time.Duration

	// nc owns the network connection; js is the JetStream context.
	nc *nats.Conn
	js jetstream.JetStream

	closeOnce sync.Once
	closed    chan struct{}
}

// NATSBusOption configures [NewNATSJetStreamBus].
type NATSBusOption func(*natsBusConfig)

type natsBusConfig struct {
	prefix      string
	publishTO   time.Duration
	connectOpts []nats.Option
	streamName  string
	ensureStrm  bool
}

// WithSubjectPrefix overrides the `aip.events` default.
func WithSubjectPrefix(p string) NATSBusOption {
	return func(c *natsBusConfig) {
		if p != "" {
			c.prefix = p
		}
	}
}

// WithPublishTimeout overrides the per-call publish timeout (default: 2s).
func WithPublishTimeout(d time.Duration) NATSBusOption {
	return func(c *natsBusConfig) {
		if d > 0 {
			c.publishTO = d
		}
	}
}

// WithNATSOptions appends nats.Option values forwarded to nats.Connect.
func WithNATSOptions(opts ...nats.Option) NATSBusOption {
	return func(c *natsBusConfig) {
		c.connectOpts = append(c.connectOpts, opts...)
	}
}

// WithStreamName overrides the JetStream stream name (default `AIP_EVENTS`).
func WithStreamName(name string) NATSBusOption {
	return func(c *natsBusConfig) {
		if name != "" {
			c.streamName = name
		}
	}
}

// WithEnsureStream toggles automatic stream creation on connect. Defaults
// to false so production deployments declare streams via Helm. Tests
// using an embedded server should opt in.
func WithEnsureStream(ensure bool) NATSBusOption {
	return func(c *natsBusConfig) { c.ensureStrm = ensure }
}

// NewNATSJetStreamBus dials the supplied NATS URL and constructs a
// JetStream-backed EventBus. The default subject prefix is
// [DefaultEventSubjectPrefix]; pass [WithSubjectPrefix] / [WithPublishTimeout]
// / [WithNATSOptions] to override.
//
// Connection failures, JetStream initialisation errors and (when
// requested via [WithEnsureStream]) stream creation errors are returned
// to the caller so the controller manager can fail fast at startup.
func NewNATSJetStreamBus(url string, opts ...NATSBusOption) (*NATSJetStreamBus, error) {
	if url == "" {
		return nil, fmt.Errorf("eventbus: empty NATS URL")
	}
	cfg := &natsBusConfig{
		prefix:     DefaultEventSubjectPrefix,
		publishTO:  2 * time.Second,
		streamName: natsStreamName,
	}
	for _, o := range opts {
		o(cfg)
	}

	nc, err := nats.Connect(url, cfg.connectOpts...)
	if err != nil {
		return nil, fmt.Errorf("eventbus: nats connect %q: %w", url, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("eventbus: jetstream context: %w", err)
	}
	if cfg.ensureStrm {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.publishTO)
		defer cancel()
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:     cfg.streamName,
			Subjects: []string{cfg.prefix + ".>"},
			Storage:  jetstream.FileStorage,
		})
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("eventbus: ensure stream %q: %w", cfg.streamName, err)
		}
	}
	return &NATSJetStreamBus{
		prefix:  cfg.prefix,
		timeout: cfg.publishTO,
		nc:      nc,
		js:      js,
		closed:  make(chan struct{}),
	}, nil
}

// SubjectPrefix returns the `aip.events`-style prefix this bus
// publishes under. Useful for tests and for wiring consumer
// subscriptions.
func (b *NATSJetStreamBus) SubjectPrefix() string {
	return b.prefix
}

// Publish encodes `e` as JSON and writes it to
// `<prefix>.<lowercase-kind>` via JetStream. The call respects the
// publish timeout supplied through [WithPublishTimeout].
func (b *NATSJetStreamBus) Publish(ctx context.Context, e DomainEvent) error {
	select {
	case <-b.closed:
		return ErrEventBusClosed
	default:
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if err := e.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("eventbus: marshal: %w", err)
	}
	subject := SubjectFor(b.prefix, e.Kind)

	pubCtx := ctx
	if b.timeout > 0 {
		var cancel context.CancelFunc
		pubCtx, cancel = context.WithTimeout(ctx, b.timeout)
		defer cancel()
	}
	if _, err := b.js.Publish(pubCtx, subject, data); err != nil {
		return fmt.Errorf("eventbus: publish %s: %w", subject, err)
	}
	return nil
}

// Close drains the connection and signals subsequent Publish calls to
// fail with [ErrEventBusClosed]. It is safe to call more than once.
func (b *NATSJetStreamBus) Close() error {
	b.closeOnce.Do(func() {
		close(b.closed)
		if b.nc != nil {
			b.nc.Close()
		}
	})
	return nil
}

// NoopBus is a stand-in EventBus used by unit tests and dev clusters
// where NATS is not provisioned. It logs publishes through the
// supplied logger (when non-nil) and records every event for later
// assertion.
type NoopBus struct {
	mu     sync.Mutex
	log    logr.Logger
	events []DomainEvent
	closed bool
}

// NewNoopBus constructs a NoopBus. A zero [logr.Logger] disables
// logging; callers that want explicit log lines should pass a real
// logger such as `logr.Discard()`'s peer or controller-runtime's
// `log.Log`.
func NewNoopBus(log logr.Logger) *NoopBus {
	return &NoopBus{log: log}
}

// Publish records the event in memory. Implementations of [EventBus]
// must validate; doing so here means tests using NoopBus exercise the
// same field constraints they would in production.
func (b *NoopBus) Publish(_ context.Context, e DomainEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrEventBusClosed
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if err := e.validate(); err != nil {
		return err
	}
	b.events = append(b.events, e)
	if b.log.GetSink() != nil {
		b.log.V(1).Info("eventbus.noop.publish",
			"kind", e.Kind,
			"subject", e.Subject,
			"traceId", e.TraceID,
		)
	}
	return nil
}

// Events returns a copy of every event observed by the bus, in
// publication order. Useful for assertions in unit tests.
func (b *NoopBus) Events() []DomainEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]DomainEvent, len(b.events))
	copy(out, b.events)
	return out
}

// Reset drops the recorded events. The closed bit is left untouched.
func (b *NoopBus) Reset() {
	b.mu.Lock()
	b.events = nil
	b.mu.Unlock()
}

// Close marks the bus closed. Subsequent Publish calls return
// [ErrEventBusClosed].
func (b *NoopBus) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

// Compile-time interface assertions.
var (
	_ EventBus = (*NATSJetStreamBus)(nil)
	_ EventBus = (*NoopBus)(nil)
)
