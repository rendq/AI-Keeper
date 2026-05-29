// Package auditapiserver implements the core logic for the AuditEvent
// read-only aggregated API server.
//
// It translates Get/List operations on audit.ai-keeper.io/v1alpha1/AuditEvent
// into ClickHouse SQL queries against the `audit_events` table.
// CREATE/UPDATE/DELETE operations are restricted to ServiceAccounts
// annotated with `ai-keeper.io/system=true`.
//
// Requirements: A1.5, B12.1
package auditapiserver
