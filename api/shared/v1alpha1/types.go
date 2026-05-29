package v1alpha1

// This file declares the primitive shared type aliases consumed by every
// AIP CRD. Each alias carries kubebuilder admission validation markers so
// that bad input is rejected by the API server before a controller sees it
// (Requirements A2.1 — A2.6).

// ResourceRef is the canonical AIP URI used to reference any other resource
// (skills, tools, models, data, prompts, channels, connectors, memory,
// quota, ref, siem, policy). The accepted shape is
// `<scheme>://<path>[@<version>]`.
//
// Validates: Requirements A2.1 (regex enforcement), F25 (Parse∘Format
// round-trip).
//
// +kubebuilder:validation:Pattern=`^(skill|agent|tool|model|data|prompt|channel|connector|memory|quota|ref|siem|policy)://[A-Za-z0-9._/\-]+(@[A-Za-z0-9._\-+]+)?$`
// +kubebuilder:validation:MinLength=8
// +kubebuilder:validation:MaxLength=512
type ResourceRef string

// Duration is a coarse-grained, human-friendly duration string used in
// CRDs. Supported units: ns, us, ms, s, m, h, d, w.
//
// Validates: Requirements A2.2.
//
// +kubebuilder:validation:Pattern=`^\d+(ns|us|ms|s|m|h|d|w)$`
// +kubebuilder:validation:MinLength=2
// +kubebuilder:validation:MaxLength=32
type Duration string

// SemVer is a strict semantic version string per https://semver.org. The
// pattern matches `MAJOR.MINOR.PATCH[-prerelease][+build]` with no leading
// zeros on numeric identifiers.
//
// Validates: Requirements A2.3.
//
// +kubebuilder:validation:Pattern=`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`
// +kubebuilder:validation:MinLength=5
// +kubebuilder:validation:MaxLength=64
type SemVer string

// Stage indicates the maturity of a Skill or other versioned artifact.
//
// Validates: Requirements A2.6.
//
// +kubebuilder:validation:Enum=experimental;beta;stable;deprecated
type Stage string

// Stage constants.
const (
	StageExperimental Stage = "experimental"
	StageBeta         Stage = "beta"
	StageStable       Stage = "stable"
	StageDeprecated   Stage = "deprecated"
)

// Classification carries the data-sensitivity tag of a request, response,
// CRD or audit event.
//
// Validates: Requirements A2.5.
//
// +kubebuilder:validation:Enum=public;internal;confidential;restricted;secret
type Classification string

// Classification constants.
const (
	ClassificationPublic       Classification = "public"
	ClassificationInternal     Classification = "internal"
	ClassificationConfidential Classification = "confidential"
	ClassificationRestricted   Classification = "restricted"
	ClassificationSecret       Classification = "secret"
)

// Phase is the coarse-grained status.phase value reported by every AIP
// controller. The combined enum covers every controller in the platform
// (Skill / Agent / Policy / Tool / DataSource / KB / ModelEndpoint /
// ModelRouter / Tenant / SA / Budget / Quota), so a single Phase type can
// be embedded in any status struct.
//
// Validates: Requirements A3, A4, A5 (state machines reference these
// values).
//
// +kubebuilder:validation:Enum=Pending;Validating;Resolving;Building;Registering;Evaluating;Active;Degraded;Failed;Deprecated;Terminating;Suspended;Expired;ResolvingSkills;AttachingPolicies;Provisioning;Deploying;Configuring;RollingOut;Running;Paused;RolledBack
type Phase string

// Phase constants enumerated in the order they appear in the controller
// state machines (design.md §6).
const (
	// Generic lifecycle.
	PhasePending     Phase = "Pending"
	PhaseValidating  Phase = "Validating"
	PhaseResolving   Phase = "Resolving"
	PhaseBuilding    Phase = "Building"
	PhaseRegistering Phase = "Registering"
	PhaseEvaluating  Phase = "Evaluating"
	PhaseActive      Phase = "Active"
	PhaseDegraded    Phase = "Degraded"
	PhaseFailed      Phase = "Failed"
	PhaseDeprecated  Phase = "Deprecated"
	PhaseTerminating Phase = "Terminating"
	PhaseSuspended   Phase = "Suspended"
	PhaseExpired     Phase = "Expired"

	// Agent-specific phases (design.md §6.2).
	PhaseResolvingSkills   Phase = "ResolvingSkills"
	PhaseAttachingPolicies Phase = "AttachingPolicies"
	PhaseProvisioning      Phase = "Provisioning"
	PhaseDeploying         Phase = "Deploying"
	PhaseConfiguring       Phase = "Configuring"
	PhaseRollingOut        Phase = "RollingOut"
	PhaseRunning           Phase = "Running"
	PhasePaused            Phase = "Paused"
	PhaseRolledBack        Phase = "RolledBack"
)
