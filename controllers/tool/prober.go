package tool

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// Prober is the abstraction the Tool controller uses to assert that
// `spec.endpoint` is reachable. The real implementation talks HTTP /
// gRPC to the Tool — see [HTTPProbe]; tests inject [NoopProber] so
// the reconciler is exercisable without real network I/O.
//
// Implementations MUST be idempotent and safe for concurrent use:
// the reconciler calls Probe on every reconcile and the controller-
// runtime workqueue may invoke it from multiple goroutines.
type Prober interface {
	// Probe checks whether the endpoint declared by the supplied Tool
	// CR is reachable. The boolean MUST be true iff the call observed
	// a successful response from the tool. The returned error is
	// non-nil only on transport or programming errors — it is not
	// used to signal "endpoint returned 5xx", which MUST be reported
	// as `(false, nil)` so the reconciler can write a clean condition.
	Probe(ctx context.Context, tool *skillv1alpha1.Tool) (reachable bool, err error)
}

// HTTPProbeTimeout caps a single probe call. Mirrors the 5s budget
// documented in design.md §14 for control-plane health checks.
const HTTPProbeTimeout = 5 * time.Second

// HTTPProbe is the default [Prober] implementation. It performs a
// single HTTP GET against `tool.spec.endpoint` and reports True iff
// the response status code is < 500. Network errors yield (false, nil)
// so the reconciler stays responsible for converting probe outcomes
// into Conditions.
//
// HTTPProbe is intentionally lenient about non-2xx responses: many
// MCP / OpenAPI tools return 401 or 405 on a bare GET because the
// real surface lives behind a different verb. Anything < 500 is
// treated as "process is up and answering"; 5xx and transport errors
// flip the gate to False.
//
// TLS verification is disabled when [HTTPProbe.InsecureSkipVerify] is
// set so dev clusters with self-signed certs work out of the box. In
// production deployments operators should leave the field at its zero
// value (verification on).
type HTTPProbe struct {
	// Client is the underlying HTTP client. When nil [HTTPProbe]
	// constructs a per-call client with [HTTPProbeTimeout] as the
	// timeout.
	Client *http.Client

	// InsecureSkipVerify disables TLS server verification on the
	// per-call client. Has no effect when [Client] is non-nil — the
	// caller is responsible for the TLS posture in that case.
	InsecureSkipVerify bool
}

// NewHTTPProbe constructs an [HTTPProbe] with sane defaults: a 5s
// timeout and TLS verification on. Callers can wrap the result and
// flip [HTTPProbe.InsecureSkipVerify] for dev clusters.
func NewHTTPProbe() *HTTPProbe { return &HTTPProbe{} }

// Probe implements [Prober].
func (p *HTTPProbe) Probe(ctx context.Context, tool *skillv1alpha1.Tool) (bool, error) {
	if tool == nil {
		return false, errors.New("HTTPProbe: nil tool")
	}
	if tool.Spec.Endpoint == "" {
		return false, errors.New("HTTPProbe: empty spec.endpoint")
	}
	client := p.Client
	if client == nil {
		client = &http.Client{
			Timeout: HTTPProbeTimeout,
		}
		if p.InsecureSkipVerify {
			client.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev-only
			}
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, HTTPProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, tool.Spec.Endpoint, nil)
	if err != nil {
		// Malformed URL — surface as a transport error.
		return false, fmt.Errorf("HTTPProbe: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		// Transport / DNS / TLS error — false + nil so the reconciler
		// records the failure as a condition.
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode < http.StatusInternalServerError, nil
}

// NoopProber is the in-memory [Prober] used by unit tests. It returns
// the seeded `Reachable / Err` pair on every call and counts how many
// times Probe was invoked.
type NoopProber struct {
	mu sync.Mutex

	// Reachable is the value returned to callers. Default: true.
	Reachable bool

	// Err is the error returned to callers. Default: nil.
	Err error

	// Calls is incremented on every Probe invocation.
	Calls int
}

// NewNoopProber returns a NoopProber that reports the endpoint as
// reachable.
func NewNoopProber() *NoopProber { return &NoopProber{Reachable: true} }

// Probe implements [Prober].
func (n *NoopProber) Probe(_ context.Context, _ *skillv1alpha1.Tool) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Calls++
	return n.Reachable, n.Err
}

// Snapshot returns the recorded call count under the mutex.
func (n *NoopProber) Snapshot() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.Calls
}

// Compile-time interface assertions.
var (
	_ Prober = (*HTTPProbe)(nil)
	_ Prober = (*NoopProber)(nil)
)
