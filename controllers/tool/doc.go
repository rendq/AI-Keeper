// Package tool implements the AIP Tool controller (design.md §6.5 —
// Tool Controller / Requirement A7.3).
//
// The reconciler turns a [skillv1alpha1.Tool] CR into a registered
// entry in the platform Tool_Registry and keeps `status.reachable`
// fresh by probing `spec.endpoint` every [SteadyStateRequeue].
//
// Scope (P0 — task 4.2):
//
//   - Endpoint reachability is exercised through the [Prober]
//     interface. The default [HTTPProbe] performs a single GET against
//     `spec.endpoint` and reports True iff the response is < 500. A
//     [NoopProber] is shipped for unit tests and dev clusters where
//     the network is unreliable.
//   - Tool registration is exercised through the [Registry] interface.
//     The default [MemoryRegistry] is used until the Tool_Registry
//     service from task 16.1 is wired up.
//   - Schema parsing is treated as already-validated: the admission
//     webhook from task 2.3 enforces structural constraints, so the
//     controller only flips `SchemaParsed=True` once the rest of the
//     gates have settled.
//   - Approval is gated on `governance.sideEffects` and
//     `governance.requiresApproval` per design.md §6.5 / Requirement
//     A9.2 lint rule `tool/destructive-needs-approval`.
//
// Conditions emitted (design.md §6.5 / Requirement A7.3):
//
//   - `EndpointProbed` — the most recent probe succeeded.
//   - `SchemaParsed` — admission has accepted the schema.
//   - `Registered` — Tool was upserted into Tool_Registry.
//   - `ApprovalConfigured` — approval flag matches side-effects.
//   - `Ready` — aggregate of the gates above.
//
// Deletion:
//
//   - The reconciler adds the `ai-keeper.io/tool-protect` finalizer so
//     deregistration runs once `metadata.deletionTimestamp` is set.
//
// Validates: Requirements A7.3.
package tool
