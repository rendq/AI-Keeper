package conversion

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ConvertPath is the URL path the API server dials against the
// webhook Service. Kubebuilder & controller-runtime conventions both
// use `/convert`; the same path is referenced from each CRD's
// `spec.conversion.webhookClientConfig.service.path` once P1 flips
// `spec.conversion.strategy=Webhook`. For P0 every CRD keeps
// `strategy=None` (the kubebuilder default), so the route is dormant
// but reachable.
const ConvertPath = "/convert"

// SetupWithManager registers the ConversionWebhook handler with the
// controller-runtime Manager's webhook server. controller-runtime
// hosts the server on port 9443 by default, which matches the
// `containerPort` declared by the manager Helm sub-chart
// (deploy/helm/ai-keeper/charts/manager/values.yaml).
//
// Validates: Requirements A11.1, A11.2 (placeholder).
func SetupWithManager(mgr manager.Manager) error {
	if mgr == nil {
		return fmt.Errorf("conversion.SetupWithManager: manager is nil")
	}
	mgr.GetWebhookServer().Register(ConvertPath, NewHandler())
	return nil
}
