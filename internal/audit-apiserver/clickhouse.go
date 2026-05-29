package auditapiserver

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AuditEventRow represents a single row returned from ClickHouse's
// audit_events table.
type AuditEventRow struct {
	// Name is metadata.name (primary identifier).
	Name string
	// Namespace is metadata.namespace.
	Namespace string
	// TenantID is the owning tenant.
	TenantID string
	// InvocationID is the unique invocation identifier.
	InvocationID string
	// OccurredAt is the event timestamp.
	OccurredAt time.Time
	// AgentName is the agent that produced this event.
	AgentName string
	// Decision is the policy decision (allow/deny/require_approval/error).
	Decision string
	// OutcomeStatus is the terminal status.
	OutcomeStatus string
	// EventHash is the canonical sha256 of the event.
	EventHash string
	// SpecJSON is the full spec serialized as JSON.
	SpecJSON string
	// Labels stores k8s-style labels for label selector filtering.
	Labels map[string]string
}

// ClickHouseClient is the interface used to query the ClickHouse
// audit_events table. Implementations include a real ClickHouse driver
// and a mock for testing.
type ClickHouseClient interface {
	// QueryAuditEvents executes the given SQL with args and returns rows.
	QueryAuditEvents(ctx context.Context, query string, args []interface{}) ([]AuditEventRow, error)
}

// ListOptions captures the parameters for listing AuditEvents.
type ListOptions struct {
	// Namespace filters by namespace. Empty means all namespaces.
	Namespace string
	// LabelSelector is a map of key=value label requirements (AND logic).
	LabelSelector map[string]string
	// FieldSelector is a map of field=value requirements (AND logic).
	// Supported fields: spec.invocationId, spec.principal.agent.name,
	// spec.policy.decision, spec.outcome.status.
	FieldSelector map[string]string
	// Limit caps the number of returned rows. 0 means default (1000).
	Limit int64
	// Continue is an opaque pagination token (base64 encoded offset).
	Continue string
}

// GetOptions captures parameters for getting a single AuditEvent.
type GetOptions struct {
	// Name is the metadata.name of the AuditEvent.
	Name string
	// Namespace is the metadata.namespace.
	Namespace string
}

// QueryBuilder translates Kubernetes-style Get/List operations into
// ClickHouse SQL queries against the `audit_events` table.
type QueryBuilder struct {
	// Table is the ClickHouse table name (default: "audit_events").
	Table string
}

// NewQueryBuilder creates a QueryBuilder with sensible defaults.
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{Table: "audit_events"}
}

// fieldToColumn maps supported field selector paths to ClickHouse columns.
var fieldToColumn = map[string]string{
	"spec.invocationId":       "invocation_id",
	"spec.principal.agent.name": "agent_name",
	"spec.policy.decision":    "decision",
	"spec.outcome.status":     "outcome_status",
	"metadata.namespace":      "namespace",
	"metadata.name":           "name",
}

// BuildGet generates a SQL query to fetch a single AuditEvent by name
// and namespace.
func (qb *QueryBuilder) BuildGet(opts GetOptions) (query string, args []interface{}) {
	query = fmt.Sprintf(
		"SELECT name, namespace, tenant_id, invocation_id, occurred_at, agent_name, decision, outcome_status, event_hash, spec_json, labels FROM %s WHERE name = ? AND namespace = ? LIMIT 1",
		qb.Table,
	)
	args = []interface{}{opts.Name, opts.Namespace}
	return query, args
}

// BuildList generates a SQL query to list AuditEvents with filtering,
// pagination, and ordering.
func (qb *QueryBuilder) BuildList(opts ListOptions) (query string, args []interface{}, err error) {
	var conditions []string
	args = make([]interface{}, 0)

	// Namespace filter.
	if opts.Namespace != "" {
		conditions = append(conditions, "namespace = ?")
		args = append(args, opts.Namespace)
	}

	// Label selector (AND of key=value).
	for k, v := range opts.LabelSelector {
		if err := validateLabelKey(k); err != nil {
			return "", nil, fmt.Errorf("invalid label key %q: %w", k, err)
		}
		// ClickHouse map access: labels['key'] = 'value'
		conditions = append(conditions, fmt.Sprintf("labels['%s'] = ?", escapeSingleQuote(k)))
		args = append(args, v)
	}

	// Field selector.
	for field, value := range opts.FieldSelector {
		col, ok := fieldToColumn[field]
		if !ok {
			return "", nil, fmt.Errorf("unsupported field selector: %q", field)
		}
		conditions = append(conditions, fmt.Sprintf("%s = ?", col))
		args = append(args, value)
	}

	// Pagination via offset (decoded from Continue token).
	offset := int64(0)
	if opts.Continue != "" {
		parsed, parseErr := decodeContinueToken(opts.Continue)
		if parseErr != nil {
			return "", nil, fmt.Errorf("invalid continue token: %w", parseErr)
		}
		offset = parsed
	}

	// Build WHERE clause.
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 1000
	}

	query = fmt.Sprintf(
		"SELECT name, namespace, tenant_id, invocation_id, occurred_at, agent_name, decision, outcome_status, event_hash, spec_json, labels FROM %s %s ORDER BY occurred_at DESC LIMIT %d OFFSET %d",
		qb.Table, where, limit, offset,
	)

	return query, args, nil
}

// validateLabelKey does basic validation on a label key to prevent
// SQL injection through map key access.
func validateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("label key must not be empty")
	}
	// K8s label keys: [prefix/]name, where name is [a-z0-9A-Z._-]{1,63}
	// and prefix is a DNS subdomain. We reject anything with quotes or
	// special SQL characters.
	for _, ch := range key {
		if ch == '\'' || ch == '"' || ch == ';' || ch == '\\' || ch == '\n' || ch == '\r' {
			return fmt.Errorf("label key contains disallowed character %q", ch)
		}
	}
	return nil
}

// escapeSingleQuote escapes single quotes for safe use in ClickHouse
// identifiers. This is a defense-in-depth measure; validateLabelKey
// already rejects keys containing quotes.
func escapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
