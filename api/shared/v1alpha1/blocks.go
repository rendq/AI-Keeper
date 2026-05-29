package v1alpha1

// This file declares the composite "blocks" that higher-level CRDs embed
// in their Spec — `governance`, `cost`, `slo`, `reliability`. Each block
// carries the same kubebuilder validation markers that
// `aip-crd-openapi.md` defines for the corresponding OpenAPI subschema,
// keeping CR admission and Go types in lock-step.

// MoneyAmount is the canonical wire representation of a USD amount used
// in CostBlock fields. We store money as a string with a constrained
// pattern (rather than `float64`) to keep CRD diffs bit-for-bit
// reproducible and to avoid the JSON-number rounding that breaks audit
// hashing (Requirements F4, F22).
//
// +kubebuilder:validation:Pattern=`^\d+(\.\d+)?$`
// +kubebuilder:validation:MaxLength=32
type MoneyAmount string

// PIIPolicy describes how PII is handled at request and response
// boundaries (design.md §5.3 governance.pii).
type PIIPolicy struct {
	// OnInput controls inbound DLP behaviour.
	//
	// +kubebuilder:validation:Enum=ignore;detect;detect_and_mask;detect_and_block
	// +optional
	OnInput string `json:"onInput,omitempty"`

	// OnOutput controls outbound DLP behaviour.
	//
	// +kubebuilder:validation:Enum=ignore;detect;detect_and_mask;detect_and_block
	// +optional
	OnOutput string `json:"onOutput,omitempty"`

	// PatternsRef points at a `ref://patterns/<name>` resource that holds
	// the regex / dictionary patterns the DLP engine uses.
	// +optional
	PatternsRef *ResourceRef `json:"patternsRef,omitempty"`
}

// DataResidency captures cross-border transfer constraints.
type DataResidency struct {
	// AllowedRegions lists the regions in which a request may be
	// processed (e.g. "cn-north", "eu-west").
	// +optional
	AllowedRegions []string `json:"allowedRegions,omitempty"`

	// CrossBorder controls whether transfers across the residency
	// boundary are permitted.
	//
	// +kubebuilder:validation:Enum=forbidden;allowed_with_approval;allowed
	// +optional
	CrossBorder string `json:"crossBorder,omitempty"`
}

// ComplianceBlock lists the compliance regimes the resource must satisfy.
type ComplianceBlock struct {
	// Required compliance tags (e.g. "GDPR", "HIPAA", "等保三级").
	// +optional
	Required []string `json:"required,omitempty"`

	// ReportTemplate references a compliance reporting template.
	// +optional
	ReportTemplate *ResourceRef `json:"reportTemplate,omitempty"`
}

// GovernanceBlock is the Skill / Tool / Agent / KB-wide governance
// envelope referenced by design.md §5.3.
//
// Validates: Requirements A2.5 (Classification), B4 (PII), C1 (residency),
// D1—D7 (compliance / holds / retention).
type GovernanceBlock struct {
	// Classification applied to data flowing through the resource.
	// +optional
	Classification *Classification `json:"classification,omitempty"`

	// DataResidency controls geographic placement of processing.
	// +optional
	DataResidency *DataResidency `json:"dataResidency,omitempty"`

	// PII policy.
	// +optional
	PII *PIIPolicy `json:"pii,omitempty"`

	// Compliance regimes required for this resource.
	// +optional
	Compliance *ComplianceBlock `json:"compliance,omitempty"`

	// RetentionDays is how many days raw payloads may be retained
	// (audit-related; mirrors `retention` Duration on Skill governance
	// but is expressed in days for budget UIs).
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=36500
	// +optional
	RetentionDays *int32 `json:"retentionDays,omitempty"`

	// Holds carries legal-hold tags. Audit events inheriting these tags
	// are exempt from automatic deletion until the holds are cleared.
	// +optional
	Holds []string `json:"holds,omitempty"`
}

// CostPercentiles models the p50/p95/p99 percentiles used by cost
// estimators. Values are MoneyAmount strings (admission validates the
// string is non-negative via the MoneyAmount regex).
type CostPercentiles struct {
	// +optional
	P50 *MoneyAmount `json:"p50,omitempty"`
	// +optional
	P95 *MoneyAmount `json:"p95,omitempty"`
	// +optional
	P99 *MoneyAmount `json:"p99,omitempty"`
}

// CostEstimator describes how the platform predicts the cost of a
// single invocation.
type CostEstimator struct {
	// Type of estimator.
	//
	// +kubebuilder:validation:Enum=static;model_based;historical
	// +optional
	Type string `json:"type,omitempty"`

	// HistoricalWindow used by historical estimators.
	// +optional
	HistoricalWindow *Duration `json:"historicalWindow,omitempty"`

	// TokensPerCall percentiles.
	// +optional
	TokensPerCall *CostPercentiles `json:"tokensPerCall,omitempty"`

	// UsdPerCall percentiles.
	// +optional
	UsdPerCall *CostPercentiles `json:"usdPerCall,omitempty"`
}

// CostBudget caps per-call spend.
type CostBudget struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerCall *int64 `json:"tokensPerCall,omitempty"`

	// +optional
	UsdPerCall *MoneyAmount `json:"usdPerCall,omitempty"`
}

// CostBlock describes the cost characteristics of a Skill / Tool /
// Agent (design.md §5.3, §8.1).
type CostBlock struct {
	// Estimator config.
	// +optional
	Estimator *CostEstimator `json:"estimator,omitempty"`

	// Budget per call.
	// +optional
	Budget *CostBudget `json:"budget,omitempty"`
}

// SLOBlock declares latency / availability SLOs.
type SLOBlock struct {
	// +kubebuilder:validation:Minimum=0
	// +optional
	P95LatencyMs *int32 `json:"p95LatencyMs,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	P99LatencyMs *int32 `json:"p99LatencyMs,omitempty"`

	// SuccessRate as a fraction in [0, 1].
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	SuccessRate *float64 `json:"successRate,omitempty"`

	// Availability as an SLO percentage string (e.g. "99.5%").
	//
	// +kubebuilder:validation:Pattern=`^(100|[0-9]{1,2}(\.[0-9]+)?)%$`
	// +optional
	Availability string `json:"availability,omitempty"`

	// ErrorBudget consumed by the resource.
	// +optional
	ErrorBudget *Duration `json:"errorBudget,omitempty"`
}

// RetryPolicy describes per-call retry behaviour.
type RetryPolicy struct {
	// Maximum number of retries.
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +optional
	Max *int32 `json:"max,omitempty"`

	// +kubebuilder:validation:Enum=fixed;linear;exponential
	// +optional
	Backoff string `json:"backoff,omitempty"`

	// RetryOn lists the failure classes that should be retried.
	//
	// +listType=set
	// +optional
	RetryOn []string `json:"retryOn,omitempty"`
}

// FallbackSpec describes the fallback chain triggered when the primary
// resource fails.
type FallbackSpec struct {
	// +kubebuilder:validation:Enum=Skill;Agent;StaticResponse
	// +optional
	Kind string `json:"kind,omitempty"`

	// +optional
	Ref *ResourceRef `json:"ref,omitempty"`

	// +optional
	StaticResponse string `json:"staticResponse,omitempty"`
}

// CircuitBreakerSpec describes the circuit-breaker policy.
type CircuitBreakerSpec struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +optional
	ErrorRateThreshold *float64 `json:"errorRateThreshold,omitempty"`

	// +optional
	Window *Duration `json:"window,omitempty"`

	// Enabled flips the breaker on/off without removing config.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ReliabilityBlock collects the retry / fallback / circuit-breaker config
// for a Skill, Tool, or Agent.
type ReliabilityBlock struct {
	// Timeout for a single invocation.
	// +optional
	Timeout *Duration `json:"timeout,omitempty"`

	// Retries policy.
	// +optional
	Retries *RetryPolicy `json:"retries,omitempty"`

	// Fallback chain — multiple entries form an ordered list.
	// +optional
	Fallback []FallbackSpec `json:"fallback,omitempty"`

	// CircuitBreaker policy.
	// +optional
	CircuitBreaker *CircuitBreakerSpec `json:"circuitBreaker,omitempty"`
}
