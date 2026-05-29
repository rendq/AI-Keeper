package skill

import (
	"context"
	"errors"
	"fmt"
	"sync"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// FinalizerSkillProtect is the finalizer the Skill controller adds to
// every reconciled Skill CR. The reconciler refuses to remove it while
// `status.referencingAgents` is non-empty (Requirement A3.11).
const FinalizerSkillProtect = "ai-keeper.io/skill-protect"

// Reasons reported through Conditions / Events. Mirrors design.md
// §6.1.3 and Requirements A3.2 / A3.4 / A3.5 / A3.6.
const (
	ReasonInvalidSchema             = "InvalidSchema"
	ReasonMissingReference          = "MissingReference"
	ReasonMissingReferencePermanent = "MissingReferencePermanent"
	ReasonCyclicDependency          = "CyclicDependency"
	ReasonImplementationNotReady    = "ImplementationNotReady"
	ReasonRegistrationFailed        = "RegistrationFailed"
	ReasonEvalNotImplemented        = "EvalNotImplemented"
	ReasonExperimentalAutoPass      = "ExperimentalAutoPass"
	ReasonReady                     = "Ready"
	ReasonNotReady                  = "NotReady"
	ReasonDeletionBlocked           = "DeletionBlocked"
	ReasonDeprecated                = "Deprecated"
)

// EventReasonSkillDeletionBlocked is the K8s Event reason published when
// the Skill controller refuses to drain a Skill that still has
// `status.referencingAgents` (Requirement A3.11).
const EventReasonSkillDeletionBlocked = "SkillDeletionBlocked"

// ErrSkillNotRegistered is returned by [Registry.Deregister] when the
// caller attempts to drop a Skill that was never recorded.
var ErrSkillNotRegistered = errors.New("skill: not registered")

// Registry persists `Skill@version` records on behalf of the Skill
// controller. Real implementations land in task 16.1 (PostgreSQL-backed)
// or are exposed by an out-of-process Skill Registry service; for P0 we
// ship the in-memory [MemoryRegistry] below so the controller is fully
// exercisable in tests and on a fresh dev cluster.
type Registry interface {
	// Register records (or updates) the supplied Skill in the registry.
	// Implementations MUST be idempotent: re-registering the same Skill
	// with the same `spec.version` MUST NOT return an error.
	Register(ctx context.Context, skill *skillv1alpha1.Skill) error

	// Deregister removes the Skill addressed by `ref` from the registry.
	// Implementations SHOULD return [ErrSkillNotRegistered] when the
	// Skill is unknown so callers can distinguish "already gone" from
	// transport errors.
	Deregister(ctx context.Context, ref sharedv1alpha1.ResourceRef) error
}

// MemoryRegistry is the P0 in-memory [Registry] implementation. It is
// safe for concurrent use and is the default wired into reconciler
// tests. Production deployments swap it out via [SkillReconciler.Registry].
type MemoryRegistry struct {
	mu      sync.RWMutex
	entries map[sharedv1alpha1.ResourceRef]MemoryRegistryEntry
}

// MemoryRegistryEntry records the canonical fields a Skill consumer
// (Agent runtime, Skill_Registry export) needs to know about the
// registered Skill.
type MemoryRegistryEntry struct {
	Ref       sharedv1alpha1.ResourceRef
	Version   sharedv1alpha1.SemVer
	Stability sharedv1alpha1.Stage
}

// NewMemoryRegistry constructs an empty MemoryRegistry.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{entries: map[sharedv1alpha1.ResourceRef]MemoryRegistryEntry{}}
}

// Register inserts or updates the Skill keyed by its canonical
// ResourceRef. Returns an error iff the supplied Skill cannot be
// addressed — for example when `metadata.name` is empty.
func (r *MemoryRegistry) Register(_ context.Context, skill *skillv1alpha1.Skill) error {
	if r == nil {
		return errors.New("skill registry: nil receiver")
	}
	if skill == nil {
		return errors.New("skill registry: nil skill")
	}
	ref, err := SkillResourceRef(skill)
	if err != nil {
		return fmt.Errorf("skill registry: build ref: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[ref] = MemoryRegistryEntry{
		Ref:       ref,
		Version:   skill.Spec.Version,
		Stability: skill.Spec.Stability,
	}
	return nil
}

// Deregister removes the Skill addressed by `ref`.
func (r *MemoryRegistry) Deregister(_ context.Context, ref sharedv1alpha1.ResourceRef) error {
	if r == nil {
		return errors.New("skill registry: nil receiver")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[ref]; !ok {
		return ErrSkillNotRegistered
	}
	delete(r.entries, ref)
	return nil
}

// Snapshot returns a copy of the current registry state. Useful for
// assertions in unit tests.
func (r *MemoryRegistry) Snapshot() []MemoryRegistryEntry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MemoryRegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

// Has reports whether the given ref is currently registered.
func (r *MemoryRegistry) Has(ref sharedv1alpha1.ResourceRef) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.entries[ref]
	return ok
}

// SkillResourceRef builds the canonical `skill://<namespace>/<name>@<version>`
// ResourceRef for the supplied Skill. Used by the controller to address
// the Skill in the Registry and in domain events.
func SkillResourceRef(skill *skillv1alpha1.Skill) (sharedv1alpha1.ResourceRef, error) {
	if skill == nil {
		return "", errors.New("skill: nil")
	}
	if skill.Name == "" {
		return "", errors.New("skill: empty metadata.name")
	}
	ns := skill.Namespace
	if ns == "" {
		ns = "default"
	}
	path := ns + "/" + skill.Name
	return sharedv1alpha1.FormatResourceRef(sharedv1alpha1.SchemeSkill, path, string(skill.Spec.Version))
}

// Compile-time interface assertions.
var _ Registry = (*MemoryRegistry)(nil)
