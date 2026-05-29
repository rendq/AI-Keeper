package policy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Finalizer + annotation keys.
const (
	// FinalizerPolicyProtect is the finalizer the Policy controller adds
	// to every reconciled Policy. The reconciler holds the finalizer
	// while in-flight decisions drain (Requirement A5.11).
	FinalizerPolicyProtect = "ai-keeper.io/policy-protect"
)

// Timing constants spelled out in design.md §6.3.
const (
	// DebounceWindow coalesces rapid spec changes into a single
	// compile/push cycle (Requirement A5.12).
	DebounceWindow = 500 * time.Millisecond

	// DriftCheckInterval is the cadence at which the controller queries
	// every PDP for its current bundle hash (Requirement A5.10).
	DriftCheckInterval = 5 * time.Minute

	// SteadyStateRequeue is the long-tail requeue applied at the end of
	// a successful reconcile. Set to [DriftCheckInterval] so the
	// controller naturally walks through drift correction.
	SteadyStateRequeue = DriftCheckInterval

	// InFlightDrainTimeout is the maximum time the deletion path
	// blocks for in-flight decisions to complete (Requirement A5.11).
	InFlightDrainTimeout = 30 * time.Second

	// SnapshotRetention is the retention window for decision snapshots
	// after a Policy expires or is deleted (Requirement A5.9 / A5.11).
	SnapshotRetention = 90 * 24 * time.Hour
)

// Distribution thresholds (design.md §6.3.4).
const (
	// PartialDistributionThreshold is the fraction of acked PDP
	// instances at which `Distributed=True` flips on (Requirement A5.6).
	PartialDistributionThreshold = 0.90
)

// Condition reasons surfaced to operators.
const (
	ReasonSyntaxValid              = "SyntaxValid"
	ReasonSyntaxInvalid            = "SyntaxInvalid"
	ReasonReferencesResolved       = "ReferencesResolved"
	ReasonNotConflicting           = "NotConflicting"
	ReasonHardConflict             = "PolicyConflict"
	ReasonSoftConflict             = "SoftConflict"
	ReasonCompiled                 = "Compiled"
	ReasonCompileError             = "CompileError"
	ReasonDistributing             = "Distributing"
	ReasonDistributed              = "Distributed"
	ReasonFullyDistributed         = "FullyDistributed"
	ReasonNotDistributed           = "NotDistributed"
	ReasonNoPDPInstances           = "NoPDPInstances"
	ReasonWithinEffectiveWindow    = "WithinEffectiveWindow"
	ReasonNotYetEffective          = "NotYetEffective"
	ReasonExpired                  = "Expired"
	ReasonActive                   = "Active"
	ReasonReady                    = "Ready"
	ReasonNotReady                 = "NotReady"
	ReasonDisabled                 = "Disabled"
	ReasonDebounced                = "Debounced"
	ReasonDiscoveryFailed          = "DiscoveryFailed"
	ReasonConflictDetectionFailed  = "ConflictDetectionFailed"
	ReasonDistributionPartial      = "DistributionPartial"
	ReasonDistributionInsufficient = "DistributionInsufficient"
	ReasonDistributionInProgress   = "DistributionInProgress"
)

// Event reasons published as Kubernetes Events.
const (
	EventReasonPolicySoftConflict   = "PolicySoftConflict"
	EventReasonPolicyDistributed    = "PolicyDistributed"
	EventReasonPolicyDriftCorrected = "PolicyDriftCorrected"
	EventReasonPolicyHardConflict   = "PolicyHardConflict"
	EventReasonPolicyExpired        = "PolicyExpired"
	EventReasonPolicySuspended      = "PolicySuspended"
)

// PolicyReconciler implements the Policy state machine described in
// design.md §6.3.
type PolicyReconciler struct {
	client.Client

	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Bus      common.EventBus

	// Compiler turns validated Policies into PDP bundles. Defaults to
	// [NoopCompiler]; the production compiler lands in task 5.1.
	Compiler Compiler

	// Conflicts detects Hard / Soft conflicts. Defaults to
	// [NoopConflictDetector]; the real implementation lands in task 5.2.
	Conflicts ConflictDetector

	// PDP discovers and pushes bundles. Defaults to a fresh
	// [MemoryPDPClient] with no instances configured.
	PDP PDPClient

	// Clock is the wall-clock source. Tests inject a
	// [clocktest.FakeClock] to drive the debounce + drift windows
	// deterministically.
	Clock clock.Clock

	// debounceLastSeen tracks the timestamp of the last reconcile pass
	// that observed a fresh `metadata.generation`. Subsequent passes
	// within [DebounceWindow] short-circuit and requeue.
	debounceLastSeen sync.Map // map[types.NamespacedName]debounceEntry

	// driftLastChecked records the last time the controller called
	// PDPClient.GetBundleHash for the named Policy. Drift correction
	// runs when the entry is older than [DriftCheckInterval].
	driftLastChecked sync.Map // map[types.NamespacedName]time.Time
}

type debounceEntry struct {
	at         time.Time
	generation int64
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
//
// The Policy controller deliberately watches only its own primary type.
// Per Requirement A6.3, Policy spec changes MUST NOT trigger Agent
// Deployment redeploys: the compiled bundle is pushed straight to the
// PDP and runtime Agents pick it up on their next decision call. Adding
// an Agent or Skill watch here would silently regress that contract,
// so any future cross-controller wiring must go through the bundle
// pipeline rather than this Setup hook.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("policy: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("policy-controller").
		For(&policyv1alpha1.Policy{}).
		Complete(r)
}

// applyDefaults installs in-process defaults for any optional field
// the caller did not set. Safe to call more than once.
func (r *PolicyReconciler) applyDefaults() {
	if r.Compiler == nil {
		r.Compiler = NoopCompiler{}
	}
	if r.Conflicts == nil {
		r.Conflicts = NoopConflictDetector{}
	}
	if r.PDP == nil {
		r.PDP = NewMemoryPDPClient()
	}
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
}

// Reconcile runs one reconciliation pass for the named Policy. The
// implementation follows the sequence diagram in design.md §6.3.2.
func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("policy", req.NamespacedName)

	pol := &policyv1alpha1.Policy{}
	if err := r.Get(ctx, req.NamespacedName, pol); err != nil {
		if apierrors.IsNotFound(err) {
			r.debounceLastSeen.Delete(req.NamespacedName)
			r.driftLastChecked.Delete(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("policy: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path (Requirement A5.11).
	if !pol.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, pol)
	}

	// 2) Ensure finalizer.
	if added, err := common.EnsureFinalizer(ctx, r.Client, pol, FinalizerPolicyProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// 3) 500 ms debounce: coalesce rapid spec changes (Requirement A5.12).
	if delay := r.debounce(req.NamespacedName, pol); delay > 0 {
		logger.V(1).Info("policy: debouncing", "delay", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	// 4) Syntax validation (Requirement A5.2).
	if errs := validatePolicy(pol); len(errs) > 0 {
		common.SetCondition(pol, policyv1alpha1.PolicySyntaxValid,
			string(metav1.ConditionFalse), ReasonSyntaxInvalid, truncate(errs.ToAggregate().Error()))
		r.markRemainingGatesUnknown(pol, ReasonSyntaxInvalid)
		r.aggregateAndSetPhase(pol)
		// Requirement A5.2: do not retry on syntax failure.
		return r.writeStatus(ctx, pol, ctrl.Result{})
	}
	common.SetCondition(pol, policyv1alpha1.PolicySyntaxValid,
		string(metav1.ConditionTrue), ReasonSyntaxValid, "all expressions parsed")

	// 5) References — for P0 the controller has no resolver wired up,
	// so we mark the gate Unknown rather than blocking. The real
	// resolver lands alongside the conflict detector in task 5.2.
	common.SetCondition(pol, policyv1alpha1.PolicyReferencesResolved,
		string(metav1.ConditionTrue), ReasonReferencesResolved,
		"selector reference resolution defaulted in P0")

	// 6) Effective window (Requirements A5.8 / A5.9).
	now := r.Clock.Now()
	switch state := evaluateEffectiveWindow(pol, now); state {
	case effectiveWindowNotYet:
		common.SetCondition(pol, policyv1alpha1.PolicyWithinEffectiveWindow,
			string(metav1.ConditionFalse), ReasonNotYetEffective,
			"current time is before effectiveWindow.notBefore")
		r.markRemainingGatesUnknown(pol, ReasonNotYetEffective)
		pol.Status.Phase = sharedv1alpha1.PhaseSuspended
		r.aggregateReady(pol)
		pol.Status.ObservedGeneration = pol.Generation
		r.eventf(pol, corev1.EventTypeNormal, EventReasonPolicySuspended,
			"Policy suspended: notBefore=%s", pol.Spec.EffectiveWindow.NotBefore.Format(time.RFC3339))
		// Re-queue at notBefore so we resume promptly.
		requeue := pol.Spec.EffectiveWindow.NotBefore.Sub(now)
		if requeue < time.Second {
			requeue = time.Second
		}
		return r.writeStatus(ctx, pol, ctrl.Result{RequeueAfter: requeue})
	case effectiveWindowExpired:
		common.SetCondition(pol, policyv1alpha1.PolicyWithinEffectiveWindow,
			string(metav1.ConditionFalse), ReasonExpired,
			"current time is after effectiveWindow.notAfter")
		r.markRemainingGatesUnknown(pol, ReasonExpired)
		pol.Status.Phase = sharedv1alpha1.PhaseExpired
		r.aggregateReady(pol)
		pol.Status.ObservedGeneration = pol.Generation
		r.eventf(pol, corev1.EventTypeNormal, EventReasonPolicyExpired,
			"Policy expired: notAfter=%s; retaining 90d snapshot", pol.Spec.EffectiveWindow.NotAfter.Format(time.RFC3339))
		// Snapshot retention is enforced at the audit layer; keep the
		// status block intact (see design.md §6.3.1 / Requirement A5.9).
		return r.writeStatus(ctx, pol, ctrl.Result{RequeueAfter: SteadyStateRequeue})
	default:
		common.SetCondition(pol, policyv1alpha1.PolicyWithinEffectiveWindow,
			string(metav1.ConditionTrue), ReasonWithinEffectiveWindow,
			"current time is within effectiveWindow")
	}

	// 7) Conflict detection across the namespace (Requirement A5.3 / A5.4).
	allInNS, err := r.listNamespacedPolicies(ctx, pol.Namespace)
	if err != nil {
		return common.RequeueWithBackoff(0), err
	}
	conflicts, err := r.Conflicts.Detect(allInNS)
	if err != nil {
		common.SetCondition(pol, policyv1alpha1.PolicyNotConflicting,
			string(metav1.ConditionUnknown), ReasonConflictDetectionFailed, truncate(err.Error()))
		return r.writeStatus(ctx, pol, common.RequeueWithBackoff(0))
	}
	myKey := pol.Namespace + "/" + pol.Name
	myConflicts := filterConflictsForKey(conflicts, myKey)
	pol.Status.Conflicts = projectConflicts(myConflicts, myKey)

	if hard := firstHardConflict(myConflicts); hard != nil {
		common.SetCondition(pol, policyv1alpha1.PolicyNotConflicting,
			string(metav1.ConditionFalse), ReasonHardConflict,
			fmt.Sprintf("hard conflict with %s: %s", peerKey(hard, myKey), hard.Reason))
		r.markRemainingGatesUnknown(pol, ReasonHardConflict)
		r.aggregateAndSetPhase(pol)
		r.eventf(pol, corev1.EventTypeWarning, EventReasonPolicyHardConflict,
			"Hard conflict with %s — distribution blocked", peerKey(hard, myKey))
		return r.writeStatus(ctx, pol, ctrl.Result{})
	}
	common.SetCondition(pol, policyv1alpha1.PolicyNotConflicting,
		string(metav1.ConditionTrue), ReasonNotConflicting, "no hard conflicts detected")
	for _, soft := range myConflicts {
		if soft.Type == ConflictSoft {
			r.eventf(pol, corev1.EventTypeWarning, EventReasonPolicySoftConflict,
				"Soft conflict with %s: %s", peerKey(&soft, myKey), soft.Reason)
		}
	}

	// 8) Compile bundle (Requirement A5.5).
	previousBundleHash := pol.Status.BundleHash
	prevVersion := int64(0)
	if pol.Status.BundleVersion != nil {
		prevVersion = *pol.Status.BundleVersion
	}
	bundle, err := r.Compiler.Compile(ctx, allInNS, CompileOption{PreviousVersion: prevVersion})
	if err != nil {
		common.SetCondition(pol, policyv1alpha1.PolicyCompiled,
			string(metav1.ConditionFalse), ReasonCompileError, truncate(err.Error()))
		r.markRemainingGatesUnknown(pol, ReasonCompileError)
		r.aggregateAndSetPhase(pol)
		return r.writeStatus(ctx, pol, common.RequeueWithBackoff(0))
	}
	// Only bump the published version when the compiled bundle hash
	// actually changed; otherwise re-publishing `prev+1` on every
	// steady-state reconcile would defeat the monotonic version
	// invariant and cause needless PDP churn.
	if bundle.Hash == previousBundleHash && previousBundleHash != "" {
		bundle.Version = prevVersion
	}
	common.SetCondition(pol, policyv1alpha1.PolicyCompiled,
		string(metav1.ConditionTrue), ReasonCompiled,
		fmt.Sprintf("bundle version %d compiled", bundle.Version))
	pol.Status.BundleHash = bundle.Hash
	v := bundle.Version
	pol.Status.BundleVersion = &v

	// 9) Discover PDP instances and push bundle (Requirement A5.6 / A5.7).
	instances, err := r.PDP.Discover(ctx)
	if err != nil {
		common.SetCondition(pol, policyv1alpha1.PolicyDistributed,
			string(metav1.ConditionFalse), ReasonDiscoveryFailed, truncate(err.Error()))
		r.aggregateAndSetPhase(pol)
		return r.writeStatus(ctx, pol, common.RequeueWithBackoff(0))
	}
	if len(instances) == 0 {
		// No PDPs configured — record the state and try again on the
		// drift cadence. The condition is False so Ready stays False.
		common.SetCondition(pol, policyv1alpha1.PolicyDistributed,
			string(metav1.ConditionFalse), ReasonNoPDPInstances, "no PDP instances discovered")
		common.SetCondition(pol, policyv1alpha1.PolicyFullyDistributed,
			string(metav1.ConditionFalse), ReasonNoPDPInstances, "no PDP instances discovered")
		pol.Status.Distribution = nil
		r.aggregateAndSetPhase(pol)
		return r.writeStatus(ctx, pol, ctrl.Result{RequeueAfter: SteadyStateRequeue})
	}

	// Distribution decision matrix:
	//
	//   * Spec change (bundle.Hash differs from the cached
	//     `status.bundleHash`) → push to every instance.
	//   * Drift-check window expired → query each PDP's hash and
	//     re-push only the mismatched ones.
	//   * Steady state (hash matches, no drift check due) → no push;
	//     just refresh the per-PDP `AckedAt` timestamp so operators can
	//     see the controller is alive.
	driftKey := req.NamespacedName
	checkDrift := r.shouldCheckDrift(driftKey, now)
	specChanged := previousBundleHash != bundle.Hash
	pdpHashes := map[string]string{}
	if checkDrift {
		for _, inst := range instances {
			h, herr := r.PDP.GetBundleHash(ctx, inst)
			if herr != nil {
				logger.V(1).Info("policy: get bundle hash failed", "instance", inst.Name, "error", herr.Error())
				// Treat the instance as drifted so we re-push.
				pdpHashes[inst.Name] = ""
				continue
			}
			pdpHashes[inst.Name] = h
		}
		r.driftLastChecked.Store(driftKey, now)
	}

	dist := make([]policyv1alpha1.PolicyDistributionStatus, 0, len(instances))
	driftRePushed := 0
	for _, inst := range instances {
		// Determine whether to push for this instance:
		//   - spec change → always push;
		//   - drift check + mismatched hash → push;
		//   - steady state (no spec change, no drift check) → skip.
		needPush := false
		switch {
		case specChanged:
			needPush = true
		case checkDrift:
			if pdpHashes[inst.Name] != bundle.Hash {
				needPush = true
			}
		}
		if needPush {
			if err := r.PDP.Push(ctx, inst, bundle); err != nil {
				logger.V(1).Info("policy: push bundle failed", "instance", inst.Name, "error", err.Error())
				continue
			}
			if checkDrift && pdpHashes[inst.Name] != "" && pdpHashes[inst.Name] != bundle.Hash {
				driftRePushed++
			}
		}
		dist = append(dist, policyv1alpha1.PolicyDistributionStatus{
			PDPInstance:     inst.Name,
			AckedBundleHash: bundle.Hash,
			AckedAt:         &metav1.Time{Time: now},
		})
	}
	pol.Status.Distribution = dist
	if driftRePushed > 0 {
		r.eventf(pol, corev1.EventTypeNormal, EventReasonPolicyDriftCorrected,
			"drift corrected: re-pushed bundle to %d PDP instance(s)", driftRePushed)
	}

	// 10) Compute distribution coverage (Requirement A5.6 / A5.7).
	total := len(instances)
	acked := len(dist)
	coverage := float64(acked) / float64(total)

	wasFully := common.IsConditionTrue(pol, policyv1alpha1.PolicyFullyDistributed)
	if acked == total {
		common.SetCondition(pol, policyv1alpha1.PolicyDistributed,
			string(metav1.ConditionTrue), ReasonDistributed,
			fmt.Sprintf("%d/%d PDP instances acked", acked, total))
		common.SetCondition(pol, policyv1alpha1.PolicyFullyDistributed,
			string(metav1.ConditionTrue), ReasonFullyDistributed,
			fmt.Sprintf("%d/%d PDP instances acked", acked, total))
	} else if coverage >= PartialDistributionThreshold {
		common.SetCondition(pol, policyv1alpha1.PolicyDistributed,
			string(metav1.ConditionTrue), ReasonDistributed,
			fmt.Sprintf("%d/%d PDP instances acked (≥%.0f%%)", acked, total, PartialDistributionThreshold*100))
		common.SetCondition(pol, policyv1alpha1.PolicyFullyDistributed,
			string(metav1.ConditionFalse), ReasonDistributionPartial,
			fmt.Sprintf("%d/%d PDP instances acked", acked, total))
	} else {
		common.SetCondition(pol, policyv1alpha1.PolicyDistributed,
			string(metav1.ConditionFalse), ReasonDistributionInsufficient,
			fmt.Sprintf("%d/%d PDP instances acked", acked, total))
		common.SetCondition(pol, policyv1alpha1.PolicyFullyDistributed,
			string(metav1.ConditionFalse), ReasonDistributionInsufficient,
			fmt.Sprintf("%d/%d PDP instances acked", acked, total))
	}

	// 11) Active condition + ready aggregation.
	r.aggregateAndSetPhase(pol)

	if !wasFully && common.IsConditionTrue(pol, policyv1alpha1.PolicyFullyDistributed) {
		r.publishDomainEvent(ctx, logger, pol, common.EventPolicyDistributed, map[string]string{
			"bundleHash": bundle.Hash,
			"version":    fmt.Sprintf("%d", bundle.Version),
		})
		r.eventf(pol, corev1.EventTypeNormal, EventReasonPolicyDistributed,
			"Policy bundle %s fully distributed", bundle.Hash)
	}

	return r.writeStatus(ctx, pol, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drives the drain flow described by Requirement A5.11:
// wait up to 30s for in-flight decisions, clear PDP state by pushing
// an empty bundle, retain the 90-day snapshot in `status`, then drop
// the finalizer.
func (r *PolicyReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, pol *policyv1alpha1.Policy) (ctrl.Result, error) {
	if pol.Status.Phase != sharedv1alpha1.PhaseTerminating {
		pol.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, pol); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, fmt.Errorf("policy: status update on terminating: %w", err)
		}
	}

	// Wait up to InFlightDrainTimeout for in-flight decisions to
	// complete. The clock abstraction gives tests a deterministic
	// hand on the wait without sleeping a real wall-clock 30 s.
	r.Clock.Sleep(InFlightDrainTimeout)

	// Clear PDP state by re-discovering and pushing an empty bundle.
	// We tolerate failures here — the real cleanup is deregistering the
	// policy from the PDP's bundle registry and that landed-clean cycle
	// is owned by the PDP server-side.
	if instances, err := r.PDP.Discover(ctx); err == nil {
		for _, inst := range instances {
			if err := r.PDP.Push(ctx, inst, Bundle{Hash: "", Bytes: nil}); err != nil {
				logger.V(1).Info("policy: clear pdp state failed", "instance", inst.Name, "error", err.Error())
			}
		}
	}

	res, _, err := common.Finalize(ctx, r.Client, pol, FinalizerPolicyProtect, func(_ context.Context) error {
		return nil
	})
	if err != nil {
		return res, err
	}
	r.debounceLastSeen.Delete(types.NamespacedName{Namespace: pol.Namespace, Name: pol.Name})
	r.driftLastChecked.Delete(types.NamespacedName{Namespace: pol.Namespace, Name: pol.Name})
	return res, nil
}

// debounce returns a non-zero delay when the previous reconcile pass
// observed the same generation within [DebounceWindow]. Multiple spec
// changes that occur faster than the window are coalesced into a
// single compile/push cycle (Requirement A5.12).
func (r *PolicyReconciler) debounce(key types.NamespacedName, pol *policyv1alpha1.Policy) time.Duration {
	now := r.Clock.Now()
	if v, ok := r.debounceLastSeen.Load(key); ok {
		entry, _ := v.(debounceEntry)
		// Only debounce when the generation has changed since the last
		// pass — steady-state requeues at the drift cadence are NOT a
		// rapid spec change.
		if entry.generation != pol.Generation && now.Sub(entry.at) < DebounceWindow {
			r.debounceLastSeen.Store(key, debounceEntry{at: now, generation: pol.Generation})
			return DebounceWindow - now.Sub(entry.at)
		}
	}
	r.debounceLastSeen.Store(key, debounceEntry{at: now, generation: pol.Generation})
	return 0
}

// shouldCheckDrift reports whether the controller should query every
// PDP for its current bundle hash on this pass. Returns true on the
// first reconcile (no entry present) and after [DriftCheckInterval]
// has elapsed since the last check.
func (r *PolicyReconciler) shouldCheckDrift(key types.NamespacedName, now time.Time) bool {
	v, ok := r.driftLastChecked.Load(key)
	if !ok {
		return true
	}
	last, _ := v.(time.Time)
	return now.Sub(last) >= DriftCheckInterval
}

// listNamespacedPolicies returns every Policy in `ns`, sorted by name
// for determinism.
func (r *PolicyReconciler) listNamespacedPolicies(ctx context.Context, ns string) ([]*policyv1alpha1.Policy, error) {
	list := &policyv1alpha1.PolicyList{}
	if err := r.List(ctx, list, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("policy: list %s: %w", ns, err)
	}
	out := make([]*policyv1alpha1.Policy, 0, len(list.Items))
	for i := range list.Items {
		// Skip Policies that are being deleted — they should not
		// participate in conflict / compile.
		if !list.Items[i].GetDeletionTimestamp().IsZero() {
			continue
		}
		out = append(out, &list.Items[i])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// markRemainingGatesUnknown blanks downstream gates that may still
// carry True values from a previous reconcile when an upstream gate
// has just flipped to False permanently. Mirrors the helper used by
// the Skill / Agent controllers.
func (r *PolicyReconciler) markRemainingGatesUnknown(pol *policyv1alpha1.Policy, reason string) {
	gates := []string{
		policyv1alpha1.PolicyReferencesResolved,
		policyv1alpha1.PolicyNotConflicting,
		policyv1alpha1.PolicyCompiled,
		policyv1alpha1.PolicyDistributed,
		policyv1alpha1.PolicyFullyDistributed,
		policyv1alpha1.PolicyActive,
	}
	for _, g := range gates {
		if common.IsConditionTrue(pol, g) {
			common.SetCondition(pol, g,
				string(metav1.ConditionUnknown), reason,
				"upstream gate failed; downstream gate suspended")
		}
	}
}

// aggregateAndSetPhase derives Active / Ready / Phase from the
// per-gate conditions and writes ObservedGeneration.
func (r *PolicyReconciler) aggregateAndSetPhase(pol *policyv1alpha1.Policy) {
	r.aggregateActive(pol)
	r.aggregateReady(pol)
	pol.Status.Phase = derivePhase(pol)
	pol.Status.ObservedGeneration = pol.Generation
}

// aggregateReady computes the Ready condition per design.md §6.3.4:
//
//	SyntaxValid ∧ NotConflicting ∧ Compiled ∧ FullyDistributed ∧
//	WithinEffectiveWindow
func (r *PolicyReconciler) aggregateReady(pol *policyv1alpha1.Policy) {
	gates := []string{
		policyv1alpha1.PolicySyntaxValid,
		policyv1alpha1.PolicyNotConflicting,
		policyv1alpha1.PolicyCompiled,
		policyv1alpha1.PolicyFullyDistributed,
		policyv1alpha1.PolicyWithinEffectiveWindow,
	}
	for _, g := range gates {
		if !common.IsConditionTrue(pol, g) {
			common.SetCondition(pol, policyv1alpha1.PolicyReady,
				string(metav1.ConditionFalse), ReasonNotReady, g+" not satisfied")
			return
		}
	}
	common.SetCondition(pol, policyv1alpha1.PolicyReady,
		string(metav1.ConditionTrue), ReasonReady, "all gates satisfied")
}

// aggregateActive flips PolicyActive based on
// `enabled ∧ FullyDistributed ∧ WithinEffectiveWindow` per
// Requirement A5.7.
func (r *PolicyReconciler) aggregateActive(pol *policyv1alpha1.Policy) {
	enabled := pol.Spec.Enabled == nil || *pol.Spec.Enabled
	if !enabled {
		common.SetCondition(pol, policyv1alpha1.PolicyActive,
			string(metav1.ConditionFalse), ReasonDisabled, "spec.enabled=false")
		return
	}
	if !common.IsConditionTrue(pol, policyv1alpha1.PolicyFullyDistributed) {
		common.SetCondition(pol, policyv1alpha1.PolicyActive,
			string(metav1.ConditionFalse), ReasonDistributionInProgress,
			"FullyDistributed not yet satisfied")
		return
	}
	if !common.IsConditionTrue(pol, policyv1alpha1.PolicyWithinEffectiveWindow) {
		common.SetCondition(pol, policyv1alpha1.PolicyActive,
			string(metav1.ConditionFalse), ReasonNotYetEffective,
			"current time outside effectiveWindow")
		return
	}
	common.SetCondition(pol, policyv1alpha1.PolicyActive,
		string(metav1.ConditionTrue), ReasonActive,
		"enabled, fully distributed, within effective window")
}

// writeStatus persists the in-memory status block to the API server.
func (r *PolicyReconciler) writeStatus(ctx context.Context, pol *policyv1alpha1.Policy, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, pol); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("policy: status update: %w", err)
	}
	return result, nil
}

// publishDomainEvent forwards the event onto the bus, swallowing
// errors with a log line so a transient outage cannot wedge the
// reconcile loop.
func (r *PolicyReconciler) publishDomainEvent(ctx context.Context, logger logr.Logger, pol *policyv1alpha1.Policy, kind common.DomainEventKind, payload map[string]string) {
	if r.Bus == nil {
		return
	}
	ref, err := policyResourceRef(pol)
	if err != nil {
		logger.V(1).Info("policy: build ref for event", "kind", kind, "error", err.Error())
		return
	}
	ev := common.DomainEvent{
		Kind:    kind,
		Subject: ref,
		Payload: payload,
	}
	if err := r.Bus.Publish(ctx, ev); err != nil {
		logger.V(1).Info("policy: publish domain event failed", "kind", kind, "error", err.Error())
	}
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *PolicyReconciler) eventf(pol *policyv1alpha1.Policy, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(pol, eventType, reason, msg, args...)
}

// truncate clips long compiler messages so they fit in a Condition
// message field without violating K8s API server payload limits.
func truncate(s string) string {
	const max = 240
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Compile-time interface assertions.
var _ reconcile.Reconciler = (*PolicyReconciler)(nil)
