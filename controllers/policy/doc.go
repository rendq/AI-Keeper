// Package policy implements the AIP Policy controller.
//
// The reconciler walks every Policy CR through the state machine
// described in design.md §6.3:
//
//	Pending → SyntaxCheck → RefCheck → ConflictCheck → Compiling →
//	Distributing → PartiallyActive → Active → Suspended/Expired →
//	Terminating
//
// The four pluggable collaborators decouple the controller from the
// concrete validation / compilation / conflict-detection / distribution
// implementations so each can land independently:
//
//   - Syntax validators ([validateCEL], [validateCron], [validateCIDR])
//     parse the strings declared in `Policy.spec` without executing
//     anything; failures flip the `SyntaxValid` condition.
//
//   - [Compiler] turns the validated, conflict-free Policy slice into
//     an opaque OPA bundle. The default [NoopCompiler] returns a
//     deterministic SHA-256 hash over the canonical JSON shape so the
//     reconciler is fully exercisable in unit tests; the production
//     compiler lands in task 5.1.
//
//   - [ConflictDetector] surfaces hard / soft conflicts across all
//     Policies in the same namespace (design.md §6.3.3 / §9.2). The
//     default [NoopConflictDetector] returns no conflicts; the real
//     algorithm lands in task 5.2.
//
//   - [PDPClient] discovers and pushes bundles to PDP instances. The
//     in-memory [MemoryPDPClient] is used by tests and dev clusters;
//     the production client lands in the data-plane wiring task.
//
// The reconciler also enforces two timing invariants spelled out in
// Requirements A5.10 and A5.12:
//
//   - 500 ms debounce: rapid spec changes are coalesced into a single
//     compile/push cycle by recording the previous reconcile timestamp
//     in a sync.Map keyed by the object's NamespacedName.
//
//   - 5 min drift correction: when the next reconcile fires after
//     [DriftCheckInterval] the controller calls
//     [PDPClient.GetBundleHash] for every known instance and re-pushes
//     when an instance's hash diverges from `status.bundleHash`.
//
// Validates: Requirements A5.1, A5.2, A5.5, A5.6, A5.7, A5.8, A5.9,
// A5.10, A5.11, A5.12, A6.3.
package policy
