package lint

import (
	"fmt"

	"gopkg.in/yaml.v3"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	"github.com/ai-keeper/ai-keeper/internal/conflictcheck"
)

// PolicyNoConflict checks for hard conflicts between policies using the
// internal/conflictcheck package.
//
// Rule: policy/no-conflict (error)
// Requirement: A9.2
type PolicyNoConflict struct{}

func (r *PolicyNoConflict) Run(rs *ResourceSet) []Result {
	var results []Result

	if len(rs.Policies) < 2 {
		return nil
	}

	// Parse policies into typed structs for conflict detection.
	var policies []policyv1alpha1.Policy
	for _, p := range rs.Policies {
		var policy policyv1alpha1.Policy
		policy.Name = p.Metadata.Name
		policy.Namespace = p.Metadata.Namespace

		// Marshal spec node back to yaml, then unmarshal into PolicySpec.
		specBytes, err := yaml.Marshal(p.RawSpec)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(specBytes, &policy.Spec); err != nil {
			continue
		}

		policies = append(policies, policy)
	}

	if len(policies) < 2 {
		return nil
	}

	// Run conflict detection.
	conflicts := conflictcheck.DetectConflicts(policies)
	for _, c := range conflicts {
		if c.Type == conflictcheck.Hard {
			// Find the file for PolicyA.
			file := ""
			for _, p := range rs.Policies {
				if p.Metadata.Name == c.PolicyA {
					file = p.File
					break
				}
			}
			results = append(results, Result{
				Rule:    "policy/no-conflict",
				Level:   LevelError,
				Message: fmt.Sprintf("Hard conflict between %q and %q: %s", c.PolicyA, c.PolicyB, c.Reason),
				File:    file,
			})
		}
	}

	return results
}
