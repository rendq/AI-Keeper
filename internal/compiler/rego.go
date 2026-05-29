package compiler

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// regoFile represents a generated .rego file.
type regoFile struct {
	Name    string
	Content []byte
}

// The main policy.rego implements the decision algorithm:
// - Higher priority wins
// - At same priority, deny wins over allow
const mainRegoTemplate = `package aip

import rego.v1

# Decision algorithm: higher priority wins; at same priority deny wins.
# This is the entry point for policy evaluation.

default allow := false

# allow is true when there is at least one allow decision at the highest
# matching priority and no deny at that same priority or higher.
allow if {
    some decision in allow_decisions
    not denied_at_or_above(decision.priority)
}

# denied_at_or_above checks if there is a deny decision at the given
# priority or higher.
denied_at_or_above(priority) if {
    some d in deny_decisions
    d.priority >= priority
}

# Collect all allow decisions from individual policy rules.
allow_decisions contains decision if {
    some decision in data.aip.aip_allow
}

# Collect all deny decisions from individual policy rules.
deny_decisions contains decision if {
    some decision in data.aip.aip_deny
}

# The final decision object.
decision := {
    "allow": allow,
    "deny": count(deny_decisions) > 0,
    "matched_policies": matched_policies,
}

matched_policies contains name if {
    some d in allow_decisions
    name := d.name
}

matched_policies contains name if {
    some d in deny_decisions
    name := d.name
}
`

// policyRegoTemplate generates a Rego module for a single policy.
// Each policy contributes to either allow_set or deny_set.
const policyRegoTemplate = `package aip.policies

import rego.v1

# Policy: {{.Name}} (effect={{.Effect}}, priority={{.Priority}})
{{.Effect}}_set contains {"name": "{{.Name}}", "priority": {{.Priority}}, "obligations": {{.ObligationsJSON}}} if {
{{.Conditions}}
}
`

// policyRegoData holds template data for a single policy module.
type policyRegoData struct {
	Name            string
	Effect          string
	Priority        int32
	ObligationsJSON string
	Conditions      string
}

// generateRegoModules produces .rego files for the main decision logic
// and for each individual policy.
func generateRegoModules(policies []policyv1alpha1.Policy) ([]regoFile, error) {
	var files []regoFile

	// Main decision logic.
	files = append(files, regoFile{
		Name:    "aip/main.rego",
		Content: []byte(mainRegoTemplate),
	})

	// Per-policy modules: we group all policies into a single rego file
	// under the aip package. allow rules go into aip_allow, deny into aip_deny.
	var buf bytes.Buffer
	buf.WriteString("package aip\n\nimport rego.v1\n\n")

	for i := range policies {
		p := &policies[i]
		priority := getPriority(*p)
		conditions := generateConditions(p)
		obligations := generateObligationsJSON(p)

		policyName := fmt.Sprintf("%s/%s", p.Namespace, p.Name)

		ruleName := "aip_allow"
		if p.Spec.Effect == "deny" {
			ruleName = "aip_deny"
		}

		fmt.Fprintf(&buf, "# Policy: %s (effect=%s, priority=%d)\n", policyName, p.Spec.Effect, priority)
		fmt.Fprintf(&buf, "%s contains {\"name\": %q, \"priority\": %d, \"obligations\": %s} if {\n", ruleName, policyName, priority, obligations)
		buf.WriteString(conditions)
		buf.WriteString("}\n\n")
	}

	files = append(files, regoFile{
		Name:    "aip/policies.rego",
		Content: buf.Bytes(),
	})

	return files, nil
}

// generateConditions produces the Rego condition block for a policy.
func generateConditions(p *policyv1alpha1.Policy) string {
	var lines []string

	// Subject conditions
	lines = append(lines, generateSubjectConditions(p.Spec.Subject)...)

	// Action/resource conditions
	lines = append(lines, generateActionConditions(p.Spec.Action)...)

	// Additional conditions from conditions block
	if p.Spec.Conditions != nil {
		lines = append(lines, generateConditionSet(p.Spec.Conditions)...)
	}

	if len(lines) == 0 {
		// Always true (no conditions)
		lines = append(lines, "    true")
	}

	return strings.Join(lines, "\n") + "\n"
}

// generateSubjectConditions generates Rego conditions for subject matching.
func generateSubjectConditions(subject policyv1alpha1.SubjectSelector) []string {
	if len(subject.AnyOf) == 0 {
		return nil
	}

	var parts []string
	for _, entry := range subject.AnyOf {
		var entryParts []string
		entryParts = append(entryParts, fmt.Sprintf("input.principal.kind == %q", entry.Kind))
		if entry.Match != nil {
			if entry.Match.Name != "" {
				entryParts = append(entryParts, fmt.Sprintf("input.principal.name == %q", entry.Match.Name))
			}
			if entry.Match.Namespace != "" {
				entryParts = append(entryParts, fmt.Sprintf("input.principal.namespace == %q", entry.Match.Namespace))
			}
			for k, v := range entry.Match.Labels {
				entryParts = append(entryParts, fmt.Sprintf("input.principal.labels[%q] == %q", k, v))
			}
		}
		parts = append(parts, strings.Join(entryParts, "; "))
	}

	// Use anyOf semantics: at least one subject entry must match
	if len(parts) == 1 {
		// Single entry: inline conditions
		return []string{"    " + parts[0]}
	}

	// Multiple entries: use helper
	var lines []string
	lines = append(lines, "    # Subject anyOf")
	lines = append(lines, "    subject_match")
	return lines
}

// generateActionConditions generates Rego conditions for action/resource matching.
func generateActionConditions(action policyv1alpha1.PolicyAction) []string {
	var lines []string

	// Verb matching
	if len(action.Verbs) > 0 {
		verbSet := make([]string, 0, len(action.Verbs))
		for _, v := range action.Verbs {
			verbSet = append(verbSet, fmt.Sprintf("%q", v))
		}
		lines = append(lines, fmt.Sprintf("    input.action.verb in {%s}", strings.Join(verbSet, ", ")))
	}

	// Resource kind matching
	if len(action.Resources.AnyOf) > 0 {
		kindSet := make([]string, 0, len(action.Resources.AnyOf))
		for _, r := range action.Resources.AnyOf {
			if r.Kind != "Any" {
				kindSet = append(kindSet, fmt.Sprintf("%q", r.Kind))
			}
		}
		if len(kindSet) > 0 && len(kindSet) < len(action.Resources.AnyOf) {
			// Mix of specific kinds and "Any" — no restriction needed
		} else if len(kindSet) > 0 {
			lines = append(lines, fmt.Sprintf("    input.action.resource.kind in {%s}", strings.Join(kindSet, ", ")))
		}
		// Additional resource match conditions
		for _, r := range action.Resources.AnyOf {
			if r.Match != nil {
				if r.Match.Name != "" {
					lines = append(lines, fmt.Sprintf("    input.action.resource.name == %q", r.Match.Name))
				}
				if r.Match.Namespace != "" {
					lines = append(lines, fmt.Sprintf("    input.action.resource.namespace == %q", r.Match.Namespace))
				}
				for k, v := range r.Match.Labels {
					lines = append(lines, fmt.Sprintf("    input.action.resource.labels[%q] == %q", k, v))
				}
			}
		}
	}

	return lines
}

// generateConditionSet generates Rego for a ConditionSet (allOf/anyOf/noneOf).
func generateConditionSet(cs *policyv1alpha1.ConditionSet) []string {
	var lines []string

	// AllOf: all conditions must be true
	for _, item := range cs.AllOf {
		if item.Expression != "" {
			// CEL expressions are compiled as opaque Rego data lookups
			lines = append(lines, fmt.Sprintf("    data.aip.cel_results[%q]", item.Expression))
		}
	}

	// NoneOf: none of the conditions may be true
	for _, item := range cs.NoneOf {
		if item.Expression != "" {
			lines = append(lines, fmt.Sprintf("    not data.aip.cel_results[%q]", item.Expression))
		}
	}

	return lines
}

// generateObligationsJSON returns a JSON representation of obligations for data.json embedding.
func generateObligationsJSON(p *policyv1alpha1.Policy) string {
	if p.Spec.Obligations == nil {
		return "{}"
	}

	var parts []string
	if p.Spec.Obligations.Audit != nil {
		parts = append(parts, fmt.Sprintf(`"audit": {"level": %q}`, p.Spec.Obligations.Audit.Level))
	}
	if p.Spec.Obligations.Redact != nil {
		parts = append(parts, `"redact": true`)
	}
	if p.Spec.Obligations.Watermark != nil && p.Spec.Obligations.Watermark.Enabled != nil && *p.Spec.Obligations.Watermark.Enabled {
		mode := "invisible"
		if p.Spec.Obligations.Watermark.Mode != "" {
			mode = p.Spec.Obligations.Watermark.Mode
		}
		parts = append(parts, fmt.Sprintf(`"watermark": {"enabled": true, "mode": %q}`, mode))
	}
	if p.Spec.Obligations.Notify != nil && len(p.Spec.Obligations.Notify.OnMatch) > 0 {
		parts = append(parts, `"notify": true`)
	}

	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// sanitizeRegoIdentifier makes a string safe for use in Rego identifiers.
func sanitizeRegoIdentifier(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "-", "_"), ".", "_")
}

// We keep the template parsed but don't actively use it in the current
// implementation (preferring direct string building for efficiency).
var _ = template.Must(template.New("policy").Parse(policyRegoTemplate))
