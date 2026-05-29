package webhook

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	auditv1alpha1 "github.com/ai-keeper/ai-keeper/api/audit/v1alpha1"
	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// NewScheme builds a runtime.Scheme containing every AIP API group plus
// `client-go`'s built-in `core/v1` types. Tests use this to construct
// fake clients and decoders that understand all 13 Kinds.
//
// Returning a fresh scheme (rather than a package-level singleton)
// keeps tests independent — each test owns its own scheme, so
// modifications cannot leak across packages.
func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(corev1alpha1.AddToScheme(s))
	utilruntime.Must(skillv1alpha1.AddToScheme(s))
	utilruntime.Must(agentv1alpha1.AddToScheme(s))
	utilruntime.Must(policyv1alpha1.AddToScheme(s))
	utilruntime.Must(datav1alpha1.AddToScheme(s))
	utilruntime.Must(modelv1alpha1.AddToScheme(s))
	utilruntime.Must(auditv1alpha1.AddToScheme(s))
	return s
}
