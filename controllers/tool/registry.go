package tool

import (
	"context"
	"errors"
	"fmt"
	"sync"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// ErrToolNotRegistered is returned by [Registry.Deregister] when the
// caller attempts to drop a Tool that was never recorded.
var ErrToolNotRegistered = errors.New("tool: not registered")

// Registry persists `Tool@version` records on behalf of the Tool
// controller. The real implementation lives in the Tool_Registry
// service introduced by task 16.1 (PostgreSQL-backed); for P0 this
// package ships [MemoryRegistry] so the controller is fully exercisable
// in unit tests and on a fresh dev cluster.
type Registry interface {
	// Register records (or updates) the supplied Tool in the registry.
	// Implementations MUST be idempotent — re-registering the same
	// Tool returns nil.
	Register(ctx context.Context, tool *skillv1alpha1.Tool) error

	// Deregister removes the Tool addressed by `ref` from the registry.
	// Implementations SHOULD return [ErrToolNotRegistered] when the
	// Tool is unknown so callers can distinguish "already gone" from
	// transport errors.
	Deregister(ctx context.Context, ref sharedv1alpha1.ResourceRef) error
}

// MemoryRegistryEntry records the canonical fields a Tool consumer
// (Agent runtime, Tool_Registry export) needs to know about the
// registered Tool.
type MemoryRegistryEntry struct {
	Ref      sharedv1alpha1.ResourceRef
	Endpoint string
	Protocol string
}

// MemoryRegistry is the P0 in-memory [Registry] implementation. It is
// safe for concurrent use and is the default wired into reconciler
// tests. Production deployments swap it out via [ToolReconciler.Registry].
type MemoryRegistry struct {
	mu      sync.RWMutex
	entries map[sharedv1alpha1.ResourceRef]MemoryRegistryEntry
}

// NewMemoryRegistry constructs an empty MemoryRegistry.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{entries: map[sharedv1alpha1.ResourceRef]MemoryRegistryEntry{}}
}

// Register inserts or updates the Tool keyed by its canonical
// ResourceRef.
func (r *MemoryRegistry) Register(_ context.Context, tool *skillv1alpha1.Tool) error {
	if r == nil {
		return errors.New("tool registry: nil receiver")
	}
	if tool == nil {
		return errors.New("tool registry: nil tool")
	}
	ref, err := ToolResourceRef(tool)
	if err != nil {
		return fmt.Errorf("tool registry: build ref: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[ref] = MemoryRegistryEntry{
		Ref:      ref,
		Endpoint: tool.Spec.Endpoint,
		Protocol: tool.Spec.Protocol,
	}
	return nil
}

// Deregister removes the Tool addressed by `ref`.
func (r *MemoryRegistry) Deregister(_ context.Context, ref sharedv1alpha1.ResourceRef) error {
	if r == nil {
		return errors.New("tool registry: nil receiver")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[ref]; !ok {
		return ErrToolNotRegistered
	}
	delete(r.entries, ref)
	return nil
}

// Snapshot returns a copy of the current registry state.
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

// ToolResourceRef builds the canonical
// `tool://<namespace>/<name>` ResourceRef for the supplied Tool. We
// don't include `@version` because the Tool CRD does not currently
// have a `spec.version` field — versioning is handled inside the
// connector configuration.
func ToolResourceRef(tool *skillv1alpha1.Tool) (sharedv1alpha1.ResourceRef, error) {
	if tool == nil {
		return "", errors.New("tool: nil")
	}
	if tool.Name == "" {
		return "", errors.New("tool: empty metadata.name")
	}
	ns := tool.Namespace
	if ns == "" {
		ns = "default"
	}
	path := ns + "/" + tool.Name
	return sharedv1alpha1.FormatResourceRef(sharedv1alpha1.SchemeTool, path, "")
}

// Compile-time interface assertions.
var _ Registry = (*MemoryRegistry)(nil)
