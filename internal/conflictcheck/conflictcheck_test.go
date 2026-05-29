package conflictcheck

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

func makePolicy(name, effect string, priority int32, subjects []policyv1alpha1.SubjectEntry, resources []policyv1alpha1.ResourceSelector) policyv1alpha1.Policy {
	return policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: policyv1alpha1.PolicySpec{
			Effect:   effect,
			Priority: ptr.To(priority),
			Subject:  policyv1alpha1.SubjectSelector{AnyOf: subjects},
			Action: policyv1alpha1.PolicyAction{
				Verbs:     []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{AnyOf: resources},
			},
		},
	}
}

func TestDetectConflicts_HardConflict(t *testing.T) {
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "contract-review"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-alice", "allow", 100, subjects, resources),
		makePolicy("deny-alice", "deny", 100, subjects, resources),
	}

	results := DetectConflicts(policies)
	if len(results) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(results))
	}
	if results[0].Type != Hard {
		t.Errorf("expected Hard conflict, got %s", results[0].Type)
	}
	if results[0].PolicyA != "allow-alice" || results[0].PolicyB != "deny-alice" {
		t.Errorf("unexpected policy names: %s, %s", results[0].PolicyA, results[0].PolicyB)
	}
	if !HasHardConflict(results) {
		t.Error("HasHardConflict should return true")
	}
}

func TestDetectConflicts_SoftConflict(t *testing.T) {
	// Partially overlapping subjects: both have User/alice, but p2 also has User/bob
	subjects1 := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	subjects2 := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "bob"}},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "contract-review"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-alice", "allow", 50, subjects1, resources),
		makePolicy("deny-users", "deny", 50, subjects2, resources),
	}

	results := DetectConflicts(policies)
	if len(results) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(results))
	}
	if results[0].Type != Soft {
		t.Errorf("expected Soft conflict, got %s", results[0].Type)
	}
	if HasHardConflict(results) {
		t.Error("HasHardConflict should return false for soft-only conflicts")
	}
}

func TestDetectConflicts_NoConflict_DifferentPriority(t *testing.T) {
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "contract-review"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-alice", "allow", 100, subjects, resources),
		makePolicy("deny-alice", "deny", 50, subjects, resources),
	}

	results := DetectConflicts(policies)
	if len(results) != 0 {
		t.Fatalf("expected 0 conflicts for different priorities, got %d", len(results))
	}
}

func TestDetectConflicts_NoConflict_SameEffect(t *testing.T) {
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "contract-review"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-1", "allow", 100, subjects, resources),
		makePolicy("allow-2", "allow", 100, subjects, resources),
	}

	results := DetectConflicts(policies)
	if len(results) != 0 {
		t.Fatalf("expected 0 conflicts for same effect, got %d", len(results))
	}
}

func TestDetectConflicts_NoConflict_DisjointSubjects(t *testing.T) {
	subjects1 := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	subjects2 := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "bob"}},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "contract-review"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-alice", "allow", 100, subjects1, resources),
		makePolicy("deny-bob", "deny", 100, subjects2, resources),
	}

	results := DetectConflicts(policies)
	if len(results) != 0 {
		t.Fatalf("expected 0 conflicts for disjoint subjects, got %d", len(results))
	}
}

func TestDetectConflicts_NoConflict_DisjointResources(t *testing.T) {
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	resources1 := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-a"}},
	}
	resources2 := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-b"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-a", "allow", 100, subjects, resources1),
		makePolicy("deny-b", "deny", 100, subjects, resources2),
	}

	results := DetectConflicts(policies)
	if len(results) != 0 {
		t.Fatalf("expected 0 conflicts for disjoint resources, got %d", len(results))
	}
}

func TestDetectConflicts_SoftConflict_PartialResourceOverlap(t *testing.T) {
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	resources1 := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-a"}},
	}
	resources2 := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-a"}},
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-b"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-a", "allow", 100, subjects, resources1),
		makePolicy("deny-ab", "deny", 100, subjects, resources2),
	}

	results := DetectConflicts(policies)
	if len(results) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(results))
	}
	if results[0].Type != Soft {
		t.Errorf("expected Soft conflict for partial resource overlap, got %s", results[0].Type)
	}
}

func TestDetectConflicts_HardConflict_WithLabels(t *testing.T) {
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{
			Labels: map[string]string{"team": "legal", "role": "analyst"},
		}},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "KnowledgeBase", Match: &policyv1alpha1.ResourceMatch{
			Labels: map[string]string{"dept": "legal"},
		}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-legal", "allow", 200, subjects, resources),
		makePolicy("deny-legal", "deny", 200, subjects, resources),
	}

	results := DetectConflicts(policies)
	if len(results) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(results))
	}
	if results[0].Type != Hard {
		t.Errorf("expected Hard conflict with labels, got %s", results[0].Type)
	}
}

func TestDetectConflicts_DefaultPriority(t *testing.T) {
	// Both policies have nil priority (defaults to 0).
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-a"}},
	}

	p1 := policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-default"},
		Spec: policyv1alpha1.PolicySpec{
			Effect:  "allow",
			Subject: policyv1alpha1.SubjectSelector{AnyOf: subjects},
			Action: policyv1alpha1.PolicyAction{
				Verbs:     []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{AnyOf: resources},
			},
		},
	}
	p2 := policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-default"},
		Spec: policyv1alpha1.PolicySpec{
			Effect:  "deny",
			Subject: policyv1alpha1.SubjectSelector{AnyOf: subjects},
			Action: policyv1alpha1.PolicyAction{
				Verbs:     []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{AnyOf: resources},
			},
		},
	}

	results := DetectConflicts([]policyv1alpha1.Policy{p1, p2})
	if len(results) != 1 {
		t.Fatalf("expected 1 conflict with default priority, got %d", len(results))
	}
	if results[0].Type != Hard {
		t.Errorf("expected Hard conflict, got %s", results[0].Type)
	}
}

func TestDetectConflicts_MultiplePolicies(t *testing.T) {
	// 3 policies: A(allow,p100), B(deny,p100,same), C(deny,p100,different resource)
	// Expect: A-B hard conflict, A-C no conflict (disjoint resource), B-C same effect
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "User", Match: &policyv1alpha1.SubjectMatch{Name: "alice"}},
	}
	resources1 := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-a"}},
	}
	resources2 := []policyv1alpha1.ResourceSelector{
		{Kind: "Skill", Match: &policyv1alpha1.ResourceMatch{Name: "skill-b"}},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("A", "allow", 100, subjects, resources1),
		makePolicy("B", "deny", 100, subjects, resources1),
		makePolicy("C", "deny", 100, subjects, resources2),
	}

	results := DetectConflicts(policies)
	if len(results) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(results))
	}
	if results[0].Type != Hard {
		t.Errorf("expected Hard, got %s", results[0].Type)
	}
	if results[0].PolicyA != "A" || results[0].PolicyB != "B" {
		t.Errorf("expected A-B conflict, got %s-%s", results[0].PolicyA, results[0].PolicyB)
	}
}

func TestDetectConflicts_EmptyPolicies(t *testing.T) {
	results := DetectConflicts(nil)
	if len(results) != 0 {
		t.Errorf("expected 0 conflicts for nil input, got %d", len(results))
	}

	results = DetectConflicts([]policyv1alpha1.Policy{})
	if len(results) != 0 {
		t.Errorf("expected 0 conflicts for empty input, got %d", len(results))
	}
}

func TestDetectConflicts_SubjectKindOnly(t *testing.T) {
	// Match on kind only (no match details) — equivalent subjects
	subjects := []policyv1alpha1.SubjectEntry{
		{Kind: "Anonymous"},
	}
	resources := []policyv1alpha1.ResourceSelector{
		{Kind: "Any"},
	}

	policies := []policyv1alpha1.Policy{
		makePolicy("allow-anon", "allow", 10, subjects, resources),
		makePolicy("deny-anon", "deny", 10, subjects, resources),
	}

	results := DetectConflicts(policies)
	if len(results) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(results))
	}
	if results[0].Type != Hard {
		t.Errorf("expected Hard conflict, got %s", results[0].Type)
	}
}
