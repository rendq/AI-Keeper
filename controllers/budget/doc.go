// Package budget implements the AIP Budget controller (design.md
// §6.5 / Requirement A8).
//
// The reconciler turns a [policyv1alpha1.Budget] CR into a periodic
// "spend tracker" by polling a [CostTrackerClient] and writing the
// observed running totals back to
// `status.{periodStart, periodEnd, current, burnRate}` every
// [SteadyStateRequeue].
//
// Scope (P0 — task 4.4):
//
//   - Period boundaries are computed deterministically from
//     `spec.period` and the wall-clock time supplied by the injected
//     [Clock]. Boundaries follow the canonical UTC anchors:
//
//     hourly    → top of hour     ; top of next hour
//     daily     → midnight today  ; midnight tomorrow
//     weekly    → previous Monday ; following Monday (00:00 UTC)
//     monthly   → start of month  ; start of next month
//     quarterly → start of quarter; start of next quarter
//     yearly    → Jan 1 00:00 UTC ; following Jan 1 00:00 UTC
//
//   - The current spend snapshot is fetched from a
//     [CostTrackerClient]. The interface is defined in this package
//     so the budget reconciler can compile without a hard dependency
//     on task 13.1 (Cost Tracker dataplane). [NoopCostTracker] returns
//     a zero-spend snapshot so the controller stays operational in
//     dev clusters and unit tests.
//
//   - WithinLimit is True iff every populated field of `current` is
//     strictly less than its `limits` counterpart. A nil limit is
//     treated as "unlimited" for that dimension; a nil current value
//     is treated as zero.
//
//   - HardCap (P0): the controller does not actually enforce
//     blocking — Budget_Enforcer reads `status` directly. We surface
//     [BudgetEnforcerReady]=True as a placeholder so the aggregate
//     Ready condition can flip and the data plane can wire in later.
//
//   - BurnRate is a coarse classification derived from the highest
//     ratio across populated dimensions:
//
//     <  50%  → ok
//     50-79% → warning
//     80-99% → critical
//     ≥ 100% → exhausted
//
// Conditions emitted:
//
//   - [BudgetEnforcerReady] — placeholder True for P0.
//   - [BudgetWithinLimit]  — current spend strictly under every limit.
//   - [BudgetReady]        — aggregate of the gates above.
//
// Phase derivation:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. Aggregate Ready=True → Active
//  3. `WithinLimit=False` → Failed (operator must lift the cap or
//     wait for the next period)
//  4. Otherwise → Pending
//
// Deletion:
//
//   - The reconciler adds the [FinalizerBudgetProtect] finalizer and
//     lifts it on deletion. Real Budget_Enforcer cleanup lands in P1.
//
// Validates: Requirements A8.1, A8.3.
package budget
