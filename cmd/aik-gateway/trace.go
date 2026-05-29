package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
)

// TraceInjector injects OTel traceparent header into requests.
type TraceInjector interface {
	Inject(r *http.Request) *http.Request
}

// DefaultTraceInjector generates a W3C Trace Context traceparent header
// with a random trace_id and span_id for each request.
// Format: version-trace_id-span_id-trace_flags
// Example: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
type DefaultTraceInjector struct{}

// Inject adds a traceparent header to the request if not already present.
func (t *DefaultTraceInjector) Inject(r *http.Request) *http.Request {
	// If traceparent already exists, preserve it (propagate from upstream).
	if r.Header.Get("traceparent") != "" {
		return r
	}

	traceID := generateTraceID()
	spanID := generateSpanID()

	// W3C Trace Context format: version(2)-trace_id(32)-span_id(16)-flags(2)
	traceparent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)
	r.Header.Set("traceparent", traceparent)

	return r
}

// generateTraceID generates a 16-byte (32 hex chars) trace ID.
func generateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// generateSpanID generates an 8-byte (16 hex chars) span ID.
func generateSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
