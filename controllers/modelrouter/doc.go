// Package modelrouter implements the AIP ModelRouter controller
// (design.md §6.5 — ModelRouter Controller / Requirement A7.7).
//
// The reconciler turns a [modelv1alpha1.ModelRouter] CR into a
// runtime routing table and pushes the table to every Model_Router
// instance discovered through the [RouterPusher] interface.
//
// Scope (P0 — task 4.3):
//
//   - Endpoint resolution: every `spec.rules[].endpoint` is parsed
//     into a typed key and looked up in the API server. The
//     ModelEndpoint must exist and report `Ready=True` before the
//     router considers the rule reachable.
//   - Routing table compilation: rules are flattened into a
//     [RoutingTable] and hashed (sha256 over canonical JSON) so the
//     downstream Model_Router can detect changes without diffing the
//     entire payload.
//   - Distribution: the table is pushed to every router instance
//     reported by [RouterPusher.Discover]. Failures stamp
//     `Distributed=False` and trigger a backoff requeue.
//   - Phase derivation: when ALL referenced endpoints are
//     unreachable the controller stamps `AllReachable=False` and
//     drops to Phase=Degraded. Partial reachability still allows
//     Phase=Active because traffic can be routed to the live subset.
//
// Conditions emitted (design.md §6.5 / Requirement A7.7):
//
//   - `Compiled` — the routing table was successfully compiled.
//   - `Distributed` — push to every router instance succeeded.
//   - `AllReachable` — every referenced ModelEndpoint is Ready.
//   - `Ready` — aggregate: Compiled ∧ Distributed ∧ AllReachable.
//
// Phase derivation:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. Aggregate Ready=True → Active
//  3. `AllReachable=False` (zero reachable endpoints) → Degraded
//  4. `Compiled=False` → Failed
//  5. Otherwise → Pending
//
// Deletion:
//
//   - The reconciler adds the `ai-keeper.io/modelrouter-protect`
//     finalizer. On deletion it asks each router instance to drop
//     the alias before lifting the finalizer.
//
// Validates: Requirements A7.7.
package modelrouter
