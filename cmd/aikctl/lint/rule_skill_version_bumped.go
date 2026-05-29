package lint

import (
	"fmt"
)

// SkillVersionBumped checks that if a Skill's spec has been modified
// (indicated by metadata.annotations["ai-keeper.io/previous-version"] being present
// and different from spec.version), then the version must have been bumped.
//
// In a directory-based lint scenario, if multiple Skills with the same name
// exist, they must have different versions. If the annotation is not present,
// this rule checks for duplicate skill names with the same version (which
// indicates a spec change without a version bump).
//
// Rule: skill/version-bumped (error)
// Requirement: A9.2
type SkillVersionBumped struct{}

func (r *SkillVersionBumped) Run(rs *ResourceSet) []Result {
	var results []Result

	for _, skill := range rs.Skills {
		version := getStringField(skill.RawSpec, "version")
		if version == "" {
			results = append(results, Result{
				Rule:    "skill/version-bumped",
				Level:   LevelError,
				Message: fmt.Sprintf("Skill %q missing spec.version field", skill.Metadata.Name),
				File:    skill.File,
			})
			continue
		}

		// Check if there's a previous-version annotation indicating spec change.
		prevVersion := ""
		if skill.Metadata.Annotations != nil {
			prevVersion = skill.Metadata.Annotations["ai-keeper.io/previous-version"]
		}

		if prevVersion != "" && prevVersion == version {
			results = append(results, Result{
				Rule:    "skill/version-bumped",
				Level:   LevelError,
				Message: fmt.Sprintf("Skill %q spec changed (previous-version=%s) but version not bumped (still %s)", skill.Metadata.Name, prevVersion, version),
				File:    skill.File,
			})
		}
	}

	// Also check for duplicate skill name+version in the same set.
	seen := make(map[string]string) // "name@version" -> file
	for _, skill := range rs.Skills {
		version := getStringField(skill.RawSpec, "version")
		key := skill.Metadata.Name + "@" + version
		if prev, ok := seen[key]; ok {
			results = append(results, Result{
				Rule:    "skill/version-bumped",
				Level:   LevelError,
				Message: fmt.Sprintf("Skill %q appears multiple times with same version %s (also in %s)", skill.Metadata.Name, version, prev),
				File:    skill.File,
			})
		} else {
			seen[key] = skill.File
		}
	}

	return results
}
