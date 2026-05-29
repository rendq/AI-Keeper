// Package audit provides the runtime AuditEvent type with deterministic
// canonical hashing (RFC 8785 JSON Canonicalization Scheme) and JSON / JSONL
// serialization round-trip.
//
// Requirements: B12.8, F22, F11
package audit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Core runtime types (independent from CRD but structurally aligned)
// ──────────────────────────────────────────────────────────────────────────────

// Event is the runtime representation of an AuditEvent used by the data plane.
// It mirrors api/audit/v1alpha1.AuditEventSpec but is decoupled from
// Kubernetes metav1.Time for easier serialization control.
type Event struct {
	InvocationID string          `json:"invocationId"`
	Timestamp    time.Time       `json:"timestamp"`
	Trace        *EventTrace     `json:"trace,omitempty"`
	Principal    EventPrincipal  `json:"principal"`
	Action       EventAction     `json:"action"`
	Policy       *EventPolicy    `json:"policy,omitempty"`
	Request      *EventRequest   `json:"request,omitempty"`
	Response     *EventResponse  `json:"response,omitempty"`
	Steps        []EventStep     `json:"steps,omitempty"`
	Cost         *EventCost      `json:"cost,omitempty"`
	Guardrails   *EventGuardrail `json:"guardrails,omitempty"`
	Compliance   *EventCompliance `json:"compliance,omitempty"`
	Outcome      *EventOutcome   `json:"outcome,omitempty"`
	EventHash    string          `json:"eventHash,omitempty"`
	RawObjectURI string          `json:"rawObjectUri,omitempty"`
}

type EventTrace struct {
	TraceID      string `json:"traceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
}

type EventPrincipalUser struct {
	ID         string            `json:"id,omitempty"`
	TenantID   string            `json:"tenantId,omitempty"`
	Department string            `json:"department,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type EventPrincipalAgent struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Version   string `json:"version,omitempty"`
}

type EventPrincipalSA struct {
	Name     string `json:"name,omitempty"`
	SpiffeID string `json:"spiffeId,omitempty"`
}

type EventPrincipal struct {
	User           *EventPrincipalUser  `json:"user,omitempty"`
	Agent          EventPrincipalAgent  `json:"agent"`
	ServiceAccount *EventPrincipalSA    `json:"serviceAccount,omitempty"`
	OnBehalfOf     string               `json:"onBehalfOf,omitempty"`
	SourceIP       string               `json:"sourceIp,omitempty"`
	UserAgent      string               `json:"userAgent,omitempty"`
	Channel        string               `json:"channel,omitempty"`
}

type EventAction struct {
	Verb     string `json:"verb"`
	Resource string `json:"resource"`
	Method   string `json:"method,omitempty"`
}

type EventPolicy struct {
	Decision           string   `json:"decision"`
	MatchedPolicies    []string `json:"matchedPolicies,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	ObligationsApplied []string `json:"obligationsApplied,omitempty"`
	ApprovalID         string   `json:"approvalId,omitempty"`
}

type EventRequest struct {
	InputHash      string   `json:"inputHash,omitempty"`
	InputRef       string   `json:"inputRef,omitempty"`
	Classification string   `json:"classification,omitempty"`
	Redactions     []string `json:"redactions,omitempty"`
	SizeBytes      *int64   `json:"sizeBytes,omitempty"`
	Language       string   `json:"language,omitempty"`
}

type EventCitation struct {
	Source  string   `json:"source,omitempty"`
	ChunkID string  `json:"chunkId,omitempty"`
	Score   *float64 `json:"score,omitempty"`
}

type EventResponse struct {
	OutputHash     string          `json:"outputHash,omitempty"`
	OutputRef      string          `json:"outputRef,omitempty"`
	Confidence     *float64        `json:"confidence,omitempty"`
	Citations      []EventCitation `json:"citations,omitempty"`
	Classification string          `json:"classification,omitempty"`
	SizeBytes      *int64          `json:"sizeBytes,omitempty"`
}

type EventStep struct {
	Index     *int32  `json:"index,omitempty"`
	Type      string  `json:"type,omitempty"`
	Tool      string  `json:"tool,omitempty"`
	Model     string  `json:"model,omitempty"`
	TokensIn  *int64  `json:"tokensIn,omitempty"`
	TokensOut *int64  `json:"tokensOut,omitempty"`
	LatencyMs *int64  `json:"latencyMs,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type EventCostTokens struct {
	Input  int64 `json:"input,omitempty"`
	Output int64 `json:"output,omitempty"`
	Cached int64 `json:"cached,omitempty"`
}

type EventCost struct {
	Tokens     *EventCostTokens `json:"tokens,omitempty"`
	Usd        string           `json:"usd,omitempty"`
	DurationMs *int64           `json:"durationMs,omitempty"`
}

type EventGuardrailHit struct {
	Rule   string   `json:"rule"`
	Stage  string   `json:"stage,omitempty"`
	Action string   `json:"action,omitempty"`
	Score  *float64 `json:"score,omitempty"`
}

type EventGuardrail struct {
	Triggered []EventGuardrailHit `json:"triggered,omitempty"`
	Blocked   *bool               `json:"blocked,omitempty"`
}

type EventCompliance struct {
	Tags          []string   `json:"tags,omitempty"`
	DataResidency string     `json:"dataResidency,omitempty"`
	Reviewed      *bool      `json:"reviewed,omitempty"`
	ReviewedBy    string     `json:"reviewedBy,omitempty"`
	ReviewedAt    *time.Time `json:"reviewedAt,omitempty"`
	Holds         []string   `json:"holds,omitempty"`
}

type EventOutcome struct {
	Status       string `json:"status,omitempty"`
	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// RFC 8785 JSON Canonicalization Scheme
// ──────────────────────────────────────────────────────────────────────────────

// Canonical serializes the Event's "spec" (all fields except eventHash) into
// RFC 8785 JSON Canonical Form: sorted object keys, no whitespace, numbers in
// shortest decimal representation.
func Canonical(e *Event) ([]byte, error) {
	// Marshal to generic interface{} to get a map we can canonicalize.
	// We exclude eventHash from canonical input to avoid circularity.
	type eventNoHash Event
	tmp := *e
	tmp.EventHash = ""
	raw, err := json.Marshal((*eventNoHash)(&tmp))
	if err != nil {
		return nil, fmt.Errorf("audit: marshal for canonical: %w", err)
	}
	var generic interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("audit: unmarshal for canonical: %w", err)
	}
	return canonicalValue(generic)
}

// canonicalValue recursively produces RFC 8785 canonical bytes for a value.
func canonicalValue(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case nil:
		return []byte("null"), nil
	case bool:
		if val {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case float64:
		return canonicalNumber(val), nil
	case string:
		return canonicalString(val), nil
	case []interface{}:
		return canonicalArray(val)
	case map[string]interface{}:
		return canonicalObject(val)
	default:
		// Fallback for unexpected types
		b, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
}

// canonicalNumber formats a float64 per RFC 8785 §3.2.2.3:
// Use shortest representation; integers without decimal point.
func canonicalNumber(f float64) []byte {
	// If the value is an integer, use integer format
	if f == float64(int64(f)) && f >= -1e15 && f <= 1e15 {
		return []byte(strconv.FormatInt(int64(f), 10))
	}
	// Use the shortest representation that round-trips
	return []byte(strconv.FormatFloat(f, 'g', -1, 64))
}

// canonicalString produces a JSON-escaped string per RFC 8785 §3.2.2.2.
func canonicalString(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

// canonicalArray produces canonical JSON for an array.
func canonicalArray(arr []interface{}) ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('[')
	for i, elem := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		b, err := canonicalValue(elem)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte(']')
	return []byte(buf.String()), nil
}

// canonicalObject produces canonical JSON for an object with sorted keys.
func canonicalObject(obj map[string]interface{}) ([]byte, error) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	// RFC 8785 §3.2.3: sort by code point (Go string comparison is correct)
	sort.Strings(keys)

	var buf strings.Builder
	buf.WriteByte('{')
	first := true
	for _, k := range keys {
		v := obj[k]
		b, err := canonicalValue(v)
		if err != nil {
			return nil, err
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(canonicalString(k))
		buf.WriteByte(':')
		buf.Write(b)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

// ──────────────────────────────────────────────────────────────────────────────
// eventHash computation
// ──────────────────────────────────────────────────────────────────────────────

// ComputeHash computes sha256(canonical(spec)) and returns the hash in the
// format "sha256:<hex>". The eventHash field of the input event is excluded
// from the hash computation.
func ComputeHash(e *Event) (string, error) {
	canonical, err := Canonical(e)
	if err != nil {
		return "", fmt.Errorf("audit: compute hash: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return fmt.Sprintf("sha256:%x", sum), nil
}

// Seal computes the eventHash and sets it on the event. This must be
// called exactly once before persisting.
func Seal(e *Event) error {
	hash, err := ComputeHash(e)
	if err != nil {
		return err
	}
	e.EventHash = hash
	return nil
}

// Verify checks that the event's eventHash matches the recomputed canonical hash.
func Verify(e *Event) (bool, error) {
	hash, err := ComputeHash(e)
	if err != nil {
		return false, err
	}
	return e.EventHash == hash, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// JSON / JSONL Serialization + Round-trip
// ──────────────────────────────────────────────────────────────────────────────

// SerializeJSON marshals the event to compact JSON (standard encoding/json).
func SerializeJSON(e *Event) ([]byte, error) {
	return json.Marshal(e)
}

// ParseJSON unmarshals an event from JSON bytes.
func ParseJSON(data []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("audit: parse json: %w", err)
	}
	return &e, nil
}

// SerializeJSONL marshals the event as a single JSONL line (compact JSON + newline).
func SerializeJSONL(e *Event) ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	// JSONL: one JSON object per line, terminated by newline
	return append(b, '\n'), nil
}

// ParseJSONL parses one or more events from JSONL bytes (one JSON object per line).
func ParseJSONL(data []byte) ([]*Event, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	events := make([]*Event, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		e, err := ParseJSON([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("audit: parse jsonl line: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}
