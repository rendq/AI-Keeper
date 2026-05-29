// Package conflictcheck implements policy conflict detection (hard/soft).
//
// Algorithm:
//  1. For each pair of policies with the same priority and opposite effect,
//     compare their subject sets and resource sets.
//  2. Hard conflict: subject sets AND resource sets are completely equivalent.
//  3. Soft conflict: subject sets OR resource sets partially overlap (non-empty
//     intersection but not full equivalence).
//
// Requirements: A5.3, A5.4, F14
package conflictcheck

import (
	"fmt"
	"sort"
	"strings"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// ConflictType distinguishes hard from soft conflicts.
type ConflictType string

const (
	// Hard means two policies with same priority, opposite effect, and
	// completely overlapping subject+resource sets.
	Hard ConflictType = "Hard"
	// Soft means two policies with same priority, opposite effect, and
	// partially overlapping subject or resource sets.
	Soft ConflictType = "Soft"
)

// ConflictResult describes a detected conflict between two policies.
type ConflictResult struct {
	// Type is Hard or Soft.
	Type ConflictType

	// PolicyA is the name/index of the first policy.
	PolicyA string

	// PolicyB is the name/index of the second policy.
	PolicyB string

	// Reason is a human-readable explanation.
	Reason string
}

// DetectConflicts checks a set of policies for hard and soft conflicts.
// It returns all detected conflicts between policy pairs.
func DetectConflicts(policies []policyv1alpha1.Policy) []ConflictResult {
	var results []ConflictResult

	for i := 0; i < len(policies); i++ {
		for j := i + 1; j < len(policies); j++ {
			p1 := &policies[i]
			p2 := &policies[j]

			// Only compare policies with the same priority.
			if getPriority(p1) != getPriority(p2) {
				continue
			}

			// Only look at opposite effects.
			if p1.Spec.Effect == p2.Spec.Effect {
				continue
			}

			subjectOverlap := computeSubjectOverlap(p1.Spec.Subject, p2.Spec.Subject)
			resourceOverlap := computeResourceOverlap(p1.Spec.Action.Resources, p2.Spec.Action.Resources)

			if subjectOverlap == overlapNone || resourceOverlap == overlapNone {
				continue
			}

			nameA := p1.Name
			if nameA == "" {
				nameA = fmt.Sprintf("policy[%d]", i)
			}
			nameB := p2.Name
			if nameB == "" {
				nameB = fmt.Sprintf("policy[%d]", j)
			}

			if subjectOverlap == overlapFull && resourceOverlap == overlapFull {
				results = append(results, ConflictResult{
					Type:    Hard,
					PolicyA: nameA,
					PolicyB: nameB,
					Reason: fmt.Sprintf(
						"policies have same priority (%d), opposite effect (%s vs %s), and completely overlapping subject and resource sets",
						getPriority(p1), p1.Spec.Effect, p2.Spec.Effect,
					),
				})
			} else {
				results = append(results, ConflictResult{
					Type:    Soft,
					PolicyA: nameA,
					PolicyB: nameB,
					Reason: fmt.Sprintf(
						"policies have same priority (%d), opposite effect (%s vs %s), with partially overlapping subject/resource sets",
						getPriority(p1), p1.Spec.Effect, p2.Spec.Effect,
					),
				})
			}
		}
	}

	return results
}

// HasHardConflict returns true if any hard conflict exists in the result set.
func HasHardConflict(results []ConflictResult) bool {
	for _, r := range results {
		if r.Type == Hard {
			return true
		}
	}
	return false
}

// getPriority extracts the priority value, defaulting to 0 if nil.
func getPriority(p *policyv1alpha1.Policy) int32 {
	if p.Spec.Priority != nil {
		return *p.Spec.Priority
	}
	return 0
}

// overlapKind represents the degree of overlap between two sets.
type overlapKind int

const (
	overlapNone    overlapKind = iota // disjoint
	overlapPartial                    // non-empty intersection but not full equivalence
	overlapFull                       // sets are equivalent
)

// computeSubjectOverlap determines the overlap between two SubjectSelectors.
func computeSubjectOverlap(s1, s2 policyv1alpha1.SubjectSelector) overlapKind {
	set1 := normalizeSubjectEntries(s1.AnyOf)
	set2 := normalizeSubjectEntries(s2.AnyOf)

	if setsEqual(set1, set2) {
		return overlapFull
	}
	if setsIntersect(set1, set2) {
		return overlapPartial
	}
	return overlapNone
}

// computeResourceOverlap determines the overlap between two PolicyActionResources.
func computeResourceOverlap(r1, r2 policyv1alpha1.PolicyActionResources) overlapKind {
	set1 := normalizeResourceSelectors(r1.AnyOf)
	set2 := normalizeResourceSelectors(r2.AnyOf)

	if setsEqual(set1, set2) {
		return overlapFull
	}
	if setsIntersect(set1, set2) {
		return overlapPartial
	}
	return overlapNone
}

// normalizeSubjectEntries converts subject entries to canonical string keys for comparison.
func normalizeSubjectEntries(entries []policyv1alpha1.SubjectEntry) []string {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, normalizeSubjectEntry(e))
	}
	sort.Strings(keys)
	return keys
}

// normalizeSubjectEntry produces a canonical string for a single SubjectEntry.
func normalizeSubjectEntry(e policyv1alpha1.SubjectEntry) string {
	var sb strings.Builder
	sb.WriteString(e.Kind)
	if e.Match != nil {
		sb.WriteString("|")
		sb.WriteString("name=")
		sb.WriteString(e.Match.Name)
		sb.WriteString(",ns=")
		sb.WriteString(e.Match.Namespace)
		sb.WriteString(",labels=")
		sb.WriteString(normalizeLabels(e.Match.Labels))
	}
	return sb.String()
}

// normalizeResourceSelectors converts resource selectors to canonical string keys.
func normalizeResourceSelectors(selectors []policyv1alpha1.ResourceSelector) []string {
	keys := make([]string, 0, len(selectors))
	for _, s := range selectors {
		keys = append(keys, normalizeResourceSelector(s))
	}
	sort.Strings(keys)
	return keys
}

// normalizeResourceSelector produces a canonical string for a single ResourceSelector.
func normalizeResourceSelector(s policyv1alpha1.ResourceSelector) string {
	var sb strings.Builder
	sb.WriteString(s.Kind)
	if s.Match != nil {
		sb.WriteString("|")
		sb.WriteString("name=")
		sb.WriteString(s.Match.Name)
		sb.WriteString(",ns=")
		sb.WriteString(s.Match.Namespace)
		sb.WriteString(",labels=")
		sb.WriteString(normalizeLabels(s.Match.Labels))
		sb.WriteString(",classification=")
		sb.WriteString(s.Match.Classification)
	}
	return sb.String()
}

// normalizeLabels produces a canonical string for a label map.
func normalizeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ";")
}

// setsEqual checks if two sorted string slices are equal.
func setsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// setsIntersect checks if two sorted string slices share any element.
func setsIntersect(a, b []string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			return true
		}
	}
	return false
}
