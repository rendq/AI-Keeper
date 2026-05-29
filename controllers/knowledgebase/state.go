package knowledgebase

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// FinalizerKnowledgeBaseProtect is the finalizer added to every
// reconciled KnowledgeBase CR so the controller can drain the index
// on deletion (Requirement A7.5 — basic finalizer; full drain in P1).
const FinalizerKnowledgeBaseProtect = "ai-keeper.io/kb-protect"

// SteadyStateRequeue is the periodic re-validation cadence applied
// while the full pipeline + cron sync is deferred to P1.
const SteadyStateRequeue = 5 * time.Minute

// ACL mode values mirrored from
// [datav1alpha1.KBACL.Mode] for readability.
const (
	ACLModeInheritFromSource = "inherit_from_source"
	ACLModeCustom            = "custom"
	// ACLModeOpen is intentionally rejected by the lint rule
	// `kb/acl-not-open` for KBs classified ≥ confidential. The KB CRD
	// does not list `open` in its enum (only `inherit_from_source` /
	// `custom`), but DataSources do — and the lint rule talks about
	// the inherited mode. We carry the literal here so the runtime
	// gate is precise.
	ACLModeOpen = "open"
)

// Reason constants surfaced on KnowledgeBase conditions and Events.
const (
	// ReasonSourcesReady marks `SourcesReady=True`.
	ReasonSourcesReady = "SourcesReady"
	// ReasonSourceMissing marks `SourcesReady=False` after an unknown
	// `sources[].ref` is observed.
	ReasonSourceMissing = "SourceMissing"
	// ReasonSourceRefInvalid marks `SourcesReady=False` after a
	// malformed `sources[].ref`.
	ReasonSourceRefInvalid = "SourceRefInvalid"

	// ReasonIndexed marks `Indexed=True`.
	ReasonIndexed = "Indexed"
	// ReasonIndexFailed marks `Indexed=False`.
	ReasonIndexFailed = "IndexFailed"
	// ReasonIndexEmpty marks `Indexed=False` when the pipeline reports
	// zero chunks.
	ReasonIndexEmpty = "IndexEmpty"

	// ReasonSynced marks `Synced=True`.
	ReasonSynced = "Synced"
	// ReasonSyncDeferred marks `Synced=Unknown` while the full
	// schedule lands in P1.
	ReasonSyncDeferred = "SyncDeferred"

	// ReasonInvalidACL marks the ACL lint failure: classification ≥
	// confidential combined with `acl.mode=open` is rejected.
	ReasonInvalidACL = "InvalidACL"

	// ReasonReady is the aggregate-Ready success reason.
	ReasonReady = "Ready"
	// ReasonNotReady is the aggregate-Ready failure reason.
	ReasonNotReady = "NotReady"
)

// derivePhase maps the current Conditions slice to a coarse phase per
// design.md §6.5. Precedence:
//
//  1. `metadata.deletionTimestamp` set → Terminating
//  2. `SourcesReady=False reason=InvalidACL` → Failed
//  3. Aggregate Ready=True → Active
//  4. `SourcesReady=False` → Degraded
//  5. Otherwise → Pending
func derivePhase(kb *datav1alpha1.KnowledgeBase) sharedv1alpha1.Phase {
	if kb == nil {
		return sharedv1alpha1.PhasePending
	}
	if !kb.GetDeletionTimestamp().IsZero() {
		return sharedv1alpha1.PhaseTerminating
	}
	conds := kb.Status.Conditions
	if c := condition(conds, datav1alpha1.KnowledgeBaseSourcesReady); c != nil &&
		c.Status == metav1.ConditionFalse && c.Reason == ReasonInvalidACL {
		return sharedv1alpha1.PhaseFailed
	}
	if isTrue(conds, datav1alpha1.KnowledgeBaseReady) {
		return sharedv1alpha1.PhaseActive
	}
	if c := condition(conds, datav1alpha1.KnowledgeBaseSourcesReady); c != nil &&
		c.Status == metav1.ConditionFalse {
		return sharedv1alpha1.PhaseDegraded
	}
	return sharedv1alpha1.PhasePending
}

// readyFromConditions implements the aggregate Ready logic: SourcesReady
// ∧ Indexed ∧ Synced ∈ {True, Unknown(reason=SyncDeferred)}.
func readyFromConditions(kb *datav1alpha1.KnowledgeBase) (status, reason, message string) {
	conds := kb.Status.Conditions
	if !isTrue(conds, datav1alpha1.KnowledgeBaseSourcesReady) {
		return string(metav1.ConditionFalse), ReasonNotReady, datav1alpha1.KnowledgeBaseSourcesReady + " not satisfied"
	}
	if !isTrue(conds, datav1alpha1.KnowledgeBaseIndexed) {
		return string(metav1.ConditionFalse), ReasonNotReady, datav1alpha1.KnowledgeBaseIndexed + " not satisfied"
	}
	sync := condition(conds, datav1alpha1.KnowledgeBaseSynced)
	switch {
	case sync == nil:
		return string(metav1.ConditionFalse), ReasonNotReady, datav1alpha1.KnowledgeBaseSynced + " missing"
	case sync.Status == metav1.ConditionTrue:
		return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied"
	case sync.Status == metav1.ConditionUnknown && sync.Reason == ReasonSyncDeferred:
		return string(metav1.ConditionTrue), ReasonReady, "all gates satisfied (sync deferred)"
	default:
		return string(metav1.ConditionFalse), ReasonNotReady, datav1alpha1.KnowledgeBaseSynced + " not satisfied"
	}
}

// classificationAtLeastConfidential reports whether the KB carries a
// classification of `confidential`, `restricted`, or `secret`.
// Treats nil pointers as `internal`.
func classificationAtLeastConfidential(kb *datav1alpha1.KnowledgeBase) bool {
	if kb == nil || kb.Spec.Governance == nil || kb.Spec.Governance.Classification == nil {
		return false
	}
	switch *kb.Spec.Governance.Classification {
	case sharedv1alpha1.ClassificationConfidential,
		sharedv1alpha1.ClassificationRestricted,
		sharedv1alpha1.ClassificationSecret:
		return true
	}
	return false
}

// aclModeOf returns the effective `acl.mode` declared on the KB, or
// the empty string when omitted.
func aclModeOf(kb *datav1alpha1.KnowledgeBase) string {
	if kb == nil || kb.Spec.ACL == nil {
		return ""
	}
	return kb.Spec.ACL.Mode
}

// condition returns a pointer to the named condition, or nil.
func condition(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}

// isTrue reports whether the named condition is present and True.
func isTrue(conds []metav1.Condition, t string) bool {
	c := condition(conds, t)
	return c != nil && c.Status == metav1.ConditionTrue
}
