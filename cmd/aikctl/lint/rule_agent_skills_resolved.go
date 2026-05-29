package lint

import (
	"fmt"
	"strings"
)

// AgentSkillsResolved checks that all skills[].ref in an Agent reference
// Skill resources that exist in the resource set.
//
// Rule: agent/skills-resolved (error)
// Requirement: A9.2
type AgentSkillsResolved struct{}

func (r *AgentSkillsResolved) Run(rs *ResourceSet) []Result {
	var results []Result

	// Build set of known skill names.
	knownSkills := make(map[string]bool)
	for _, skill := range rs.Skills {
		knownSkills[skill.Metadata.Name] = true
	}

	for _, agent := range rs.Agents {
		skills := getSliceField(agent.RawSpec, "skills")
		for _, s := range skills {
			skillMap, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			ref, _ := skillMap["ref"].(string)
			if ref == "" {
				results = append(results, Result{
					Rule:    "agent/skills-resolved",
					Level:   LevelError,
					Message: fmt.Sprintf("Agent %q has a skill binding with empty ref", agent.Metadata.Name),
					File:    agent.File,
				})
				continue
			}

			// Extract skill name from ref like "skill://contract-review@^1.0.0"
			skillName := extractSkillNameFromRef(ref)
			if skillName != "" && !knownSkills[skillName] {
				results = append(results, Result{
					Rule:    "agent/skills-resolved",
					Level:   LevelError,
					Message: fmt.Sprintf("Agent %q references skill %q (ref=%s) which is not found in the resource set", agent.Metadata.Name, skillName, ref),
					File:    agent.File,
				})
			}
		}
	}

	return results
}

// extractSkillNameFromRef parses a ResourceRef like "skill://name@version"
// and returns the name part.
func extractSkillNameFromRef(ref string) string {
	// Format: "skill://name" or "skill://name@version"
	if !strings.HasPrefix(ref, "skill://") {
		return ""
	}
	name := strings.TrimPrefix(ref, "skill://")
	// Strip version suffix.
	if idx := strings.Index(name, "@"); idx >= 0 {
		name = name[:idx]
	}
	// Strip path components if any.
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}
