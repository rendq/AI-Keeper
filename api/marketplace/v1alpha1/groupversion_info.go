// Package v1alpha1 contains the AIP `marketplace.ai-keeper.io` API group
// v1alpha1 types — SkillListing (Namespaced).
//
// +kubebuilder:object:generate=true
// +groupName=marketplace.ai-keeper.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group / version for the marketplace.ai-keeper.io group.
var GroupVersion = schema.GroupVersion{Group: "marketplace.ai-keeper.io", Version: "v1alpha1"}

// SchemeBuilder collects Go types into the runtime.Scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme is the canonical entrypoint used by cmd/manager wiring.
var AddToScheme = SchemeBuilder.AddToScheme
