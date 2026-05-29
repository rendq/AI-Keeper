package skill

import (
	"context"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// ResolveResult captures the outcome of a single dependency resolution
// pass. The semantics follow design.md §6.1.3:
//
//   - `Resolved` lists the fully-qualified references the resolver
//     selected for each entry in `spec.implementation.requires`.
//   - `Missing` lists the references that could not be located in the
//     cluster (e.g. the referenced Tool / Skill / Model has not been
//     created yet, or its `Ready` condition is False).
//   - `Cyclic` is true when the dependency graph contains a cycle that
//     prevents resolution. Cycles are a permanent failure (Requirement
//     A3.6) and the controller does not retry.
type ResolveResult struct {
	// Resolved is the set of dependencies the resolver successfully
	// pinned. The slice mirrors the structure expected on
	// `Skill.status.resolvedDependencies` (Requirement A3.3).
	Resolved skillv1alpha1.SkillResolvedDependencies

	// Missing references that the resolver could not satisfy.
	Missing []sharedv1alpha1.ResourceRef

	// Cyclic indicates a dependency cycle was detected. When true the
	// caller MUST treat the result as permanent failure and stop
	// retrying.
	Cyclic bool
}

// Resolver locates the live cluster objects that satisfy a Skill's
// `spec.implementation.requires` block. The real implementation lands
// in task 3.3 under `internal/resolver/`; the controller here depends
// only on the interface so that it can be unit-tested with stubs and so
// that the resolver implementation can evolve independently.
type Resolver interface {
	// Resolve returns the resolution outcome. A non-nil error indicates
	// a transient backend failure (etcd timeout, informer cache miss);
	// callers should requeue with backoff.
	Resolve(ctx context.Context, skill *skillv1alpha1.Skill) (ResolveResult, error)
}

// NoopResolver returns an empty `Resolved` and no missing references.
// It is the default Resolver used by tests that do not exercise the
// resolver path explicitly.
type NoopResolver struct{}

// Resolve trivially succeeds.
func (NoopResolver) Resolve(_ context.Context, _ *skillv1alpha1.Skill) (ResolveResult, error) {
	return ResolveResult{}, nil
}

// FuncResolver adapts a plain Go function to the [Resolver] interface.
// Useful in unit tests where a closure-based stub is more readable than
// a dedicated mock type.
type FuncResolver func(ctx context.Context, skill *skillv1alpha1.Skill) (ResolveResult, error)

// Resolve delegates to the wrapped function.
func (f FuncResolver) Resolve(ctx context.Context, skill *skillv1alpha1.Skill) (ResolveResult, error) {
	return f(ctx, skill)
}

// Compile-time interface assertions.
var (
	_ Resolver = NoopResolver{}
	_ Resolver = FuncResolver(nil)
)
