package datasource

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

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the DataSource CR.
const (
	// EventReasonConnected is published the first time the connector
	// reports True for a DataSource.
	EventReasonConnected = "Connected"
	// EventReasonDegraded is published when Connected transitions True
	// → False.
	EventReasonDegraded = "Degraded"
	// EventReasonReady is published the first time aggregate Ready
	// flips True.
	EventReasonReady = "DataSourceReady"
)

// DataSourceReconciler implements the DataSource state machine
// documented in design.md §6.5 / Requirement A7.4.
type DataSourceReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// Connector talks to the underlying connector adapter. Defaults
	// to a [NoopConnector] when nil.
	Connector ConnectorClient
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
func (r *DataSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("datasource: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("datasource-controller").
		For(&datav1alpha1.DataSource{}).
		Complete(r)
}

// applyDefaults wires the no-op stand-ins when the operator did not
// supply real implementations.
func (r *DataSourceReconciler) applyDefaults() {
	if r.Connector == nil {
		r.Connector = NewNoopConnector()
	}
}

// Reconcile runs one reconciliation pass for a DataSource CR.
func (r *DataSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("datasource", req.NamespacedName)

	ds := &datav1alpha1.DataSource{}
	if err := r.Get(ctx, req.NamespacedName, ds); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("datasource: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path.
	if !ds.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, ds)
	}

	// 2) Ensure the protect finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, ds, FinalizerDataSourceProtect); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	wasReady := common.IsReady(ds)
	wasConnected := common.IsConditionTrue(ds, datav1alpha1.DataSourceConnected)

	// 3) Connector connect.
	info, err := r.Connector.Connect(ctx, ds)
	if err != nil {
		flag := false
		ds.Status.Connected = &flag
		common.SetCondition(ds, datav1alpha1.DataSourceConnected,
			string(metav1.ConditionFalse), ReasonConnectFailed, truncateErr(err))
		// Sync gate stays Unknown — the connector failure already
		// drives the aggregate to False.
		common.SetCondition(ds, datav1alpha1.DataSourceSyncing,
			string(metav1.ConditionUnknown), ReasonSyncDeferred,
			"sync schedule lands in P1; connector currently unavailable")
		applyACLCondition(ds)
		r.aggregate(ds)
		if wasConnected {
			r.eventf(ds, corev1.EventTypeWarning, EventReasonDegraded,
				"connector connect failed: %v", err)
		}
		return r.writeStatus(ctx, ds, common.RequeueWithBackoff(0))
	}

	flag := true
	ds.Status.Connected = &flag
	ds.Status.DocumentCount = info.DocumentCount
	ds.Status.SizeBytes = info.SizeBytes
	if !info.LastSyncAt.IsZero() {
		t := metav1.NewTime(info.LastSyncAt)
		ds.Status.LastSyncAt = &t
	}
	common.SetCondition(ds, datav1alpha1.DataSourceConnected,
		string(metav1.ConditionTrue), ReasonConnected,
		fmt.Sprintf("connector %q reachable", ds.Spec.Connector.Kind))
	if !wasConnected {
		r.eventf(ds, corev1.EventTypeNormal, EventReasonConnected,
			"DataSource connected (kind=%s)", ds.Spec.Connector.Kind)
	}

	// 4) Sync gate (P0 placeholder).
	common.SetCondition(ds, datav1alpha1.DataSourceSyncing,
		string(metav1.ConditionUnknown), ReasonSyncDeferred,
		"full sync schedule lands in P1")

	// 5) ACL gate.
	applyACLCondition(ds)

	// 6) Aggregate Ready + derive phase.
	r.aggregate(ds)
	if !wasReady && common.IsReady(ds) {
		r.eventf(ds, corev1.EventTypeNormal, EventReasonReady,
			"DataSource %q is Ready", ds.Name)
	}

	return r.writeStatus(ctx, ds, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drains the connector and removes the finalizer.
// P0 has no real drain step; we simply lift the finalizer once the
// CR is being deleted so etcd GC can complete.
func (r *DataSourceReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, ds *datav1alpha1.DataSource) (ctrl.Result, error) {
	if ds.Status.Phase != sharedv1alpha1.PhaseTerminating {
		ds.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, ds); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("datasource: status update on terminating: %w", err)
		}
	}
	if _, err := common.RemoveFinalizer(ctx, r.Client, ds, FinalizerDataSourceProtect); err != nil {
		return ctrl.Result{}, err
	}
	logger.V(1).Info("datasource: finalizer removed", "name", ds.Name)
	return ctrl.Result{}, nil
}

// applyACLCondition updates `ACLEnforced` based on `spec.acl.mode`.
func applyACLCondition(ds *datav1alpha1.DataSource) {
	if ds.Spec.ACL != nil && ds.Spec.ACL.Mode != "" {
		common.SetCondition(ds, datav1alpha1.DataSourceACLEnforced,
			string(metav1.ConditionTrue), ReasonACLEnforced,
			fmt.Sprintf("ACL mode %q in effect", ds.Spec.ACL.Mode))
		return
	}
	common.SetCondition(ds, datav1alpha1.DataSourceACLEnforced,
		string(metav1.ConditionUnknown), ReasonACLNotConfigured,
		"spec.acl.mode is not configured")
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *DataSourceReconciler) aggregate(ds *datav1alpha1.DataSource) {
	status, reason, message := readyFromConditions(ds)
	common.SetCondition(ds, datav1alpha1.DataSourceReady, status, reason, message)
	ds.Status.Phase = derivePhase(ds)
	ds.Status.ObservedGeneration = ds.Generation
}

// writeStatus persists the in-memory status block to the API server.
func (r *DataSourceReconciler) writeStatus(ctx context.Context, ds *datav1alpha1.DataSource, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, ds); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("datasource: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *DataSourceReconciler) eventf(ds *datav1alpha1.DataSource, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(ds, eventType, reason, msg, args...)
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
var _ reconcile.Reconciler = (*DataSourceReconciler)(nil)
