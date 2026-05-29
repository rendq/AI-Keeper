package policy

import (
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// ConflictType enumerates the conflict categories from design.md
// §6.3.3:
//
//	Hard:       same priority, complete subject + resource overlap,
//	            opposite effect ⇒ controller refuses to distribute.
//	Soft:       same priority, partial subject + resource overlap,
//	            opposite effect ⇒ controller distributes but emits a
//	            Warning event so operators can disambiguate.
//	Shadow:     a higher-priority allow fully covers a lower-priority
//	            deny ⇒ warn only.
//	Redundant:  same priority + same effect + complete overlap ⇒ warn.
//	Tautology:  the policy's `conditions` always evaluate true / false
//	            ⇒ warn.
type ConflictType string

// Canonical conflict types.
const (
	ConflictHard      ConflictType = "Hard"
	ConflictSoft      ConflictType = "Soft"
	ConflictShadow    ConflictType = "Shadow"
	ConflictRedundant ConflictType = "Redundant"
	ConflictTautology ConflictType = "Tautology"
)

// Conflict captures a single detected conflict between two Policies.
// The reconciler echoes a flattened version of this struct onto
// `Policy.status.conflicts`.
type Conflict struct {
	// Type identifies the conflict category.
	Type ConflictType

	// A is the namespaced name of the first conflicting Policy in the
	// canonical form `namespace/name`.
	A string

	// B is the namespaced name of the second conflicting Policy.
	// Empty for `Tautology`, which involves a single Policy.
	B string

	// Reason is a human-readable explanation surfaced through
	// `status.conflicts[*].reason`.
	Reason string
}

// Involves reports whether `key` (`namespace/name`) appears as either
// A or B of the conflict.
func (c Conflict) Involves(key string) bool {
	return c.A == key || c.B == key
}

// IsHard reports whether the conflict blocks distribution.
func (c Conflict) IsHard() bool { return c.Type == ConflictHard }

// ConflictDetector inspects every Policy in a single namespace and
// returns the detected conflicts. Real implementations land in task 5.2;
// the [NoopConflictDetector] below makes the reconciler driveable in P0.
type ConflictDetector interface {
	Detect(policies []*policyv1alpha1.Policy) ([]Conflict, error)
}

// NoopConflictDetector returns no conflicts.
type NoopConflictDetector struct{}

// Detect always returns an empty slice and no error.
func (NoopConflictDetector) Detect(_ []*policyv1alpha1.Policy) ([]Conflict, error) {
	return nil, nil
}

// FuncConflictDetector adapts a plain function to the
// [ConflictDetector] interface. Useful in unit tests where a closure
// is more readable than a dedicated mock type.
type FuncConflictDetector func(policies []*policyv1alpha1.Policy) ([]Conflict, error)

// Detect delegates to the wrapped function.
func (f FuncConflictDetector) Detect(policies []*policyv1alpha1.Policy) ([]Conflict, error) {
	return f(policies)
}

// Compile-time interface assertions.
var (
	_ ConflictDetector = NoopConflictDetector{}
	_ ConflictDetector = FuncConflictDetector(nil)
)
