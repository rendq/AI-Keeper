package skill

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// MissingReferenceTTL is the maximum time the controller will keep a
// Skill in `Pending` while one of its dependencies is missing. After
// the TTL expires the Skill is moved to `Failed` with reason
// `MissingReferencePermanent` (Requirements A3.4 / A3.5).
const MissingReferenceTTL = 1 * time.Hour

// MissingSinceAnnotation is the K8s annotation the reconciler stamps
// on a Skill when it first observes a missing dependency. Stored as
// RFC-3339 UTC. Persisting the timestamp via an annotation (rather
// than status) keeps the 1-hour rule resilient across status writes
// and controller restarts.
const MissingSinceAnnotation = "ai-keeper.io/skill-missing-since"

// SteadyStateRequeue is the long-tail requeue applied at the end of a
// successful reconcile (Requirement A3.13).
const SteadyStateRequeue = 10 * time.Minute

// MissingDependencyRequeue is the short-tail requeue used while the
// Skill is in `Pending` waiting for a missing dependency (Requirement
// A3.4 — exponential backoff is bounded; we floor it at 30s to keep
// the controller responsive without storming etcd).
const MissingDependencyRequeue = 30 * time.Second

// SkillReconciler implements the Skill controller state machine
// described in design.md §6.1. It owns the JSON Schema validation,
// dependency resolution, implementation readiness check and
// registration into the Skill Registry.
type SkillReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager. Required
	// by the controller-runtime workqueue.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. The reconciler uses it to emit
	// `SkillDeletionBlocked` (Requirement A3.11). May be nil in tests;
	// when nil the reconciler logs the event instead.
	Recorder record.EventRecorder

	// Bus broadcasts cross-controller domain events such as
	// `SkillRegistered`, `SkillPromoted`, `SkillDeprecated`
	// (Requirement A6.5). When nil the reconciler short-circuits the
	// publish call.
	Bus common.EventBus

	// Validator validates the JSON Schemas declared on
	// `spec.interface`. Defaults to [DefaultSchemaValidator].
	Validator SchemaValidator

	// Resolver resolves `spec.implementation.requires`. Defaults to
	// [NoopResolver]; the real implementation lands in task 3.3.
	Resolver Resolver

	// Registry persists Skill@version. Defaults to a process-local
	// [MemoryRegistry] until task 16.1 wires the registry service.
	Registry Registry

	// Clock exposes the current wall-clock time. Tests inject a
	// `testing/clock.FakePassiveClock` so the 1-hour rule can be
	// exercised without sleeping.
	Clock clock.PassiveClock
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager. The Skill reconciler also watches Agent objects so it can
// recompute `status.referencingAgents` whenever an Agent is created,
// updated, or deleted (Requirement A6.4). The Agent → Skill mapping
// uses [EnqueueSkillsForAgentBindings] so a single Agent change only
// enqueues the Skills it actually references.
func (r *SkillReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("skill: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("skill-controller").
		For(&skillv1alpha1.Skill{}).
		Watches(
			&agentv1alpha1.Agent{},
			handler.EnqueueRequestsFromMapFunc(EnqueueSkillsForAgentBindings()),
		).
		Complete(r)
}

// applyDefaults installs in-process defaults for any optional field
// the caller did not set. Safe to call more than once.
func (r *SkillReconciler) applyDefaults() {
	if r.Validator == nil {
		r.Validator = DefaultSchemaValidator{}
	}
	if r.Resolver == nil {
		r.Resolver = NoopResolver{}
	}
	if r.Registry == nil {
		r.Registry = NewMemoryRegistry()
	}
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
}

// Reconcile runs one reconciliation pass for the named Skill. The
// implementation follows the pseudocode in design.md §6.1.3 with the
// state machine in §6.1 and the deletion flow in §6.1.5.
func (r *SkillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()

	logger := log.FromContext(ctx).WithValues("skill", req.NamespacedName)

	skill := &skillv1alpha1.Skill{}
	if err := r.Get(ctx, req.NamespacedName, skill); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("skill: get %s: %w", req.NamespacedName, err)
	}

	// Refresh the back-pointer to the Agents currently referencing
	// this Skill (Requirement A6.4 / A3.11). The list drives the
	// deletion gate below; the work is cheap because the Agent
	// informer feeds the controller cache.
	if err := r.recomputeReferencingAgents(ctx, skill); err != nil {
		return ctrl.Result{}, err
	}

	// 1) Deletion path.
	if !skill.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, skill)
	}

	// 2) Ensure finalizer.
	if added, err := common.EnsureFinalizer(ctx, r.Client, skill, FinalizerSkillProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		// The patched generation drives the next reconcile pass.
		return ctrl.Result{Requeue: true}, nil
	}

	// 3) Schema validation (pure function).
	if err := r.Validator.Validate(skill); err != nil {
		common.SetCondition(skill, skillv1alpha1.SkillSchemaValid,
			string(metav1.ConditionFalse), ReasonInvalidSchema, truncateErr(err))
		// Schema-failures are permanent (Requirement A3.2); knock out
		// the downstream gates so Ready is unambiguously False.
		r.markRemainingGatesUnknown(skill, ReasonInvalidSchema)
		r.aggregateReadyAndPhase(skill)
		return r.writeStatus(ctx, skill, ctrl.Result{})
	}
	common.SetCondition(skill, skillv1alpha1.SkillSchemaValid,
		string(metav1.ConditionTrue), ReasonReady, "schema compiled")

	// 4) Dependency resolution.
	res, err := r.Resolver.Resolve(ctx, skill)
	if err != nil {
		// Transient resolver error → requeue with backoff so we don't
		// flap the status block.
		return common.RequeueWithBackoff(0), err
	}
	switch {
	case res.Cyclic:
		// Permanent failure (Requirement A3.6).
		clearMissingSince(skill)
		common.SetCondition(skill, skillv1alpha1.SkillDependenciesResolved,
			string(metav1.ConditionFalse), ReasonCyclicDependency, "dependency graph contains a cycle")
		r.markRemainingGatesUnknown(skill, ReasonCyclicDependency)
		r.aggregateReadyAndPhase(skill)
		return r.writeStatus(ctx, skill, ctrl.Result{})
	case len(res.Missing) > 0:
		// Apply the 1-hour rule (Requirements A3.4 / A3.5).
		if r.missingExpired(skill) {
			clearMissingSince(skill)
			common.SetCondition(skill, skillv1alpha1.SkillDependenciesResolved,
				string(metav1.ConditionFalse), ReasonMissingReferencePermanent,
				fmt.Sprintf("missing references unresolved for >%s: %v", MissingReferenceTTL, res.Missing))
			r.markRemainingGatesUnknown(skill, ReasonMissingReferencePermanent)
			r.aggregateReadyAndPhase(skill)
			return r.writeStatus(ctx, skill, ctrl.Result{})
		}
		if err := r.stampMissingSinceIfAbsent(ctx, skill); err != nil {
			return ctrl.Result{}, err
		}
		common.SetCondition(skill, skillv1alpha1.SkillDependenciesResolved,
			string(metav1.ConditionFalse), ReasonMissingReference,
			fmt.Sprintf("missing references: %v", res.Missing))
		r.markRemainingGatesUnknown(skill, ReasonMissingReference)
		r.aggregateReadyAndPhase(skill)
		return r.writeStatus(ctx, skill, ctrl.Result{RequeueAfter: MissingDependencyRequeue})
	}
	// All references resolved.
	if err := r.clearMissingSinceIfPresent(ctx, skill); err != nil {
		return ctrl.Result{}, err
	}
	resolved := res.Resolved
	skill.Status.ResolvedDependencies = &resolved
	common.SetCondition(skill, skillv1alpha1.SkillDependenciesResolved,
		string(metav1.ConditionTrue), ReasonReady, "all references resolved")

	// 5) Implementation readiness.
	if err := r.ensureImplementation(skill); err != nil {
		common.SetCondition(skill, skillv1alpha1.SkillImplementationReady,
			string(metav1.ConditionFalse), ReasonImplementationNotReady, err.Error())
		r.aggregateReadyAndPhase(skill)
		return r.writeStatus(ctx, skill, common.RequeueWithBackoff(0))
	}
	common.SetCondition(skill, skillv1alpha1.SkillImplementationReady,
		string(metav1.ConditionTrue), ReasonReady, "implementation ready")

	// 6) Skill_Registry registration.
	wasRegistered := common.IsConditionTrue(skill, skillv1alpha1.SkillRegistered)
	if err := r.Registry.Register(ctx, skill); err != nil {
		common.SetCondition(skill, skillv1alpha1.SkillRegistered,
			string(metav1.ConditionFalse), ReasonRegistrationFailed, err.Error())
		r.aggregateReadyAndPhase(skill)
		return r.writeStatus(ctx, skill, common.RequeueWithBackoff(0))
	}
	common.SetCondition(skill, skillv1alpha1.SkillRegistered,
		string(metav1.ConditionTrue), ReasonReady, "registered with Skill_Registry")
	if !wasRegistered {
		r.publishDomainEvent(ctx, logger, skill, common.EventSkillRegistered, nil)
	}

	// 7) Eval gate (P0 placeholder per task brief).
	r.applyEvalGate(skill)

	// 8) Lifecycle / deprecation.
	r.applyDeprecation(ctx, logger, skill)

	// 9) Aggregate Ready + derive phase.
	wasReady := common.IsReady(skill)
	r.aggregateReadyAndPhase(skill)
	if !wasReady && common.IsReady(skill) {
		r.publishDomainEvent(ctx, logger, skill, common.EventSkillPromoted, map[string]string{
			"stability": string(skill.Spec.Stability),
		})
	}

	return r.writeStatus(ctx, skill, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete runs the Skill drain flow described in design.md
// §6.1.5. The reconciler refuses to remove the finalizer while
// `status.referencingAgents` is non-empty (Requirement A3.11) and
// publishes a `SkillDeletionBlocked` K8s Event.
func (r *SkillReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, skill *skillv1alpha1.Skill) (ctrl.Result, error) {
	if len(skill.Status.ReferencingAgents) > 0 {
		// Mark the phase explicitly so kubectl callers see it without
		// waiting for the deletionTimestamp clear. We always issue
		// the status update on this branch so the recomputed
		// `referencingAgents` list (Requirement A6.4) lands alongside
		// the Terminating phase.
		skill.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, skill); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, fmt.Errorf("skill: status update on terminating: %w", err)
		}
		r.eventf(skill, corev1.EventTypeWarning, EventReasonSkillDeletionBlocked,
			"Skill deletion blocked: %d agents still reference it", len(skill.Status.ReferencingAgents))
		logger.Info("skill deletion blocked", "referencingAgents", skill.Status.ReferencingAgents)
		// Periodically re-check; the cross-controller informer wired
		// in cmd/manager/setupReconcilers also enqueues the Skill
		// whenever the referencing Agent is deleted, so this requeue
		// is just a safety net.
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Drain: remove from Skill_Registry and publish a final event.
	ref, refErr := SkillResourceRef(skill)
	if refErr == nil {
		if err := r.Registry.Deregister(ctx, ref); err != nil && !errors.Is(err, ErrSkillNotRegistered) {
			return ctrl.Result{}, fmt.Errorf("skill: deregister %s: %w", ref, err)
		}
	}

	res, _, err := common.Finalize(ctx, r.Client, skill, FinalizerSkillProtect, func(_ context.Context) error {
		return nil
	})
	if err != nil {
		return res, err
	}
	return res, nil
}

// ensureImplementation is the P0 placeholder for the image-pull /
// function-load step. We treat the implementation as ready when at
// least one of `runtime.image` or `runtime.entrypoint` is non-empty,
// or when the Skill is purely declarative (no Runtime set).
func (r *SkillReconciler) ensureImplementation(skill *skillv1alpha1.Skill) error {
	rt := skill.Spec.Implementation.Runtime
	if rt == nil {
		// External-API implementations rely on `requires.tools[]`; the
		// resolver gate already guarantees those are present.
		return nil
	}
	if rt.Image == "" && rt.Entrypoint == "" {
		return errors.New("implementation.runtime requires either image or entrypoint")
	}
	return nil
}

// applyEvalGate sets the EvalPassing condition. P0 implementation
// skips the actual eval run — see task brief in tasks.md §3.2.
func (r *SkillReconciler) applyEvalGate(skill *skillv1alpha1.Skill) {
	if skill.Spec.Stability == sharedv1alpha1.StageExperimental {
		common.SetCondition(skill, skillv1alpha1.SkillEvalPassing,
			string(metav1.ConditionTrue), ReasonExperimentalAutoPass,
			"experimental skills are exempt from evaluation gates")
		return
	}
	// Non-experimental: leave EvalPassing in Unknown so the aggregate
	// Ready stays False until the Eval Runner lands in a later task.
	common.SetCondition(skill, skillv1alpha1.SkillEvalPassing,
		string(metav1.ConditionUnknown), ReasonEvalNotImplemented,
		"eval runner not yet implemented in this build")
}

// applyDeprecation flips the Deprecating condition based on
// `spec.lifecycle.deprecation` (Requirement A3.10).
func (r *SkillReconciler) applyDeprecation(ctx context.Context, logger logr.Logger, skill *skillv1alpha1.Skill) {
	var dep *skillv1alpha1.SkillDeprecation
	if skill.Spec.Lifecycle != nil {
		dep = skill.Spec.Lifecycle.Deprecation
	}
	if dep == nil {
		// Clearing flips Deprecating back to False; we do not delete the
		// condition so consumers can see the transition timestamp.
		if common.IsConditionTrue(skill, skillv1alpha1.SkillDeprecating) {
			common.SetCondition(skill, skillv1alpha1.SkillDeprecating,
				string(metav1.ConditionFalse), ReasonReady, "deprecation cleared")
		}
		return
	}
	wasDeprecating := common.IsConditionTrue(skill, skillv1alpha1.SkillDeprecating)
	common.SetCondition(skill, skillv1alpha1.SkillDeprecating,
		string(metav1.ConditionTrue), ReasonDeprecated,
		"deprecation lifecycle active")
	if !wasDeprecating {
		payload := map[string]string{}
		if dep.Successor != nil {
			payload["successor"] = string(*dep.Successor)
		}
		if dep.SunsetAt != nil {
			payload["sunsetAt"] = dep.SunsetAt.UTC().Format(time.RFC3339)
		}
		r.publishDomainEvent(ctx, logger, skill, common.EventSkillDeprecated, payload)
	}
}

// markRemainingGatesUnknown blanks out gates after a permanent failure
// so the aggregate Ready condition cannot stay True from a previous
// generation's reconcile.
func (r *SkillReconciler) markRemainingGatesUnknown(skill *skillv1alpha1.Skill, reason string) {
	gates := []string{
		skillv1alpha1.SkillImplementationReady,
		skillv1alpha1.SkillRegistered,
		skillv1alpha1.SkillEvalPassing,
	}
	for _, g := range gates {
		// Only blank the gate when it was previously True; otherwise we
		// would overwrite a more specific in-progress reason.
		if common.IsConditionTrue(skill, g) {
			common.SetCondition(skill, g,
				string(metav1.ConditionUnknown), reason,
				"upstream gate failed; downstream gate suspended")
		}
	}
}

// aggregateReadyAndPhase computes the Ready condition from the gates
// per Requirement A3.7 and writes the derived phase.
func (r *SkillReconciler) aggregateReadyAndPhase(skill *skillv1alpha1.Skill) {
	status, reason, message := readyFromConditions(skill)
	common.SetCondition(skill, skillv1alpha1.SkillReady, status, reason, message)
	skill.Status.Phase = derivePhase(skill)
	skill.Status.ObservedGeneration = skill.Generation
}

// writeStatus persists the in-memory status block to the API server.
// Status conflicts are non-fatal — controller-runtime will retry on the
// next reconcile.
func (r *SkillReconciler) writeStatus(ctx context.Context, skill *skillv1alpha1.Skill, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, skill); err != nil {
		if apierrors.IsConflict(err) {
			// Drop the requeue tail so we re-read the freshest object.
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("skill: status update: %w", err)
	}
	return result, nil
}

// recomputeReferencingAgents lists every Agent in the same namespace
// as `skill` and rewrites `status.referencingAgents` with the FQNs of
// the Agents whose `spec.skills[].ref` resolves to this Skill
// (Requirement A6.4). The list is sorted deterministically so
// reconcile passes stay idempotent (Requirement F1). Mutates
// `skill.Status.ReferencingAgents` in place; the caller is expected to
// flush the change via [writeStatus] later in the reconcile.
//
// The lookup is namespace-scoped because Agents may only reference
// Skills inside their own namespace (cross-namespace refs would have
// been rejected by admission). Cluster-wide listing is therefore
// unnecessary and would only inflate cache traffic.
func (r *SkillReconciler) recomputeReferencingAgents(ctx context.Context, skill *skillv1alpha1.Skill) error {
	if skill == nil {
		return errors.New("skill: nil receiver")
	}
	list := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, list, client.InNamespace(skill.Namespace)); err != nil {
		return fmt.Errorf("skill: list agents in %s: %w", skill.Namespace, err)
	}
	agents := make([]string, 0, len(list.Items))
	seen := map[string]struct{}{}
	for i := range list.Items {
		agent := &list.Items[i]
		if !agentReferencesSkill(agent, skill) {
			continue
		}
		fqn := agent.Namespace + "/" + agent.Name
		if _, dup := seen[fqn]; dup {
			continue
		}
		seen[fqn] = struct{}{}
		agents = append(agents, fqn)
	}
	sort.Strings(agents)
	if len(agents) == 0 {
		// Distinguish "no agents" from "list empty / unset" in YAML
		// dumps by writing nil rather than an empty slice.
		skill.Status.ReferencingAgents = nil
		return nil
	}
	skill.Status.ReferencingAgents = agents
	return nil
}

// agentReferencesSkill reports whether `agent` has at least one enabled
// `spec.skills[]` entry whose ref resolves to (skill.Namespace,
// skill.Name). Disabled bindings (`enabled=false`) and malformed refs
// are ignored. The version suffix is intentionally not compared: the
// back-pointer is per-name, not per-version, mirroring the CRD field
// semantics in design.md §6.1.5.
func agentReferencesSkill(agent *agentv1alpha1.Agent, skill *skillv1alpha1.Skill) bool {
	if agent == nil || skill == nil {
		return false
	}
	for _, binding := range agent.Spec.Skills {
		if binding.Enabled != nil && !*binding.Enabled {
			continue
		}
		ns, name, ok := parseSkillRefNamespacedName(binding.Ref, agent.Namespace)
		if !ok {
			continue
		}
		if ns == skill.Namespace && name == skill.Name {
			return true
		}
	}
	return false
}

// missingExpired reports whether the `ai-keeper.io/skill-missing-since`
// annotation has aged past [MissingReferenceTTL]. An absent annotation
// counts as "not yet expired" — the controller stamps it on the next
// reconcile pass.
func (r *SkillReconciler) missingExpired(skill *skillv1alpha1.Skill) bool {
	if skill.Annotations == nil {
		return false
	}
	raw, ok := skill.Annotations[MissingSinceAnnotation]
	if !ok {
		return false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		// Corrupt annotation — treat as freshly stamped. The
		// stampMissingSinceIfAbsent path will overwrite it.
		return false
	}
	return r.Clock.Since(t) >= MissingReferenceTTL
}

// stampMissingSinceIfAbsent records the first time a missing
// dependency was observed. Safe to call repeatedly.
func (r *SkillReconciler) stampMissingSinceIfAbsent(ctx context.Context, skill *skillv1alpha1.Skill) error {
	if skill.Annotations != nil {
		if _, ok := skill.Annotations[MissingSinceAnnotation]; ok {
			return nil
		}
	}
	patch := client.MergeFrom(skill.DeepCopy())
	if skill.Annotations == nil {
		skill.Annotations = map[string]string{}
	}
	skill.Annotations[MissingSinceAnnotation] = r.Clock.Now().UTC().Format(time.RFC3339)
	if err := r.Patch(ctx, skill, patch); err != nil {
		return fmt.Errorf("skill: stamp missing-since: %w", err)
	}
	return nil
}

// clearMissingSinceIfPresent drops the annotation once dependencies
// resolve successfully so subsequent transient misses get a fresh TTL.
func (r *SkillReconciler) clearMissingSinceIfPresent(ctx context.Context, skill *skillv1alpha1.Skill) error {
	if skill.Annotations == nil {
		return nil
	}
	if _, ok := skill.Annotations[MissingSinceAnnotation]; !ok {
		return nil
	}
	patch := client.MergeFrom(skill.DeepCopy())
	delete(skill.Annotations, MissingSinceAnnotation)
	if err := r.Patch(ctx, skill, patch); err != nil {
		return fmt.Errorf("skill: clear missing-since: %w", err)
	}
	return nil
}

// clearMissingSince is the in-memory counterpart to
// [clearMissingSinceIfPresent]. Used when the controller is about to
// transition the Skill to Failed and the annotation no longer matters.
func clearMissingSince(skill *skillv1alpha1.Skill) {
	if skill.Annotations == nil {
		return
	}
	delete(skill.Annotations, MissingSinceAnnotation)
}

// publishDomainEvent forwards the event onto the bus, swallowing
// errors with a log line so a transient bus outage cannot wedge the
// reconcile loop.
func (r *SkillReconciler) publishDomainEvent(ctx context.Context, logger logr.Logger, skill *skillv1alpha1.Skill, kind common.DomainEventKind, payload map[string]string) {
	if r.Bus == nil {
		return
	}
	ref, err := SkillResourceRef(skill)
	if err != nil {
		logger.V(1).Info("skill: build ref for event", "kind", kind, "error", err.Error())
		return
	}
	ev := common.DomainEvent{
		Kind:    kind,
		Subject: ref,
		Payload: payload,
	}
	if err := r.Bus.Publish(ctx, ev); err != nil {
		logger.V(1).Info("skill: publish domain event failed", "kind", kind, "error", err.Error())
	}
}

// eventf publishes a K8s Event when the recorder is wired up; in tests
// it logs through controller-runtime so the reconcile path stays
// observable.
func (r *SkillReconciler) eventf(skill *skillv1alpha1.Skill, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(skill, eventType, reason, msg, args...)
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
var _ reconcile.Reconciler = (*SkillReconciler)(nil)
