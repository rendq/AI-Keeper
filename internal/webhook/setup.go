package webhook

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	auditv1alpha1 "github.com/ai-keeper/ai-keeper/api/audit/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// SetupWithManager registers every AIP CustomValidator with the
// controller-runtime Manager's webhook server.
//
// The function fans out into per-Kind builders so that adding a new
// validator stays a one-line change. Each call uses the canonical
// kubebuilder URL pattern `/validate-<group>-<version>-<kind>`, which
// is also what the helm template `webhook-validating-config.yaml`
// references.
//
// Validates: Requirements A1.3, A1.5, A2.1—A2.6.
func SetupWithManager(mgr manager.Manager) error {
	if mgr == nil {
		return fmt.Errorf("webhook.SetupWithManager: manager is nil")
	}

	saChecker := NewSystemSAChecker(mgr.GetClient())

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&skillv1alpha1.Skill{}).
		WithValidator(&SkillValidator{}).
		Complete(); err != nil {
		return fmt.Errorf("registering Skill validator: %w", err)
	}
	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&skillv1alpha1.Tool{}).
		WithValidator(&ToolValidator{}).
		Complete(); err != nil {
		return fmt.Errorf("registering Tool validator: %w", err)
	}
	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&agentv1alpha1.Agent{}).
		WithValidator(&AgentValidator{}).
		Complete(); err != nil {
		return fmt.Errorf("registering Agent validator: %w", err)
	}
	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&policyv1alpha1.Policy{}).
		WithValidator(&PolicyValidator{}).
		Complete(); err != nil {
		return fmt.Errorf("registering Policy validator: %w", err)
	}
	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&auditv1alpha1.AuditEvent{}).
		WithValidator(&AuditEventValidator{SystemSAChecker: saChecker}).
		Complete(); err != nil {
		return fmt.Errorf("registering AuditEvent validator: %w", err)
	}
	return nil
}
