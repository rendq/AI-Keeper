package auditapiserver

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	auditv1alpha1 "github.com/ai-keeper/ai-keeper/api/audit/v1alpha1"
)

// Store is the read-only storage backend for AuditEvent resources.
// It translates Get/List requests into ClickHouse queries and returns
// Kubernetes-native AuditEvent objects.
type Store struct {
	queryBuilder *QueryBuilder
	client       ClickHouseClient
}

// NewStore creates a new AuditEvent store backed by ClickHouse.
func NewStore(chClient ClickHouseClient) *Store {
	return &Store{
		queryBuilder: NewQueryBuilder(),
		client:       chClient,
	}
}

// Get retrieves a single AuditEvent by name and namespace.
func (s *Store) Get(ctx context.Context, opts GetOptions) (*auditv1alpha1.AuditEvent, error) {
	query, args := s.queryBuilder.BuildGet(opts)
	rows, err := s.client.QueryAuditEvents(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("ClickHouse query failed: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("AuditEvent %s/%s not found", opts.Namespace, opts.Name)
	}
	return rowToAuditEvent(rows[0])
}

// ListResult contains the results of a List operation.
type ListResult struct {
	Items         []auditv1alpha1.AuditEvent
	ContinueToken string
}

// List retrieves AuditEvents matching the given options.
func (s *Store) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	query, args, err := s.queryBuilder.BuildList(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build list query: %w", err)
	}

	rows, err := s.client.QueryAuditEvents(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("ClickHouse query failed: %w", err)
	}

	result := &ListResult{
		Items: make([]auditv1alpha1.AuditEvent, 0, len(rows)),
	}

	for _, row := range rows {
		ae, convErr := rowToAuditEvent(row)
		if convErr != nil {
			return nil, fmt.Errorf("failed to convert row %q: %w", row.Name, convErr)
		}
		result.Items = append(result.Items, *ae)
	}

	// If we got a full page, provide a continue token for the next page.
	limit := opts.Limit
	if limit <= 0 {
		limit = 1000
	}
	if int64(len(rows)) == limit {
		currentOffset := int64(0)
		if opts.Continue != "" {
			currentOffset, _ = decodeContinueToken(opts.Continue)
		}
		result.ContinueToken = encodeContinueToken(currentOffset + limit)
	}

	return result, nil
}

// rowToAuditEvent converts a ClickHouse row into a typed AuditEvent.
func rowToAuditEvent(row AuditEventRow) (*auditv1alpha1.AuditEvent, error) {
	ae := &auditv1alpha1.AuditEvent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "audit.ai-keeper.io/v1alpha1",
			Kind:       "AuditEvent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              row.Name,
			Namespace:         row.Namespace,
			CreationTimestamp: metav1.NewTime(row.OccurredAt),
			Labels:            row.Labels,
		},
	}

	// Unmarshal spec from stored JSON.
	if row.SpecJSON != "" {
		if err := json.Unmarshal([]byte(row.SpecJSON), &ae.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec JSON: %w", err)
		}
	}

	return ae, nil
}
