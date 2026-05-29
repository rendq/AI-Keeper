package cedar

import "fmt"

// ConflictType classifies the severity of a policy conflict.
type ConflictType string

const (
	// HardConflict means two policies have the exact same scope but opposite effects.
	HardConflict ConflictType = "hard_conflict"
	// SoftConflict means two policies have partially overlapping scope (wildcard vs specific) with opposite effects.
	SoftConflict ConflictType = "soft_conflict"
)

// PolicyConflict describes a detected conflict between two policies.
type PolicyConflict struct {
	PolicyA     string
	PolicyB     string
	Type        ConflictType
	Description string
}

// ConflictDetector performs static analysis on Cedar policy sets to find conflicts.
type ConflictDetector struct{}

// NewConflictDetector creates a new ConflictDetector.
func NewConflictDetector() *ConflictDetector {
	return &ConflictDetector{}
}

// DetectConflicts analyzes Cedar policy text for conflicting rules.
// Two policies with opposite effects (permit vs forbid) sharing the same scope
// produce a hard conflict. Partial overlap (wildcard vs specific) produces a soft conflict.
func (d *ConflictDetector) DetectConflicts(cedarText string) ([]PolicyConflict, error) {
	rules, err := parsePolicies(cedarText)
	if err != nil {
		return nil, fmt.Errorf("parsing policies: %w", err)
	}

	var conflicts []PolicyConflict
	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			if c, ok := checkConflict(rules[i], rules[j]); ok {
				conflicts = append(conflicts, c)
			}
		}
	}
	return conflicts, nil
}

// checkConflict checks whether two rules conflict.
func checkConflict(a, b policyRule) (PolicyConflict, bool) {
	// Same effect → no conflict
	if a.Effect == b.Effect {
		return PolicyConflict{}, false
	}

	principalRel := fieldRelation(a.Principal, b.Principal)
	actionRel := fieldRelation(a.Action, b.Action)
	resourceRel := fieldRelation(a.Resource, b.Resource)

	// If any dimension is disjoint, no overlap → no conflict
	if principalRel == disjoint || actionRel == disjoint || resourceRel == disjoint {
		return PolicyConflict{}, false
	}

	policyADesc := ruleLabel(a)
	policyBDesc := ruleLabel(b)

	// All dimensions are exact match → hard conflict
	if principalRel == exact && actionRel == exact && resourceRel == exact {
		return PolicyConflict{
			PolicyA:     policyADesc,
			PolicyB:     policyBDesc,
			Type:        HardConflict,
			Description: "same scope with opposite effects (permit vs forbid)",
		}, true
	}

	// At least one dimension overlaps partially → soft conflict
	return PolicyConflict{
		PolicyA:     policyADesc,
		PolicyB:     policyBDesc,
		Type:        SoftConflict,
		Description: "partial scope overlap with opposite effects",
	}, true
}

type relation int

const (
	exact    relation = iota // both values are identical
	overlap                  // one is wildcard, other is specific
	disjoint                 // different specific values
)

// fieldRelation determines the relationship between two scope field values.
func fieldRelation(a, b string) relation {
	aWild := a == "" || a == "*"
	bWild := b == "" || b == "*"

	if aWild && bWild {
		return exact
	}
	if aWild || bWild {
		return overlap
	}
	if a == b {
		return exact
	}
	return disjoint
}

// ruleLabel returns a human-readable label for a policy rule.
func ruleLabel(r policyRule) string {
	return fmt.Sprintf("%s(principal=%s, action=%s, resource=%s)", r.Effect, r.Principal, r.Action, r.Resource)
}
