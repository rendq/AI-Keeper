// Package webhook implements the AIP ValidatingAdmissionWebhook
// (Requirements A1.3, A1.5, A2.1—A2.6).
//
// The kubebuilder Pattern markers on `api/shared/v1alpha1` already cause
// the K8s API server to reject malformed regex values before any
// admission webhook fires. This package is the *defence-in-depth*
// layer:
//
//  1. It centralises cross-field invariants the OpenAPI schema cannot
//     express (e.g. "AuditEvent CREATE/UPDATE/DELETE is allowed only
//     for ServiceAccounts annotated `ai-keeper.io/system=true`",
//     `Tool.spec.governance.requiresApproval=true` when
//     `sideEffects=destructive`, etc.).
//  2. It re-validates the regex inputs that kubebuilder enforces, so
//     in-cluster mutators that bypass admission still produce errors
//     identical to the API-server's rejection (Requirements A2.1—A2.6).
//  3. It denies requests that look fine to OpenAPI but violate
//     domain-level rules — most importantly Requirement A1.5 which
//     locks the AuditEvent kind to system writers.
//
// Wiring lives in `SetupWithManager`: call it from
// `cmd/manager/main.go` once a controller-runtime Manager exists. Each
// resource is registered with the Manager's webhook server under the
// canonical kubebuilder URL pattern
// `/validate-<group>-<version>-<kind>`.
package webhook
