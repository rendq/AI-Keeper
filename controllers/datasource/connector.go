package datasource

import (
	"context"
	"errors"
	"sync"
	"time"

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
)

// ConnectorInfo summarises the state of a connector after a successful
// [ConnectorClient.Connect] call. The reconciler copies these values
// into `status.{documentCount, sizeBytes, lastSyncAt}` so dashboards
// and the KB controller can pick them up.
type ConnectorInfo struct {
	// DocumentCount is the number of documents the connector reports
	// for the supplied DataSource.
	DocumentCount int64

	// SizeBytes is the total size of the documents in bytes.
	SizeBytes int64

	// LastSyncAt is the timestamp of the connector's last successful
	// sync. Defaults to time.Now() in [NoopConnector].
	LastSyncAt time.Time
}

// ConnectorClient is the abstraction over a connector adapter (Feishu
// Wiki / Confluence / Postgres / ...) the DataSource controller
// consumes. Real implementations land in P1 — for P0 this package
// ships [NoopConnector] so the reconciler is exercisable in unit tests.
//
// Implementations MUST be idempotent: every call may be retried by the
// controller-runtime workqueue.
type ConnectorClient interface {
	// Connect establishes (or refreshes) a connection to the supplied
	// DataSource and returns the connector's current view. The error
	// is non-nil only on transport failures.
	Connect(ctx context.Context, ds *datav1alpha1.DataSource) (ConnectorInfo, error)
}

// NoopConnector is the in-memory [ConnectorClient] used by unit tests.
type NoopConnector struct {
	mu sync.Mutex

	// Info is the value returned to callers. Default: a stub with
	// zero counters and `time.Now()` as the timestamp.
	Info ConnectorInfo

	// Err is the error returned to callers. Default: nil.
	Err error

	// Calls is incremented on every successful call.
	Calls int

	// Last captures the DataSource the last successful call observed
	// (for assertion convenience).
	Last *datav1alpha1.DataSource
}

// NewNoopConnector returns a NoopConnector with a stub info value.
func NewNoopConnector() *NoopConnector {
	return &NoopConnector{
		Info: ConnectorInfo{
			DocumentCount: 100,
			SizeBytes:     1024,
			LastSyncAt:    time.Now(),
		},
	}
}

// Connect implements [ConnectorClient].
func (n *NoopConnector) Connect(_ context.Context, ds *datav1alpha1.DataSource) (ConnectorInfo, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Err != nil {
		return ConnectorInfo{}, n.Err
	}
	n.Calls++
	if ds != nil {
		n.Last = ds.DeepCopy()
	}
	info := n.Info
	if info.LastSyncAt.IsZero() {
		info.LastSyncAt = time.Now()
	}
	return info, nil
}

// Snapshot returns the recorded call count under the mutex.
func (n *NoopConnector) Snapshot() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.Calls
}

// ErrConnectorUnavailable is the canonical sentinel for tests that
// want to simulate a transient connector outage.
var ErrConnectorUnavailable = errors.New("datasource: connector unavailable")

// Compile-time interface assertions.
var _ ConnectorClient = (*NoopConnector)(nil)
