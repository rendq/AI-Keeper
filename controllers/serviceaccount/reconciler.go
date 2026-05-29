package serviceaccount

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

	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

// Event reasons surfaced as Kubernetes Events on the SA CR.
const (
	// EventReasonRegistered is published when the SA is successfully
	// registered at the Identity Broker for the first time.
	EventReasonRegistered = "Registered"
	// EventReasonOBOEnabled is published the first time OBO is enabled.
	EventReasonOBOEnabled = "OBOEnabled"
	// EventReasonRevoked is published once the deletion path has
	// confirmed token revocation at the Broker.
	EventReasonRevoked = "Revoked"
)

// ServiceAccountReconciler implements the SA state machine documented
// in design.md §6.5 and Requirement A7.2.
type ServiceAccountReconciler struct {
	client.Client

	// Scheme is the runtime.Scheme registered with the manager.
	Scheme *runtime.Scheme

	// Recorder publishes K8s Events. May be nil; the reconciler
	// short-circuits when nil.
	Recorder record.EventRecorder

	// IdentityBroker is the platform Identity Broker the controller
	// talks to. May be nil — the reconciler defaults to a process-wide
	// [NoopIdentityBroker] in [SetupWithManager] / [applyDefaults] so
	// unit tests can construct the reconciler from a bare struct.
	IdentityBroker IdentityBrokerClient
}

// SetupWithManager registers the reconciler with the controller-runtime
// manager.
func (r *ServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r == nil {
		return errors.New("serviceaccount: nil reconciler")
	}
	r.applyDefaults()
	return ctrl.NewControllerManagedBy(mgr).
		Named("serviceaccount-controller").
		For(&corev1alpha1.ServiceAccount{}).
		Complete(r)
}

// applyDefaults wires the no-op Identity Broker when none is supplied.
func (r *ServiceAccountReconciler) applyDefaults() {
	if r.IdentityBroker == nil {
		r.IdentityBroker = &NoopIdentityBroker{}
	}
}

// Reconcile runs one reconciliation pass for a ServiceAccount CR.
func (r *ServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.applyDefaults()
	logger := log.FromContext(ctx).WithValues("serviceaccount", req.NamespacedName)

	sa := &corev1alpha1.ServiceAccount{}
	if err := r.Get(ctx, req.NamespacedName, sa); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("serviceaccount: get %s: %w", req.NamespacedName, err)
	}

	// 1) Deletion path — drive Deregister + DisableOBO and then drop
	//    the finalizer.
	if !sa.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, logger, sa)
	}

	// 2) Ensure the revoke finalizer is present.
	if added, err := common.EnsureFinalizer(ctx, r.Client, sa, FinalizerSARevoke); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// 3) Validate `spec.identityProvider`. Although CRD admission
	//    enforces MinLength=1, defensive checks here keep the
	//    reconciler honest when the controller is exercised against a
	//    fake client without webhooks.
	if sa.Spec.IdentityProvider == "" {
		common.SetCondition(sa, corev1alpha1.ServiceAccountIdentityProviderReady,
			string(metav1.ConditionFalse), ReasonInvalidIdentityProvider,
			"spec.identityProvider must not be empty")
		// Token exchange depends on identity provider — knock to
		// Unknown so the aggregate cannot stay True from a previous
		// generation.
		common.SetCondition(sa, corev1alpha1.ServiceAccountTokenExchangeReady,
			string(metav1.ConditionUnknown), ReasonInvalidIdentityProvider,
			"identity provider invalid")
		r.aggregate(sa)
		return r.writeStatus(ctx, sa, ctrl.Result{})
	}

	// 4) Register at the Identity Broker.
	wasRegistered := common.IsConditionTrue(sa, corev1alpha1.ServiceAccountIdentityProviderReady)
	if err := r.IdentityBroker.Register(ctx, sa); err != nil {
		common.SetCondition(sa, corev1alpha1.ServiceAccountIdentityProviderReady,
			string(metav1.ConditionFalse), ReasonRegistrationFailed, truncateErr(err))
		r.aggregate(sa)
		return r.writeStatus(ctx, sa, common.RequeueWithBackoff(0))
	}
	common.SetCondition(sa, corev1alpha1.ServiceAccountIdentityProviderReady,
		string(metav1.ConditionTrue), ReasonRegistered,
		fmt.Sprintf("registered at identity provider %q", sa.Spec.IdentityProvider))
	if !wasRegistered {
		r.eventf(sa, corev1.EventTypeNormal, EventReasonRegistered,
			"ServiceAccount registered at Identity Broker (provider=%s)", sa.Spec.IdentityProvider)
	}

	// 5) OBO branch — gated on `spec.allowOnBehalfOf`.
	wasOBO := common.IsConditionTrue(sa, corev1alpha1.ServiceAccountTokenExchangeReady)
	if oboEnabled(sa) {
		if err := r.IdentityBroker.EnableOBO(ctx, sa); err != nil {
			common.SetCondition(sa, corev1alpha1.ServiceAccountTokenExchangeReady,
				string(metav1.ConditionFalse), ReasonOBOFailed, truncateErr(err))
			r.aggregate(sa)
			return r.writeStatus(ctx, sa, common.RequeueWithBackoff(0))
		}
		common.SetCondition(sa, corev1alpha1.ServiceAccountTokenExchangeReady,
			string(metav1.ConditionTrue), ReasonOBOEnabled,
			"RFC 8693 token exchange enabled")
		if !wasOBO {
			r.eventf(sa, corev1.EventTypeNormal, EventReasonOBOEnabled,
				"On-Behalf-Of token exchange enabled")
		}
	} else {
		// Operator flipped allowOnBehalfOf back to false (or never
		// enabled it) — best-effort disable so the Broker side is
		// consistent. We swallow the error on the disable path because
		// it is not Ready-blocking.
		if wasOBO {
			if err := r.IdentityBroker.DisableOBO(ctx, sa); err != nil {
				logger.V(1).Info("serviceaccount: DisableOBO failed",
					"error", err.Error())
			}
		}
		common.SetCondition(sa, corev1alpha1.ServiceAccountTokenExchangeReady,
			string(metav1.ConditionUnknown), ReasonOBODisabled,
			"spec.allowOnBehalfOf is not true")
	}

	// 6) Aggregate Ready + derive phase.
	r.aggregate(sa)
	return r.writeStatus(ctx, sa, ctrl.Result{RequeueAfter: SteadyStateRequeue})
}

// reconcileDelete drives the drain flow: best-effort DisableOBO →
// Deregister → remove finalizer (Requirement A7.2 / C8.4).
func (r *ServiceAccountReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, sa *corev1alpha1.ServiceAccount) (ctrl.Result, error) {
	// Reflect Terminating phase to operators looking at the CR.
	if sa.Status.Phase != sharedv1alpha1.PhaseTerminating {
		sa.Status.Phase = sharedv1alpha1.PhaseTerminating
		if err := r.Status().Update(ctx, sa); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("serviceaccount: status update on terminating: %w", err)
		}
	}

	// Disable OBO best-effort. Errors are bubbled up so controller-
	// runtime applies workqueue backoff.
	if oboEnabled(sa) {
		if err := r.IdentityBroker.DisableOBO(ctx, sa); err != nil {
			return common.RequeueWithBackoff(0), fmt.Errorf("serviceaccount: DisableOBO: %w", err)
		}
	}

	// Deregister revokes outstanding tokens. MUST succeed before the
	// finalizer can be removed (Requirement C8.4 — "30 秒内回收 token").
	if err := r.IdentityBroker.Deregister(ctx, sa); err != nil {
		return common.RequeueWithBackoff(0), fmt.Errorf("serviceaccount: Deregister: %w", err)
	}

	if removed, err := common.RemoveFinalizer(ctx, r.Client, sa, FinalizerSARevoke); err != nil {
		return ctrl.Result{}, err
	} else if removed {
		r.eventf(sa, corev1.EventTypeNormal, EventReasonRevoked,
			"ServiceAccount tokens revoked at Identity Broker")
	}
	logger.V(1).Info("serviceaccount: finalizer removed", "name", sa.Name)
	return ctrl.Result{}, nil
}

// aggregate computes Ready + Phase + ObservedGeneration in one place.
func (r *ServiceAccountReconciler) aggregate(sa *corev1alpha1.ServiceAccount) {
	status, reason, message := readyFromConditions(sa)
	common.SetCondition(sa, corev1alpha1.ServiceAccountReady, status, reason, message)
	sa.Status.Phase = derivePhase(sa)
	sa.Status.ObservedGeneration = sa.Generation
}

// writeStatus persists the in-memory status block to the API server.
// Conflicts are non-fatal — controller-runtime retries on the next
// reconcile.
func (r *ServiceAccountReconciler) writeStatus(ctx context.Context, sa *corev1alpha1.ServiceAccount, result ctrl.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, sa); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("serviceaccount: status update: %w", err)
	}
	return result, nil
}

// eventf publishes a K8s Event when the recorder is wired up.
func (r *ServiceAccountReconciler) eventf(sa *corev1alpha1.ServiceAccount, eventType, reason, msg string, args ...interface{}) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(sa, eventType, reason, msg, args...)
}

// oboEnabled reports whether the SA has opted into RFC 8693 token
// exchange (`spec.allowOnBehalfOf=true`). The field is a pointer so
// nil is treated as false.
func oboEnabled(sa *corev1alpha1.ServiceAccount) bool {
	return sa != nil && sa.Spec.AllowOnBehalfOf != nil && *sa.Spec.AllowOnBehalfOf
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
var _ reconcile.Reconciler = (*ServiceAccountReconciler)(nil)
