// Package v1alpha1 contains the AIP `core.ai-keeper.io` API group v1alpha1
// types — Tenant (Cluster scope) and ServiceAccount (Namespaced).
//
// +kubebuilder:object:generate=true
// +groupName=core.ai-keeper.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group / version for the core.ai-keeper.io group.
var GroupVersion = schema.GroupVersion{Group: "core.ai-keeper.io", Version: "v1alpha1"}

// SchemeBuilder collects Go types into the runtime.Scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme is the canonical entrypoint used by cmd/manager wiring and
// by clients that need to (de)serialize this group.
var AddToScheme = SchemeBuilder.AddToScheme
