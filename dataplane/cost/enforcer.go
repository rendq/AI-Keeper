package cost

import (
	"context"
	"sort"
	"time"
)

// AlertAction defines what happens when a budget alert threshold is crossed.
type AlertAction string

const (
	// ActionNotify sends a notification but allows the request.
	ActionNotify AlertAction = "notify"
	// ActionThrottle slows down requests but still allows them.
	ActionThrottle AlertAction = "throttle"
	// ActionBlock denies the request entirely.
	ActionBlock AlertAction = "block"
)

// AlertConfig defines a single alert threshold and its associated action.
type AlertConfig struct {
	// Threshold is the fraction of budget (0.0–1.0) at which this alert triggers.
	Threshold float64
	// Channel is the notification channel (e.g., "slack", "email", "webhook").
	Channel string
	// Action determines the enforcement behavior when this threshold is crossed.
	Action AlertAction
}

// BudgetDecision is the result of a budget enforcement check.
type BudgetDecision struct {
	// Allowed indicates whether the request should proceed.
	Allowed bool
	// Action describes what enforcement action was taken (e.g., "allow", "notify", "throttle", "block").
	Action string
	// Reason provides a human-readable explanation of the decision.
	Reason string
	// ThrottleDelay is the suggested delay before processing the request (only relevant when Action == "throttle").
	ThrottleDelay time.Duration
}

// BudgetEnforcer evaluates current usage against budget limits and alert thresholds,
// producing enforcement decisions (allow, notify, throttle, block) and supporting
// budget rollover between periods.
type BudgetEnforcer struct {
	// Alerts is a list of alert configurations sorted by ascending threshold.
	Alerts []AlertConfig
	// ThrottleBase is the base delay applied when throttle action is triggered.
	// The actual delay scales with how far usage exceeds the throttle threshold.
	ThrottleBase time.Duration
}

// NewBudgetEnforcer creates a BudgetEnforcer with the given alert configs.
// Alerts are sorted by threshold (ascending) internally.
func NewBudgetEnforcer(alerts []AlertConfig, throttleBase time.Duration) *BudgetEnforcer {
	// Copy and sort alerts by threshold ascending.
	sorted := make([]AlertConfig, len(alerts))
	copy(sorted, alerts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Threshold < sorted[j].Threshold
	})

	if throttleBase == 0 {
		throttleBase = 500 * time.Millisecond
	}

	return &BudgetEnforcer{
		Alerts:       sorted,
		ThrottleBase: throttleBase,
	}
}

// Check evaluates current usage against the budget limit and alert thresholds.
// It returns a BudgetDecision indicating whether the request is allowed and any
// enforcement actions to take.
//
// Decision logic:
//   - usage >= limit (hardCap) → deny (block)
//   - usage >= highest triggered threshold with action=throttle → allow + throttle delay
//   - usage >= highest triggered threshold with action=notify → allow + notify
//   - usage < lowest alert threshold → allow unconditionally
func (e *BudgetEnforcer) Check(_ context.Context, tenantID string, currentUsage, limit float64) BudgetDecision {
	// If no limit configured, always allow.
	if limit <= 0 {
		return BudgetDecision{
			Allowed: true,
			Action:  "allow",
			Reason:  "no budget limit configured",
		}
	}

	ratio := currentUsage / limit

	// Hard cap: usage at or above limit → block.
	if ratio >= 1.0 {
		return BudgetDecision{
			Allowed: false,
			Action:  "block",
			Reason:  "budget exhausted for tenant " + tenantID,
		}
	}

	// Walk alerts from highest threshold down to find the most severe triggered alert.
	for i := len(e.Alerts) - 1; i >= 0; i-- {
		alert := e.Alerts[i]
		if ratio >= alert.Threshold {
			switch alert.Action {
			case ActionBlock:
				return BudgetDecision{
					Allowed: false,
					Action:  "block",
					Reason:  "budget alert block threshold reached for tenant " + tenantID,
				}
			case ActionThrottle:
				// Scale delay: the further past the threshold, the longer the delay.
				overage := ratio - alert.Threshold
				remaining := 1.0 - alert.Threshold
				scale := 1.0
				if remaining > 0 {
					scale = 1.0 + (overage/remaining)*4.0 // up to 5x base at hard cap
				}
				delay := time.Duration(float64(e.ThrottleBase) * scale)
				return BudgetDecision{
					Allowed:       true,
					Action:        "throttle",
					Reason:        "budget throttle threshold reached for tenant " + tenantID,
					ThrottleDelay: delay,
				}
			case ActionNotify:
				return BudgetDecision{
					Allowed: true,
					Action:  "notify",
					Reason:  "budget alert threshold reached for tenant " + tenantID,
				}
			}
		}
	}

	// Below all thresholds — allow unconditionally.
	return BudgetDecision{
		Allowed: true,
		Action:  "allow",
		Reason:  "usage within budget",
	}
}

// Rollover calculates the new budget limit for the next period by carrying over
// unused budget from the current period.
//
// The new limit is: nextLimit + unused (capped at 2x nextLimit to prevent unbounded accumulation).
// If unused is negative (overspend), it is ignored (no penalty carried forward).
func (e *BudgetEnforcer) Rollover(_ context.Context, _ string, unused, nextLimit float64) float64 {
	if unused <= 0 {
		// No rollover if the period was overspent or exactly consumed.
		return nextLimit
	}

	newLimit := nextLimit + unused

	// Cap rollover at 2x the base period limit to prevent unbounded accumulation.
	maxLimit := nextLimit * 2.0
	if newLimit > maxLimit {
		return maxLimit
	}

	return newLimit
}
