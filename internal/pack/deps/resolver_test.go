package deps

import (
	"strings"
	"testing"
)

// mockRegistry implements both Registry and ManifestRegistry for testing.
type mockRegistry struct {
	versions  map[string][]string
	manifests map[string]map[string]*PackManifest // name -> version -> manifest
}

func (r *mockRegistry) GetAvailableVersions(name string) []string {
	return r.versions[name]
}

func (r *mockRegistry) GetManifest(name, version string) *PackManifest {
	if r.manifests == nil {
		return nil
	}
	byVersion, ok := r.manifests[name]
	if !ok {
		return nil
	}
	return byVersion[version]
}

func TestDependencyResolverSimpleChain(t *testing.T) {
	// A -> B -> C (simple chain resolves correctly in topological order)
	reg := &mockRegistry{
		versions: map[string][]string{
			"B": {"1.0.0", "1.1.0"},
			"C": {"2.0.0", "2.1.0"},
		},
		manifests: map[string]map[string]*PackManifest{
			"B": {
				"1.1.0": {
					Name:    "B",
					Version: "1.1.0",
					Requires: []PackDependency{
						{Name: "C", Version: ">=2.0.0"},
					},
				},
			},
			"C": {
				"2.1.0": {
					Name:    "C",
					Version: "2.1.0",
				},
			},
		},
	}

	manifest := PackManifest{
		Name:    "A",
		Version: "1.0.0",
		Requires: []PackDependency{
			{Name: "B", Version: ">=1.0.0"},
		},
	}

	result, err := Resolve(manifest, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Order) != 2 {
		t.Fatalf("expected 2 resolved deps, got %d: %+v", len(result.Order), result.Order)
	}

	// C should come before B (topological order).
	if result.Order[0].Name != "C" || result.Order[1].Name != "B" {
		t.Errorf("expected order [C, B], got [%s, %s]", result.Order[0].Name, result.Order[1].Name)
	}
}

func TestDependencyResolverVersionConstraint(t *testing.T) {
	reg := &mockRegistry{
		versions: map[string][]string{
			"lib": {"0.9.0", "1.0.0", "1.2.0", "2.0.0"},
		},
	}

	manifest := PackManifest{
		Name:    "app",
		Version: "1.0.0",
		Requires: []PackDependency{
			{Name: "lib", Version: "^1.0.0"},
		},
	}

	result, err := Resolve(manifest, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Order) != 1 {
		t.Fatalf("expected 1 resolved dep, got %d", len(result.Order))
	}

	// Should pick highest matching: 1.2.0 (^1.0.0 excludes 2.0.0 and 0.9.0).
	if result.Order[0].Version != "1.2.0" {
		t.Errorf("expected version 1.2.0, got %s", result.Order[0].Version)
	}
}

func TestDependencyResolverCycleDetection(t *testing.T) {
	// A -> B -> C -> B (cycle)
	reg := &mockRegistry{
		versions: map[string][]string{
			"B": {"1.0.0"},
			"C": {"1.0.0"},
		},
		manifests: map[string]map[string]*PackManifest{
			"B": {
				"1.0.0": {
					Name:    "B",
					Version: "1.0.0",
					Requires: []PackDependency{
						{Name: "C", Version: ">=1.0.0"},
					},
				},
			},
			"C": {
				"1.0.0": {
					Name:    "C",
					Version: "1.0.0",
					Requires: []PackDependency{
						{Name: "B", Version: ">=1.0.0"},
					},
				},
			},
		},
	}

	manifest := PackManifest{
		Name:    "A",
		Version: "1.0.0",
		Requires: []PackDependency{
			{Name: "B", Version: ">=1.0.0"},
		},
	}

	_, err := Resolve(manifest, reg)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cyclic dependency") {
		t.Errorf("expected 'cyclic dependency' in error, got: %v", err)
	}
}

func TestDependencyResolverMissingDependency(t *testing.T) {
	reg := &mockRegistry{
		versions: map[string][]string{}, // empty registry
	}

	manifest := PackManifest{
		Name:    "app",
		Version: "1.0.0",
		Requires: []PackDependency{
			{Name: "nonexistent", Version: ">=1.0.0"},
		},
	}

	_, err := Resolve(manifest, reg)
	if err == nil {
		t.Fatal("expected missing dependency error, got nil")
	}
	if !strings.Contains(err.Error(), "missing dependency") {
		t.Errorf("expected 'missing dependency' in error, got: %v", err)
	}
}

func TestDependencyResolverPicksHighestVersion(t *testing.T) {
	reg := &mockRegistry{
		versions: map[string][]string{
			"lib": {"1.0.0", "1.5.0", "1.3.0", "1.9.0", "2.0.0"},
		},
	}

	manifest := PackManifest{
		Name:    "app",
		Version: "1.0.0",
		Requires: []PackDependency{
			{Name: "lib", Version: ">=1.0.0"},
		},
	}

	result, err := Resolve(manifest, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pick highest: 2.0.0.
	if result.Order[0].Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", result.Order[0].Version)
	}
}

func TestDependencyResolverDiamond(t *testing.T) {
	// A -> B, A -> C, B -> D, C -> D (diamond)
	reg := &mockRegistry{
		versions: map[string][]string{
			"B": {"1.0.0"},
			"C": {"1.0.0"},
			"D": {"1.0.0", "1.1.0"},
		},
		manifests: map[string]map[string]*PackManifest{
			"B": {
				"1.0.0": {
					Name:    "B",
					Version: "1.0.0",
					Requires: []PackDependency{
						{Name: "D", Version: ">=1.0.0"},
					},
				},
			},
			"C": {
				"1.0.0": {
					Name:    "C",
					Version: "1.0.0",
					Requires: []PackDependency{
						{Name: "D", Version: ">=1.0.0"},
					},
				},
			},
			"D": {
				"1.1.0": {
					Name:    "D",
					Version: "1.1.0",
				},
			},
		},
	}

	manifest := PackManifest{
		Name:    "A",
		Version: "1.0.0",
		Requires: []PackDependency{
			{Name: "B", Version: ">=1.0.0"},
			{Name: "C", Version: ">=1.0.0"},
		},
	}

	result, err := Resolve(manifest, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should resolve D, B, C (D before B and C since both depend on it).
	if len(result.Order) != 3 {
		t.Fatalf("expected 3 resolved deps, got %d: %+v", len(result.Order), result.Order)
	}

	// D must appear before B and C.
	posD, posB, posC := -1, -1, -1
	for i, dep := range result.Order {
		switch dep.Name {
		case "D":
			posD = i
		case "B":
			posB = i
		case "C":
			posC = i
		}
	}
	if posD >= posB {
		t.Errorf("D (pos %d) should come before B (pos %d)", posD, posB)
	}
	if posD >= posC {
		t.Errorf("D (pos %d) should come before C (pos %d)", posD, posC)
	}

	// D should be resolved only once.
	dCount := 0
	for _, dep := range result.Order {
		if dep.Name == "D" {
			dCount++
		}
	}
	if dCount != 1 {
		t.Errorf("D should appear exactly once, got %d", dCount)
	}
}

func TestSatisfiesConstraint(t *testing.T) {
	tests := []struct {
		version    string
		constraint string
		want       bool
	}{
		{"1.0.0", ">=1.0.0", true},
		{"0.9.0", ">=1.0.0", false},
		{"2.0.0", ">=1.0.0", true},
		{"1.2.3", "^1.0.0", true},
		{"2.0.0", "^1.0.0", false},
		{"0.9.0", "^1.0.0", false},
		{"1.2.5", "~1.2.3", true},
		{"1.2.2", "~1.2.3", false},
		{"1.3.0", "~1.2.3", false},
		{"1.0.0", "=1.0.0", true},
		{"1.0.1", "=1.0.0", false},
		{"1.0.0", ">0.9.0", true},
		{"0.9.0", ">0.9.0", false},
		{"1.0.0", "<2.0.0", true},
		{"2.0.0", "<2.0.0", false},
		{"1.0.0", "<=1.0.0", true},
		{"1.0.1", "<=1.0.0", false},
		{"1.0.0", "*", true},
		{"1.0.0", "", true},
	}

	for _, tt := range tests {
		got := SatisfiesConstraint(tt.version, tt.constraint)
		if got != tt.want {
			t.Errorf("SatisfiesConstraint(%q, %q) = %v, want %v",
				tt.version, tt.constraint, got, tt.want)
		}
	}
}
