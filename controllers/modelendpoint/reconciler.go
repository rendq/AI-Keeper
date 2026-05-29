package modelendpoint

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the ModelEndpoint CR.
const (
	// EventReasonHealthy is published the first time Healthy flips
	// True for an endpoint.
	EventReasonHealthy = "Healthy"
	// EventReasonDegraded is published when Healthy transitions True
	// → False.
	EventReasonDegraded = "Degraded"
	// EventReasonRecovered is published when Healthy transitions
	// False → True.
	EventReasonRecovered = "Recovered"
	// EventReasonDPAMissing is published when the compliance gate
	// fails.
	EventReasonDPAMissing = "DPAMissing"
	// EventReasonReady is published the first time aggregate Ready
	// flips True.
	EventReasonReady = "ModelEndpointReady"
)

// ModelEndpointReconciler implements the ModelEndpoint state machine
// documented in design.md §6.5 / Requirement A7.6.
type ModelEndpointReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// Prober checks endpoint reachability + observes latency.
	// Defaults to a [NoopProber] when nil.
	Prober Prober
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
func (r *ModelEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("modelendpoint: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("modelendpoint-controller").
		For(&modelv1alpha1.ModelEndpoint{}).
		Complete(r)
}

// applyDefaults wires the no-op stand-ins when the operator did not
// supply real implementations.
func (r *ModelEndpointReconciler) applyDefaults() {
	if r.Prober == nil {
		r.Prober = NewNoopProber()
	}
}

// Reconcile runs one reconciliation pass for a ModelEndpoint CR.
func (r *ModelEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("modelendpoint", req.NamespacedName)

	me := &modelv1alpha1.ModelEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, me); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("modelendpoint: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path.
	if !me.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, me)
	}

	// 2) Ensure the protect finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, me, FinalizerModelEndpointProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	wasReady := common.IsReady(me)
	wasHealthy := common.IsConditionTrue(me, modelv1alpha1.ModelEndpointHealthy)

	// 3) Endpoint probe.
	now := metav1.Now()
	me.Status.LastProbeAt = &now
	latency, probeErr := r.Prober.Probe(ctx, me)
	if probeErr != nil {
		flag := false
		me.Status.Healthy = &flag
		// Record latest observed latency (may be the elapsed-on-error
		// duration) so dashboards still see something.
		if latency > 0 {
			ms := latency
			me.Status.AvgLatencyMs = &ms
		}
		common.SetCondition(me, modelv1alpha1.ModelEndpointHealthy,
			string(metav1.ConditionFalse), ReasonProbeFailed, truncateErr(probeErr))
		applyDPACondition(me)
		applyWithinQuotaCondition(me)
		r.aggregate(me)
		if wasHealthy {
			r.eventf(me, corev1.EventTypeWarning, EventReasonDegraded,
				"endpoint %q probe failed: %v", me.Spec.Endpoint, probeErr)
		}
		return r.writeStatus(ctx, me, common.RequeueWithBackoff(0))
	}
	flag := true
	me.Status.Healthy = &flag
	// Rolling latency average — for P0 we just store the last sample
	// since real metric collection lands in 11.1.
	ms := latency
	me.Status.AvgLatencyMs = &ms
	common.SetCondition(me, modelv1alpha1.ModelEndpointHealthy,
		string(metav1.ConditionTrue), ReasonProbeOK,
		fmt.Sprintf("endpoint %q reachable (%dms)", me.Spec.Endpoint, latency))
	if !wasHealthy && hasCondition(me, modelv1alpha1.ModelEndpointHealthy) {
		r.eventf(me, corev1.EventTypeNormal, EventReasonRecovered,
			"endpoint %q reachable again", me.Spec.Endpoint)
	}

	// 4) Initialise rolling counters. Real values come from task 11.1.
	zeroTPM := int64(0)
	zeroRPM := int64(0)
	zeroErr := float64(0)
	if me.Status.CurrentTpm == nil {
		me.Status.CurrentTpm = &zeroTPM
	}
	if me.Status.CurrentRpm == nil {
		me.Status.CurrentRpm = &zeroRPM
	}
	if me.Status.ErrorRate24h == nil {
		me.Status.ErrorRate24h = &zeroErr
	}

	// 5) DPA gate.
	wasDPAMissing := false
	if c := common.GetCondition(me, modelv1alpha1.ModelEndpointDPASigned); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonDPAMissing {
		wasDPAMissing = true
	}
	applyDPACondition(me)
	if !wasDPAMissing {
		if c := common.GetCondition(me, modelv1alpha1.ModelEndpointDPASigned); c != nil &&
			c.Status == metav1.ConditionFalse && c.Reason == ReasonDPAMissing {
			r.eventf(me, corev1.EventTypeWarning, EventReasonDPAMissing,
				"endpoint %q lists GDPR/HIPAA compliance but spec.privacy.dpaSigned is not true", me.Spec.Endpoint)
		}
	}

	// 6) WithinQuota gate.
	applyWithinQuotaCondition(me)

	// 7) Aggregate Ready + derive phase.
	r.aggregate(me)
	if !wasReady && common.IsReady(me) {
		r.eventf(me, corev1.EventTypeNormal, EventReasonReady,
			"ModelEndpoint %q is Ready", me.Name)
	}

	return r.writeStatus(ctx, me, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drives the drain flow: lift the finalizer once the
// CR is being deleted. Real registry / token revocation lands in P1.
func (r *ModelEndpointReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, me *modelv1alpha1.ModelEndpoint) (ctrl.Result, error) {
	if me.Status.Phase != sharedv1alpha1.PhaseTerminating {
		me.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, me); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("modelendpoint: status update on terminating: %w", err)
		}
	}
	if _, err := common.RemoveFinalizer(ctx, r.Client, me, FinalizerModelEndpointProtect); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("modelendpoint: finalizer removed", "name", me.Name)
	return ctrl.Result{}, nil
}

// applyDPACondition stamps the DPASigned condition based on
// `spec.compliance` ∩ {GDPR, HIPAA} and `spec.privacy.dpaSigned`.
func applyDPACondition(me *modelv1alpha1.ModelEndpoint) {
	if !requiresDPA(me) {
		common.SetCondition(me, modelv1alpha1.ModelEndpointDPASigned,
			string(metav1.ConditionUnknown), ReasonDPANotRequired,
			"compliance does not require a DPA")
		return
	}
	if dpaSigned(me) {
		common.SetCondition(me, modelv1alpha1.ModelEndpointDPASigned,
			string(metav1.ConditionTrue), ReasonDPASigned,
			"spec.privacy.dpaSigned=true and required by compliance")
		return
	}
	common.SetCondition(me, modelv1alpha1.ModelEndpointDPASigned,
		string(metav1.ConditionFalse), ReasonDPAMissing,
		"spec.compliance includes GDPR/HIPAA but spec.privacy.dpaSigned is not true")
}

// applyWithinQuotaCondition stamps the WithinQuota condition based on
// the rolling TPM/RPM observed on `status` against `spec.quota`. For
// P0 the controller has no real metric source so the gate stays True
// unless the operator pre-populates `status.currentTpm/currentRpm`
// (covered by tests).
func applyWithinQuotaCondition(me *modelv1alpha1.ModelEndpoint) {
	tpmCap := quotaTPM(me)
	rpmCap := quotaRPM(me)
	tpmCur := int64(0)
	if me.Status.CurrentTpm != nil {
		tpmCur = *me.Status.CurrentTpm
	}
	rpmCur := int64(0)
	if me.Status.CurrentRpm != nil {
		rpmCur = *me.Status.CurrentRpm
	}
	if tpmCap > 0 && tpmCur > tpmCap {
		common.SetCondition(me, modelv1alpha1.ModelEndpointWithinQuota,
			string(metav1.ConditionFalse), ReasonQuotaExceeded,
			fmt.Sprintf("currentTpm=%d > quota.tpm=%d", tpmCur, tpmCap))
		return
	}
	if rpmCap > 0 && rpmCur > rpmCap {
		common.SetCondition(me, modelv1alpha1.ModelEndpointWithinQuota,
			string(metav1.ConditionFalse), ReasonQuotaExceeded,
			fmt.Sprintf("currentRpm=%d > quota.rpm=%d", rpmCur, rpmCap))
		return
	}
	common.SetCondition(me, modelv1alpha1.ModelEndpointWithinQuota,
		string(metav1.ConditionTrue), ReasonWithinQuota,
		"current TPM/RPM under spec.quota")
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *ModelEndpointReconciler) aggregate(me *modelv1alpha1.ModelEndpoint) {
	status, reason, message := readyFromConditions(me)
	common.SetCondition(me, modelv1alpha1.ModelEndpointReady, status, reason, message)
	me.Status.Phase = derivePhase(me)
	me.Status.ObservedGeneration = me.Generation
}

// writeStatus persists the in-memory status block to the API server.
func (r *ModelEndpointReconciler) writeStatus(ctx context.Context, me *modelv1alpha1.ModelEndpoint, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, me); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("modelendpoint: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *ModelEndpointReconciler) eventf(me *modelv1alpha1.ModelEndpoint, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(me, eventType, reason, msg, args...)
}

// hasCondition reports whether the named condition is present.
func hasCondition(me *modelv1alpha1.ModelEndpoint, t string) bool {
	return condition(me.Status.Conditions, t) != nil
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
var _ reconcile.Reconciler = (*ModelEndpointReconciler)(nil)
