package serviceaccount

import (
	"context"
	"sync"

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
)

// IdentityBrokerClient is the abstraction over the AIP Identity Broker
// the ServiceAccount controller consumes. The real implementation
// lands in task 6.1 — for P0 this package ships [NoopIdentityBroker]
// so the reconciler is driveable end-to-end without the data plane.
//
// Implementations MUST be idempotent: every method may be invoked more
// than once (controller retries, API server conflicts, manager
// restarts) and is expected to converge on a consistent state without
// surfacing duplicate-registration errors.
type IdentityBrokerClient interface {
	// Register installs (or refreshes) the SA in the Identity Broker
	// and configures `spec.tokenLifetime`. Idempotent — re-registering
	// an already-known SA returns nil.
	Register(ctx context.Context, sa *corev1alpha1.ServiceAccount) error

	// Deregister removes the SA from the Broker and revokes any
	// outstanding tokens. Invoked from the deletion path — MUST tolerate
	// "already revoked" gracefully so the finalizer is re-entrant.
	Deregister(ctx context.Context, sa *corev1alpha1.ServiceAccount) error

	// EnableOBO turns on RFC 8693 token exchange for the SA. Called
	// only when `spec.allowOnBehalfOf=true`.
	EnableOBO(ctx context.Context, sa *corev1alpha1.ServiceAccount) error

	// DisableOBO turns the OBO capability off. Invoked when the
	// operator flips `allowOnBehalfOf` to false on a previously OBO-
	// enabled SA, and as part of the deletion path.
	DisableOBO(ctx context.Context, sa *corev1alpha1.ServiceAccount) error
}

// NoopIdentityBroker is the in-memory [IdentityBrokerClient] used by
// unit tests and dev clusters where the real Broker is not wired. It
// records every call so tests can assert on call ordering.
//
// All methods are safe for concurrent use.
type NoopIdentityBroker struct {
	mu sync.Mutex

	// RegisterErr / DeregisterErr / EnableOBOErr / DisableOBOErr can be
	// pre-seeded by tests to simulate Broker failures.
	RegisterErr   error
	DeregisterErr error
	EnableOBOErr  error
	DisableOBOErr error

	// Counters incremented on every successful call.
	RegisterCalls   int
	DeregisterCalls int
	EnableOBOCalls  int
	DisableOBOCalls int

	// LastRegistered captures a copy of the SA the last successful
	// Register call observed, for assertion convenience.
	LastRegistered *corev1alpha1.ServiceAccount
}

// Register records the call and returns the seeded error (if any).
func (n *NoopIdentityBroker) Register(_ context.Context, sa *corev1alpha1.ServiceAccount) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.RegisterErr != nil {
		return n.RegisterErr
	}
	n.RegisterCalls++
	if sa != nil {
		n.LastRegistered = sa.DeepCopy()
	}
	return nil
}

// Deregister records the call and returns the seeded error (if any).
func (n *NoopIdentityBroker) Deregister(_ context.Context, _ *corev1alpha1.ServiceAccount) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.DeregisterErr != nil {
		return n.DeregisterErr
	}
	n.DeregisterCalls++
	return nil
}

// EnableOBO records the call and returns the seeded error (if any).
func (n *NoopIdentityBroker) EnableOBO(_ context.Context, _ *corev1alpha1.ServiceAccount) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.EnableOBOErr != nil {
		return n.EnableOBOErr
	}
	n.EnableOBOCalls++
	return nil
}

// DisableOBO records the call and returns the seeded error (if any).
func (n *NoopIdentityBroker) DisableOBO(_ context.Context, _ *corev1alpha1.ServiceAccount) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.DisableOBOErr != nil {
		return n.DisableOBOErr
	}
	n.DisableOBOCalls++
	return nil
}

// Snapshot returns the current call counters as a struct so tests can
// take a single read under the mutex.
func (n *NoopIdentityBroker) Snapshot() (register, deregister, enableOBO, disableOBO int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.RegisterCalls, n.DeregisterCalls, n.EnableOBOCalls, n.DisableOBOCalls
}

// Compile-time interface assertion.
var _ IdentityBrokerClient = (*NoopIdentityBroker)(nil)
