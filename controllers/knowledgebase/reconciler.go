package knowledgebase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the KnowledgeBase CR.
const (
	// EventReasonIndexed is published the first time the pipeline
	// reports a non-zero chunk count.
	EventReasonIndexed = "Indexed"
	// EventReasonInvalidACL is published when the ACL lint rule
	// rejects the KB combination.
	EventReasonInvalidACL = "InvalidACL"
	// EventReasonReady is published the first time aggregate Ready
	// flips True.
	EventReasonReady = "KnowledgeBaseReady"
)

// KnowledgeBaseReconciler implements the KnowledgeBase state machine
// documented in design.md §6.5 / Requirement A7.5 (basic).
type KnowledgeBaseReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// Pipeline runs the indexing pipeline. Defaults to a [NoopPipeline]
	// when nil.
	Pipeline Pipeline
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
func (r *KnowledgeBaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("knowledgebase: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("knowledgebase-controller").
		For(&datav1alpha1.KnowledgeBase{}).
		Complete(r)
}

// applyDefaults wires the no-op stand-ins when the operator did not
// supply real implementations.
func (r *KnowledgeBaseReconciler) applyDefaults() {
	if r.Pipeline == nil {
		r.Pipeline = NewNoopPipeline()
	}
}

// Reconcile runs one reconciliation pass for a KnowledgeBase CR.
func (r *KnowledgeBaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("knowledgebase", req.NamespacedName)

	kb := &datav1alpha1.KnowledgeBase{}
	if err := r.Get(ctx, req.NamespacedName, kb); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("knowledgebase: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path.
	if !kb.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, kb)
	}

	// 2) Ensure the protect finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, kb, FinalizerKnowledgeBaseProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	wasReady := common.IsReady(kb)
	wasInvalid := false
	if c := getCondition(kb, datav1alpha1.KnowledgeBaseSourcesReady); c != nil &&
		c.Reason == ReasonInvalidACL {
		wasInvalid = true
	}

	// 3) ACL lint — Requirement A9.2 `kb/acl-not-open`. KBs classified
	//    ≥ confidential cannot inherit / declare an `open` ACL mode.
	if classificationAtLeastConfidential(kb) && aclModeOf(kb) == ACLModeOpen {
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseSourcesReady,
			string(metav1.ConditionFalse), ReasonInvalidACL,
			"classification ≥ confidential forbids acl.mode=open")
		// Knock the rest of the gates so aggregate Ready stays False
		// and Phase=Failed (state.go derivePhase precedence).
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseIndexed,
			string(metav1.ConditionFalse), ReasonInvalidACL,
			"index gate suspended pending ACL fix")
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseSynced,
			string(metav1.ConditionUnknown), ReasonInvalidACL,
			"sync gate suspended pending ACL fix")
		r.aggregate(kb)
		if !wasInvalid {
			r.eventf(kb, corev1.EventTypeWarning, EventReasonInvalidACL,
				"classification ≥ confidential forbids acl.mode=open")
		}
		// Don't requeue with backoff — the operator must fix the spec.
		return r.writeStatus(ctx, kb, ctrl.Result{RequeueAfter: SteadyStateRequeue})
	}

	// 4) Source resolution — every `sources[].ref` must point at an
	//    existing DataSource in the KB's namespace.
	missing, parseErr := r.resolveSources(ctx, kb)
	if parseErr != nil {
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseSourcesReady,
			string(metav1.ConditionFalse), ReasonSourceRefInvalid, truncateErr(parseErr))
		applyEmptyIndexCondition(kb, "source ref invalid")
		applySyncDeferredCondition(kb)
		r.aggregate(kb)
		return r.writeStatus(ctx, kb, common.RequeueWithBackoff(0))
	}
	if len(missing) > 0 {
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseSourcesReady,
			string(metav1.ConditionFalse), ReasonSourceMissing,
			fmt.Sprintf("missing DataSource(s): %s", strings.Join(missing, ", ")))
		applyEmptyIndexCondition(kb, "missing referenced DataSource(s)")
		applySyncDeferredCondition(kb)
		r.aggregate(kb)
		return r.writeStatus(ctx, kb, common.RequeueWithBackoff(0))
	}
	common.SetCondition(kb, datav1alpha1.KnowledgeBaseSourcesReady,
		string(metav1.ConditionTrue), ReasonSourcesReady,
		fmt.Sprintf("%d DataSource(s) resolved", len(kb.Spec.Sources)))

	// 5) Pipeline (P0 placeholder).
	info, err := r.Pipeline.Index(ctx, kb)
	if err != nil {
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseIndexed,
			string(metav1.ConditionFalse), ReasonIndexFailed, truncateErr(err))
		applySyncDeferredCondition(kb)
		r.aggregate(kb)
		return r.writeStatus(ctx, kb, common.RequeueWithBackoff(0))
	}

	wasIndexed := common.IsConditionTrue(kb, datav1alpha1.KnowledgeBaseIndexed)
	kb.Status.ChunkCount = info.ChunkCount
	kb.Status.IndexSizeBytes = info.IndexSizeBytes
	if !info.LastIndexedAt.IsZero() {
		t := metav1.NewTime(info.LastIndexedAt)
		kb.Status.LastIndexedAt = &t
	}

	if info.ChunkCount > 0 {
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseIndexed,
			string(metav1.ConditionTrue), ReasonIndexed,
			fmt.Sprintf("%d chunks indexed", info.ChunkCount))
		if !wasIndexed {
			r.eventf(kb, corev1.EventTypeNormal, EventReasonIndexed,
				"KnowledgeBase indexed (chunks=%d, size=%dB)", info.ChunkCount, info.IndexSizeBytes)
		}
	} else {
		common.SetCondition(kb, datav1alpha1.KnowledgeBaseIndexed,
			string(metav1.ConditionFalse), ReasonIndexEmpty,
			"pipeline reported 0 chunks")
	}

	// 6) Sync gate — defer to P1.
	applySyncDeferredCondition(kb)

	// 7) Aggregate Ready + derive phase.
	r.aggregate(kb)
	if !wasReady && common.IsReady(kb) {
		r.eventf(kb, corev1.EventTypeNormal, EventReasonReady,
			"KnowledgeBase %q is Ready", kb.Name)
	}

	return r.writeStatus(ctx, kb, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// resolveSources looks up every `sources[].ref` against the live API
// server. Returns the list of missing DataSource keys (formatted as
// `<namespace>/<name>`) and a parse error iff any ref is malformed.
func (r *KnowledgeBaseReconciler) resolveSources(ctx context.Context, kb *datav1alpha1.KnowledgeBase) ([]string, error) {
	missing := []string{}
	for _, src := range kb.Spec.Sources {
		key, err := dataSourceKeyFor(src.Ref, kb.Namespace)
		if err != nil {
			return nil, fmt.Errorf("invalid source ref %q: %w", src.Ref, err)
		}
		ds := &datav1alpha1.DataSource{}
		if err := r.Get(ctx, key, ds); err != nil {
			if apierrors.IsNotFound(err) {
				missing = append(missing, key.String())
				continue
			}
			return nil, fmt.Errorf("get DataSource %s: %w", key, err)
		}
	}
	return missing, nil
}

// dataSourceKeyFor turns a `data://<path>[@<version>]` ResourceRef
// into a [types.NamespacedName] usable with controller-runtime client.
// The path may be `<name>` (defaults to the KB's namespace) or
// `<namespace>/<name>`. Anything deeper is rejected for P0.
func dataSourceKeyFor(ref sharedv1alpha1.ResourceRef, fallbackNamespace string) (types.NamespacedName, error) {
	scheme, path, _, err := ref.Parse()
	if err != nil {
		return types.NamespacedName{}, err
	}
	if scheme != sharedv1alpha1.SchemeData {
		return types.NamespacedName{}, fmt.Errorf("expected scheme %q, got %q", sharedv1alpha1.SchemeData, scheme)
	}
	parts := strings.Split(path, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return types.NamespacedName{}, errors.New("empty data ref path")
		}
		return types.NamespacedName{Namespace: fallbackNamespace, Name: parts[0]}, nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return types.NamespacedName{}, errors.New("empty namespace or name in data ref")
		}
		return types.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
	default:
		return types.NamespacedName{}, fmt.Errorf("data ref path must be <name> or <namespace>/<name>, got %q", path)
	}
}

// reconcileDelete drains the pipeline and removes the finalizer.
// P0 has no real drain step.
func (r *KnowledgeBaseReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, kb *datav1alpha1.KnowledgeBase) (ctrl.Result, error) {
	if kb.Status.Phase != sharedv1alpha1.PhaseTerminating {
		kb.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, kb); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("knowledgebase: status update on terminating: %w", err)
		}
	}
	if _, err := common.RemoveFinalizer(ctx, r.Client, kb, FinalizerKnowledgeBaseProtect); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("knowledgebase: finalizer removed", "name", kb.Name)
	return ctrl.Result{}, nil
}

// applyEmptyIndexCondition stamps Indexed=False with the supplied
// message — used when a precondition (sources or ACL) blocks the
// pipeline.
func applyEmptyIndexCondition(kb *datav1alpha1.KnowledgeBase, msg string) {
	common.SetCondition(kb, datav1alpha1.KnowledgeBaseIndexed,
		string(metav1.ConditionFalse), ReasonIndexEmpty, msg)
}

// applySyncDeferredCondition stamps Synced=Unknown reason=SyncDeferred.
func applySyncDeferredCondition(kb *datav1alpha1.KnowledgeBase) {
	common.SetCondition(kb, datav1alpha1.KnowledgeBaseSynced,
		string(metav1.ConditionUnknown), ReasonSyncDeferred,
		"full sync schedule lands in P1")
}

// getCondition returns a pointer to the named condition, or nil.
func getCondition(kb *datav1alpha1.KnowledgeBase, t string) *metav1.Condition {
	return condition(kb.Status.Conditions, t)
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *KnowledgeBaseReconciler) aggregate(kb *datav1alpha1.KnowledgeBase) {
	status, reason, message := readyFromConditions(kb)
	common.SetCondition(kb, datav1alpha1.KnowledgeBaseReady, status, reason, message)
	kb.Status.Phase = derivePhase(kb)
	kb.Status.ObservedGeneration = kb.Generation
}

// writeStatus persists the in-memory status block to the API server.
func (r *KnowledgeBaseReconciler) writeStatus(ctx context.Context, kb *datav1alpha1.KnowledgeBase, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, kb); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("knowledgebase: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *KnowledgeBaseReconciler) eventf(kb *datav1alpha1.KnowledgeBase, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(kb, eventType, reason, msg, args...)
}

// truncateErr clips long messages so they fit in a Condition message
// field without exceeding K8s API server payload limits.
func truncateErr(err error) string {
	const max = 240
	s := err.Error()
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Compile-time interface assertions.
var _ reconcile.Reconciler = (*KnowledgeBaseReconciler)(nil)
