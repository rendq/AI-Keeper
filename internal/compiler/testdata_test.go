package compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestGenerateTestdataBundle generates a bundle in testdata/bundle/ for use with
// `opa eval -b testdata/bundle data.aip.allow`. This test always regenerates
// the bundle so it stays in sync with the compiler output.
func TestGenerateTestdataBundle(t *testing.T) {
	c := New()

	policies := []policyv1alpha1.Policy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "allow-research", Namespace: "default"},
			Spec: policyv1alpha1.PolicySpec{
				Effect:   "allow",
				Priority: ptr(int32(100)),
				Subject: policyv1alpha1.SubjectSelector{
					AnyOf: []policyv1alpha1.SubjectEntry{
						{Kind: "User", Match: &policyv1alpha1.SubjectMatch{
							Labels: map[string]string{"team": "research"},
						}},
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
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "deny-external", Namespace: "default"},
			Spec: policyv1alpha1.PolicySpec{
				Effect:   "deny",
				Priority: ptr(int32(200)),
				Subject: policyv1alpha1.SubjectSelector{
					AnyOf: []policyv1alpha1.SubjectEntry{
						{Kind: "Anonymous"},
					},
				},
				Action: policyv1alpha1.PolicyAction{
					Verbs: []string{"invoke", "read"},
					Resources: policyv1alpha1.PolicyActionResources{
						AnyOf: []policyv1alpha1.ResourceSelector{
							{Kind: "Any"},
						},
					},
				},
			},
		},
	}

	input := CompileInput{
		Policies: policies,
		Subjects: []SubjectCacheEntry{
			{Kind: "User", Name: "alice", Labels: map[string]string{"team": "research"}},
			{Kind: "Anonymous", Name: "guest"},
		},
		Resources: []ResourceIndexEntry{
			{Kind: "Skill", Name: "contract-review", Namespace: "default", Classification: "internal"},
		},
	}

	bundle, err := c.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// Write bundle tar.gz to testdata/bundle.tar.gz
	bundleDir := filepath.Join("testdata", "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Extract bundle files to the directory (OPA can load from directory)
	files := extractBundle(t, bundle.Data)
	for name, content := range files {
		path := filepath.Join(bundleDir, name)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// Also write the tar.gz for reference
	tarGzPath := filepath.Join("testdata", "bundle.tar.gz")
	if err := os.WriteFile(tarGzPath, bundle.Data, 0644); err != nil {
		t.Fatalf("write bundle.tar.gz: %v", err)
	}

	t.Logf("Generated bundle v%d hash=%s (%d bytes)", bundle.Version, bundle.Hash, len(bundle.Data))
	t.Logf("Bundle extracted to %s", bundleDir)
}
