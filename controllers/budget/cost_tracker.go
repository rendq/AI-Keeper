package budget

import (
	"context"
	"errors"
	"fmt"
	"sync"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// ErrInvalidPeriod is returned by [computePeriod] when the supplied
// period name is not one of the canonical [PeriodHourly]…
// [PeriodYearly] constants.
var ErrInvalidPeriod = errors.New("budget: invalid period")

// ScopeKey is the canonical identifier passed to [CostTrackerClient]
// methods. It mirrors `spec.scope` on the Budget CR so the data
// plane can index its in-memory counters by a stable tuple.
type ScopeKey struct {
	// Kind is one of `Tenant`, `Team`, `User`, `Agent`, `Skill`,
	// `Project` (kubebuilder enum on [policyv1alpha1.BudgetScope.Kind]).
	Kind string
	// Name is the scope target.
	Name string
	// Period mirrors the Budget's `spec.period` so the tracker can
	// pick the correct counter window.
	Period string
}

// String returns a stable textual encoding of the scope for log /
// debug output.
func (s ScopeKey) String() string {
	return fmt.Sprintf("%s/%s@%s", s.Kind, s.Name, s.Period)
}

// CostTrackerClient is the abstraction the Budget controller depends
// on for looking up the running spend of a scope. Task 13.1 will own
// the real implementation (Redis-backed counters); for P0 we ship
// [NoopCostTracker] so the controller is exercisable in unit tests
// and dev clusters where no tracker is provisioned.
//
// Implementations MUST be safe for concurrent use — the controller-
// runtime workqueue may invoke Current from multiple goroutines.
//
// The contract is intentionally minimal:
//
//   - Current(ctx, scope) returns the spend snapshot for the supplied
//     scope. A nil snapshot is treated as zero spend; a non-nil error
//     bubbles up to the reconciler and triggers a backoff requeue.
type CostTrackerClient interface {
	Current(ctx context.Context, scope ScopeKey) (*policyv1alpha1.BudgetCurrent, error)
}

// NoopCostTracker is an in-memory [CostTrackerClient] used by unit
// tests and dev clusters. It returns the seeded snapshot on every
// call and counts how many times Current was invoked.
//
// Tests can pre-load `Snapshots` (keyed by [ScopeKey]) to express
// per-scope behaviour, or set `Default` for a uniform value across
// scopes. `Err`, when non-nil, is returned to the caller before any
// snapshot lookup.
type NoopCostTracker struct {
	mu sync.Mutex

	// Default is returned when no per-scope override matches the
	// caller's [ScopeKey]. Nil means "zero spend".
	Default *policyv1alpha1.BudgetCurrent

	// Snapshots overrides [Default] for specific scopes.
	Snapshots map[ScopeKey]*policyv1alpha1.BudgetCurrent

	// Err lets tests inject a transport failure.
	Err error

	// Calls counts Current invocations. Useful for assertions.
	Calls int
}

// NewNoopCostTracker returns a NoopCostTracker that reports zero
// spend across every scope.
func NewNoopCostTracker() *NoopCostTracker {
	return &NoopCostTracker{Snapshots: map[ScopeKey]*policyv1alpha1.BudgetCurrent{}}
}

// Current implements [CostTrackerClient].
func (n *NoopCostTracker) Current(_ context.Context, scope ScopeKey) (*policyv1alpha1.BudgetCurrent, error) {
	if n == nil {
		return nil, errors.New("budget: nil cost tracker")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Calls++
	if n.Err != nil {
		return nil, n.Err
	}
	if v, ok := n.Snapshots[scope]; ok {
		return cloneCurrent(v), nil
	}
	return cloneCurrent(n.Default), nil
}

// Snapshot returns the recorded call count under the mutex.
func (n *NoopCostTracker) Snapshot() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.Calls
}

// Set stores a per-scope snapshot for subsequent Current calls.
func (n *NoopCostTracker) Set(scope ScopeKey, current *policyv1alpha1.BudgetCurrent) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Snapshots == nil {
		n.Snapshots = map[ScopeKey]*policyv1alpha1.BudgetCurrent{}
	}
	n.Snapshots[scope] = cloneCurrent(current)
}

// cloneCurrent returns a shallow copy of `c` so callers can mutate
// the result without racing with the tracker's bookkeeping.
func cloneCurrent(c *policyv1alpha1.BudgetCurrent) *policyv1alpha1.BudgetCurrent {
	if c == nil {
		return nil
	}
	return c.DeepCopy()
}

// Compile-time interface assertion.
var _ CostTrackerClient = (*NoopCostTracker)(nil)
