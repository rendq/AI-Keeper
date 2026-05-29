// Package tenant implements the AIP Tenant controller (design.md §6.5).
//
// The reconciler turns a [corev1alpha1.Tenant] CR into the platform's
// per-tenant footprint:
//
//   - one Kubernetes Namespace named `tenant-<tenant-name>` with the
//     `ai-keeper.io/tenant=<name>` label so other controllers can scope
//     informer caches by tenant
//   - a default `policyv1alpha1.Budget` named `default` seeded from
//     `spec.defaultBudget` (skipped when the field is nil)
//   - a default `policyv1alpha1.Quota` named `default` carrying the
//     placeholder limits documented in [DefaultQuotaLimits]; production
//     deployments override these via aikctl / Helm values.
//
// Scope (P0 — task 4.1):
//
//   - Connector templates are NOT initialised in this build; the
//     `ConnectorsReady` condition is defaulted to True with reason
//     [ReasonConnectorsDeferred] so the aggregate Ready condition can
//     reach True. Real wiring lands together with task 4.2.
//   - Namespace deletion is left to operators — the Tenant finalizer
//     only removes the cleanup tag from `status.namespaces`. The
//     namespace itself is kept so dependent CRs can be inspected
//     post-delete.
//
// Conditions emitted (design.md §6.5 / Requirement A7.1):
//
//   - `NamespacesReady` — Namespace + default Budget/Quota provisioned.
//   - `ConnectorsReady` — defaulted True (P0 placeholder).
//   - `Ready` — aggregate of the gates above per design.md §6.5.
//
// Validates: Requirements A7.1.
package tenant
