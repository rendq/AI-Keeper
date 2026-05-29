// Package serviceaccount implements the AIP ServiceAccount controller
// (design.md §6.5 — ServiceAccount Controller / Requirement A7.2).
//
// The reconciler turns a [corev1alpha1.ServiceAccount] CR into a
// registered identity at the platform's [IdentityBrokerClient] and,
// when `spec.allowOnBehalfOf=true`, enables RFC 8693 token exchange so
// downstream tools can be invoked on behalf of the end user
// (Requirement B3 / C8.4).
//
// Scope (P0 — task 4.1):
//
//   - The Identity Broker integration is interface-typed; the real
//     wiring lands in task 6.1. This package ships a [NoopIdentityBroker]
//     stand-in so the reconciler is exercisable in unit tests today.
//   - Spec validation is minimal — `spec.identityProvider` MUST be
//     non-empty. CRD admission already enforces the rest of the field
//     constraints (regex on `spiffeId`, length bounds, ...).
//
// Conditions emitted (design.md §6.5 / Requirement A7.2):
//
//   - `IdentityProviderReady` — `IdentityBroker.Register` succeeded.
//   - `TokenExchangeReady` — `IdentityBroker.EnableOBO` succeeded when
//     `spec.allowOnBehalfOf=true`; otherwise Unknown reason=OBOdisabled
//     so the aggregate Ready condition is correctly satisfied without
//     the extra round-trip.
//   - `Ready` — aggregate of the gates above per design.md §6.5.
//
// Deletion (Requirement A7.2 / C8.4 — token revocation within 30 s):
//
//   - The reconciler adds the `ai-keeper.io/serviceaccount-revoke` finalizer
//     so deletion drives `IdentityBroker.Deregister`. The finalizer is
//     only removed once the Broker confirms revocation.
//
// Validates: Requirements A7.2.
package serviceaccount
