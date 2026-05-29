package lint

import (
	"fmt"
	"strings"
)

// AgentSandboxRequired checks that Agents with pattern=react that use code
// tools have sandbox enabled.
//
// P0 scope: field-level declaration check only.
// If runtime.pattern == "react" and any skill ref contains "code" or the
// agent references a tool with protocol "mcp" that has "code" in its name,
// then runtime.sandbox.enabled must be true.
//
// Rule: agent/sandbox-required (error)
// Requirement: A9.2
type AgentSandboxRequired struct{}

func (r *AgentSandboxRequired) Run(rs *ResourceSet) []Result {
	var results []Result

	// Build set of tool names that are "code tools" based on name heuristic.
	codeTools := make(map[string]bool)
	for _, tool := range rs.Tools {
		name := strings.ToLower(tool.Metadata.Name)
		if strings.Contains(name, "code") || strings.Contains(name, "exec") || strings.Contains(name, "shell") || strings.Contains(name, "sandbox") {
			codeTools[tool.Metadata.Name] = true
		}
	}

	for _, agent := range rs.Agents {
		runtime := getMapField(agent.RawSpec, "runtime")
		if runtime == nil {
			continue
		}

		pattern, _ := runtime["pattern"].(string)
		if pattern != "react" {
			continue
		}

		// Check if agent uses code-related tools or skills.
		usesCodeTool := false

		// Check skills for code-related references.
		skills := getSliceField(agent.RawSpec, "skills")
		for _, s := range skills {
			skillMap, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			ref, _ := skillMap["ref"].(string)
			refLower := strings.ToLower(ref)
			if strings.Contains(refLower, "code") || strings.Contains(refLower, "exec") || strings.Contains(refLower, "shell") {
				usesCodeTool = true
				break
			}
		}

		// Also check if the agent's skills reference tools known to be code tools.
		if !usesCodeTool && len(codeTools) > 0 {
			// For P0, the heuristic is: if any tool in the set is a code tool
			// and the agent is pattern=react, flag it as needing sandbox.
			// A more thorough check would resolve the skill->tool dependency.
			// But for field-level check, we look at the agent's skill names.
			for _, s := range skills {
				skillMap, ok := s.(map[string]interface{})
				if !ok {
					continue
				}
				ref, _ := skillMap["ref"].(string)
				skillName := extractSkillNameFromRef(ref)
				if codeTools[skillName] {
					usesCodeTool = true
					break
				}
			}
		}

		if !usesCodeTool {
			continue
		}

		// Check sandbox.enabled.
		sandbox := getMapField(runtime, "sandbox")
		if sandbox == nil {
			results = append(results, Result{
				Rule:    "agent/sandbox-required",
				Level:   LevelError,
				Message: fmt.Sprintf("Agent %q has pattern=react with code tools but runtime.sandbox is not configured", agent.Metadata.Name),
				File:    agent.File,
			})
			continue
		}

		enabled, _ := sandbox["enabled"].(bool)
		if !enabled {
			results = append(results, Result{
				Rule:    "agent/sandbox-required",
				Level:   LevelError,
				Message: fmt.Sprintf("Agent %q has pattern=react with code tools but runtime.sandbox.enabled is not true", agent.Metadata.Name),
				File:    agent.File,
			})
		}
	}

	return results
}
