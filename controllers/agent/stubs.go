package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
)

// PolicyBinder binds an Agent's policy graph to the Policy Decision
// Point (Requirement A4.1 — "policy 绑定到 PDP"). Real implementations
// land in tasks 5.x; the [NoopPolicyBinder] below makes the reconciler
// driveable in P0.
type PolicyBinder interface {
	// Bind compiles `agent`'s effective policy set and returns the
	// canonical names that the PDP exposes for the agent. The names are
	// echoed onto `Agent.status.effectivePolicies`.
	Bind(ctx context.Context, agent *agentv1alpha1.Agent) (effectivePolicies []string, err error)
}

// NoopPolicyBinder always succeeds and returns an empty policy slice.
// Used as the default until task 5.1 ships the real compiler.
type NoopPolicyBinder struct{}

// Bind returns no policies and no error.
func (NoopPolicyBinder) Bind(_ context.Context, _ *agentv1alpha1.Agent) ([]string, error) {
	return nil, nil
}

// IdentityProvisioner mints / refreshes the Agent's runtime identity
// (ServiceAccount + token exchanger) per Requirement A4.1 and tears it
// down on drain (Requirement A4.7). Real implementation lands in task
// 6.1.
type IdentityProvisioner interface {
	// Provision must be idempotent.
	Provision(ctx context.Context, agent *agentv1alpha1.Agent) error
	// Revoke is invoked on drain — it MUST tolerate "already revoked"
	// gracefully so the deletion path is re-entrant.
	Revoke(ctx context.Context, agent *agentv1alpha1.Agent) error
}

// NoopIdentityProvisioner is a no-op.
type NoopIdentityProvisioner struct{}

// Provision returns nil.
func (NoopIdentityProvisioner) Provision(_ context.Context, _ *agentv1alpha1.Agent) error {
	return nil
}

// Revoke returns nil.
func (NoopIdentityProvisioner) Revoke(_ context.Context, _ *agentv1alpha1.Agent) error {
	return nil
}

// ChannelRegistrar registers / deregisters channel webhooks for an
// Agent (Requirement A4.1 / A4.7). Real implementation lives in
// tasks 14.x; for P0 the [NoopChannelRegistrar] keeps the reconciler
// simple.
type ChannelRegistrar interface {
	RegisterChannels(ctx context.Context, agent *agentv1alpha1.Agent) error
	DeregisterChannels(ctx context.Context, agent *agentv1alpha1.Agent) error
}

// NoopChannelRegistrar is a no-op.
type NoopChannelRegistrar struct{}

// RegisterChannels returns nil.
func (NoopChannelRegistrar) RegisterChannels(_ context.Context, _ *agentv1alpha1.Agent) error {
	return nil
}

// DeregisterChannels returns nil.
func (NoopChannelRegistrar) DeregisterChannels(_ context.Context, _ *agentv1alpha1.Agent) error {
	return nil
}

// AuditFlusher persists pending audit events before the Agent's
// Deployment is scaled down (Requirement A4.7 — "刷新待落审计事件").
// Real implementation lands in task 12.x.
type AuditFlusher interface {
	Flush(ctx context.Context, agent *agentv1alpha1.Agent) error
}

// NoopAuditFlusher returns nil.
type NoopAuditFlusher struct{}

// Flush returns nil.
func (NoopAuditFlusher) Flush(_ context.Context, _ *agentv1alpha1.Agent) error { return nil }

// SessionTracker reports the number of in-flight sessions for an
// Agent during drain (Requirement A4.7). Real impl pulls from
// Redis / runtime telemetry; the unit-test stub returns whatever the
// caller seeded.
type SessionTracker interface {
	InFlight(ctx context.Context, agent *agentv1alpha1.Agent) (int, error)
}

// NoopSessionTracker always reports zero.
type NoopSessionTracker struct{}

// InFlight returns 0.
func (NoopSessionTracker) InFlight(_ context.Context, _ *agentv1alpha1.Agent) (int, error) {
	return 0, nil
}

// DeploymentManager owns the lifecycle of the Agent's Kubernetes
// Deployment + (HPA + Service in production) — Requirement A4.1.
// Real implementations may compose multiple K8s objects; this
// interface only exposes the methods the reconciler needs.
type DeploymentManager interface {
	// EnsureDeployment converges the underlying K8s object(s) and
	// reports the desired/observed replica counts.
	EnsureDeployment(ctx context.Context, agent *agentv1alpha1.Agent) (replicas, readyReplicas int32, err error)
	// Drain scales the Deployment to zero and removes platform-owned
	// resources. MUST be idempotent.
	Drain(ctx context.Context, agent *agentv1alpha1.Agent) error
}

// KubeDeploymentManager is the production [DeploymentManager]: it
// creates / updates a single `appsv1.Deployment` whose replica count
// follows `agent.spec.deployment.replicas`. HPA + Service wiring is
// stubbed at the call site for P0 — the manager only owns the
// Deployment so the unit tests can exercise the reconcile path with
// the controller-runtime fake client.
type KubeDeploymentManager struct {
	Client client.Client
	// Scheme, when non-nil, is used to set the Agent as the controller
	// owner of the Deployment so garbage collection cleans up after
	// the Agent is deleted. Tests bypass this by leaving it nil.
	Scheme *runtime.Scheme
}

// NewKubeDeploymentManager constructs a KubeDeploymentManager.
func NewKubeDeploymentManager(c client.Client, scheme *runtime.Scheme) *KubeDeploymentManager {
	return &KubeDeploymentManager{Client: c, Scheme: scheme}
}

// EnsureDeployment creates or updates the Deployment owned by the
// Agent and returns the desired/observed replica counts.
func (m *KubeDeploymentManager) EnsureDeployment(ctx context.Context, agent *agentv1alpha1.Agent) (int32, int32, error) {
	if m == nil || m.Client == nil {
		return 0, 0, errors.New("agent: nil KubeDeploymentManager")
	}
	desired := desiredReplicas(agent)
	dep := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: agent.Namespace, Name: agent.Name}
	switch err := m.Client.Get(ctx, key, dep); {
	case err == nil:
		// Update mutable fields. We deliberately do not rebuild the
		// Pod template here — production wiring lands in the data plane
		// task.
		dep.Spec.Replicas = ptrInt32(desired)
		if err := m.Client.Update(ctx, dep); err != nil {
			return desired, dep.Status.ReadyReplicas, fmt.Errorf("agent: update Deployment %s: %w", key, err)
		}
	case apierrors.IsNotFound(err):
		dep = m.buildDeployment(agent, desired)
		if err := m.Client.Create(ctx, dep); err != nil {
			return desired, 0, fmt.Errorf("agent: create Deployment %s: %w", key, err)
		}
	default:
		return desired, 0, fmt.Errorf("agent: get Deployment %s: %w", key, err)
	}
	return desired, dep.Status.ReadyReplicas, nil
}

// Drain scales the Deployment to zero and waits for the controller to
// observe it. Subsequent reconciles drop the object once the API
// server confirms removal.
func (m *KubeDeploymentManager) Drain(ctx context.Context, agent *agentv1alpha1.Agent) error {
	if m == nil || m.Client == nil {
		return errors.New("agent: nil KubeDeploymentManager")
	}
	dep := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: agent.Namespace, Name: agent.Name}
	switch err := m.Client.Get(ctx, key, dep); {
	case err == nil:
		// Scale to zero before deleting so any in-flight pods are
		// drained gracefully by their preStop hooks.
		zero := int32(0)
		if dep.Spec.Replicas == nil || *dep.Spec.Replicas != zero {
			dep.Spec.Replicas = &zero
			if err := m.Client.Update(ctx, dep); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("agent: scale Deployment to zero %s: %w", key, err)
			}
		}
		if err := m.Client.Delete(ctx, dep); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("agent: delete Deployment %s: %w", key, err)
		}
	case apierrors.IsNotFound(err):
		// Already gone.
	default:
		return fmt.Errorf("agent: get Deployment %s: %w", key, err)
	}
	return nil
}

// buildDeployment renders the canonical Deployment for `agent`. The
// Pod template intentionally mirrors what the data-plane wiring task
// will replace; we keep it minimal so the controller is exercisable in
// unit tests today.
func (m *KubeDeploymentManager) buildDeployment(agent *agentv1alpha1.Agent, desired int32) *appsv1.Deployment {
	labels := map[string]string{
		"app.kubernetes.io/name":       "aip-agent",
		"app.kubernetes.io/instance":   agent.Name,
		"app.kubernetes.io/managed-by": "aip-agent-controller",
		"ai-keeper.io/agent":                 agent.Name,
	}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(desired),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       intstrPtr(intstr.FromString("25%")),
					MaxUnavailable: intstrPtr(intstr.FromString("25%")),
				},
			},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: agent.Spec.Identity.ServiceAccount,
					Containers: []corev1.Container{
						{
							Name:    "agent-runtime",
							Image:   "ghcr.io/aip-io/aip-runtime:placeholder",
							Command: []string{"/aip-runtime"},
							Env: []corev1.EnvVar{
								{Name: "AIP_AGENT_NAMESPACE", Value: agent.Namespace},
								{Name: "AIP_AGENT_NAME", Value: agent.Name},
								{Name: "AIP_AGENT_PATTERN", Value: agent.Spec.Runtime.Pattern},
							},
						},
					},
				},
			},
		},
	}
	if rt := agent.Spec.Runtime.Sandbox; rt != nil && rt.Enabled != nil && *rt.Enabled && rt.Type != "" && rt.Type != "none" {
		rcName := runtimeClassNameForSandbox(rt.Type)
		dep.Spec.Template.Spec.RuntimeClassName = &rcName
	}
	// Best-effort owner reference. SetControllerReference requires a
	// scheme that recognises the Agent kind; KubeDeploymentManager is
	// constructed without a scheme today (data-plane task) so we skip
	// the call when m.Scheme is nil. The production wiring will pass a
	// non-nil scheme via the manager bootstrap.
	if m.Scheme != nil {
		_ = controllerutil.SetControllerReference(agent, dep, m.Scheme)
	}
	return dep
}

// desiredReplicas returns the replica count from the spec, defaulting
// to 1 when unspecified (Requirement A4.1 — Deployment is created with
// at least one replica unless explicitly told otherwise).
func desiredReplicas(agent *agentv1alpha1.Agent) int32 {
	if agent == nil || agent.Spec.Deployment == nil || agent.Spec.Deployment.Replicas == nil {
		return 1
	}
	if *agent.Spec.Deployment.Replicas < 0 {
		return 0
	}
	return *agent.Spec.Deployment.Replicas
}

// runtimeClassNameForSandbox maps the sandbox.type enum to the
// canonical RuntimeClass name (Requirement A4.3). The map is
// exhaustive over the {gvisor, firecracker, kata, e2b} set; callers
// MUST validate the type before calling.
func runtimeClassNameForSandbox(t string) string {
	switch t {
	case "gvisor":
		return "gvisor"
	case "firecracker":
		return "firecracker"
	case "kata":
		return "kata-containers"
	case "e2b":
		return "e2b"
	default:
		return t
	}
}

// FakeDeploymentManager is the in-memory [DeploymentManager] used in
// unit tests. It records every call and returns whatever replica
// counts the caller seeded. Concurrency-safe.
type FakeDeploymentManager struct {
	mu              sync.Mutex
	Replicas        int32
	ReadyReplicas   int32
	EnsureCalls     int
	DrainCalls      int
	EnsureErr       error
	DrainErr        error
	LastEnsureAgent *agentv1alpha1.Agent
}

// EnsureDeployment records the call and returns the seeded values.
func (f *FakeDeploymentManager) EnsureDeployment(_ context.Context, agent *agentv1alpha1.Agent) (int32, int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EnsureCalls++
	f.LastEnsureAgent = agent
	if f.EnsureErr != nil {
		return f.Replicas, f.ReadyReplicas, f.EnsureErr
	}
	return f.Replicas, f.ReadyReplicas, nil
}

// Drain records the call.
func (f *FakeDeploymentManager) Drain(_ context.Context, _ *agentv1alpha1.Agent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DrainCalls++
	return f.DrainErr
}

// Snapshot returns a copy of the recorded counters for assertions.
func (f *FakeDeploymentManager) Snapshot() (ensure, drain int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.EnsureCalls, f.DrainCalls
}

// Compile-time interface assertions.
var (
	_ PolicyBinder        = NoopPolicyBinder{}
	_ IdentityProvisioner = NoopIdentityProvisioner{}
	_ ChannelRegistrar    = NoopChannelRegistrar{}
	_ AuditFlusher        = NoopAuditFlusher{}
	_ SessionTracker      = NoopSessionTracker{}
	_ DeploymentManager   = (*KubeDeploymentManager)(nil)
	_ DeploymentManager   = (*FakeDeploymentManager)(nil)
)

// ptrInt32 returns a heap-allocated pointer to v.
func ptrInt32(v int32) *int32 { return &v }

// intstrPtr returns a heap-allocated pointer to v.
func intstrPtr(v intstr.IntOrString) *intstr.IntOrString { return &v }
