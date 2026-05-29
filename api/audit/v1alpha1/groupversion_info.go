// Package v1alpha1 contains the AIP `audit.ai-keeper.io` API group v1alpha1
// types — AuditEvent (Namespaced).
//
// AuditEvent is *read-mostly* from a user perspective: only system
// components (with the `ai-keeper.io/system=true` ServiceAccount annotation)
// may CREATE/UPDATE/DELETE. The admission guard enforcing that lives in
// task 2.3; this package only declares the Go types and the CRD shell.
//
// +kubebuilder:object:generate=true
// +groupName=audit.ai-keeper.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group / version for the audit.ai-keeper.io group.
var GroupVersion = schema.GroupVersion{Group: "audit.ai-keeper.io", Version: "v1alpha1"}

// SchemeBuilder collects Go types into the runtime.Scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme is the canonical entrypoint used by cmd/manager wiring.
var AddToScheme = SchemeBuilder.AddToScheme
