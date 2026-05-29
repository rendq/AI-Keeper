// Package datasource implements the AIP DataSource controller
// (design.md §6.5 — DataSource Controller / Requirement A7.4).
//
// The reconciler turns a [datav1alpha1.DataSource] CR into a live
// connector connection and keeps `status.connected / lastSyncAt /
// documentCount / sizeBytes` fresh by re-checking the connector at
// [SteadyStateRequeue] intervals.
//
// Scope (P0 — task 4.2):
//
//   - Connector connectivity is exercised through the [ConnectorClient]
//     interface. The default [NoopConnector] is used for unit tests
//     and dev clusters; real connector adapters (Feishu Wiki, Confluence,
//     Postgres, etc.) land in P1.
//   - ACL is admission-validated; the reconciler only flips
//     `ACLEnforced=True` once `spec.acl.mode` is set.
//   - The full sync schedule (cron / CDC / watermark) is left to P1 —
//     the controller emits a `Syncing=Unknown` condition with reason
//     [ReasonSyncDeferred] so dashboards do not surface a false-True
//     state.
//
// Conditions emitted (design.md §6.5 / Requirement A7.4):
//
//   - `Connected` — the most recent connector probe succeeded.
//   - `Syncing` — Unknown in P0 (full pipeline lands in P1).
//   - `ACLEnforced` — `spec.acl.mode` declared.
//   - `Ready` — aggregate of Connected ∧ ACLEnforced.
//
// Deletion:
//
//   - The reconciler adds the `ai-keeper.io/datasource-protect` finalizer so
//     a future P1 sync drainer has a safe hook before the CR
//     disappears.
//
// Validates: Requirements A7.4.
package datasource
