package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Budget conditions (design.md §6.5 BudgetController).
const (
	BudgetEnforcerReady = "EnforcerReady"
	BudgetWithinLimit   = "WithinLimit"
	BudgetReady         = "Ready"
)

// BudgetScope identifies which dimension a budget applies to.
type BudgetScope struct {
	// +kubebuilder:validation:Enum=Tenant;Team;User;Agent;Skill;Project
	Kind string `json:"kind"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// BudgetLimits is the per-period cap.
type BudgetLimits struct {
	// +optional
	Usd *shared.MoneyAmount `json:"usd,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	Tokens *int64 `json:"tokens,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	Calls *int64 `json:"calls,omitempty"`
}

// BudgetAlert is one threshold-trigger.
type BudgetAlert struct {
	// Threshold is a percentage like "80%".
	//
	// +kubebuilder:validation:Pattern=`^\d{1,3}%$`
	Threshold string `json:"threshold"`

	// Channels are notification destinations.
	//
	// +kubebuilder:validation:MinItems=1
	Channels []string `json:"channels"`

	// Action when the threshold trips.
	//
	// +kubebuilder:validation:Enum=notify;throttle;block
	// +optional
	Action string `json:"action,omitempty"`
}

// BudgetSpec is the desired state of a Budget.
type BudgetSpec struct {
	Scope BudgetScope `json:"scope"`

	// +kubebuilder:validation:Enum=hourly;daily;weekly;monthly;quarterly;yearly
	Period string `json:"period"`

	Limits BudgetLimits `json:"limits"`

	// +optional
	Alerts []BudgetAlert `json:"alerts,omitempty"`

	// +optional
	Rollover *bool `json:"rollover,omitempty"`

	// HardCap defaults to true. When set, calls are blocked at 100%.
	// +optional
	HardCap *bool `json:"hardCap,omitempty"`
}

// BudgetCurrent is the running spend.
type BudgetCurrent struct {
	// +optional
	Usd *shared.MoneyAmount `json:"usd,omitempty"`

	// +optional
	Tokens int64 `json:"tokens,omitempty"`

	// +optional
	Calls int64 `json:"calls,omitempty"`
}

// BudgetStatus is the observed state.
type BudgetStatus struct {
	// +optional
	Phase shared.Phase `json:"phase,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +patchStrategy=merge
	// +patchMergeKey=type
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// +optional
	Current *BudgetCurrent `json:"current,omitempty"`

	// +optional
	PeriodStart *metav1.Time `json:"periodStart,omitempty"`

	// +optional
	PeriodEnd *metav1.Time `json:"periodEnd,omitempty"`

	// +optional
	DaysRemaining *int32 `json:"daysRemaining,omitempty"`

	// BurnRate is one of {ok, warning, critical, exhausted}.
	//
	// +kubebuilder:validation:Enum=ok;warning;critical;exhausted
	// +optional
	BurnRate string `json:"burnRate,omitempty"`

	// +optional
	ProjectedExhaustionAt *metav1.Time `json:"projectedExhaustionAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=bg,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Scope",type=string,JSONPath=`.spec.scope.kind`
// +kubebuilder:printcolumn:name="Period",type=string,JSONPath=`.spec.period`
// +kubebuilder:printcolumn:name="Used",type=string,JSONPath=`.status.current.usd`
// +kubebuilder:printcolumn:name="Limit",type=string,JSONPath=`.spec.limits.usd`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Budget caps spend on a (kind, name) scope per period.
type Budget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BudgetSpec   `json:"spec,omitempty"`
	Status BudgetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BudgetList is the canonical list type for Budget.
type BudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Budget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Budget{}, &BudgetList{})
}
