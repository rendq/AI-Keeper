// Package merge provides a three-way merge engine for Industry Pack upgrades.
// When upgrading a pack, we merge: base (pack old version), theirs (pack new version),
// and ours (user's current customized state).
// Covers requirement C5.4.
package merge

import "fmt"

// ResolutionHint suggests how a conflict might be resolved.
type ResolutionHint string

const (
	// HintAcceptTheirs suggests taking the upstream (new pack) value.
	HintAcceptTheirs ResolutionHint = "accept-theirs"

	// HintAcceptOurs suggests keeping the user's customized value.
	HintAcceptOurs ResolutionHint = "accept-ours"

	// HintManual indicates the conflict requires manual resolution.
	HintManual ResolutionHint = "manual"
)

// Conflict describes a single merge conflict at a specific field path.
type Conflict struct {
	// Path is the dot-separated path to the conflicting field (e.g. "spec.replicas").
	Path string

	// Base is the original value from the old pack version (nil if absent).
	Base interface{}

	// Ours is the user's current value (nil if deleted by user).
	Ours interface{}

	// Theirs is the new pack version's value (nil if deleted upstream).
	Theirs interface{}

	// Hint provides a suggested resolution strategy.
	Hint ResolutionHint

	// Reason explains why this conflict occurred.
	Reason string
}

// ConflictReport aggregates all conflicts from a merge operation with
// human-readable guidance.
type ConflictReport struct {
	// ResourceKey identifies the resource (e.g. "Deployment/my-app").
	ResourceKey string

	// Conflicts lists all detected conflicts for this resource.
	Conflicts []Conflict
}

// HasConflicts returns true if any conflicts exist in the report.
func (r *ConflictReport) HasConflicts() bool {
	return len(r.Conflicts) > 0
}

// Summary returns a human-readable summary of all conflicts.
func (r *ConflictReport) Summary() string {
	if !r.HasConflicts() {
		return fmt.Sprintf("resource %s: no conflicts", r.ResourceKey)
	}
	msg := fmt.Sprintf("resource %s: %d conflict(s)\n", r.ResourceKey, len(r.Conflicts))
	for i, c := range r.Conflicts {
		msg += fmt.Sprintf("  [%d] path=%q hint=%s reason=%s\n", i+1, c.Path, c.Hint, c.Reason)
	}
	return msg
}

// classifyConflict determines the resolution hint for a given conflict scenario.
func classifyConflict(path string, base, ours, theirs interface{}) Conflict {
	c := Conflict{
		Path:   path,
		Base:   base,
		Ours:   ours,
		Theirs: theirs,
	}

	switch {
	// User deleted, upstream modified — likely accept-theirs (new feature).
	case ours == nil && theirs != nil:
		c.Hint = HintAcceptTheirs
		c.Reason = "user deleted field but upstream modified it"

	// Upstream deleted, user modified — likely accept-ours (user customization).
	case theirs == nil && ours != nil:
		c.Hint = HintAcceptOurs
		c.Reason = "upstream deleted field but user modified it"

	// Both added independently with different values.
	case base == nil && ours != nil && theirs != nil:
		c.Hint = HintManual
		c.Reason = "both sides added this field with different values"

	// All three differ — full conflict requiring manual review.
	default:
		c.Hint = HintManual
		c.Reason = "both user and upstream modified this field"
	}

	return c
}
