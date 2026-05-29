package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	nodev1 "k8s.io/api/node/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clocktest "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/agent"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := agentv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register agent scheme: %v", err)
	}
	if err := skillv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register skill scheme: %v", err)
	}
	if err := nodev1.AddToScheme(s); err != nil {
		t.Fatalf("register node scheme: %v", err)
	}
	return s
}

type agentBuilder struct {
	agent *agentv1alpha1.Agent
}

func newAgentBuilder(name string) *agentBuilder {
	return &agentBuilder{
		agent: &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Generation: 1,
			},
			Spec: agentv1alpha1.AgentSpec{
				DisplayName: "Test " + name,
				Identity: agentv1alpha1.AgentIdentity{
					ServiceAccount: "default",
				},
				Skills: []agentv1alpha1.AgentSkillBinding{
					{
						Ref: sharedv1alpha1.ResourceRef("skill://default/contract-review"),
					},
				},
				Runtime: agentv1alpha1.AgentRuntime{
					Pattern: "tool_calling",
				},
			},
		},
	}
}

func (b *agentBuilder) withPattern(p string) *agentBuilder {
	b.agent.Spec.Runtime.Pattern = p
	return b
}

func (b *agentBuilder) withSandbox(enabled bool, kind string) *agentBuilder {
	en := enabled
	b.agent.Spec.Runtime.Sandbox = &agentv1alpha1.AgentSandbox{
		Enabled: &en,
		Type:    kind,
	}
	return b
}

func (b *agentBuilder) withSkillBinding(ref, versionConstraint string) *agentBuilder {
	b.agent.Spec.Skills = []agentv1alpha1.AgentSkillBinding{
		{
			Ref:               sharedv1alpha1.ResourceRef(ref),
			VersionConstraint: versionConstraint,
		},
	}
	return b
}

func (b *agentBuilder) withDeletionTimestamp(t time.Time) *agentBuilder {
	mt := metav1.NewTime(t)
	b.agent.DeletionTimestamp = &mt
	if !controllerutil.ContainsFinalizer(b.agent, agent.FinalizerAgentDrain) {
		controllerutil.AddFinalizer(b.agent, agent.FinalizerAgentDrain)
	}
	return b
}

func (b *agentBuilder) build() *agentv1alpha1.Agent { return b.agent }

func newSkill(name, version string, stability sharedv1alpha1.Stage, deprecating bool) *skillv1alpha1.Skill {
	s := &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: 1,
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   sharedv1alpha1.SemVer(version),
			Stability: stability,
		},
	}
	if deprecating {
		s.Status.Conditions = append(s.Status.Conditions, metav1.Condition{
			Type:               skillv1alpha1.SkillDeprecating,
			Status:             metav1.ConditionTrue,
			Reason:             "Deprecated",
			Message:            "deprecation lifecycle active",
			LastTransitionTime: metav1.Now(),
		})
	}
	return s
}

type reconcilerOpts struct {
	skillResolver  agent.SkillResolver
	policyBinder   agent.PolicyBinder
	identity       agent.IdentityProvisioner
	channels       agent.ChannelRegistrar
	auditFlusher   agent.AuditFlusher
	sessionTracker agent.SessionTracker
	deployment     agent.DeploymentManager
	clock          *clocktest.FakeClock
}

func newReconciler(t *testing.T, opts reconcilerOpts, objs ...client.Object) (*agent.AgentReconciler, client.Client, *common.NoopBus, *clocktest.FakeClock) {
	t.Helper()
	s := mustScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&agentv1alpha1.Agent{}, &skillv1alpha1.Skill{}).
		Build()
	bus := common.NewNoopBus(logr.Discard())
	fakeClock := clocktest.NewFakeClock(time.Now())
	if opts.clock != nil {
		fakeClock = opts.clock
	}
	r := &agent.AgentReconciler{
		Client:              c,
		Scheme:              s,
		Bus:                 bus,
		SkillResolver:       opts.skillResolver,
		PolicyBinder:        opts.policyBinder,
		IdentityProvisioner: opts.identity,
		ChannelRegistrar:    opts.channels,
		AuditFlusher:        opts.auditFlusher,
		SessionTracker:      opts.sessionTracker,
		DeploymentManager:   opts.deployment,
		Clock:               fakeClock,
	}
	if r.SkillResolver == nil {
		r.SkillResolver = agent.NewClusterSkillResolver(c)
	}
	if r.DeploymentManager == nil {
		r.DeploymentManager = &agent.FakeDeploymentManager{Replicas: 1, ReadyReplicas: 1}
	}
	return r, c, bus, fakeClock
}

func reconcileOnce(t *testing.T, r *agent.AgentReconciler, key types.NamespacedName) reconcile.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func reconcileToSteady(t *testing.T, r *agent.AgentReconciler, key types.NamespacedName, max int) reconcile.Result {
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

func getAgent(t *testing.T, c client.Client, key types.NamespacedName) *agentv1alpha1.Agent {
	t.Helper()
	got := &agentv1alpha1.Agent{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get agent: %v", err)
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestReconcile_HappyPath_ToolCalling exercises Requirements A4.1, A6.1:
// finalizer added → spec valid → skills resolved → policy + identity →
// Deployed=True → channels healthy → Ready=True → phase=Running →
// rollout 100%.
func TestReconcile_HappyPath_ToolCalling(t *testing.T) {
	t.Parallel()

	sk := newSkill("contract-review", "1.0.0", sharedv1alpha1.StageStable, false)
	a := newAgentBuilder("legal-copilot").build()

	r, c, bus, _ := newReconciler(t, reconcilerOpts{}, sk, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getAgent(t, c, key)
	if !controllerutil.ContainsFinalizer(got, agent.FinalizerAgentDrain) {
		t.Fatalf("finalizer not added: %v", got.Finalizers)
	}
	gates := []string{
		agentv1alpha1.AgentSpecValid,
		agentv1alpha1.AgentSandboxReady,
		agentv1alpha1.AgentSkillsResolved,
		agentv1alpha1.AgentPolicyAttached,
		agentv1alpha1.AgentIdentityReady,
		agentv1alpha1.AgentDeployed,
		agentv1alpha1.AgentChannelsHealthy,
		agentv1alpha1.AgentGuardrailsHealthy,
		agentv1alpha1.AgentRolloutComplete,
		agentv1alpha1.AgentBudgetWithinLimit,
		agentv1alpha1.AgentReady,
	}
	for _, g := range gates {
		if status := conditionStatus(got.Status.Conditions, g); status != metav1.ConditionTrue {
			t.Fatalf("%s = %s, want True", g, status)
		}
	}
	if got.Status.Phase != sharedv1alpha1.PhaseRunning {
		t.Fatalf("phase = %s, want Running", got.Status.Phase)
	}
	if got.Status.RolloutStatus == nil || got.Status.RolloutStatus.Phase != "Succeeded" {
		t.Fatalf("rolloutStatus = %+v, want phase=Succeeded", got.Status.RolloutStatus)
	}
	if got.Status.RolloutStatus.TrafficWeight == nil || *got.Status.RolloutStatus.TrafficWeight != 100 {
		t.Fatalf("trafficWeight = %v, want 100", got.Status.RolloutStatus.TrafficWeight)
	}
	if got.Status.Replicas != 1 || got.Status.ReadyReplicas != 1 {
		t.Fatalf("replicas = %d / %d, want 1 / 1", got.Status.Replicas, got.Status.ReadyReplicas)
	}
	if len(got.Status.AttachedSkills) != 1 {
		t.Fatalf("attachedSkills = %v, want 1 entry", got.Status.AttachedSkills)
	}
	if last.RequeueAfter != agent.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, agent.SteadyStateRequeue)
	}

	// Bus event: AgentDeployed once.
	count := 0
	for _, ev := range bus.Events() {
		if ev.Kind == common.EventAgentDeployed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("AgentDeployed events = %d, want 1", count)
	}
}

// TestReconcile_UnsupportedPattern exercises tasks.md §3.4: a pattern
// outside {react, tool_calling} flips SpecValid=False and Phase=Failed
// without retrying.
func TestReconcile_UnsupportedPattern(t *testing.T) {
	t.Parallel()

	a := newAgentBuilder("multi-agent-experiment").withPattern("multi_agent").build()
	r, c, _, _ := newReconciler(t, reconcilerOpts{}, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getAgent(t, c, key)
	if status := conditionStatus(got.Status.Conditions, agentv1alpha1.AgentSpecValid); status != metav1.ConditionFalse {
		t.Fatalf("SpecValid = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, agentv1alpha1.AgentSpecValid); reason != agent.ReasonUnsupportedPattern {
		t.Fatalf("SpecValid reason = %s, want %s", reason, agent.ReasonUnsupportedPattern)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0 (no retry on permanent failure)", last.RequeueAfter)
	}
}

// TestReconcile_MissingSkill exercises Requirement A4.2 / A6.1 transient
// path: a referenced Skill that has not been created yet keeps
// SkillsResolved=False (reason=MissingSkill) and re-queues with backoff
// rather than declaring permanent failure.
func TestReconcile_MissingSkill(t *testing.T) {
	t.Parallel()

	a := newAgentBuilder("waiting-on-skill").build()
	// No Skill object seeded.
	r, c, _, _ := newReconciler(t, reconcilerOpts{}, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getAgent(t, c, key)
	if status := conditionStatus(got.Status.Conditions, agentv1alpha1.AgentSkillsResolved); status != metav1.ConditionFalse {
		t.Fatalf("SkillsResolved = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, agentv1alpha1.AgentSkillsResolved); reason != agent.ReasonMissingSkill {
		t.Fatalf("SkillsResolved reason = %s, want %s", reason, agent.ReasonMissingSkill)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseResolvingSkills {
		t.Fatalf("phase = %s, want ResolvingSkills", got.Status.Phase)
	}
	if last.RequeueAfter == 0 {
		t.Fatalf("RequeueAfter = 0, want positive backoff")
	}
}

// TestReconcile_UnsatisfiableConstraint exercises Requirement A4.2:
// candidates exist but the versionConstraint matches none → permanent
// Failed phase, no retry.
func TestReconcile_UnsatisfiableConstraint(t *testing.T) {
	t.Parallel()

	sk := newSkill("contract-review", "1.0.0", sharedv1alpha1.StageStable, false)
	a := newAgentBuilder("strict-version").
		withSkillBinding("skill://default/contract-review", "^2.0.0").
		build()

	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getAgent(t, c, key)
	if reason := conditionReason(got.Status.Conditions, agentv1alpha1.AgentSkillsResolved); reason != agent.ReasonUnsatisfiableConstraint {
		t.Fatalf("SkillsResolved reason = %s, want %s", reason, agent.ReasonUnsatisfiableConstraint)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", last.RequeueAfter)
	}
}

// TestReconcile_DeprecatedSkill exercises Requirements A4.6 / A6.2: a
// deprecated Skill flips UsingDeprecatedSkill=True without blocking
// the agent.
func TestReconcile_DeprecatedSkill(t *testing.T) {
	t.Parallel()

	sk := newSkill("contract-review", "1.0.0", sharedv1alpha1.StageStable, true /*deprecating*/)
	a := newAgentBuilder("legal-copilot").build()

	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getAgent(t, c, key)
	if status := conditionStatus(got.Status.Conditions, agentv1alpha1.AgentUsingDeprecatedSkill); status != metav1.ConditionTrue {
		t.Fatalf("UsingDeprecatedSkill = %s, want True", status)
	}
	if status := conditionStatus(got.Status.Conditions, agentv1alpha1.AgentReady); status != metav1.ConditionTrue {
		t.Fatalf("Ready = %s, want True (deprecation must not block)", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseRunning {
		t.Fatalf("phase = %s, want Running", got.Status.Phase)
	}
	if last.RequeueAfter != agent.SteadyStateRequeue {
		t.Fatalf("RequeueAfter = %s, want %s", last.RequeueAfter, agent.SteadyStateRequeue)
	}
}

// TestReconcile_SandboxUnavailable exercises Requirement A4.3 indirectly
// via the SandboxReady gate: sandbox enabled but RuntimeClass missing
// → Failed phase, no retry.
func TestReconcile_SandboxUnavailable(t *testing.T) {
	t.Parallel()

	sk := newSkill("contract-review", "1.0.0", sharedv1alpha1.StageStable, false)
	a := newAgentBuilder("sandboxed-agent").withSandbox(true, "gvisor").build()

	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	last := reconcileToSteady(t, r, key, 4)

	got := getAgent(t, c, key)
	if status := conditionStatus(got.Status.Conditions, agentv1alpha1.AgentSandboxReady); status != metav1.ConditionFalse {
		t.Fatalf("SandboxReady = %s, want False", status)
	}
	if reason := conditionReason(got.Status.Conditions, agentv1alpha1.AgentSandboxReady); reason != agent.ReasonSandboxUnavailable {
		t.Fatalf("SandboxReady reason = %s, want %s", reason, agent.ReasonSandboxUnavailable)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if last.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", last.RequeueAfter)
	}
}

// TestReconcile_SandboxAvailable exercises the positive sandbox path:
// the RuntimeClass exists, the gate flips True, and the agent reaches
// Running.
func TestReconcile_SandboxAvailable(t *testing.T) {
	t.Parallel()

	sk := newSkill("contract-review", "1.0.0", sharedv1alpha1.StageStable, false)
	rc := &nodev1.RuntimeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "gvisor"},
		Handler:    "runsc",
	}
	a := newAgentBuilder("sandboxed-agent").withSandbox(true, "gvisor").build()

	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk, rc, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	reconcileToSteady(t, r, key, 4)

	got := getAgent(t, c, key)
	if status := conditionStatus(got.Status.Conditions, agentv1alpha1.AgentSandboxReady); status != metav1.ConditionTrue {
		t.Fatalf("SandboxReady = %s, want True", status)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseRunning {
		t.Fatalf("phase = %s, want Running", got.Status.Phase)
	}
}

// TestReconcile_DrainSuccess covers the deletion happy path: with
// inflight=0 the reconciler removes the finalizer in one pass.
func TestReconcile_DrainSuccess(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	a := newAgentBuilder("retiring").withDeletionTimestamp(now).build()
	deployment := &agent.FakeDeploymentManager{}
	r, c, _, _ := newReconciler(t, reconcilerOpts{deployment: deployment}, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	res := reconcileOnce(t, r, key)
	if res.RequeueAfter != 0 || res.Requeue {
		t.Fatalf("result = %+v, want zero", res)
	}

	// Agent should be gone (finalizer removed → fake client deletes
	// the object).
	got := &agentv1alpha1.Agent{}
	err := c.Get(context.Background(), key, got)
	if err == nil {
		// If still present, finalizer must be gone.
		if controllerutil.ContainsFinalizer(got, agent.FinalizerAgentDrain) {
			t.Fatalf("finalizer still present after drain: %v", got.Finalizers)
		}
	}
	if _, drainCalls := deployment.Snapshot(); drainCalls != 1 {
		t.Fatalf("deployment Drain calls = %d, want 1", drainCalls)
	}
}

// TestReconcile_DrainTimeout exercises Requirement A4.8: with inflight
// > 0 and the wall clock past the per-pass deadline, the reconciler
// forces the drain through (channels deregistered, identity revoked,
// audit flushed, deployment drained, finalizer removed).
func TestReconcile_DrainTimeout(t *testing.T) {
	t.Parallel()

	deletionAt := time.Now().UTC()
	a := newAgentBuilder("retiring-with-traffic").withDeletionTimestamp(deletionAt).build()
	tracker := &countingSessionTracker{remaining: 5}
	deployment := &agent.FakeDeploymentManager{}
	// Fake clock starts well past the in-flight wait deadline so the
	// first drain pass forces through.
	fc := clocktest.NewFakeClock(deletionAt.Add(agent.InFlightWaitTimeout + 30*time.Second))
	r, c, _, _ := newReconciler(t, reconcilerOpts{
		sessionTracker: tracker,
		deployment:     deployment,
		clock:          fc,
	}, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	res := reconcileOnce(t, r, key)
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("result = %+v, want zero", res)
	}
	if _, drainCalls := deployment.Snapshot(); drainCalls != 1 {
		t.Fatalf("deployment Drain calls = %d, want 1", drainCalls)
	}
	// Finalizer should be gone.
	got := &agentv1alpha1.Agent{}
	if err := c.Get(context.Background(), key, got); err == nil {
		if controllerutil.ContainsFinalizer(got, agent.FinalizerAgentDrain) {
			t.Fatalf("finalizer still present after forced drain: %v", got.Finalizers)
		}
	}
}

// TestReconcile_DrainWaitsForInflight exercises Requirement A4.7:
// before the in-flight deadline, in-flight > 0 keeps the finalizer
// in place and re-queues.
func TestReconcile_DrainWaitsForInflight(t *testing.T) {
	t.Parallel()

	deletionAt := time.Now().UTC()
	a := newAgentBuilder("draining").withDeletionTimestamp(deletionAt).build()
	tracker := &countingSessionTracker{remaining: 3}
	deployment := &agent.FakeDeploymentManager{}
	// Clock barely past the deletion timestamp.
	fc := clocktest.NewFakeClock(deletionAt.Add(5 * time.Second))
	r, c, _, _ := newReconciler(t, reconcilerOpts{
		sessionTracker: tracker,
		deployment:     deployment,
		clock:          fc,
	}, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	res := reconcileOnce(t, r, key)
	if res.RequeueAfter == 0 {
		t.Fatalf("RequeueAfter = 0, want positive while in-flight > 0")
	}
	got := &agentv1alpha1.Agent{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("agent should still exist with finalizer: %v", err)
	}
	if !controllerutil.ContainsFinalizer(got, agent.FinalizerAgentDrain) {
		t.Fatalf("finalizer missing during drain wait: %v", got.Finalizers)
	}
	if got.Status.Phase != sharedv1alpha1.PhaseTerminating {
		t.Fatalf("phase = %s, want Terminating", got.Status.Phase)
	}
	if got.Annotations[agent.AnnotationDraining] != "true" {
		t.Fatalf("draining annotation = %q, want %q", got.Annotations[agent.AnnotationDraining], "true")
	}
	if _, drain := deployment.Snapshot(); drain != 0 {
		t.Fatalf("Drain called %d times during wait, want 0", drain)
	}
}

// TestReconcile_Idempotent verifies that a second steady-state
// reconcile pass produces identical status (Requirements F1, F2). We
// compare condition statuses + phase + replicas because timestamps
// naturally differ.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()

	sk := newSkill("contract-review", "1.0.0", sharedv1alpha1.StageStable, false)
	a := newAgentBuilder("legal-copilot").build()

	r, c, _, _ := newReconciler(t, reconcilerOpts{}, sk, a)
	key := types.NamespacedName{Name: a.Name, Namespace: a.Namespace}

	reconcileToSteady(t, r, key, 4)
	first := getAgent(t, c, key).DeepCopy()

	// Run a second pass; nothing meaningful should change.
	reconcileOnce(t, r, key)
	second := getAgent(t, c, key)

	if first.Status.Phase != second.Status.Phase {
		t.Fatalf("phase changed across reconciles: %s → %s", first.Status.Phase, second.Status.Phase)
	}
	if len(first.Status.Conditions) != len(second.Status.Conditions) {
		t.Fatalf("condition count changed: %d → %d", len(first.Status.Conditions), len(second.Status.Conditions))
	}
	if first.Status.Replicas != second.Status.Replicas || first.Status.ReadyReplicas != second.Status.ReadyReplicas {
		t.Fatalf("replicas changed: %d/%d → %d/%d",
			first.Status.Replicas, first.Status.ReadyReplicas,
			second.Status.Replicas, second.Status.ReadyReplicas)
	}
	for _, c1 := range first.Status.Conditions {
		c2 := findCondition(second.Status.Conditions, c1.Type)
		if c2 == nil {
			t.Fatalf("condition %s missing in second pass", c1.Type)
		}
		if c1.Status != c2.Status || c1.Reason != c2.Reason {
			t.Fatalf("condition %s mutated: %s/%s → %s/%s",
				c1.Type, c1.Status, c1.Reason, c2.Status, c2.Reason)
		}
	}
}

// ---------------------------------------------------------------------------
// Misc helpers
// ---------------------------------------------------------------------------

type countingSessionTracker struct {
	remaining int
}

func (c *countingSessionTracker) InFlight(_ context.Context, _ *agentv1alpha1.Agent) (int, error) {
	return c.remaining, nil
}

func findCondition(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}
