// Package deps implements Pack dependency resolution with version constraints,
// cycle detection, and topological ordering.
package deps

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PackDependency declares a dependency on another pack with a version constraint.
type PackDependency struct {
	Name    string // Pack name, e.g. "base-policies"
	Version string // Semver constraint, e.g. ">=1.0.0", "^2.3.0", "~1.2.0"
}

// PackManifest describes a pack's identity and its requirements.
type PackManifest struct {
	Name     string           // Pack name
	Version  string           // Pack's own version
	Requires []PackDependency // Declared dependencies
}

// Registry provides available versions for packs.
type Registry interface {
	// GetAvailableVersions returns all published versions for a pack name.
	// Returns nil or empty slice if the pack does not exist.
	GetAvailableVersions(name string) []string
}

// ResolvedDep represents a single resolved dependency with its chosen version.
type ResolvedDep struct {
	Name    string
	Version string
}

// ResolvedDeps holds the result of a successful dependency resolution.
type ResolvedDeps struct {
	// Order contains resolved dependencies in topological install order.
	Order []ResolvedDep
}

// Resolve resolves all transitive dependencies for a manifest using the given registry.
// It returns an error if:
//   - A required pack is not found in the registry
//   - No version satisfies the declared constraint
//   - A dependency cycle is detected
func Resolve(manifest PackManifest, registry Registry) (*ResolvedDeps, error) {
	r := &resolver{
		registry: registry,
		resolved: make(map[string]string),
		visiting: make(map[string]bool),
		order:    nil,
	}

	// Resolve each direct dependency.
	for _, dep := range manifest.Requires {
		if err := r.resolve(dep, []string{manifest.Name}); err != nil {
			return nil, err
		}
	}

	return &ResolvedDeps{Order: r.order}, nil
}

type resolver struct {
	registry Registry
	resolved map[string]string // name → resolved version
	visiting map[string]bool   // currently in DFS path (cycle detection)
	order    []ResolvedDep     // topological order
}

func (r *resolver) resolve(dep PackDependency, path []string) error {
	// Already resolved — check constraint compatibility.
	if ver, ok := r.resolved[dep.Name]; ok {
		if !SatisfiesConstraint(ver, dep.Version) {
			return fmt.Errorf("version conflict: %s@%s does not satisfy constraint %q (required by %s)",
				dep.Name, ver, dep.Version, path[len(path)-1])
		}
		return nil
	}

	// Cycle detection.
	if r.visiting[dep.Name] {
		cycle := append(path, dep.Name)
		return fmt.Errorf("cyclic dependency detected: %s", strings.Join(cycle, " -> "))
	}

	// Get available versions.
	versions := r.registry.GetAvailableVersions(dep.Name)
	if len(versions) == 0 {
		return fmt.Errorf("missing dependency: pack %q not found in registry", dep.Name)
	}

	// Find highest version satisfying constraint.
	chosen, err := selectBestVersion(versions, dep.Version)
	if err != nil {
		return fmt.Errorf("no version of %q satisfies constraint %q: %w", dep.Name, dep.Version, err)
	}

	// Mark as visiting for cycle detection.
	r.visiting[dep.Name] = true
	newPath := append(path, dep.Name)

	// Look up the manifest for the chosen version to resolve transitive deps.
	// For now, we check if the registry also provides manifests. If it implements
	// ManifestRegistry, we use it; otherwise, we treat the dep as a leaf.
	if mr, ok := r.registry.(ManifestRegistry); ok {
		m := mr.GetManifest(dep.Name, chosen)
		if m != nil {
			for _, sub := range m.Requires {
				if err := r.resolve(sub, newPath); err != nil {
					return err
				}
			}
		}
	}

	// Mark resolved and add to order.
	delete(r.visiting, dep.Name)
	r.resolved[dep.Name] = chosen
	r.order = append(r.order, ResolvedDep{Name: dep.Name, Version: chosen})
	return nil
}

// ManifestRegistry is an optional extension of Registry that also provides
// manifests for transitive dependency resolution.
type ManifestRegistry interface {
	Registry
	GetManifest(name, version string) *PackManifest
}

// --- Semver parsing and constraint satisfaction ---

// semver represents a parsed semantic version.
type semver struct {
	Major int
	Minor int
	Patch int
}

func parseSemver(s string) (semver, error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		// Try partial versions.
		for len(parts) < 3 {
			parts = append(parts, "0")
		}
	}
	// Strip pre-release/build metadata for core comparison.
	parts[2] = strings.SplitN(parts[2], "-", 2)[0]
	parts[2] = strings.SplitN(parts[2], "+", 2)[0]

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, fmt.Errorf("invalid major version: %s", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, fmt.Errorf("invalid minor version: %s", parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, fmt.Errorf("invalid patch version: %s", parts[2])
	}
	return semver{Major: major, Minor: minor, Patch: patch}, nil
}

func (v semver) compare(other semver) int {
	if v.Major != other.Major {
		return intCompare(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return intCompare(v.Minor, other.Minor)
	}
	return intCompare(v.Patch, other.Patch)
}

func intCompare(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// SatisfiesConstraint checks whether a version string satisfies a constraint expression.
// Supported constraint formats:
//   - ">=1.0.0" — greater than or equal
//   - ">1.0.0"  — strictly greater
//   - "<=1.0.0" — less than or equal
//   - "<1.0.0"  — strictly less
//   - "=1.0.0" or "1.0.0" — exact match
//   - "^1.2.3"  — compatible (same major, >=minor.patch)
//   - "~1.2.3"  — approximately (same major.minor, >=patch)
func SatisfiesConstraint(version, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return true
	}

	ver, err := parseSemver(version)
	if err != nil {
		return false
	}

	// Handle caret (^) — compatible with major version.
	if strings.HasPrefix(constraint, "^") {
		target, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		if ver.Major != target.Major {
			return false
		}
		return ver.compare(target) >= 0
	}

	// Handle tilde (~) — approximately equal (same major.minor).
	if strings.HasPrefix(constraint, "~") {
		target, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		if ver.Major != target.Major || ver.Minor != target.Minor {
			return false
		}
		return ver.Patch >= target.Patch
	}

	// Handle comparison operators.
	if strings.HasPrefix(constraint, ">=") {
		target, err := parseSemver(constraint[2:])
		if err != nil {
			return false
		}
		return ver.compare(target) >= 0
	}
	if strings.HasPrefix(constraint, ">") {
		target, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		return ver.compare(target) > 0
	}
	if strings.HasPrefix(constraint, "<=") {
		target, err := parseSemver(constraint[2:])
		if err != nil {
			return false
		}
		return ver.compare(target) <= 0
	}
	if strings.HasPrefix(constraint, "<") {
		target, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		return ver.compare(target) < 0
	}

	// Exact match (with optional = prefix).
	c := strings.TrimPrefix(constraint, "=")
	target, err := parseSemver(c)
	if err != nil {
		return false
	}
	return ver.compare(target) == 0
}

// selectBestVersion finds the highest version from candidates that satisfies the constraint.
func selectBestVersion(versions []string, constraint string) (string, error) {
	// Sort versions descending so we pick highest first.
	sorted := make([]string, len(versions))
	copy(sorted, versions)
	sort.Slice(sorted, func(i, j int) bool {
		vi, _ := parseSemver(sorted[i])
		vj, _ := parseSemver(sorted[j])
		return vi.compare(vj) > 0 // descending
	})

	for _, v := range sorted {
		if SatisfiesConstraint(v, constraint) {
			return v, nil
		}
	}
	return "", fmt.Errorf("none of %v satisfies %q", versions, constraint)
}
