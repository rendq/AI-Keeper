package policy_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clocktest "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
	"github.com/ai-keeper/ai-keeper/controllers/policy"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := policyv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register policy scheme: %v", err)
	}
	return s
}

type policyBuilder struct{ pol *policyv1alpha1.Policy }

func newPolicyBuilder(name string) *policyBuilder {
	return &policyBuilder{
		pol: &policyv1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Generation: 1,
			},
			Spec: policyv1alpha1.PolicySpec{
				Effect: "deny",
				Subject: policyv1alpha1.SubjectSelector{
					AnyOf: []policyv1alpha1.SubjectEntry{{Kind: "User"}},
				},
				Action: policyv1alpha1.PolicyAction{
					Verbs: []string{"invoke"},
					Resources: policyv1alpha1.PolicyActionResources{
						AnyOf: []policyv1alpha1.ResourceSelector{{Kind: "Skill"}},
					},
				},
			},
		},
	}
}

func (b *policyBuilder) withCEL(expr string) *policyBuilder {
	b.pol.Spec.Conditions = &policyv1alpha1.ConditionSet{
		AllOf: []policyv1alpha1.ConditionItem{{Expression: expr}},
	}
	return b
}

func (b *policyBuilder) withWindow(notBefore, notAfter *time.Time) *policyBuilder {
	w := &policyv1alpha1.PolicyEffectiveWindow{}
	if notBefore != nil {
		t := metav1.NewTime(*notBefore)
		w.NotBefore = &t
	}
	if notAfter != nil {
		t := metav1.NewTime(*notAfter)
		w.NotAfter = &t
	}
	b.pol.Spec.EffectiveWindow = w
	return b
}

func (b *policyBuilder) withGeneration(g int64) *policyBuilder {
	b.pol.Generation = g
	return b
}

func (b *policyBuilder) withFinalizer() *policyBuilder {
	controllerutil.AddFinalizer(b.pol, policy.FinalizerPolicyProtect)
	return b
}

func (b *policyBuilder) withDeletionTimestamp(t time.Time) *policyBuilder {
	mt := metav1.NewTime(t)
	b.pol.DeletionTimestamp = &mt
	if !controllerutil.ContainsFinalizer(b.pol, policy.FinalizerPolicyProtect) {
		controllerutil.AddFinalizer(b.pol, policy.FinalizerPolicyProtect)
	}
	return b
}

func (b *policyBuilder) build() *policyv1alpha1.Policy { return b.pol }

type reconcilerOpts struct {
	pdp       policy.PDPClient
	conflicts policy.ConflictDetector
	compiler  policy.Compiler
	clock     *clocktest.FakeClock
	recorder  *record.FakeRecorder
}

func newReconciler(t *testing.T, opts reconcilerOpts, objs ...client.Object) (
	*policy.PolicyReconciler, client.Client, *common.NoopBus, *clocktest.FakeClock, *record.FakeRecorder,
) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&policyv1alpha1.Policy{}).
		Build()
	bus := common.NewNoopBus(logr.Discard())
	fakeClock := opts.clock
	if fakeClock == nil {
		fakeClock = clocktest.NewFakeClock(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	}
	rec := opts.recorder
	if rec == nil {
		rec = record.NewFakeRecorder(64)
	}
	r := &policy.PolicyReconciler{
		Client:    c,
		Scheme:    s,
		Bus:       bus,
		Recorder:  rec,
		PDP:       opts.pdp,
		Conflicts: opts.conflicts,
		Compiler:  opts.compiler,
		Clock:     fakeClock,
	}
	return r, c, bus, fakeClock, rec
}

func reconcileOnce(t *testing.T, r *policy.PolicyReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *policy.PolicyReconciler, key types.NamespacedName, max int) reconcile.Result {
	t.Helper()
	var last reconcile.Result
	for i := 0; i < max; i++ {
		last = reconcileOnce(t, r, key)
		if !last.Requeue && last.RequeueAfter != policy.DebounceWindow {
			return last
		}
	}
	t.Fatalf("Reconcile did not reach steady state after %d passes", max)
	return last
}

func getPolicy(t *testing.T, c client.Client, key types.NamespacedName) *policyv1alpha1.Policy {
	t.Helper()
	got := &policyv1alpha1.Policy{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get: %v", err)
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

func conditionReason(conds []metav1.Condition, condType string) string {
	for _, c := range conds {
		if c.Type == condType {
			return c.Reason
		}
	}
	return ""
}

func drainEvents(rec *record.FakeRecorder) []string {
	var out []string
	for {
		select {
		case ev := <-rec.Events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestReconcile_HappyPath_TwoPDPsAck exercises the full happy path:
// finalizer added → syntax ok → no conflicts → bundle compiled →
// pushed to 2 PDPs → both ack → FullyDistributed → phase=Active.
//
// Validates: Requirements A5.1, A5.5, A5.6, A5.7.
func TestReconcile_HappyPath_TwoPDPsAck(t *testing.T) {
	t.Parallel()

	pdp := policy.NewMemoryPDPClient(
		policy.Instance{Name: "pdp-a"},
		policy.Instance{Name: "pdp-b"},
	)
	pol := newPolicyBuilder("contract-allow").build()
	r, c, bus, _, rec := newReconciler(t, reconcilerOpts{pdp: pdp}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	last := reconcileToSteady(t, r, key, 4)
	got := getPolicy(t, c, key)

	if !controllerutil.ContainsFinalizer(got, policy.FinalizerPolicyProtect) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	for _, condType := range []string{
		policyv1alpha1.PolicySyntaxValid,
		policyv1alpha1.PolicyNotConflicting,
		policyv1alpha1.PolicyCompiled,
		policyv1alpha1.PolicyDistributed,
		policyv1alpha1.PolicyFullyDistributed,
		policyv1alpha1.PolicyWithinEffectiveWindow,
		policyv1alpha1.PolicyActive,
		policyv1alpha1.PolicyReady,
	} {
		if status := conditionStatus(got.Status.Conditions, condType); status != metav1.ConditionTrue {
			t.Fatalf("%s = %s, want True", condType, status)
		}
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if got.Status.BundleHash == "" || !strings.HasPrefix(got.Status.BundleHash, "sha256:") {
		t.Fatalf("BundleHash = %q, want sha256:* prefix", got.Status.BundleHash)
	}
	if got.Status.BundleVersion == nil || *got.Status.BundleVersion != 1 {
		t.Fatalf("BundleVersion = %v, want 1", got.Status.BundleVersion)
	}
	if len(got.Status.Distribution) != 2 {
		t.Fatalf("Distribution = %d entries, want 2", len(got.Status.Distribution))
	}
	if last.RequeueAfter != policy.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, policy.SteadyStateRequeue)
	}

	// Each PDP should have been pushed exactly once.
	if got, want := pdp.PushCalls("pdp-a"), 1; got != want {
		t.Fatalf("pdp-a pushes = %d, want %d", got, want)
	}
	if got, want := pdp.PushCalls("pdp-b"), 1; got != want {
		t.Fatalf("pdp-b pushes = %d, want %d", got, want)
	}

	// Domain event PolicyDistributed should have been emitted.
	kinds := map[common.DomainEventKind]int{}
	for _, ev := range bus.Events() {
		kinds[ev.Kind]++
	}
	if kinds[common.EventPolicyDistributed] != 1 {
		t.Fatalf("EventPolicyDistributed count = %d, want 1; got %v", kinds[common.EventPolicyDistributed], kinds)
	}
	// At least one Normal Event should have been recorded.
	events := drainEvents(rec)
	if len(events) == 0 {
		t.Fatalf("no K8s events recorded")
	}
}

// TestReconcile_SyntaxError exercises Requirement A5.2: invalid CEL
// flips SyntaxValid=False, sets phase=Failed and does not retry.
func TestReconcile_SyntaxError(t *testing.T) {
	t.Parallel()

	pol := newPolicyBuilder("bad-cel").withCEL("not && a valid && expression =====").build()
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	last := reconcileToSteady(t, r, key, 4)
	got := getPolicy(t, c, key)

	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.PolicySyntaxValid); status != metav1.ConditionFalse {
		t.Fatalf("SyntaxValid = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, policyv1alpha1.PolicySyntaxValid); reason != policy.ReasonSyntaxInvalid {
		t.Fatalf("SyntaxValid reason = %s, want %s", reason, policy.ReasonSyntaxInvalid)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("requeue = %s, want 0 (no retry on permanent failure)", last.RequeueAfter)
	}
}

// TestReconcile_HardConflict — Requirements A5.3 / A5.4: hard conflict
// flips NotConflicting=False, sets phase=Failed and skips PDP push.
func TestReconcile_HardConflict(t *testing.T) {
	t.Parallel()

	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	pol := newPolicyBuilder("hard-conflict-allow").build()
	other := newPolicyBuilder("hard-conflict-deny").build()
	other.Spec.Effect = "deny"

	conflicts := policy.FuncConflictDetector(func(_ []*policyv1alpha1.Policy) ([]policy.Conflict, error) {
		return []policy.Conflict{{
			Type:   policy.ConflictHard,
			A:      "default/" + pol.Name,
			B:      "default/" + other.Name,
			Reason: "same priority + opposite effect + complete overlap",
		}}, nil
	})

	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp, conflicts: conflicts}, pol, other)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	last := reconcileToSteady(t, r, key, 4)
	got := getPolicy(t, c, key)

	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.PolicyNotConflicting); status != metav1.ConditionFalse {
		t.Fatalf("NotConflicting = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, policyv1alpha1.PolicyNotConflicting); reason != policy.ReasonHardConflict {
		t.Fatalf("NotConflicting reason = %s, want %s", reason, policy.ReasonHardConflict)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if pdp.TotalPushCalls() != 0 {
		t.Fatalf("pdp.TotalPushCalls = %d, want 0 (hard conflict must not push)", pdp.TotalPushCalls())
	}
	if len(got.Status.Conflicts) == 0 {
		t.Fatalf("status.conflicts empty; want recorded conflict")
	}
	if got.Status.Conflicts[0].ConflictsWith != "default/"+other.Name {
		t.Fatalf("conflicts[0].ConflictsWith = %q, want default/%s", got.Status.Conflicts[0].ConflictsWith, other.Name)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("requeue = %s, want 0", last.RequeueAfter)
	}
}

// TestReconcile_SoftConflict_StillDistributes — Requirement A5.4:
// soft conflict still allows compile + distribute but emits a Warning
// Event.
func TestReconcile_SoftConflict_StillDistributes(t *testing.T) {
	t.Parallel()

	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	pol := newPolicyBuilder("soft-conflict").build()
	other := newPolicyBuilder("soft-conflict-peer").build()

	conflicts := policy.FuncConflictDetector(func(_ []*policyv1alpha1.Policy) ([]policy.Conflict, error) {
		return []policy.Conflict{{
			Type:   policy.ConflictSoft,
			A:      "default/" + pol.Name,
			B:      "default/" + other.Name,
			Reason: "partial overlap",
		}}, nil
	})

	r, c, _, _, rec := newReconciler(t, reconcilerOpts{pdp: pdp, conflicts: conflicts}, pol, other)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	reconcileToSteady(t, r, key, 4)
	got := getPolicy(t, c, key)

	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.PolicyNotConflicting); status != metav1.ConditionTrue {
		t.Fatalf("NotConflicting = %s, want True (soft conflict permits distribution)", status)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.PolicyFullyDistributed); status != metav1.ConditionTrue {
		t.Fatalf("FullyDistributed = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseActive {
		t.Fatalf("phase = %s, want Active", got.Status.Phase)
	}
	if len(got.Status.Conflicts) == 0 {
		t.Fatalf("status.conflicts empty; want soft conflict recorded")
	}
	if pdp.PushCalls("pdp-a") != 1 {
		t.Fatalf("pdp-a push count = %d, want 1", pdp.PushCalls("pdp-a"))
	}

	events := drainEvents(rec)
	var sawSoftWarning bool
	for _, e := range events {
		if strings.Contains(e, policy.EventReasonPolicySoftConflict) {
			sawSoftWarning = true
			break
		}
	}
	if !sawSoftWarning {
		t.Fatalf("expected %q warning event, got %v", policy.EventReasonPolicySoftConflict, events)
	}
}

// TestReconcile_NotBefore — Requirement A5.8: notBefore in the future
// flips phase=Suspended.
func TestReconcile_NotBefore_Suspended(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	notBefore := now.Add(2 * time.Hour)
	pol := newPolicyBuilder("future-window").
		withWindow(&notBefore, nil).
		build()
	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp, clock: clocktest.NewFakeClock(now)}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	// First pass adds the finalizer. Second pass evaluates the window.
	reconcileOnce(t, r, key)
	last := reconcileOnce(t, r, key)

	got := getPolicy(t, c, key)
	if got.Status.Phase != sharedv1alpha1.PhaseSuspended {
		t.Fatalf("phase = %s, want Suspended", got.Status.Phase)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.PolicyWithinEffectiveWindow); status != metav1.ConditionFalse {
		t.Fatalf("WithinEffectiveWindow = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, policyv1alpha1.PolicyWithinEffectiveWindow); reason != policy.ReasonNotYetEffective {
		t.Fatalf("WithinEffectiveWindow reason = %s, want %s", reason, policy.ReasonNotYetEffective)
	}
	// PDP must not have been pushed.
	if pdp.TotalPushCalls() != 0 {
		t.Fatalf("pdp.TotalPushCalls = %d, want 0", pdp.TotalPushCalls())
	}
	// Requeue should be at-most the wait until notBefore.
	if last.RequeueAfter <= 0 || last.RequeueAfter > 2*time.Hour+time.Second {
		t.Fatalf("RequeueAfter = %s, want in (0, ~2h]", last.RequeueAfter)
	}
}

// TestReconcile_NotAfter_Expired — Requirement A5.9: notAfter past
// flips phase=Expired.
func TestReconcile_NotAfter_Expired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	notAfter := now.Add(-2 * time.Hour)
	pol := newPolicyBuilder("expired-window").
		withWindow(nil, &notAfter).
		build()
	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp, clock: clocktest.NewFakeClock(now)}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	reconcileOnce(t, r, key)
	reconcileOnce(t, r, key)

	got := getPolicy(t, c, key)
	if got.Status.Phase != sharedv1alpha1.PhaseExpired {
		t.Fatalf("phase = %s, want Expired", got.Status.Phase)
	}
	if reason := conditionReason(got.Status.Conditions, policyv1alpha1.PolicyWithinEffectiveWindow); reason != policy.ReasonExpired {
		t.Fatalf("WithinEffectiveWindow reason = %s, want %s", reason, policy.ReasonExpired)
	}
	if pdp.TotalPushCalls() != 0 {
		t.Fatalf("pdp.TotalPushCalls = %d, want 0", pdp.TotalPushCalls())
	}
}

// TestReconcile_DriftDetection — Requirement A5.10: when the PDP's
// hash diverges from `status.bundleHash` after the drift window, the
// controller re-pushes.
func TestReconcile_DriftDetection_RePushes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	fakeClock := clocktest.NewFakeClock(now)
	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	pol := newPolicyBuilder("drift").build()
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp, clock: fakeClock}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	reconcileToSteady(t, r, key, 4)
	if got := pdp.PushCalls("pdp-a"); got != 1 {
		t.Fatalf("initial push count = %d, want 1", got)
	}

	got := getPolicy(t, c, key)
	originalHash := got.Status.BundleHash
	// Simulate the PDP reverting to a stale hash.
	pdp.SimulateDrift("pdp-a", "sha256:stale")

	// Move the clock past the drift window so the next reconcile
	// performs a drift check.
	fakeClock.Step(policy.DriftCheckInterval + time.Second)

	reconcileOnce(t, r, key)
	if got := pdp.PushCalls("pdp-a"); got != 2 {
		t.Fatalf("after drift, push count = %d, want 2", got)
	}
	got = getPolicy(t, c, key)
	if got.Status.BundleHash != originalHash {
		t.Fatalf("status.bundleHash = %q, want %q (re-push must keep canonical hash)", got.Status.BundleHash, originalHash)
	}
}

// TestReconcile_Debounce — Requirement A5.12: two reconciles within
// 500ms with a *new* generation each time → second one is requeued.
func TestReconcile_Debounce_RequeuesSecond(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	fakeClock := clocktest.NewFakeClock(now)
	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	pol := newPolicyBuilder("debounce").withFinalizer().build()

	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp, clock: fakeClock}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	// First reconcile observes generation=1, completes normally.
	first := reconcileOnce(t, r, key)
	if first.RequeueAfter == policy.DebounceWindow {
		t.Fatalf("first reconcile should not be debounced, got %s", first.RequeueAfter)
	}

	// Bump generation on the live object to simulate an immediate spec
	// change and re-reconcile within the debounce window.
	got := getPolicy(t, c, key)
	got.Generation = 2
	if err := c.Update(context.Background(), got); err != nil {
		t.Fatalf("update generation: %v", err)
	}
	fakeClock.Step(100 * time.Millisecond)
	second := reconcileOnce(t, r, key)
	if second.RequeueAfter <= 0 || second.RequeueAfter > policy.DebounceWindow {
		t.Fatalf("second reconcile RequeueAfter = %s, want in (0, %s]", second.RequeueAfter, policy.DebounceWindow)
	}
}

// TestReconcile_IdempotentSteadyState — running the reconciler twice
// in a row at steady state must not change status materially and must
// not push an extra bundle.
func TestReconcile_IdempotentSteadyState(t *testing.T) {
	t.Parallel()

	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	pol := newPolicyBuilder("idempotent").build()
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	reconcileToSteady(t, r, key, 4)
	first := getPolicy(t, c, key)
	firstHash := first.Status.BundleHash
	firstVersion := *first.Status.BundleVersion
	pushesAfterFirst := pdp.PushCalls("pdp-a")

	// Drive a second reconcile *without* advancing the clock — drift
	// check should be skipped, hash unchanged, version unchanged, no
	// extra push.
	reconcileOnce(t, r, key)
	second := getPolicy(t, c, key)

	if second.Status.BundleHash != firstHash {
		t.Fatalf("BundleHash mutated: %q → %q", firstHash, second.Status.BundleHash)
	}
	if *second.Status.BundleVersion != firstVersion {
		t.Fatalf("BundleVersion bumped at steady state: %d → %d", firstVersion, *second.Status.BundleVersion)
	}
	if pdp.PushCalls("pdp-a") != pushesAfterFirst {
		t.Fatalf("steady-state push leak: %d → %d", pushesAfterFirst, pdp.PushCalls("pdp-a"))
	}
}

// TestReconcile_DeletionFlow — Requirement A5.11: deletion sleeps for
// the in-flight drain timeout, clears PDP state and removes the
// finalizer.
func TestReconcile_DeletionFlow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	pdp := policy.NewMemoryPDPClient(policy.Instance{Name: "pdp-a"})
	// Pre-load the PDP with a hash so we can verify clear-on-delete.
	pdp.SimulateDrift("pdp-a", "sha256:before-delete")

	pol := newPolicyBuilder("delete-flow").
		withFinalizer().
		withDeletionTimestamp(now.Add(-time.Second)).
		build()

	fakeClock := clocktest.NewFakeClock(now)
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp, clock: fakeClock}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
		t.Fatalf("Reconcile delete: %v", err)
	}

	got := &policyv1alpha1.Policy{}
	if err := c.Get(context.Background(), key, got); err == nil {
		// The fake client should have removed the object once the
		// finalizer was dropped.
		if controllerutil.ContainsFinalizer(got, policy.FinalizerPolicyProtect) {
			t.Fatalf("finalizer still present after delete: %v", got.Finalizers)
		}
	}
	// PDP should have received a clear-push (empty bundle hash).
	if pdp.PushCalls("pdp-a") < 1 {
		t.Fatalf("PDP push count = %d, want >= 1 (clear pushed)", pdp.PushCalls("pdp-a"))
	}
}

// TestReconcile_NoPDPInstances — when the discoverer returns zero
// instances the controller records `Distributed=False` with reason
// NoPDPInstances and re-queues at the steady-state cadence.
func TestReconcile_NoPDPInstances(t *testing.T) {
	t.Parallel()

	pdp := policy.NewMemoryPDPClient() // no instances seeded
	pol := newPolicyBuilder("no-pdp").build()
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	reconcileToSteady(t, r, key, 4)
	got := getPolicy(t, c, key)
	if reason := conditionReason(got.Status.Conditions, policyv1alpha1.PolicyDistributed); reason != policy.ReasonNoPDPInstances {
		t.Fatalf("Distributed reason = %s, want %s", reason, policy.ReasonNoPDPInstances)
	}
	if status := conditionStatus(got.Status.Conditions, policyv1alpha1.PolicyReady); status != metav1.ConditionFalse {
		t.Fatalf("Ready = %s, want False", status)
	}
}

// TestReconcile_DiscoveryError — when Discover errors the controller
// flips Distributed=False and requests a backoff requeue.
func TestReconcile_DiscoveryError(t *testing.T) {
	t.Parallel()

	pdp := &erroringPDP{err: errors.New("network unreachable")}
	pol := newPolicyBuilder("discovery-fail").build()
	r, c, _, _, _ := newReconciler(t, reconcilerOpts{pdp: pdp}, pol)
	key := types.NamespacedName{Name: pol.Name, Namespace: pol.Namespace}

	reconcileOnce(t, r, key) // finalizer
	last := reconcileOnce(t, r, key)

	got := getPolicy(t, c, key)
	if reason := conditionReason(got.Status.Conditions, policyv1alpha1.PolicyDistributed); reason != policy.ReasonDiscoveryFailed {
		t.Fatalf("Distributed reason = %s, want %s", reason, policy.ReasonDiscoveryFailed)
	}
	if last.RequeueAfter <= 0 {
		t.Fatalf("expected backoff requeue, got %s", last.RequeueAfter)
	}
}

// erroringPDP is a tiny stub that errors out on Discover. Used to
// exercise the failure path.
type erroringPDP struct{ err error }

func (p *erroringPDP) Discover(_ context.Context) ([]policy.Instance, error) {
	return nil, p.err
}

func (p *erroringPDP) Push(_ context.Context, _ policy.Instance, _ policy.Bundle) error {
	return p.err
}

func (p *erroringPDP) GetBundleHash(_ context.Context, _ policy.Instance) (string, error) {
	return "", p.err
}

// silence record import when the file uses it indirectly.
var _ = corev1.EventTypeNormal
