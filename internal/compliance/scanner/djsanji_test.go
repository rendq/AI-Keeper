package scanner

import "testing"

func compliantResource() K8sResource {
	return K8sResource{
		Kind:      "ServiceAccount",
		Name:      "my-sa",
		Namespace: "default",
		Labels: map[string]string{
			"ai-keeper.io/sm3-hash":       "abc123",
			"ai-keeper.io/network-policy": "enforced",
			"rbac.ai-keeper.io/role":      "viewer",
		},
		Annotations: map[string]string{
			"ai-keeper.io/token-expiry":   "3600",
			"ai-keeper.io/audit-logging":  "enabled",
			"ai-keeper.io/encryption":     "SM4",
		},
	}
}

func TestDJSanjiScanner_AllCompliant(t *testing.T) {
	s := NewDJSanjiScanner()
	results := s.Scan([]K8sResource{compliantResource()})

	for _, r := range results {
		if !r.Compliant {
			t.Errorf("expected all compliant, got non-compliant for rule %s: %s", r.RuleID, r.Details)
		}
	}
}

func TestDJSanjiScanner_Violations(t *testing.T) {
	s := NewDJSanjiScanner()

	// ServiceAccount without token-expiry annotation
	sa := K8sResource{
		Kind:        "ServiceAccount",
		Name:        "bad-sa",
		Namespace:   "default",
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	// ClusterRoleBinding with cluster-admin
	crb := K8sResource{
		Kind:      "ClusterRoleBinding",
		Name:      "admin-binding",
		Namespace: "",
		Labels: map[string]string{
			"rbac.ai-keeper.io/role": "cluster-admin",
		},
		Annotations: map[string]string{},
	}

	results := s.Scan([]K8sResource{sa, crb})

	var violations int
	for _, r := range results {
		if !r.Compliant {
			violations++
		}
	}
	if violations == 0 {
		t.Error("expected violations but found none")
	}
}

func TestDJSanjiScanner_EmptyResources(t *testing.T) {
	s := NewDJSanjiScanner()
	results := s.Scan([]K8sResource{})

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestDJSanjiScanner_DefaultRules(t *testing.T) {
	s := NewDJSanjiScanner()
	rules := s.GetRules()

	if len(rules) != 6 {
		t.Fatalf("expected 6 default rules, got %d", len(rules))
	}

	categories := map[ControlCategory]bool{}
	for _, r := range rules {
		categories[r.Category] = true
	}

	expected := []ControlCategory{
		IdentityAuth, AccessControl, SecurityAudit,
		IntrusionPrevention, DataIntegrity, DataConfidentiality,
	}
	for _, c := range expected {
		if !categories[c] {
			t.Errorf("missing category %s in default rules", c)
		}
	}
}
