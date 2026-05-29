package policy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Instance identifies a single PDP replica that the controller can
// push bundles to. The `Name` field is opaque to the reconciler — it
// just echoes the value into `status.distribution[*].pdpInstance`.
type Instance struct {
	// Name is a stable identifier for the PDP replica (e.g. a pod
	// name, an Envoy cluster member, or a logical region tag).
	Name string
}

// PDPClient is the abstraction the reconciler uses to discover and
// push bundles to PDP instances. Real implementations land in the
// data-plane wiring task; the [MemoryPDPClient] below is the
// in-process stand-in used by unit tests.
type PDPClient interface {
	// Discover returns the current set of PDP instances. The set may
	// change between calls (autoscaling, pod restarts); the reconciler
	// re-discovers on every reconcile pass to guarantee newly-joined
	// PDPs receive the latest bundle.
	Discover(ctx context.Context) ([]Instance, error)

	// Push uploads the bundle to the given instance. Implementations
	// MUST be idempotent — re-pushing the same `Bundle.Hash` MUST NOT
	// return an error.
	Push(ctx context.Context, instance Instance, bundle Bundle) error

	// GetBundleHash returns the bundle hash currently loaded by the
	// instance. Used by drift detection (Requirement A5.10). When the
	// instance has not received any bundle yet implementations SHOULD
	// return an empty string, not an error.
	GetBundleHash(ctx context.Context, instance Instance) (string, error)
}

// ErrPDPInstanceUnknown is returned by [MemoryPDPClient] when a Push
// or GetBundleHash call references an instance that was not
// configured.
var ErrPDPInstanceUnknown = errors.New("policy: pdp instance unknown")

// MemoryPDPClient is the in-process [PDPClient] used by tests. It
// records every Push call for assertion and lets tests simulate
// drift by overwriting the in-memory hash for a given instance.
type MemoryPDPClient struct {
	mu        sync.Mutex
	instances []Instance
	hashes    map[string]string // instance name → loaded bundle hash
	pushCalls map[string]int    // instance name → push count
	pushErr   map[string]error  // instance name → next push error
	getErr    map[string]error  // instance name → next get error
}

// NewMemoryPDPClient constructs a MemoryPDPClient seeded with the
// supplied instance set.
func NewMemoryPDPClient(instances ...Instance) *MemoryPDPClient {
	c := &MemoryPDPClient{
		hashes:    map[string]string{},
		pushCalls: map[string]int{},
		pushErr:   map[string]error{},
		getErr:    map[string]error{},
	}
	c.instances = append(c.instances, instances...)
	return c
}

// Discover returns the seeded instance slice. The result is sorted by
// name to make assertions in tests deterministic.
func (c *MemoryPDPClient) Discover(_ context.Context) ([]Instance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]Instance(nil), c.instances...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Push records the bundle hash for the given instance and increments
// the per-instance push counter. Returns [ErrPDPInstanceUnknown] when
// the instance is not in the discovery set unless the test seeded a
// hash for it explicitly.
func (c *MemoryPDPClient) Push(_ context.Context, instance Instance, bundle Bundle) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err, ok := c.pushErr[instance.Name]; ok && err != nil {
		delete(c.pushErr, instance.Name) // single-shot
		return err
	}
	if !c.hasInstanceLocked(instance.Name) {
		return fmt.Errorf("%w: %q", ErrPDPInstanceUnknown, instance.Name)
	}
	c.hashes[instance.Name] = bundle.Hash
	c.pushCalls[instance.Name]++
	return nil
}

// GetBundleHash returns the last hash pushed to the instance, or the
// empty string when the instance has never been written.
func (c *MemoryPDPClient) GetBundleHash(_ context.Context, instance Instance) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err, ok := c.getErr[instance.Name]; ok && err != nil {
		delete(c.getErr, instance.Name) // single-shot
		return "", err
	}
	return c.hashes[instance.Name], nil
}

// SetInstances replaces the discovery set. Does NOT clear recorded
// hashes or push counts — tests that need a clean slate should
// construct a new client.
func (c *MemoryPDPClient) SetInstances(instances ...Instance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instances = append([]Instance(nil), instances...)
}

// SimulateDrift overwrites the recorded hash for the named instance
// without going through Push, allowing tests to mimic a PDP that
// reverted to an older bundle.
func (c *MemoryPDPClient) SimulateDrift(name, newHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashes[name] = newHash
}

// PushCalls returns the cumulative push counter for the named
// instance.
func (c *MemoryPDPClient) PushCalls(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pushCalls[name]
}

// TotalPushCalls returns the cumulative push counter across all
// instances.
func (c *MemoryPDPClient) TotalPushCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, n := range c.pushCalls {
		total += n
	}
	return total
}

// SetPushError makes the next Push call against `name` return `err`.
// Useful for testing distribution failure paths.
func (c *MemoryPDPClient) SetPushError(name string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pushErr[name] = err
}

// SetGetHashError makes the next GetBundleHash call against `name`
// return `err`.
func (c *MemoryPDPClient) SetGetHashError(name string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getErr[name] = err
}

func (c *MemoryPDPClient) hasInstanceLocked(name string) bool {
	for _, i := range c.instances {
		if i.Name == name {
			return true
		}
	}
	return false
}

// Compile-time interface assertion.
var _ PDPClient = (*MemoryPDPClient)(nil)
