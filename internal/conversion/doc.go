// Package conversion implements the AIP CRD ConversionWebhook.
//
// Task 2.4 (P0 placeholder): with v1alpha1 the only served & stored
// version, the webhook is registered and reachable but performs an
// **echo identity** transform — the same RawExtension bytes are returned
// unchanged with `result.status="Success"`. This satisfies the
// API-server contract that every CRD in `spec.conversion.strategy=Webhook`
// must respond to a /convert POST, and gives task P1 a stable
// integration point to drop in v1alpha1↔v1beta1 logic without
// restructuring the package.
//
// Design references:
//   - design.md §5.4 — three principles (fail-tolerant / lossy
//     recording / two-way convergence) and the conversion framework
//     skeleton.
//   - design.md §11.2 (lossy annotation key
//     `ai-keeper.io/conversion-lossy`) — pipe-separated audit trail recording
//     `from→to: reason` entries.
//
// Validates: Requirements A11.1, A11.2 (placeholder).
package conversion
