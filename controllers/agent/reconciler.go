package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events. Mirrors the controller
// vocabulary used by the Skill controller.
const (
	// EventReasonUsingDeprecatedSkill is published when a referenced
	// Skill is in the Deprecating phase (Requirement A4.6 / A6.2).
	EventReasonUsingDeprecatedSkill = "UsingDeprecatedSkill"
	// EventReasonAgentDeployed is published when the agent first
	// reaches Deployed=True.
	EventReasonAgentDeployed = "AgentDeployed"
	// EventReasonAgentDraining is published when the deletion path
	// stops accepting new sessions.
	EventReasonAgentDraining = "AgentDraining"
	// EventReasonAgentDrained is published when the drain path
	// completes successfully.
	EventReasonAgentDrained = "AgentDrained"
)

// AgentReconciler implements the agent state machine documented in
// design.md §6.2.
type AgentReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// Bus broadcasts cross-controller domain events such as
	// `AgentDeployed` / `AgentRolledBack` (Requirement A6.5).
	Bus common.EventBus

	// Pluggable collaborators. All optional — defaults are wired in
	// via [applyDefaults] so unit tests can construct a reconciler
	// from a bare struct.
	SkillResolver       SkillResolver
	PolicyBinder        PolicyBinder
	IdentityProvisioner IdentityProvisioner
	ChannelRegistrar    ChannelRegistrar
	AuditFlusher        AuditFlusher
	SessionTracker      SessionTracker
	DeploymentManager   DeploymentManager

	// Clock exposes the current wall-clock time. Tests inject a
	// fake clock to drive the drain timeout deterministically.
	Clock clock.PassiveClock
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager. The Agent reconciler also watches Skill objects so that a
// Skill phase / Deprecating change re-queues every Agent that
// references it (Requirements A6.1, A6.2). The mapping is namespace
// scoped — Agents may only bind same-namespace Skills — and is
// filtered by [SkillStatusChangedPredicate] so spec-only churn on the
// Skill object does not flood the Agent workqueue.
//
// Policy changes are intentionally NOT watched here; the PDP loads the
// compiled bundle out-of-band and Agent Deployments stay in place
// (Requirement A6.3).
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("agent: nil reconciler")
	}
	r.applyDefaults()
	if r.DeploymentManager == nil {
		// Build the production Deployment manager when the manager
		// supplies a client + scheme.
		r.DeploymentManager = NewKubeDeploymentManager(mgr.GetClient(), mgr.GetScheme())
	}
	if r.SkillResolver == nil {
		r.SkillResolver = NewClusterSkillResolver(mgr.GetClient())
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("agent-controller").
		For(&agentv1alpha1.Agent{}).
		Watches(
			&skillv1alpha1.Skill{},
			handler.EnqueueRequestsFromMapFunc(EnqueueAgentsForSkill(mgr.GetClient())),
			builder.WithPredicates(SkillStatusChangedPredicate()),
		).
		Complete(r)
}

// applyDefaults wires no-op defaults for any optional collaborator.
func (r *AgentReconciler) applyDefaults() {
	if r.PolicyBinder == nil {
		r.PolicyBinder = NoopPolicyBinder{}
	}
	if r.IdentityProvisioner == nil {
		r.IdentityProvisioner = NoopIdentityProvisioner{}
	}
	if r.ChannelRegistrar == nil {
		r.ChannelRegistrar = NoopChannelRegistrar{}
	}
	if r.AuditFlusher == nil {
		r.AuditFlusher = NoopAuditFlusher{}
	}
	if r.SessionTracker == nil {
		r.SessionTracker = NoopSessionTracker{}
	}
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
}

// Reconcile runs one reconciliation pass. The state machine follows
// design.md §6.2.1; the deletion path follows §6.2.4.
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("agent", req.NamespacedName)

	agent := &agentv1alpha1.Agent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("agent: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path (design.md §6.2.4).
	if !agent.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, agent)
	}

	// 2) Ensure the drain finalizer is in place.
	if added, err := common.EnsureFinalizer(ctx, r.Client, agent, FinalizerAgentDrain); err != nil {
		return ctrl.Result{}, err
	} else if added {
		// The patched generation drives the next reconcile pass.
		return ctrl.Result{Requeue: true}, nil
	}

	// 3) Validate spec — only `react` and `tool_calling` patterns are
	//    accepted in P0 (tasks.md §3.4).
	if !patternSupported(agent.Spec.Runtime.Pattern) {
		common.SetCondition(agent, agentv1alpha1.AgentSpecValid,
			string(metav1.ConditionFalse), ReasonUnsupportedPattern,
			fmt.Sprintf("runtime.pattern %q is not supported in P0; must be one of {react, tool_calling}", agent.Spec.Runtime.Pattern))
		// Knock the downstream gates to Unknown so the aggregate Ready
		// cannot stay True from a prior generation.
		r.markGatesUnknown(agent, ReasonUnsupportedPattern)
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, ctrl.Result{})
	}
	common.SetCondition(agent, agentv1alpha1.AgentSpecValid,
		string(metav1.ConditionTrue), ReasonSpecValid, "runtime.pattern is supported")

	// 4) Sandbox readiness — checked early because it is a permanent
	//    failure that gates the rest of the pipeline.
	sandboxResult, err := r.evaluateSandbox(ctx, agent)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !sandboxResult.satisfied {
		common.SetCondition(agent, agentv1alpha1.AgentSandboxReady,
			string(metav1.ConditionFalse), ReasonSandboxUnavailable, sandboxResult.message)
		r.markGatesUnknown(agent, ReasonSandboxUnavailable)
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, ctrl.Result{})
	}
	common.SetCondition(agent, agentv1alpha1.AgentSandboxReady,
		string(metav1.ConditionTrue), sandboxResult.reason, sandboxResult.message)

	// 5) Resolve skills.
	if r.SkillResolver == nil {
		return ctrl.Result{}, errors.New("agent: SkillResolver is nil; SetupWithManager not called")
	}
	resolveRes, err := r.SkillResolver.Resolve(ctx, agent)
	if err != nil {
		// Transient error — backoff and retry without flapping
		// status.
		return common.RequeueWithBackoff(0), err
	}
	switch {
	case len(resolveRes.Unsatisfiable) > 0:
		// Permanent failure (Requirement A4.2).
		common.SetCondition(agent, agentv1alpha1.AgentSkillsResolved,
			string(metav1.ConditionFalse), ReasonUnsatisfiableConstraint,
			fmt.Sprintf("versionConstraint unsatisfiable for: %v", resolveRes.Unsatisfiable))
		r.markGatesUnknown(agent, ReasonUnsatisfiableConstraint)
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, ctrl.Result{})
	case len(resolveRes.Missing) > 0:
		// Transient — Skill may not have been created yet. Stay in
		// ResolvingSkills and retry with backoff.
		common.SetCondition(agent, agentv1alpha1.AgentSkillsResolved,
			string(metav1.ConditionFalse), ReasonMissingSkill,
			fmt.Sprintf("waiting for referenced Skills: %v", resolveRes.Missing))
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, common.RequeueWithBackoff(0))
	}
	common.SetCondition(agent, agentv1alpha1.AgentSkillsResolved,
		string(metav1.ConditionTrue), ReasonSkillsResolved, "all skill bindings resolved")
	agent.Status.AttachedSkills = append([]sharedv1alpha1.ResourceRef(nil), resolveRes.Resolved...)

	// Deprecation warning condition (Requirement A4.6 / A6.2).
	r.applyDeprecation(agent, resolveRes)

	// 6) Bind policies via the PDP.
	policies, err := r.PolicyBinder.Bind(ctx, agent)
	if err != nil {
		common.SetCondition(agent, agentv1alpha1.AgentPolicyAttached,
			string(metav1.ConditionFalse), ReasonPolicyBindFailed, truncateErr(err))
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, common.RequeueWithBackoff(0))
	}
	agent.Status.EffectivePolicies = append([]string(nil), policies...)
	common.SetCondition(agent, agentv1alpha1.AgentPolicyAttached,
		string(metav1.ConditionTrue), ReasonPolicyAttached, "policies bound to PDP")

	// 7) Provision identity (SA + token exchanger).
	if err := r.IdentityProvisioner.Provision(ctx, agent); err != nil {
		common.SetCondition(agent, agentv1alpha1.AgentIdentityReady,
			string(metav1.ConditionFalse), ReasonIdentityFailed, truncateErr(err))
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, common.RequeueWithBackoff(0))
	}
	common.SetCondition(agent, agentv1alpha1.AgentIdentityReady,
		string(metav1.ConditionTrue), ReasonIdentityReady, "service account ready")

	// 8) Ensure the Deployment.
	if r.DeploymentManager == nil {
		return ctrl.Result{}, errors.New("agent: DeploymentManager is nil; SetupWithManager not called")
	}
	desired, ready, err := r.DeploymentManager.EnsureDeployment(ctx, agent)
	if err != nil {
		common.SetCondition(agent, agentv1alpha1.AgentDeployed,
			string(metav1.ConditionFalse), ReasonDeploymentFailed, truncateErr(err))
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, common.RequeueWithBackoff(0))
	}
	agent.Status.Replicas = desired
	agent.Status.ReadyReplicas = ready
	wasDeployed := common.IsConditionTrue(agent, agentv1alpha1.AgentDeployed)
	if ready < desired {
		common.SetCondition(agent, agentv1alpha1.AgentDeployed,
			string(metav1.ConditionFalse), ReasonDeploymentProgressing,
			fmt.Sprintf("replicas ready %d/%d", ready, desired))
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, common.RequeueWithBackoff(0))
	}
	common.SetCondition(agent, agentv1alpha1.AgentDeployed,
		string(metav1.ConditionTrue), ReasonDeploymentReady,
		fmt.Sprintf("replicas ready %d/%d", ready, desired))

	// 9) Register channels.
	if err := r.ChannelRegistrar.RegisterChannels(ctx, agent); err != nil {
		common.SetCondition(agent, agentv1alpha1.AgentChannelsHealthy,
			string(metav1.ConditionFalse), ReasonChannelRegisterFailed, truncateErr(err))
		r.aggregate(agent)
		return r.writeStatus(ctx, agent, common.RequeueWithBackoff(0))
	}
	common.SetCondition(agent, agentv1alpha1.AgentChannelsHealthy,
		string(metav1.ConditionTrue), ReasonChannelsHealthy, "channel webhooks registered")

	// 10) Guardrails — defaulted True for P0 (no provider chain wired yet).
	common.SetCondition(agent, agentv1alpha1.AgentGuardrailsHealthy,
		string(metav1.ConditionTrue), ReasonGuardrailsHealthy,
		"guardrail provider chain not yet wired in this build")

	// 11) Budget — defaulted True for P0.
	common.SetCondition(agent, agentv1alpha1.AgentBudgetWithinLimit,
		string(metav1.ConditionTrue), ReasonBudgetWithinLimit,
		"budget enforcement not yet wired in this build")

	// 12) Skip rollout (P0 100 % directly per tasks.md §3.4).
	common.SetCondition(agent, agentv1alpha1.AgentRolloutComplete,
		string(metav1.ConditionTrue), ReasonRolloutComplete,
		"rolled out at 100% (P0 skips canary)")
	agent.Status.RolloutStatus = &agentv1alpha1.AgentRolloutStatus{
		Phase:         "Succeeded",
		TrafficWeight: ptrInt32(100),
	}

	// 13) Aggregate Ready + derive phase.
	wasReady := common.IsReady(agent)
	r.aggregate(agent)
	if !wasReady && common.IsReady(agent) {
		r.publishDomainEvent(ctx, logger, agent, common.EventAgentDeployed, nil)
	}
	if !wasDeployed && common.IsConditionTrue(agent, agentv1alpha1.AgentDeployed) {
		r.eventf(agent, corev1.EventTypeNormal, EventReasonAgentDeployed,
			"Deployment is healthy with %d/%d replicas ready", ready, desired)
	}

	return r.writeStatus(ctx, agent, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drives the drain flow described in design.md §6.2.4
// and Requirements A4.7 / A4.8:
//
//  1. Phase=Terminating + DeregisterChannels.
//  2. Mark `ai-keeper.io/agent-draining=true` so new sessions are rejected.
//  3. Wait up to [InFlightWaitTimeout] for in-flight sessions; force
//     past the timeout while honouring the absolute [DrainTimeout].
//  4. Revoke the Agent's ServiceAccount tokens.
//  5. Flush pending audit events — MUST succeed before the finalizer
//     can be removed (design.md §6.2.4 invariant / Requirement A4.8).
//  6. Drain the Deployment (scale to 0 + delete).
//  7. Remove the finalizer.
func (r *AgentReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, agent *agentv1alpha1.Agent) (ctrl.Result, error) {
	// Always reflect the Terminating phase to operators looking at
	// the CR.
	if agent.Status.Phase != sharedv1alpha1.PhaseTerminating {
		agent.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, agent); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, fmt.Errorf("agent: status update on terminating: %w", err)
		}
	}

	// Step 1: deregister channels first so no new traffic arrives.
	if err := r.ChannelRegistrar.DeregisterChannels(ctx, agent); err != nil {
		return common.RequeueWithBackoff(0), fmt.Errorf("agent: deregister channels: %w", err)
	}

	// Step 2: stop accepting new sessions by stamping a draining
	// annotation. Idempotent.
	if err := r.markDraining(ctx, agent); err != nil {
		return ctrl.Result{}, err
	}

	// Step 3: wait for in-flight sessions, bounded by the per-pass
	// timeout and the absolute drain deadline.
	deadline := drainDeadline(agent, r.Clock.Now())
	inflight, err := r.SessionTracker.InFlight(ctx, agent)
	if err != nil {
		return common.RequeueWithBackoff(0), fmt.Errorf("agent: in-flight session count: %w", err)
	}
	if inflight > 0 && r.Clock.Now().Before(deadline) {
		// Re-queue and let the workqueue exponential backoff handle
		// the wait. The next pass re-reads the inflight count and
		// the absolute deadline.
		logger.Info("agent: waiting for in-flight sessions to drain",
			"inflight", inflight, "deadline", deadline.Format(time.RFC3339))
		r.eventf(agent, corev1.EventTypeNormal, EventReasonAgentDraining,
			"Draining: %d in-flight sessions; deadline %s", inflight, deadline.Format(time.RFC3339))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Step 4: revoke SA tokens. Always invoked, even on forced-drain.
	if err := r.IdentityProvisioner.Revoke(ctx, agent); err != nil {
		return common.RequeueWithBackoff(0), fmt.Errorf("agent: revoke identity: %w", err)
	}

	// Step 5: audit events MUST persist before the finalizer is
	// removed (design.md §6.2.4 invariant — Requirement A4.8 + F4).
	if err := r.AuditFlusher.Flush(ctx, agent); err != nil {
		return common.RequeueWithBackoff(0), fmt.Errorf("agent: flush audit: %w", err)
	}

	// Step 6: drain the Deployment.
	if r.DeploymentManager != nil {
		if err := r.DeploymentManager.Drain(ctx, agent); err != nil {
			return common.RequeueWithBackoff(0), fmt.Errorf("agent: drain deployment: %w", err)
		}
	}

	// Step 7: remove the finalizer.
	res, _, err := common.Finalize(ctx, r.Client, agent, FinalizerAgentDrain, func(_ context.Context) error {
		return nil
	})
	if err != nil {
		return res, err
	}
	r.eventf(agent, corev1.EventTypeNormal, EventReasonAgentDrained, "Agent drain complete")
	return res, nil
}

// markDraining stamps the `ai-keeper.io/agent-draining` annotation so the
// data plane knows to reject new sessions. Idempotent.
func (r *AgentReconciler) markDraining(ctx context.Context, agent *agentv1alpha1.Agent) error {
	if agent.Annotations != nil {
		if v, ok := agent.Annotations[AnnotationDraining]; ok && v == "true" {
			return nil
		}
	}
	patch := client.MergeFrom(agent.DeepCopy())
	if agent.Annotations == nil {
		agent.Annotations = map[string]string{}
	}
	agent.Annotations[AnnotationDraining] = "true"
	if err := r.Patch(ctx, agent, patch); err != nil {
		return fmt.Errorf("agent: stamp draining annotation: %w", err)
	}
	return nil
}

// drainDeadline returns the wall-clock instant by which drain MUST
// complete. The deadline is anchored on the deletion timestamp so it
// survives controller restarts.
func drainDeadline(agent *agentv1alpha1.Agent, now time.Time) time.Time {
	deletion := agent.GetDeletionTimestamp()
	anchor := now
	if deletion != nil && !deletion.IsZero() {
		anchor = deletion.Time
	}
	// Use the in-flight wait timeout as the operational deadline; the
	// absolute drain timeout (§6.2.4 invariant) gives audit flushing
	// extra headroom but is enforced at the orchestration layer.
	if InFlightWaitTimeout < DrainTimeout {
		return anchor.Add(InFlightWaitTimeout)
	}
	return anchor.Add(DrainTimeout)
}

// sandboxEvalResult bundles the sandbox check outcome.
type sandboxEvalResult struct {
	satisfied bool
	reason    string
	message   string
}

// evaluateSandbox verifies the cluster has the requested RuntimeClass
// installed when sandbox is enabled (Requirement A4.3 / design.md §6.2.5
// SandboxReady gate). Disabled sandbox is treated as satisfied.
func (r *AgentReconciler) evaluateSandbox(ctx context.Context, agent *agentv1alpha1.Agent) (sandboxEvalResult, error) {
	sb := agent.Spec.Runtime.Sandbox
	if sb == nil || sb.Enabled == nil || !*sb.Enabled {
		return sandboxEvalResult{satisfied: true, reason: ReasonSandboxDisabled, message: "sandbox disabled"}, nil
	}
	if sb.Type == "" || sb.Type == "none" {
		return sandboxEvalResult{satisfied: true, reason: ReasonSandboxDisabled, message: "sandbox.type is none"}, nil
	}
	rcName := runtimeClassNameForSandbox(sb.Type)
	rc := &nodev1.RuntimeClass{}
	switch err := r.Get(ctx, types.NamespacedName{Name: rcName}, rc); {
	case err == nil:
		return sandboxEvalResult{satisfied: true, reason: ReasonSandboxReady, message: fmt.Sprintf("RuntimeClass %q installed", rcName)}, nil
	case apierrors.IsNotFound(err):
		return sandboxEvalResult{
			satisfied: false,
			reason:    ReasonSandboxUnavailable,
			message:   fmt.Sprintf("RuntimeClass %q not installed in cluster", rcName),
		}, nil
	default:
		return sandboxEvalResult{}, fmt.Errorf("agent: get RuntimeClass %q: %w", rcName, err)
	}
}

// applyDeprecation flips the UsingDeprecatedSkill condition based on
// the resolver result (Requirement A4.6 / A6.2). Setting the
// condition does not block reconciliation — the agent stays Active /
// Running while warning operators about the deprecation.
func (r *AgentReconciler) applyDeprecation(agent *agentv1alpha1.Agent, res SkillResolverResult) {
	wasUsing := common.IsConditionTrue(agent, agentv1alpha1.AgentUsingDeprecatedSkill)
	if res.UsesDeprecated {
		common.SetCondition(agent, agentv1alpha1.AgentUsingDeprecatedSkill,
			string(metav1.ConditionTrue), ReasonUsingDeprecatedSkill,
			fmt.Sprintf("agent references deprecated skills: %v", res.DeprecatedSkills))
		if !wasUsing {
			r.eventf(agent, corev1.EventTypeWarning, EventReasonUsingDeprecatedSkill,
				"Agent references deprecated Skill(s): %v", res.DeprecatedSkills)
		}
		return
	}
	if wasUsing {
		common.SetCondition(agent, agentv1alpha1.AgentUsingDeprecatedSkill,
			string(metav1.ConditionFalse), ReasonNoDeprecatedSkill,
			"no resolved skill is in the Deprecating phase")
	}
}

// markGatesUnknown blanks downstream gates that may still carry True
// values from a previous reconcile when an upstream gate has just
// flipped to False permanently. The aggregate Ready condition would
// otherwise stay True, which would mislead operators.
func (r *AgentReconciler) markGatesUnknown(agent *agentv1alpha1.Agent, reason string) {
	gates := []string{
		agentv1alpha1.AgentSkillsResolved,
		agentv1alpha1.AgentPolicyAttached,
		agentv1alpha1.AgentIdentityReady,
		agentv1alpha1.AgentDeployed,
		agentv1alpha1.AgentChannelsHealthy,
		agentv1alpha1.AgentGuardrailsHealthy,
		agentv1alpha1.AgentRolloutComplete,
	}
	for _, t := range gates {
		if common.IsConditionTrue(agent, t) {
			common.SetCondition(agent, t,
				string(metav1.ConditionUnknown), reason,
				"upstream gate failed; downstream gate suspended")
		}
	}
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *AgentReconciler) aggregate(agent *agentv1alpha1.Agent) {
	status, reason, message := readyFromConditions(agent)
	common.SetCondition(agent, agentv1alpha1.AgentReady, status, reason, message)
	agent.Status.Phase = derivePhase(agent)
	agent.Status.ObservedGeneration = agent.Generation
}

// writeStatus persists the in-memory status block to the API server.
// Status conflicts are non-fatal — controller-runtime retries on the
// next reconcile.
func (r *AgentReconciler) writeStatus(ctx context.Context, agent *agentv1alpha1.Agent, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, agent); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("agent: status update: %w", err)
	}
	return result, nil
}

// publishDomainEvent forwards the event onto the bus, swallowing
// errors with a log line so a transient outage cannot wedge the
// reconcile loop.
func (r *AgentReconciler) publishDomainEvent(ctx context.Context, logger logr.Logger, agent *agentv1alpha1.Agent, kind common.DomainEventKind, payload map[string]string) {
	if r.Bus == nil {
		return
	}
	ref, err := agentResourceRef(agent)
	if err != nil {
		logger.V(1).Info("agent: build ref for event", "kind", kind, "error", err.Error())
		return
	}
	ev := common.DomainEvent{
		Kind:    kind,
		Subject: ref,
		Payload: payload,
	}
	if err := r.Bus.Publish(ctx, ev); err != nil {
		logger.V(1).Info("agent: publish domain event failed", "kind", kind, "error", err.Error())
	}
}

// agentResourceRef builds the canonical `agent://<ns>/<name>` ref for
// a given Agent.
func agentResourceRef(agent *agentv1alpha1.Agent) (sharedv1alpha1.ResourceRef, error) {
	if agent == nil {
		return "", errors.New("agent: nil")
	}
	if agent.Name == "" {
		return "", errors.New("agent: empty metadata.name")
	}
	ns := agent.Namespace
	if ns == "" {
		ns = "default"
	}
	return sharedv1alpha1.FormatResourceRef(sharedv1alpha1.SchemeAgent, ns+"/"+agent.Name, "")
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *AgentReconciler) eventf(agent *agentv1alpha1.Agent, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(agent, eventType, reason, msg, args...)
}

// truncateErr clips long compiler messages so they fit in a Condition
// message field without violating the K8s API server payload limits.
func truncateErr(err error) string {
	const max = 240
	s := err.Error()
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Compile-time interface assertions.
var _ reconcile.Reconciler = (*AgentReconciler)(nil)
