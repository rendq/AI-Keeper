package compiler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func makePolicy(name, ns, effect string, priority int32) policyv1alpha1.Policy {
	return policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: policyv1alpha1.PolicySpec{
			Effect:   effect,
			Priority: &priority,
			Subject: policyv1alpha1.SubjectSelector{
				AnyOf: []policyv1alpha1.SubjectEntry{
					{Kind: "User"},
				},
			},
			Action: policyv1alpha1.PolicyAction{
				Verbs: []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{
					AnyOf: []policyv1alpha1.ResourceSelector{
						{Kind: "Skill"},
					},
				},
			},
		},
	}
}

func TestCompile_BasicBundle(t *testing.T) {
	c := New()
	input := CompileInput{
		Policies: []policyv1alpha1.Policy{
			makePolicy("allow-all", "default", "allow", 100),
			makePolicy("deny-admin", "default", "deny", 200),
		},
		Subjects: []SubjectCacheEntry{
			{Kind: "User", Name: "alice", Labels: map[string]string{"team": "research"}},
		},
		Resources: []ResourceIndexEntry{
			{Kind: "Skill", Name: "contract-review", Namespace: "default"},
		},
	}

	bundle, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// Verify bundle fields
	if bundle.Data == nil || len(bundle.Data) == 0 {
		t.Fatal("bundle data is empty")
	}
	if !strings.HasPrefix(bundle.Hash, "sha256:") {
		t.Fatalf("hash should start with sha256:, got %s", bundle.Hash)
	}
	if bundle.Version != 1 {
		t.Fatalf("expected version 1, got %d", bundle.Version)
	}

	// Extract and verify tar.gz contents
	files := extractBundle(t, bundle.Data)

	// Must contain main.rego, policies.rego, data.json, .manifest
	requiredFiles := []string{"aip/main.rego", "aip/policies.rego", "data.json", ".manifest"}
	for _, rf := range requiredFiles {
		if _, ok := files[rf]; !ok {
			t.Errorf("missing required file in bundle: %s", rf)
		}
	}

	// Verify manifest
	var manifest manifestJSON
	if err := json.Unmarshal(files[".manifest"], &manifest); err != nil {
		t.Fatalf("invalid manifest: %v", err)
	}
	if manifest.Revision != "1" {
		t.Errorf("manifest revision should be 1, got %s", manifest.Revision)
	}
	if len(manifest.Roots) == 0 || manifest.Roots[0] != "aip" {
		t.Errorf("manifest roots should be [aip], got %v", manifest.Roots)
	}

	// Verify data.json
	var data dataJSONRoot
	if err := json.Unmarshal(files["data.json"], &data); err != nil {
		t.Fatalf("invalid data.json: %v", err)
	}
	if data.AIP.Metadata.PolicyCount != 2 {
		t.Errorf("expected 2 policies in metadata, got %d", data.AIP.Metadata.PolicyCount)
	}

	// Verify subjects and resources in context
	if len(data.AIP.Context.Subjects) != 1 {
		t.Errorf("expected 1 subject, got %d", len(data.AIP.Context.Subjects))
	}
	if len(data.AIP.Context.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(data.AIP.Context.Resources))
	}
}

func TestCompile_MonotonicVersion(t *testing.T) {
	c := New()
	input := CompileInput{
		Policies: []policyv1alpha1.Policy{
			makePolicy("p1", "ns", "allow", 100),
		},
	}

	b1, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}

	b2, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}

	if b2.Version <= b1.Version {
		t.Errorf("version should be monotonically increasing: v1=%d, v2=%d", b1.Version, b2.Version)
	}
}

func TestCompile_NoPolicies(t *testing.T) {
	c := New()
	_, err := c.Compile(context.Background(), CompileInput{})
	if err == nil {
		t.Fatal("expected error for empty policies")
	}
}

func TestCompile_AllDisabled(t *testing.T) {
	c := New()
	p := makePolicy("p1", "ns", "allow", 100)
	p.Spec.Enabled = ptr(false)

	_, err := c.Compile(context.Background(), CompileInput{Policies: []policyv1alpha1.Policy{p}})
	if err == nil {
		t.Fatal("expected error when all policies are disabled")
	}
}

func TestCompile_DecisionAlgorithm_HigherPriorityWins(t *testing.T) {
	// Higher priority deny at 200, lower priority allow at 100.
	// The rego should place deny at higher priority.
	c := New()
	input := CompileInput{
		Policies: []policyv1alpha1.Policy{
			makePolicy("low-allow", "ns", "allow", 100),
			makePolicy("high-deny", "ns", "deny", 200),
		},
	}

	bundle, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	files := extractBundle(t, bundle.Data)
	policiesRego := string(files["aip/policies.rego"])

	// Verify that the deny policy has priority 200
	if !strings.Contains(policiesRego, `"priority": 200`) {
		t.Error("deny policy should have priority 200 in rego")
	}
	// Verify that the allow policy has priority 100
	if !strings.Contains(policiesRego, `"priority": 100`) {
		t.Error("allow policy should have priority 100 in rego")
	}
}

func TestCompile_DecisionAlgorithm_SamePriorityDenyWins(t *testing.T) {
	// Both at priority 100: deny should appear in deny_set, allow in allow_set.
	// The main.rego decision algorithm handles "deny wins at same priority".
	c := New()
	input := CompileInput{
		Policies: []policyv1alpha1.Policy{
			makePolicy("p-allow", "ns", "allow", 100),
			makePolicy("p-deny", "ns", "deny", 100),
		},
	}

	bundle, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	files := extractBundle(t, bundle.Data)
	mainRego := string(files["aip/main.rego"])

	// Main rego should implement "deny at or above priority" logic
	if !strings.Contains(mainRego, "denied_at_or_above") {
		t.Error("main.rego should contain denied_at_or_above rule")
	}

	// Verify data.json has metadata
	var data dataJSONRoot
	if err := json.Unmarshal(files["data.json"], &data); err != nil {
		t.Fatalf("invalid data.json: %v", err)
	}
	if data.AIP.Metadata.PolicyCount != 2 {
		t.Errorf("expected 2 policies in metadata, got %d", data.AIP.Metadata.PolicyCount)
	}

	// Verify the rego has both aip_allow and aip_deny rules
	policiesRego := string(files["aip/policies.rego"])
	if !strings.Contains(policiesRego, "aip_deny contains") {
		t.Error("policies.rego should contain aip_deny rule")
	}
	if !strings.Contains(policiesRego, "aip_allow contains") {
		t.Error("policies.rego should contain aip_allow rule")
	}
}

func TestCompile_HashDeterministic(t *testing.T) {
	c := New()
	input := CompileInput{
		Policies: []policyv1alpha1.Policy{
			makePolicy("p1", "ns", "allow", 100),
		},
	}

	b1, _ := c.Compile(context.Background(), input)
	// Reset version for deterministic comparison
	c2 := NewWithVersion(0)
	b2, _ := c2.Compile(context.Background(), input)

	// Same input should produce same hash (same version, same content)
	if b1.Hash != b2.Hash {
		t.Errorf("same input should produce same hash: %s vs %s", b1.Hash, b2.Hash)
	}
}

func TestCompile_WithObligations(t *testing.T) {
	c := New()
	p := makePolicy("p1", "ns", "allow", 100)
	p.Spec.Obligations = &policyv1alpha1.PolicyObligations{
		Audit: &policyv1alpha1.ObligationAudit{Level: "high"},
		Watermark: &policyv1alpha1.ObligationWatermark{
			Enabled: ptr(true),
			Mode:    "visible",
		},
	}

	input := CompileInput{Policies: []policyv1alpha1.Policy{p}}
	bundle, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	files := extractBundle(t, bundle.Data)
	policiesRego := string(files["aip/policies.rego"])

	if !strings.Contains(policiesRego, `"audit"`) {
		t.Error("obligations should contain audit")
	}
	if !strings.Contains(policiesRego, `"watermark"`) {
		t.Error("obligations should contain watermark")
	}
}

func TestCompile_DefaultPriority(t *testing.T) {
	c := New()
	p := policyv1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "no-priority", Namespace: "ns"},
		Spec: policyv1alpha1.PolicySpec{
			Effect: "allow",
			// Priority is nil, should default to 500
			Subject: policyv1alpha1.SubjectSelector{
				AnyOf: []policyv1alpha1.SubjectEntry{{Kind: "User"}},
			},
			Action: policyv1alpha1.PolicyAction{
				Verbs: []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{
					AnyOf: []policyv1alpha1.ResourceSelector{{Kind: "Skill"}},
				},
			},
		},
	}

	input := CompileInput{Policies: []policyv1alpha1.Policy{p}}
	bundle, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	files := extractBundle(t, bundle.Data)
	policiesRego := string(files["aip/policies.rego"])
	if !strings.Contains(policiesRego, "priority=500") {
		t.Error("default priority should be 500")
	}
}

func TestSortPolicies(t *testing.T) {
	policies := []policyv1alpha1.Policy{
		makePolicy("low-allow", "ns", "allow", 100),
		makePolicy("high-allow", "ns", "allow", 200),
		makePolicy("same-deny", "ns", "deny", 200),
		makePolicy("mid-deny", "ns", "deny", 150),
	}

	sortPolicies(policies)

	// Expected order: high-deny(200) > high-allow(200) > mid-deny(150) > low-allow(100)
	expected := []struct {
		name   string
		effect string
		prio   int32
	}{
		{"same-deny", "deny", 200},
		{"high-allow", "allow", 200},
		{"mid-deny", "deny", 150},
		{"low-allow", "allow", 100},
	}

	for i, e := range expected {
		if policies[i].Name != e.name {
			t.Errorf("position %d: expected %s, got %s", i, e.name, policies[i].Name)
		}
	}
}

func TestNewWithVersion(t *testing.T) {
	c := NewWithVersion(10)
	input := CompileInput{
		Policies: []policyv1alpha1.Policy{
			makePolicy("p1", "ns", "allow", 100),
		},
	}

	bundle, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if bundle.Version != 11 {
		t.Errorf("expected version 11 (10+1), got %d", bundle.Version)
	}
}

// extractBundle unpacks a tar.gz bundle and returns file name → content map.
func extractBundle(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	files := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("reading %s: %v", hdr.Name, err)
		}
		files[hdr.Name] = content
	}
	return files
}
