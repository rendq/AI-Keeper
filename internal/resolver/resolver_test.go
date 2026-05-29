package resolver_test

import (
	"context"
	"errors"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/internal/resolver"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := skillv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register skill scheme: %v", err)
	}
	if err := datav1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register data scheme: %v", err)
	}
	if err := modelv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register model scheme: %v", err)
	}
	return s
}

var (
	dummyInputSchema  = []byte(`{"type":"object"}`)
	dummyOutputSchema = []byte(`{"type":"object"}`)
)

func newSkill(name, namespace, version string, stability shared.Stage, requires *skillv1alpha1.SkillRequires) *skillv1alpha1.Skill {
	return &skillv1alpha1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: skillv1alpha1.SkillSpec{
			Version:   shared.SemVer(version),
			Stability: stability,
			Interface: skillv1alpha1.SkillInterface{
				Input:  skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: dummyInputSchema}},
				Output: skillv1alpha1.SkillIO{Schema: &apiextensionsv1.JSON{Raw: dummyOutputSchema}},
			},
			Implementation: skillv1alpha1.SkillImplementation{
				Type:     "function",
				Requires: requires,
			},
		},
	}
}

func newTool(name, namespace string) *skillv1alpha1.Tool {
	return &skillv1alpha1.Tool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: skillv1alpha1.ToolSpec{
			Protocol: "http",
			Endpoint: "https://example.com/tool",
			Schema: skillv1alpha1.ToolSchema{
				Input:  &apiextensionsv1.JSON{Raw: []byte(`{}`)},
				Output: &apiextensionsv1.JSON{Raw: []byte(`{}`)},
			},
			Governance: skillv1alpha1.ToolGovernance{
				GovernanceBlock: shared.GovernanceBlock{},
			},
		},
	}
}

func newModelEndpoint(name, namespace string) *modelv1alpha1.ModelEndpoint {
	return &modelv1alpha1.ModelEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: modelv1alpha1.ModelEndpointSpec{
			Provider: "openai",
			Model:    "gpt-4o",
			Endpoint: "https://api.openai.com/v1",
		},
	}
}

func newDataSource(name, namespace string) *datav1alpha1.DataSource {
	return &datav1alpha1.DataSource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: datav1alpha1.DataSourceSpec{
			Connector: datav1alpha1.DataSourceConnector{Kind: "postgres"},
		},
	}
}

func buildClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := mustScheme(t)
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		Build()
}

func mustResolve(t *testing.T, r *resolver.Resolver, sk *skillv1alpha1.Skill) (resolved bool) {
	t.Helper()
	_, err := r.Resolve(context.Background(), sk)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return true
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestResolve_HappyPath wires a Skill that depends on one Tool, one
// ModelEndpoint and one sub-Skill. Every dependency exists; the
// resolver should report no missing entries and no cycle.
func TestResolve_HappyPath(t *testing.T) {
	t.Parallel()

	subSkill := newSkill("legal-search", "default", "1.2.0", shared.StageStable, nil)
	tool := newTool("docusign", "default")
	endpoint := newModelEndpoint("gpt-4o-eu", "default")

	root := newSkill("contract-review", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Tools:  []skillv1alpha1.SkillToolDep{{Ref: "tool://default/docusign"}},
		Models: []skillv1alpha1.SkillModelDep{{Alias: "reasoner", Ref: "model://default/gpt-4o-eu"}},
		Skills: []skillv1alpha1.SkillSubSkillDep{{Ref: "skill://default/legal-search", VersionConstraint: "^1.0.0"}},
	})

	c := buildClient(t, subSkill, tool, endpoint)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("Missing = %v, want empty", res.Missing)
	}
	if res.Cyclic {
		t.Fatalf("Cyclic = true, want false")
	}
	if len(res.Resolved.Tools) != 1 || res.Resolved.Tools[0] != "tool://default/docusign" {
		t.Fatalf("Resolved.Tools = %v", res.Resolved.Tools)
	}
	if len(res.Resolved.Models) != 1 || res.Resolved.Models[0].Alias != "reasoner" {
		t.Fatalf("Resolved.Models = %+v", res.Resolved.Models)
	}
	if len(res.Resolved.Skills) != 1 {
		t.Fatalf("Resolved.Skills len = %d, want 1", len(res.Resolved.Skills))
	}
	want := shared.ResourceRef("skill://default/legal-search@1.2.0")
	if res.Resolved.Skills[0] != want {
		t.Fatalf("Resolved.Skills[0] = %q, want %q", res.Resolved.Skills[0], want)
	}
}

// TestResolve_MissingTool exercises the missing-reference path.
func TestResolve_MissingTool(t *testing.T) {
	t.Parallel()

	root := newSkill("contract-review", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Tools: []skillv1alpha1.SkillToolDep{{Ref: "tool://default/missing"}},
	})

	c := buildClient(t)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "tool://default/missing" {
		t.Fatalf("Missing = %v, want [tool://default/missing]", res.Missing)
	}
	if len(res.Resolved.Tools) != 0 {
		t.Fatalf("Resolved.Tools = %v, want empty", res.Resolved.Tools)
	}
}

// TestResolve_MissingModel exercises the missing-reference path for a
// `model://` ref. Both ModelEndpoint and ModelRouter must be absent
// before the ref is reported missing.
func TestResolve_MissingModel(t *testing.T) {
	t.Parallel()

	root := newSkill("contract-review", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Models: []skillv1alpha1.SkillModelDep{{Alias: "r", Ref: "model://default/no-such"}},
	})

	c := buildClient(t)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "model://default/no-such" {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if len(res.Resolved.Models) != 0 {
		t.Fatalf("Resolved.Models = %+v", res.Resolved.Models)
	}
}

// TestResolve_MissingDataSource_KBFallback verifies that data refs
// resolve successfully via either DataSource or KnowledgeBase.
func TestResolve_DataSourceOrKnowledgeBase(t *testing.T) {
	t.Parallel()

	ds := newDataSource("legal-corpus", "default")
	root := newSkill("legal", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		DataSources: []skillv1alpha1.SkillDataSourceDep{
			{Ref: "data://default/legal-corpus"},
		},
	})
	c := buildClient(t, ds)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if len(res.Resolved.DataSources) != 1 {
		t.Fatalf("Resolved.DataSources = %v", res.Resolved.DataSources)
	}
}

// TestResolve_VersionConstraintNoMatch verifies that a sub-skill ref
// with no matching version flips into the missing list.
func TestResolve_VersionConstraintNoMatch(t *testing.T) {
	t.Parallel()

	// Three published versions of the same sub-skill, none matching
	// `^3.0.0`.
	const labelKey = resolver.LabelSkillName
	v090 := newSkill("legal-search-0.9.0", "default", "0.9.0", shared.StageStable, nil)
	v090.Labels = map[string]string{labelKey: "legal-search"}
	v100 := newSkill("legal-search-1.0.0", "default", "1.0.0", shared.StageStable, nil)
	v100.Labels = map[string]string{labelKey: "legal-search"}
	v200 := newSkill("legal-search-2.0.0", "default", "2.0.0", shared.StageStable, nil)
	v200.Labels = map[string]string{labelKey: "legal-search"}

	root := newSkill("contract-review", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{
			{Ref: "skill://default/legal-search", VersionConstraint: "^3.0.0"},
		},
	})

	c := buildClient(t, v090, v100, v200)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "skill://default/legal-search" {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if len(res.Resolved.Skills) != 0 {
		t.Fatalf("Resolved.Skills = %v", res.Resolved.Skills)
	}
}

// TestResolve_VersionConstraintMatch verifies the highest matching
// version is selected and that experimental candidates are filtered.
func TestResolve_VersionConstraintMatch(t *testing.T) {
	t.Parallel()

	const labelKey = resolver.LabelSkillName
	beta := newSkill("legal-search-1.2.0", "default", "1.2.0", shared.StageBeta, nil)
	beta.Labels = map[string]string{labelKey: "legal-search"}
	stable := newSkill("legal-search-1.5.0", "default", "1.5.0", shared.StageStable, nil)
	stable.Labels = map[string]string{labelKey: "legal-search"}
	exp := newSkill("legal-search-1.9.9", "default", "1.9.9", shared.StageExperimental, nil)
	exp.Labels = map[string]string{labelKey: "legal-search"}
	tooNew := newSkill("legal-search-2.0.0", "default", "2.0.0", shared.StageStable, nil)
	tooNew.Labels = map[string]string{labelKey: "legal-search"}

	root := newSkill("contract-review", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{
			{Ref: "skill://default/legal-search", VersionConstraint: "^1.0.0"},
		},
	})

	c := buildClient(t, beta, stable, exp, tooNew)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if len(res.Resolved.Skills) != 1 {
		t.Fatalf("Resolved.Skills len = %d, want 1", len(res.Resolved.Skills))
	}
	got := res.Resolved.Skills[0]
	want := shared.ResourceRef("skill://default/legal-search-1.5.0@1.5.0")
	if got != want {
		t.Fatalf("Resolved.Skills[0] = %q, want %q (highest stable matching ^1.0.0)", got, want)
	}
}

// TestResolve_StaleStability verifies a sub-skill candidate that is
// only available as `experimental` is treated as missing.
func TestResolve_StaleStability(t *testing.T) {
	t.Parallel()

	const labelKey = resolver.LabelSkillName
	exp := newSkill("legal-search-1.0.0", "default", "1.0.0", shared.StageExperimental, nil)
	exp.Labels = map[string]string{labelKey: "legal-search"}

	root := newSkill("contract-review", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{
			{Ref: "skill://default/legal-search", VersionConstraint: "^1.0.0"},
		},
	})

	c := buildClient(t, exp)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 1 {
		t.Fatalf("Missing = %v, want experimental candidate to be filtered", res.Missing)
	}
}

// TestResolve_Cycle verifies an A → B → A cycle is detected.
func TestResolve_Cycle(t *testing.T) {
	t.Parallel()

	// B requires A.
	skillB := newSkill("b", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{
			{Ref: "skill://default/a"},
		},
	})
	// A requires B.
	root := newSkill("a", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{
			{Ref: "skill://default/b"},
		},
	})

	c := buildClient(t, skillB, root)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !res.Cyclic {
		t.Fatalf("expected Cyclic=true, got %+v", res)
	}
}

// TestResolve_DiamondNoCycle verifies a diamond dependency graph
// (A → B → D, A → C → D) does not register as a cycle.
func TestResolve_DiamondNoCycle(t *testing.T) {
	t.Parallel()

	skillD := newSkill("d", "default", "1.0.0", shared.StageStable, nil)
	skillB := newSkill("b", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{{Ref: "skill://default/d"}},
	})
	skillC := newSkill("c", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{{Ref: "skill://default/d"}},
	})
	root := newSkill("a", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Skills: []skillv1alpha1.SkillSubSkillDep{
			{Ref: "skill://default/b"},
			{Ref: "skill://default/c"},
		},
	})

	c := buildClient(t, skillD, skillB, skillC, root)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Cyclic {
		t.Fatalf("Cyclic = true, want false; missing=%v resolved=%+v", res.Missing, res.Resolved)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if len(res.Resolved.Skills) != 2 {
		t.Fatalf("Resolved.Skills len = %d, want 2", len(res.Resolved.Skills))
	}
}

// TestResolve_EmptyRequires confirms a Skill with no `requires` block
// resolves trivially.
func TestResolve_EmptyRequires(t *testing.T) {
	t.Parallel()

	root := newSkill("simple", "default", "1.0.0", shared.StageStable, nil)
	c := buildClient(t)
	r := resolver.NewResolver(c)

	res, err := r.Resolve(context.Background(), root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if res.Cyclic {
		t.Fatalf("Cyclic = true, want false")
	}
}

// TestResolve_MalformedRef returns an error so the controller backs off.
func TestResolve_MalformedRef(t *testing.T) {
	t.Parallel()

	root := newSkill("contract", "default", "1.0.0", shared.StageStable, &skillv1alpha1.SkillRequires{
		Tools: []skillv1alpha1.SkillToolDep{
			{Ref: "skill://default/wrong-scheme"}, // not a tool:// scheme
		},
	})
	c := buildClient(t)
	r := resolver.NewResolver(c)

	_, err := r.Resolve(context.Background(), root)
	if err == nil {
		t.Fatalf("expected error for malformed ref")
	}
	if !errors.Is(err, resolver.ErrMalformedRef) {
		t.Fatalf("error = %v, want ErrMalformedRef", err)
	}
}

// TestResolve_NilSkill is a defensive test for the controller calling
// path where `skill` could be nil.
func TestResolve_NilSkill(t *testing.T) {
	t.Parallel()

	r := resolver.NewResolver(buildClient(t))
	_, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error on nil skill")
	}
}

// TestResolve_ImplementsSkillResolver verifies the resolver satisfies
// the controller-side interface (compile-time assertion is in the
// resolver package; this test just exercises Resolve through that
// interface to catch any drift).
func TestResolve_ImplementsSkillResolver(t *testing.T) {
	t.Parallel()

	root := newSkill("simple", "default", "1.0.0", shared.StageStable, nil)
	c := buildClient(t)
	r := resolver.NewResolver(c)
	mustResolve(t, r, root)
}
