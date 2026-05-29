// Package v1alpha1 contains the AIP `agent.ai-keeper.io` API group v1alpha1
// types — Agent (Namespaced).
//
// +kubebuilder:object:generate=true
// +groupName=agent.ai-keeper.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group / version for the agent.ai-keeper.io group.
var GroupVersion = schema.GroupVersion{Group: "agent.ai-keeper.io", Version: "v1alpha1"}

// SchemeBuilder collects Go types into the runtime.Scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme is the canonical entrypoint used by cmd/manager wiring.
var AddToScheme = SchemeBuilder.AddToScheme
