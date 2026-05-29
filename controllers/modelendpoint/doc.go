// Package modelendpoint implements the AIP ModelEndpoint controller
// (design.md §6.5 — ModelEndpoint Controller / Requirement A7.6).
//
// The reconciler turns a [modelv1alpha1.ModelEndpoint] CR into a
// continuously-probed inference target and writes the observed
// metrics back to `status.{healthy, lastProbeAt, currentTpm,
// currentRpm, errorRate24h, avgLatencyMs}` every
// [SteadyStateRequeue].
//
// Scope (P0 — task 4.3):
//
//   - Endpoint reachability is exercised through the [Prober]
//     interface. The default [HTTPProbe] performs a single GET against
//     `spec.endpoint` with a 5-second timeout and reports the observed
//     latency. A [NoopProber] is shipped for unit tests.
//   - The DPA gate is satisfied when either (a) `spec.compliance` does
//     not list `GDPR` or `HIPAA` (treated as `DPASigned=Unknown
//     reason=NotRequired` — counts as satisfied) or (b) the operator
//     sets `spec.privacy.dpaSigned=true`. Anything else stamps
//     `DPASigned=False` and drives Phase=Failed (operator must fix
//     the spec).
//   - The WithinQuota gate is True by default; the controller does
//     not yet enforce real metrics in P0. The hooks for `currentTpm`
//     and `currentRpm` are kept so task 11.1 can populate them.
//   - The aggregate Ready condition is True iff `Healthy=True` ∧
//     `DPASigned ∈ {True, Unknown(reason=NotRequired)}` ∧
//     `WithinQuota=True`.
//
// Conditions emitted (design.md §6.5 / Requirement A7.6):
//
//   - `Healthy` — the most recent probe succeeded.
//   - `DPASigned` — DPA gate satisfied (or not required).
//   - `WithinQuota` — current TPM/RPM observed under spec.quota.
//   - `Ready` — aggregate of the gates above.
//
// Phase derivation:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. `DPASigned=False` (compliance requires DPA but not signed) → Failed
//  3. Aggregate Ready=True → Active
//  4. `Healthy=False` → Degraded
//  5. Otherwise → Pending
//
// Deletion:
//
//   - The reconciler adds the `ai-keeper.io/modelendpoint-protect`
//     finalizer and lifts it on deletion. Real drain (token
//     revocation, registry cleanup) lands in P1.
//
// Validates: Requirements A7.6.
package modelendpoint
