package lint

import (
	"fmt"
)

// ToolDestructiveNeedsApproval checks that Tools with
// governance.sideEffects=destructive have governance.requiresApproval=true.
//
// Rule: tool/destructive-needs-approval (error)
// Requirement: A9.2
type ToolDestructiveNeedsApproval struct{}

func (r *ToolDestructiveNeedsApproval) Run(rs *ResourceSet) []Result {
	var results []Result

	for _, tool := range rs.Tools {
		governance := getMapField(tool.RawSpec, "governance")
		if governance == nil {
			continue
		}

		sideEffects, _ := governance["sideEffects"].(string)
		if sideEffects != "destructive" {
			continue
		}

		requiresApproval, _ := governance["requiresApproval"].(bool)
		if !requiresApproval {
			results = append(results, Result{
				Rule:    "tool/destructive-needs-approval",
				Level:   LevelError,
				Message: fmt.Sprintf("Tool %q has governance.sideEffects=destructive but governance.requiresApproval is not true", tool.Metadata.Name),
				File:    tool.File,
			})
		}
	}

	return results
}
