// Package common houses cross-cutting helpers used by every AIP
// controller in this repo. The framework is intentionally small and has
// **no** controller-specific knowledge — it is consumed by `controllers/skill`,
// `controllers/agent`, `controllers/policy` and the eight auxiliary
// controllers.
//
// The four building blocks delivered here are:
//
//  1. Conditions — a thin wrapper around
//     [github.com/ai-keeper/ai-keeper/api/shared/v1alpha1.SetCondition] that
//     operates on any object satisfying [ConditionsAware]. Each AIP Kind
//     ships an adapter (see `*_conditions.go` siblings under
//     `api/<group>/v1alpha1`).
//
//  2. Finalizers — `EnsureFinalizer / RemoveFinalizer / Finalize` glue
//     that delegates to controller-runtime's
//     [sigs.k8s.io/controller-runtime/pkg/controller/controllerutil] and
//     persists the change via the runtime client.
//
//  3. RequeueWithBackoff — exponential backoff with ±20% jitter,
//     capped at 5 minutes (design.md §14.2 / Requirement F23).
//
//  4. EventBus — a tiny `Publish(ctx, DomainEvent) error` interface with
//     two concrete implementations (NATS JetStream + a no-op fallback)
//     used to broadcast cross-controller domain events such as
//     `SkillPromoted`, `PolicyDistributed` and `AgentDeployed`
//     (Requirement A6.5).
//
// Validates: Requirements A6.5, F23.
package common
