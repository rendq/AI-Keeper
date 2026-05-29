package budget_test

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/budget"
)

// fakeClock returns a frozen wall-clock time for deterministic
// period-boundary computation.
type fakeClock struct{ t time.Time }

func (f fakeClock) Now() time.Time { return f.t }

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := policyv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register policy.ai-keeper.io scheme: %v", err)
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&policyv1alpha1.Budget{}).
		Build()
	return c, s
}

type budgetBuilder struct {
	b *policyv1alpha1.Budget
}

func newBudget(name string) *budgetBuilder {
	hardCap := true
	return &budgetBuilder{
		b: &policyv1alpha1.Budget{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "tenant-acme",
				Generation: 1,
			},
			Spec: policyv1alpha1.BudgetSpec{
				Scope: policyv1alpha1.BudgetScope{
					Kind: "Tenant",
					Name: "acme",
				},
				Period:  budget.PeriodMonthly,
				HardCap: &hardCap,
			},
		},
	}
}

func (b *budgetBuilder) withPeriod(p string) *budgetBuilder {
	b.b.Spec.Period = p
	return b
}

func (b *budgetBuilder) withUSDLimit(amount string) *budgetBuilder {
	v := sharedv1alpha1.MoneyAmount(amount)
	b.b.Spec.Limits.Usd = &v
	return b
}

func (b *budgetBuilder) withTokensLimit(n int64) *budgetBuilder {
	b.b.Spec.Limits.Tokens = &n
	return b
}

func (b *budgetBuilder) withCallsLimit(n int64) *budgetBuilder {
	b.b.Spec.Limits.Calls = &n
	return b
}

func (b *budgetBuilder) build() *policyv1alpha1.Budget { return b.b }

func newReconciler(t *testing.T, tracker budget.CostTrackerClient, clk budget.Clock, objs ...client.Object) (*budget.BudgetReconciler, client.Client) {
	t.Helper()
	c, s := newFakeClient(t, objs...)
	return &budget.BudgetReconciler{
		Client:      c,
		Scheme:      s,
		CostTracker: tracker,
		Clock:       clk,
	}, c
}

func reconcileOnce(t *testing.T, r *budget.BudgetReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *budget.BudgetReconciler, key types.NamespacedName, max int) reconcile.Result {
	t.Helper()
	var last reconcile.Result
	for i := 0; i < max; i++ {
		last = reconcileOnce(t, r, key)
		if !last.Requeue {
			return last
		}
	}
	t.Fatalf("Reconcile did not reach steady state after %d passes", max)
	return last
}

func getBudget(t *testing.T, c client.Client, key types.NamespacedName) *policyv1alpha1.Budget {
	t.Helper()
	got := &policyv1alpha1.Budget{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get budget: %v", err)
	}
	return got
}

func conditionStatus(conds []metav1.Condition, condType string) metav1.ConditionStatus {
	for _, c := range conds {
		if c.Type == condType {
			return c.Status
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestReconcile_HappyPath exercises Requirement A8.1: a freshly-
// created Budget initialises `status.{periodStart, periodEnd,
// current, burnRate}` and reaches Phase=Active.
func TestReconcile_HappyPath(t *testing.T) {
	t.Parallel()

	clk := fakeClock{t: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	tracker := budget.NewNoopCostTracker()
	usd := sharedv1alpha1.MoneyAmount("10.00")
	tracker.Default = &policyv1alpha1.BudgetCurrent{Usd: &usd, Tokens: 1000, Calls: 5}

	b := newBudget("default").withUSDLimit("100.00").withTokensLimit(100000).build()
	r, c := newReconciler(t, tracker, clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}

	last := reconcileToSteady(t, r, key, 4)

	got := getBudget(t, c, key)
	if !controllerutil.ContainsFinalizer(got, budget.FinalizerBudgetProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.BudgetWithinLimit); status != metav1.ConditionTrue {
		t.Fatalf("WithinLimit = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.BudgetEnforcerReady); status != metav1.ConditionTrue {
		t.Fatalf("EnforcerReady = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.BudgetReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.PeriodStart == nil || got.Status.PeriodStart.UTC() != time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("periodStart = %v, want 2024-03-01", got.Status.PeriodStart)
	}
	if got.Status.PeriodEnd == nil || got.Status.PeriodEnd.UTC() != time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("periodEnd = %v, want 2024-04-01", got.Status.PeriodEnd)
	}
	if got.Status.Current == nil || got.Status.Current.Calls != 5 {
		t.Fatalf("current = %+v, want calls=5", got.Status.Current)
	}
	if got.Status.BurnRate != budget.BurnRateOK {
		t.Fatalf("burnRate = %q, want %q", got.Status.BurnRate, budget.BurnRateOK)
	}
	if got.Status.ObservedGeneration != got.Generation {
		t.Fatalf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
	if last.RequeueAfter != budget.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, budget.SteadyStateRequeue)
	}
}

// TestReconcile_Exhausted exercises Requirement A8.3: a Budget whose
// current spend reaches the limit flips WithinLimit=False and Phase=Failed.
func TestReconcile_Exhausted(t *testing.T) {
	t.Parallel()

	clk := fakeClock{t: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	tracker := budget.NewNoopCostTracker()
	usd := sharedv1alpha1.MoneyAmount("100.00")
	tracker.Default = &policyv1alpha1.BudgetCurrent{Usd: &usd, Tokens: 50000, Calls: 1}

	b := newBudget("over-budget").withUSDLimit("100.00").build()
	r, c := newReconciler(t, tracker, clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}

	reconcileToSteady(t, r, key, 4)

	got := getBudget(t, c, key)
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.BudgetWithinLimit); status != metav1.ConditionFalse {
		t.Fatalf("WithinLimit = %s, want False", status)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.BudgetReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if got.Status.BurnRate != budget.BurnRateExhausted {
		t.Fatalf("burnRate = %q, want %q", got.Status.BurnRate, budget.BurnRateExhausted)
	}
}

// TestReconcile_BurnRateClassification exercises the four burn-rate
// buckets across populated dimensions.
func TestReconcile_BurnRateClassification(t *testing.T) {
	t.Parallel()

	clk := fakeClock{t: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}

	cases := []struct {
		name    string
		current *policyv1alpha1.BudgetCurrent
		want    string
	}{
		{
			name:    "ok_under_50",
			current: &policyv1alpha1.BudgetCurrent{Tokens: 100},
			want:    budget.BurnRateOK,
		},
		{
			name:    "warning_50_to_79",
			current: &policyv1alpha1.BudgetCurrent{Tokens: 600},
			want:    budget.BurnRateWarning,
		},
		{
			name:    "critical_80_to_99",
			current: &policyv1alpha1.BudgetCurrent{Tokens: 850},
			want:    budget.BurnRateCritical,
		},
		{
			name:    "exhausted_at_100",
			current: &policyv1alpha1.BudgetCurrent{Tokens: 1000},
			want:    budget.BurnRateExhausted,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tracker := budget.NewNoopCostTracker()
			tracker.Default = tc.current
			b := newBudget("burn-" + tc.name).withTokensLimit(1000).build()
			r, c := newReconciler(t, tracker, clk, b)
			key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}
			reconcileToSteady(t, r, key, 4)
			got := getBudget(t, c, key)
			if got.Status.BurnRate != tc.want {
				t.Fatalf("burnRate = %q, want %q", got.Status.BurnRate, tc.want)
			}
		})
	}
}

// TestReconcile_PeriodBoundaries exercises every supported `spec.period`
// against the canonical anchor table from doc.go.
func TestReconcile_PeriodBoundaries(t *testing.T) {
	t.Parallel()

	// Wednesday 2024-03-13 14:35 UTC. Calendar context:
	//
	//   - Hourly start: 2024-03-13 14:00
	//   - Daily  start: 2024-03-13 00:00
	//   - Weekly start (Monday): 2024-03-11 00:00
	//   - Monthly start: 2024-03-01 00:00
	//   - Quarterly start (Q1=Jan): 2024-01-01 00:00
	//   - Yearly start: 2024-01-01 00:00
	now := time.Date(2024, time.March, 13, 14, 35, 0, 0, time.UTC)
	clk := fakeClock{t: now}

	cases := []struct {
		period    string
		wantStart time.Time
		wantEnd   time.Time
	}{
		{budget.PeriodHourly, time.Date(2024, time.March, 13, 14, 0, 0, 0, time.UTC), time.Date(2024, time.March, 13, 15, 0, 0, 0, time.UTC)},
		{budget.PeriodDaily, time.Date(2024, time.March, 13, 0, 0, 0, 0, time.UTC), time.Date(2024, time.March, 14, 0, 0, 0, 0, time.UTC)},
		{budget.PeriodWeekly, time.Date(2024, time.March, 11, 0, 0, 0, 0, time.UTC), time.Date(2024, time.March, 18, 0, 0, 0, 0, time.UTC)},
		{budget.PeriodMonthly, time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)},
		{budget.PeriodQuarterly, time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)},
		{budget.PeriodYearly, time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.period, func(t *testing.T) {
			t.Parallel()
			b := newBudget("period-" + tc.period).withPeriod(tc.period).withTokensLimit(1000).build()
			r, c := newReconciler(t, budget.NewNoopCostTracker(), clk, b)
			key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}
			reconcileToSteady(t, r, key, 4)
			got := getBudget(t, c, key)
			if got.Status.PeriodStart == nil || !got.Status.PeriodStart.UTC().Equal(tc.wantStart) {
				t.Fatalf("periodStart = %v, want %v", got.Status.PeriodStart, tc.wantStart)
			}
			if got.Status.PeriodEnd == nil || !got.Status.PeriodEnd.UTC().Equal(tc.wantEnd) {
				t.Fatalf("periodEnd = %v, want %v", got.Status.PeriodEnd, tc.wantEnd)
			}
		})
	}
}

// TestReconcile_PeriodBoundaries_QuarterlyEdges exercises the quarter
// boundary rounding for January, April, and October so the `(month-1)/3`
// integer-division branch is covered.
func TestReconcile_PeriodBoundaries_QuarterlyEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			// Q1 — covers Jan, Feb, Mar.
			now:       time.Date(2024, time.February, 14, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			// Q2 — covers Apr, May, Jun.
			now:       time.Date(2024, time.May, 31, 23, 59, 0, 0, time.UTC),
			wantStart: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			// Q4 — covers Oct, Nov, Dec.
			now:       time.Date(2024, time.December, 25, 12, 0, 0, 0, time.UTC),
			wantStart: time.Date(2024, time.October, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.now.Format("2006-01"), func(t *testing.T) {
			t.Parallel()
			b := newBudget("q-edge").withPeriod(budget.PeriodQuarterly).withTokensLimit(1).build()
			r, c := newReconciler(t, budget.NewNoopCostTracker(), fakeClock{t: tc.now}, b)
			key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}
			reconcileToSteady(t, r, key, 4)
			got := getBudget(t, c, key)
			if got.Status.PeriodStart == nil || !got.Status.PeriodStart.UTC().Equal(tc.wantStart) {
				t.Fatalf("periodStart = %v, want %v", got.Status.PeriodStart, tc.wantStart)
			}
			if got.Status.PeriodEnd == nil || !got.Status.PeriodEnd.UTC().Equal(tc.wantEnd) {
				t.Fatalf("periodEnd = %v, want %v", got.Status.PeriodEnd, tc.wantEnd)
			}
		})
	}
}

// TestReconcile_WeeklyOnSunday verifies the Monday anchor for weeks
// that span a Sunday → Monday boundary. Sunday 2024-03-17 should
// resolve to the previous Monday (2024-03-11), not the upcoming one.
func TestReconcile_WeeklyOnSunday(t *testing.T) {
	t.Parallel()
	now := time.Date(2024, time.March, 17, 22, 0, 0, 0, time.UTC) // Sunday
	clk := fakeClock{t: now}
	b := newBudget("weekly-sunday").withPeriod(budget.PeriodWeekly).withTokensLimit(1).build()
	r, c := newReconciler(t, budget.NewNoopCostTracker(), clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}
	reconcileToSteady(t, r, key, 4)
	got := getBudget(t, c, key)
	wantStart := time.Date(2024, time.March, 11, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2024, time.March, 18, 0, 0, 0, 0, time.UTC)
	if got.Status.PeriodStart == nil || !got.Status.PeriodStart.UTC().Equal(wantStart) {
		t.Fatalf("periodStart = %v, want %v", got.Status.PeriodStart, wantStart)
	}
	if got.Status.PeriodEnd == nil || !got.Status.PeriodEnd.UTC().Equal(wantEnd) {
		t.Fatalf("periodEnd = %v, want %v", got.Status.PeriodEnd, wantEnd)
	}
}

// TestReconcile_Idempotent verifies that a second steady-state pass
// does not change phase or conditions, and observedGeneration stays
// consistent.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	clk := fakeClock{t: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	tracker := budget.NewNoopCostTracker()

	b := newBudget("default").withTokensLimit(1000).build()
	r, c := newReconciler(t, tracker, clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}

	reconcileToSteady(t, r, key, 4)
	first := getBudget(t, c, key).DeepCopy()
	beforeCalls := tracker.Snapshot()

	reconcileOnce(t, r, key)
	second := getBudget(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	if calls := tracker.Snapshot(); calls <= beforeCalls {
		t.Fatalf("tracker calls did not increase: before=%d after=%d", beforeCalls, calls)
	}
	if second.Status.ObservedGeneration != second.Generation {
		t.Fatalf("observedGeneration drift: %d, want %d", second.Status.ObservedGeneration, second.Generation)
	}
}

// TestReconcile_TrackerError keeps the prior status snapshot when the
// cost tracker returns an error and asks for a backoff requeue.
func TestReconcile_TrackerError(t *testing.T) {
	t.Parallel()
	clk := fakeClock{t: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	tracker := budget.NewNoopCostTracker()
	tracker.Err = errors.New("redis: connection refused")

	b := newBudget("flaky").withTokensLimit(1000).build()
	r, c := newReconciler(t, tracker, clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}

	// First pass adds the finalizer; second pass exercises the
	// tracker error path.
	reconcileOnce(t, r, key)
	res := reconcileOnce(t, r, key)
	if res.RequeueAfter == 0 {
		t.Fatalf("expected backoff RequeueAfter > 0, got %+v", res)
	}
	got := getBudget(t, c, key)
	// PeriodStart/End should still have been written.
	if got.Status.PeriodStart == nil || got.Status.PeriodEnd == nil {
		t.Fatalf("expected periodStart/periodEnd written despite tracker error: %+v", got.Status)
	}
}

// TestReconcile_Deletion verifies the drain path: the finalizer is
// removed and the CR disappears from the API server.
func TestReconcile_Deletion(t *testing.T) {
	t.Parallel()
	clk := fakeClock{t: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	b := newBudget("default").withTokensLimit(1000).build()
	r, c := newReconciler(t, budget.NewNoopCostTracker(), clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}

	reconcileToSteady(t, r, key, 4)
	got := getBudget(t, c, key)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatalf("Delete budget: %v", err)
	}
	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("delete path result = %+v, want zero", res)
	}
	if err := c.Get(context.Background(), key, &policyv1alpha1.Budget{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after finalizer removal, got %v", err)
	}
}

// TestReconcile_NotFound ensures a Reconcile call for a deleted CR
// returns a clean (no error, no requeue) result.
func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()
	r, _ := newReconciler(t, budget.NewNoopCostTracker(), fakeClock{t: time.Now()})
	res, err := r.Reconcile(context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "tenant-acme", Name: "missing"}})
	if err != nil {
		t.Fatalf("Reconcile on missing budget: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected zero result for missing budget, got %+v", res)
	}
}

// TestReconcile_InvalidPeriod verifies the controller surfaces
// `EnforcerReady=False reason=InvalidPeriod` when `spec.period` is
// not in the canonical enum (defensive — admission would normally
// reject this).
func TestReconcile_InvalidPeriod(t *testing.T) {
	t.Parallel()
	clk := fakeClock{t: time.Now()}
	b := newBudget("bad-period").withPeriod("nonexistent").withTokensLimit(1).build()
	r, c := newReconciler(t, budget.NewNoopCostTracker(), clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}

	// Add finalizer, then exercise the validation path.
	reconcileOnce(t, r, key)
	res := reconcileOnce(t, r, key)
	if res.RequeueAfter == 0 {
		t.Fatalf("expected backoff RequeueAfter > 0 on invalid period, got %+v", res)
	}
	got := getBudget(t, c, key)
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.BudgetEnforcerReady); status != metav1.ConditionFalse {
		t.Fatalf("EnforcerReady = %s, want False", status)
	}
	for _, c := range got.Status.Conditions {
		if c.Type == policyv1alpha1.BudgetEnforcerReady && c.Reason != budget.ReasonInvalidPeriod {
			t.Fatalf("EnforcerReady.reason = %q, want %q", c.Reason, budget.ReasonInvalidPeriod)
		}
	}
}

// TestReconcile_NoLimitsConsideredWithin verifies the boundary case
// where every limit is nil → WithinLimit=True and BurnRate=ok.
func TestReconcile_NoLimitsConsideredWithin(t *testing.T) {
	t.Parallel()
	clk := fakeClock{t: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	tracker := budget.NewNoopCostTracker()
	tracker.Default = &policyv1alpha1.BudgetCurrent{Tokens: 9999, Calls: 42}

	b := newBudget("no-limits").build() // no limits set
	r, c := newReconciler(t, tracker, clk, b)
	key := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}
	reconcileToSteady(t, r, key, 4)
	got := getBudget(t, c, key)
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.BudgetWithinLimit); status != metav1.ConditionTrue {
		t.Fatalf("WithinLimit = %s, want True (no limits)", status)
	}
	if got.Status.BurnRate != budget.BurnRateOK {
		t.Fatalf("burnRate = %q, want %q", got.Status.BurnRate, budget.BurnRateOK)
	}
}
