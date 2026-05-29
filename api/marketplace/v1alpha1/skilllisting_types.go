package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// PricingModel describes the pricing strategy for a marketplace listing.
//
// +kubebuilder:validation:Enum=free;per_call;per_month;per_token
type PricingModel string

// PricingModel constants.
const (
	PricingFree     PricingModel = "free"
	PricingPerCall  PricingModel = "per_call"
	PricingPerMonth PricingModel = "per_month"
	PricingPerToken PricingModel = "per_token"
)

// SkillListingPhase is the marketplace-specific lifecycle phase.
//
// +kubebuilder:validation:Enum=Draft;PendingReview;Published;Rejected;Suspended;Archived
type SkillListingPhase string

// SkillListingPhase constants.
const (
	SkillListingPhaseDraft         SkillListingPhase = "Draft"
	SkillListingPhasePendingReview SkillListingPhase = "PendingReview"
	SkillListingPhasePublished     SkillListingPhase = "Published"
	SkillListingPhaseRejected      SkillListingPhase = "Rejected"
	SkillListingPhaseSuspended     SkillListingPhase = "Suspended"
	SkillListingPhaseArchived      SkillListingPhase = "Archived"
)

// PublisherInfo identifies the publisher of a skill listing.
type PublisherInfo struct {
	// Name of the publisher.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=200
	Name string `json:"name"`

	// Email contact for the publisher.
	//
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=320
	// +optional
	Email string `json:"email,omitempty"`

	// URL of the publisher's website or profile.
	// +optional
	URL string `json:"url,omitempty"`
}

// PricingSpec defines the pricing model and amount for a listing.
type PricingSpec struct {
	// Model is the pricing strategy.
	Model PricingModel `json:"model"`

	// Amount is the price in USD (e.g. "0.01" per call, "29.99" per month).
	// Only required when Model is not "free".
	// +optional
	Amount *shared.MoneyAmount `json:"amount,omitempty"`
}

// Screenshot holds a reference to a screenshot image for the listing.
type Screenshot struct {
	// URL of the screenshot image.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	URL string `json:"url"`

	// Caption describes the screenshot.
	// +optional
	Caption string `json:"caption,omitempty"`
}

// SkillListingSpec describes the desired state of a SkillListing.
type SkillListingSpec struct {
	// SkillRef references the underlying Skill being listed.
	SkillRef shared.ResourceRef `json:"skillRef"`

	// Publisher identifies who is publishing this skill.
	Publisher PublisherInfo `json:"publisher"`

	// Pricing defines the pricing model for this listing.
	Pricing PricingSpec `json:"pricing"`

	// Category classifies the listing for marketplace browsing.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	Category string `json:"category"`

	// Tags for search and filtering.
	//
	// +listType=set
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Screenshots for the listing detail page.
	//
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Screenshots []Screenshot `json:"screenshots,omitempty"`

	// Readme is the long-form description (Markdown).
	//
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	Readme string `json:"readme,omitempty"`
}

// SkillListingStatus is the observed state of a SkillListing.
type SkillListingStatus struct {
	// Phase is the marketplace lifecycle phase.
	// +optional
	Phase SkillListingPhase `json:"phase,omitempty"`

	// Downloads is the total number of installs/downloads.
	//
	// +kubebuilder:validation:Minimum=0
	// +optional
	Downloads int64 `json:"downloads,omitempty"`

	// AverageRating is the mean star rating (1.0–5.0).
	// +optional
	AverageRating string `json:"averageRating,omitempty"`

	// Revenue is the total revenue generated in USD.
	// +optional
	Revenue *shared.MoneyAmount `json:"revenue,omitempty"`

	// ObservedGeneration is the .metadata.generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions track transitions of the controller state machine.
	//
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=sl,categories={aip}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Category",type=string,JSONPath=`.spec.category`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Downloads",type=integer,JSONPath=`.status.downloads`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SkillListing represents a skill published to the AIP marketplace.
type SkillListing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillListingSpec   `json:"spec,omitempty"`
	Status SkillListingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SkillListingList is the canonical list type for SkillListing.
type SkillListingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SkillListing `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SkillListing{}, &SkillListingList{})
}
