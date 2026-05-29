package modelendpoint

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
)

// Prober is the abstraction the ModelEndpoint controller uses to
// assert that `spec.endpoint` is reachable and to observe the
// round-trip latency. The real implementation talks HTTP / OpenAI-
// compatible health-check to the provider — see [HTTPProbe]; tests
// inject [NoopProber] so the reconciler is exercisable without real
// network I/O.
//
// Implementations MUST be idempotent and safe for concurrent use:
// the reconciler calls Probe on every reconcile and the controller-
// runtime workqueue may invoke it from multiple goroutines.
type Prober interface {
	// Probe checks whether the endpoint declared by the supplied
	// ModelEndpoint CR is reachable. The returned `latencyMs` is the
	// observed round-trip time in milliseconds (≥ 0). A non-nil error
	// signals the probe failed (transport error, 5xx, programming
	// error) and the reconciler should stamp `Healthy=False`.
	Probe(ctx context.Context, ep *modelv1alpha1.ModelEndpoint) (latencyMs int32, err error)
}

// HTTPProbeTimeout caps a single probe call. Mirrors the 5s budget
// documented in design.md §14 for control-plane health checks.
const HTTPProbeTimeout = 5 * time.Second

// HTTPProbe is the default [Prober] implementation. It performs a
// single HTTP GET against `spec.endpoint` and reports the observed
// latency. Any response with status code < 500 counts as "endpoint
// up" — many provider URLs return 401 or 405 on a bare GET because
// the inference surface lives behind POST /v1/chat/completions, but
// the process itself is reachable. 5xx responses and transport errors
// flip the gate to Healthy=False.
//
// TLS verification is disabled when [HTTPProbe.InsecureSkipVerify] is
// set so dev clusters with self-signed certs work out of the box.
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
// timeout and TLS verification on.
func NewHTTPProbe() *HTTPProbe { return &HTTPProbe{} }

// Probe implements [Prober].
func (p *HTTPProbe) Probe(ctx context.Context, ep *modelv1alpha1.ModelEndpoint) (int32, error) {
	if ep == nil {
		return 0, errors.New("HTTPProbe: nil endpoint")
	}
	if ep.Spec.Endpoint == "" {
		return 0, errors.New("HTTPProbe: empty spec.endpoint")
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: HTTPProbeTimeout}
		if p.InsecureSkipVerify {
			client.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev-only
			}
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, HTTPProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, ep.Spec.Endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("HTTPProbe: build request: %w", err)
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		// Transport / DNS / TLS error — return the elapsed time so the
		// reconciler can still record approximate latency in
		// `status.avgLatencyMs` if it wants.
		return clampMS(time.Since(start)), err
	}
	defer resp.Body.Close()
	latency := clampMS(time.Since(start))
	if resp.StatusCode >= http.StatusInternalServerError {
		return latency, fmt.Errorf("HTTPProbe: endpoint returned %d", resp.StatusCode)
	}
	return latency, nil
}

// clampMS converts a Duration to a non-negative int32 millisecond
// count, capping at math.MaxInt32 to avoid overflow on absurdly large
// latencies.
func clampMS(d time.Duration) int32 {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	if ms > int64(int32MaxMS) {
		return int32MaxMS
	}
	return int32(ms)
}

const int32MaxMS int32 = 1<<31 - 1

// NoopProber is the in-memory [Prober] used by unit tests. It returns
// the seeded `Latency / Err` pair on every call and counts how many
// times Probe was invoked.
type NoopProber struct {
	mu sync.Mutex

	// Latency is the latency value (in ms) returned to callers.
	// Default: 50.
	Latency int32

	// Err is the error returned to callers. Default: nil.
	Err error

	// Calls is incremented on every Probe invocation.
	Calls int
}

// NewNoopProber returns a NoopProber that reports a 50ms healthy
// probe.
func NewNoopProber() *NoopProber { return &NoopProber{Latency: 50} }

// Probe implements [Prober].
func (n *NoopProber) Probe(_ context.Context, _ *modelv1alpha1.ModelEndpoint) (int32, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Calls++
	return n.Latency, n.Err
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
