// Package v1alpha1 contains the shared, embeddable API types reused across
// every AIP CRD group (skill / agent / policy / data / model / core / audit).
//
// Concretely it exposes:
//
//   - Type aliases with admission-time validation: ResourceRef, Duration,
//     SemVer, Stage, Phase, Classification (Requirements A2.1 — A2.6).
//   - Composite specs embedded by higher-level CRDs: GovernanceBlock,
//     CostBlock, SLOBlock, ReliabilityBlock (design.md §5.3).
//   - Conditions helpers used by controllers when reconciling resources.
//
// The package itself does not register a Kind — `shared.ai-keeper.io` only carries
// reusable schemas. controller-gen emits deepcopy methods because the
// `+kubebuilder:object:generate=true` marker is set on the package below.
//
// +kubebuilder:object:generate=true
// +groupName=shared.ai-keeper.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group / version for shared embedded types. While
// no Kind is registered today, downstream packages may import this constant
// when wiring their own SchemeBuilder so that conversion webhooks can refer
// to a single source of truth.
var GroupVersion = schema.GroupVersion{Group: "shared.ai-keeper.io", Version: "v1alpha1"}

// SchemeBuilder is provided for symmetry with the Kind-bearing API groups.
// It currently has no Kinds registered but stays here so that downstream
// packages can call SchemeBuilder.Register(...) if they wish to attach
// additional schemas later.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme adds the (currently empty) shared types to a runtime.Scheme.
// Kept for kubebuilder ergonomics — most callers will not need it.
var AddToScheme = SchemeBuilder.AddToScheme
