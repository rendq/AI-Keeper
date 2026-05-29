package modelrouter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Instance identifies a single Model_Router replica registered with
// the control plane. The fields mirror the gRPC discovery protocol
// task 11.1 will own; for P0 they double as a stable map key.
type Instance struct {
	// ID is the stable identifier of the router instance (typically
	// `<namespace>/<pod-name>` or a service-account hash).
	ID string

	// Address is the dial-string the controller would use to push
	// updates (e.g. `aik-router.aik-system.svc.cluster.local:9090`).
	// Unused in P0 but kept for parity with the future gRPC client.
	Address string
}

// RouterPusher is the abstraction over the Model_Router fleet. The
// controller calls [Discover] to enumerate live instances and [Push]
// to deliver compiled routing tables. Real implementations land in
// task 11.1 — for P0 this package ships [MemoryRouterPusher] so the
// reconciler is exercisable in unit tests.
//
// Implementations MUST be idempotent: every call may be retried by
// the controller-runtime workqueue.
type RouterPusher interface {
	// Discover returns the set of router instances currently
	// registered with the control plane. An empty slice + nil error
	// means "no instances yet" — the reconciler treats this as
	// `Distributed=Unknown reason=NoInstances`.
	Discover(ctx context.Context) ([]Instance, error)

	// Push delivers the compiled routing table to the addressed
	// router instance. A non-nil error counts as a transport failure
	// and triggers a backoff requeue.
	Push(ctx context.Context, instance Instance, table *RoutingTable) error
}

// MemoryRouterPusher is the in-memory [RouterPusher] used by unit
// tests and dev clusters where no real router fleet exists yet.
type MemoryRouterPusher struct {
	mu sync.Mutex

	// Instances is the set of pre-registered instances returned by
	// [MemoryRouterPusher.Discover]. May be empty (no router yet).
	Instances []Instance

	// PushErr lets tests inject a transport failure for [Push].
	PushErr error

	// DiscoverErr lets tests inject a transport failure for
	// [Discover].
	DiscoverErr error

	// Tables records the most recent table pushed per instance. The
	// outer key is `Instance.ID`, the inner value is the
	// `RoutingTable.Hash`. Useful for assertions in tests.
	Tables map[string]string

	// PushCalls counts the number of successful Push invocations.
	PushCalls int
}

// NewMemoryRouterPusher returns a MemoryRouterPusher pre-loaded with
// the supplied instances. Pass nothing to construct a "no router
// instances yet" pusher.
func NewMemoryRouterPusher(instances ...Instance) *MemoryRouterPusher {
	return &MemoryRouterPusher{
		Instances: append([]Instance(nil), instances...),
		Tables:    map[string]string{},
	}
}

// Discover implements [RouterPusher].
func (p *MemoryRouterPusher) Discover(_ context.Context) ([]Instance, error) {
	if p == nil {
		return nil, errors.New("modelrouter: nil pusher")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.DiscoverErr != nil {
		return nil, p.DiscoverErr
	}
	out := append([]Instance(nil), p.Instances...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Push implements [RouterPusher].
func (p *MemoryRouterPusher) Push(_ context.Context, instance Instance, table *RoutingTable) error {
	if p == nil {
		return errors.New("modelrouter: nil pusher")
	}
	if table == nil {
		return errors.New("modelrouter: nil table")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.PushErr != nil {
		return p.PushErr
	}
	if p.Tables == nil {
		p.Tables = map[string]string{}
	}
	p.Tables[instance.ID] = table.Hash
	p.PushCalls++
	return nil
}

// Snapshot returns a shallow copy of the recorded tables so tests can
// assert without holding the mutex. The map shape is preserved.
func (p *MemoryRouterPusher) Snapshot() map[string]string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]string, len(p.Tables))
	for k, v := range p.Tables {
		out[k] = v
	}
	return out
}

// NoopRouterPusher is the zero-value [RouterPusher] used by the
// production manager when task 11.1 has not yet been wired up. It
// reports "no instances" on [Discover] and accepts every Push as a
// no-op so the controller can still surface `Compiled=True`.
type NoopRouterPusher struct{}

// Discover implements [RouterPusher].
func (NoopRouterPusher) Discover(_ context.Context) ([]Instance, error) { return nil, nil }

// Push implements [RouterPusher].
func (NoopRouterPusher) Push(_ context.Context, _ Instance, _ *RoutingTable) error { return nil }

// ErrPushFailed is the canonical sentinel for tests that want to
// simulate a transient push failure.
var ErrPushFailed = fmt.Errorf("modelrouter: push failed")

// Compile-time interface assertions.
var (
	_ RouterPusher = (*MemoryRouterPusher)(nil)
	_ RouterPusher = NoopRouterPusher{}
)
