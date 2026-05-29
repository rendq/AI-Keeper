// Package quota implements the AIP Quota controller (design.md
// §6.5 / Requirement A8.5).
//
// The reconciler turns a [policyv1alpha1.Quota] CR into a periodic
// resource-count tally. It walks `spec.limits` and, for each
// supported resource kind, lists the live CRs in the scope and
// writes the observed count back to `status.used` every
// [SteadyStateRequeue].
//
// Scope (P0 — task 4.4):
//
//   - Supported resource kinds: `agents`, `skills`, `tools`,
//     `modelEndpoints`, `knowledgeBases`, `dataSources`. The keys
//     mirror the camelCase aliases used by aikctl / Helm values; an
//     unknown key is recorded as `0` and surfaces a warning event so
//     operators can correct the spec without halting reconcile.
//   - Scope resolution: `Tenant` / `Team` / `User` scopes count CRs
//     across the entire cluster (the controller queries all namespaces
//     and lets aggregate stats reflect tenancy through naming
//     conventions). `Namespace` scope counts CRs only inside the
//     Quota's own namespace. The interface is intentionally
//     conservative for P0 — task 4.6 will add label-driven scope
//     filtering once Tenant labels are universal.
//   - WithinLimit gate: True iff every populated `used` count is
//     strictly below its corresponding `limit`. A nil limit means
//     "unlimited" for that resource kind.
//   - QuotaServiceReady (P0): the controller does not actually run a
//     downstream admission webhook. We surface
//     [QuotaServiceReady]=True as a placeholder so the aggregate
//     Ready condition can flip and admission can wire in later.
//
// Conditions emitted (design.md §6.5 / Requirement A8.5):
//
//   - [QuotaServiceReady] — placeholder True for P0.
//   - [QuotaWithinLimit]  — every used count strictly below its limit.
//   - [QuotaReady]        — aggregate of the gates above.
//
// Phase derivation:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. Aggregate Ready=True → Active
//  3. `WithinLimit=False` → Failed (operator must lift the cap or
//     remove resources)
//  4. Otherwise → Pending
//
// Deletion:
//
//   - The reconciler adds the [FinalizerQuotaProtect] finalizer and
//     lifts it on deletion. Real Quota_Service cleanup lands in P1.
//
// Validates: Requirements A8.5.
package quota
