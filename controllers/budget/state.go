package budget

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// FinalizerBudgetProtect is the finalizer added to every reconciled
// Budget CR so the controller can drain Budget_Enforcer state on
// deletion (Requirement A8.1 — basic finalizer; real drain lands in
// P1).
const FinalizerBudgetProtect = "ai-keeper.io/budget-protect"

// SteadyStateRequeue is the periodic settlement cadence applied while
// the budget is stable. Tighter than the long-tail 5/10 minutes used
// by other Kinds so dashboards see fresh `current` values quickly.
const SteadyStateRequeue = time.Minute

// BurnRate classifications surfaced on `status.burnRate`. Mirrors the
// `+kubebuilder:validation:Enum` markers on
// [policyv1alpha1.BudgetStatus.BurnRate].
const (
	BurnRateOK        = "ok"
	BurnRateWarning   = "warning"
	BurnRateCritical  = "critical"
	BurnRateExhausted = "exhausted"
)

// Reason constants surfaced on Budget conditions and Events.
const (
	// ReasonEnforcerReady marks `EnforcerReady=True`. P0 placeholder
	// — the data plane reads `status` directly.
	ReasonEnforcerReady = "EnforcerReady"

	// ReasonWithinLimit marks `WithinLimit=True`.
	ReasonWithinLimit = "WithinLimit"
	// ReasonExhausted marks `WithinLimit=False`.
	ReasonExhausted = "Exhausted"

	// ReasonInvalidPeriod marks any condition False when
	// `spec.period` cannot be resolved into a (start, end) pair. The
	// kubebuilder enum already gates this at admission; the constant
	// exists so the controller can surface a deterministic reason if
	// a future change widens the enum.
	ReasonInvalidPeriod = "InvalidPeriod"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// Period names accepted on `spec.period`. Mirrors the
// `+kubebuilder:validation:Enum` markers on
// [policyv1alpha1.BudgetSpec.Period].
const (
	PeriodHourly    = "hourly"
	PeriodDaily     = "daily"
	PeriodWeekly    = "weekly"
	PeriodMonthly   = "monthly"
	PeriodQuarterly = "quarterly"
	PeriodYearly    = "yearly"
)

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. Aggregate Ready=True → Active
//  3. `WithinLimit=False` → Failed (operator action required)
//  4. Otherwise → Pending
func derivePhase(b *policyv1alpha1.Budget) sharedv1alpha1.Phase {
	if b == nil {
		return sharedv1alpha1.PhasePending
	}
	if !b.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := b.Status.Conditions
	if isTrue(conds, policyv1alpha1.BudgetReady) {
		return sharedv1alpha1.PhaseActive
	}
	if c := condition(conds, policyv1alpha1.BudgetWithinLimit); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseFailed
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic for
// Budget: EnforcerReady=True ∧ WithinLimit=True.
func readyFromConditions(b *policyv1alpha1.Budget) (status, reason, message string) {
	conds := b.Status.Conditions
	gates := []string{
		policyv1alpha1.BudgetEnforcerReady,
		policyv1alpha1.BudgetWithinLimit,
	}
	for _, t := range gates {
		if !isTrue(conds, t) {
			return string(metav1.ConditionFalse), ReasonNotReady, t + " not satisfied"
		}
	}
	return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
}

// condition returns a pointer to the named condition, or nil.
func condition(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}

// isTrue reports whether the named condition is present and True.
func isTrue(conds []metav1.Condition, t string) bool {
	c := condition(conds, t)
	return c != nil && c.Status == metav1.ConditionTrue
}

// computePeriod returns the (start, end) UTC anchors for the supplied
// `spec.period` and wall-clock time. The boundaries follow the
// conventions documented in [doc.go]: weekly aligns on Monday 00:00,
// quarterly aligns on Jan/Apr/Jul/Oct 00:00, etc.
//
// An unknown period name returns ErrInvalidPeriod so the caller can
// surface a deterministic condition reason.
func computePeriod(period string, now time.Time) (time.Time, time.Time, error) {
	t := now.UTC()
	switch period {
	case PeriodHourly:
		start := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
		return start, start.Add(time.Hour), nil
	case PeriodDaily:
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.Add(24 * time.Hour), nil
	case PeriodWeekly:
		// Monday-anchored. Weekday returns Sunday=0 ... Saturday=6;
		// shift so Monday=0.
		dow := int(t.Weekday()-time.Monday+7) % 7
		monday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).
			AddDate(0, 0, -dow)
		return monday, monday.AddDate(0, 0, 7), nil
	case PeriodMonthly:
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	case PeriodQuarterly:
		// time.Month is 1-based; quarter index 0..3.
		q := int((t.Month() - 1) / 3)
		startMonth := time.Month(q*3 + 1)
		start := time.Date(t.Year(), startMonth, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 3, 0), nil
	case PeriodYearly:
		start := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(1, 0, 0), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, period)
	}
}

// classifyBurnRate maps the highest dimension utilisation ratio to a
// coarse bucket. Ratios are computed only for dimensions where both
// the limit and the current value are populated; an unconstrained
// dimension does not influence the classification.
//
// When no populated limits exist the function returns [BurnRateOK]
// because there is nothing to overshoot.
func classifyBurnRate(limits policyv1alpha1.BudgetLimits, current *policyv1alpha1.BudgetCurrent) string {
	max := highestUtilisation(limits, current)
	switch {
	case max >= 1.0:
		return BurnRateExhausted
	case max >= 0.80:
		return BurnRateCritical
	case max >= 0.50:
		return BurnRateWarning
	default:
		return BurnRateOK
	}
}

// highestUtilisation returns the highest current/limit ratio across
// populated dimensions. Returns 0 when no dimension has both fields
// populated.
func highestUtilisation(limits policyv1alpha1.BudgetLimits, current *policyv1alpha1.BudgetCurrent) float64 {
	if current == nil {
		return 0
	}
	max := 0.0
	if limits.Usd != nil && current.Usd != nil {
		if cur, ok := parseMoney(*current.Usd); ok {
			if lim, ok := parseMoney(*limits.Usd); ok && lim > 0 {
				max = maxFloat(max, cur/lim)
			}
		}
	}
	if limits.Tokens != nil && *limits.Tokens > 0 {
		max = maxFloat(max, float64(current.Tokens)/float64(*limits.Tokens))
	}
	if limits.Calls != nil && *limits.Calls > 0 {
		max = maxFloat(max, float64(current.Calls)/float64(*limits.Calls))
	}
	return max
}

// withinLimit reports whether every populated current dimension is
// strictly less than its corresponding limit. A nil limit means
// "unlimited" for that dimension; a nil current value is treated as
// zero.
func withinLimit(limits policyv1alpha1.BudgetLimits, current *policyv1alpha1.BudgetCurrent) bool {
	if current == nil {
		return true
	}
	if limits.Usd != nil && current.Usd != nil {
		if cur, ok := parseMoney(*current.Usd); ok {
			if lim, ok := parseMoney(*limits.Usd); ok && cur >= lim {
				return false
			}
		}
	}
	if limits.Tokens != nil && current.Tokens >= *limits.Tokens {
		return false
	}
	if limits.Calls != nil && current.Calls >= *limits.Calls {
		return false
	}
	return true
}

// parseMoney parses a [sharedv1alpha1.MoneyAmount] (string-encoded
// non-negative decimal) into a float64. Returns ok=false when the
// string is empty or malformed; the caller should treat such inputs
// as "unconstrained".
func parseMoney(m sharedv1alpha1.MoneyAmount) (float64, bool) {
	s := string(m)
	if s == "" {
		return 0, false
	}
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	if err != nil {
		return 0, false
	}
	return v, true
}

// maxFloat returns the larger of a and b.
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
