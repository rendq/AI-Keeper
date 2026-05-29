// Package v1beta1 contains the AIP `skill.ai-keeper.io` API group v1beta1
// types — Skill (Namespaced). This version extends v1alpha1 with
// compliance and enhanced evaluation fields.
//
// +kubebuilder:object:generate=true
// +groupName=skill.ai-keeper.io
package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group / version for the skill.ai-keeper.io v1beta1 group.
var GroupVersion = schema.GroupVersion{Group: "skill.ai-keeper.io", Version: "v1beta1"}

// SchemeBuilder collects Go types into the runtime.Scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme is the canonical entrypoint used by cmd/manager wiring.
var AddToScheme = SchemeBuilder.AddToScheme
