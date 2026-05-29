// Package cedar implements Cedar policy schema generation and compilation
// for the AIP Policy Decision Point (PDP).
package cedar

import (
	"fmt"
	"strings"
)

// EntityType represents a Cedar entity type with optional parent relationships and attributes.
type EntityType struct {
	Name       string
	Parents    []string
	Attributes map[string]string
}

// ActionType represents a Cedar action with principal and resource applicability.
type ActionType struct {
	Name      string
	AppliesTo ActionAppliesTo
}

// ActionAppliesTo defines which principals and resources an action applies to.
type ActionAppliesTo struct {
	Principals []string
	Resources  []string
}

// CedarSchema represents the full Cedar entity/action schema for AIP.
type CedarSchema struct {
	Namespace   string
	EntityTypes []EntityType
	Actions     []ActionType
}

// GenerateSchema produces the AIP Cedar schema with predefined entity types and actions.
func GenerateSchema() *CedarSchema {
	principals := []EntityType{
		{Name: "User", Attributes: map[string]string{"name": "String", "namespace": "String"}},
		{Name: "ServiceAccount", Attributes: map[string]string{"name": "String", "namespace": "String"}},
		{Name: "Agent", Attributes: map[string]string{"name": "String", "namespace": "String"}},
	}

	resources := []EntityType{
		{Name: "Skill", Attributes: map[string]string{"name": "String", "namespace": "String", "classification": "String"}},
		{Name: "Tool", Attributes: map[string]string{"name": "String", "namespace": "String"}},
		{Name: "KnowledgeBase", Attributes: map[string]string{"name": "String", "namespace": "String", "classification": "String"}},
		{Name: "Model", Attributes: map[string]string{"name": "String", "namespace": "String"}},
		{Name: "Data", Attributes: map[string]string{"name": "String", "namespace": "String", "classification": "String"}},
	}

	allPrincipals := []string{"User", "ServiceAccount", "Agent"}
	allResources := []string{"Skill", "Tool", "KnowledgeBase", "Model", "Data"}

	actions := []ActionType{
		{Name: "invoke", AppliesTo: ActionAppliesTo{Principals: allPrincipals, Resources: allResources}},
		{Name: "read", AppliesTo: ActionAppliesTo{Principals: allPrincipals, Resources: allResources}},
		{Name: "write", AppliesTo: ActionAppliesTo{Principals: allPrincipals, Resources: allResources}},
		{Name: "delete", AppliesTo: ActionAppliesTo{Principals: allPrincipals, Resources: allResources}},
		{Name: "execute", AppliesTo: ActionAppliesTo{Principals: allPrincipals, Resources: allResources}},
		{Name: "evaluate", AppliesTo: ActionAppliesTo{Principals: allPrincipals, Resources: allResources}},
	}

	entityTypes := append(principals, resources...)

	return &CedarSchema{
		Namespace:   "AIP",
		EntityTypes: entityTypes,
		Actions:     actions,
	}
}

// ToCedarText renders the schema as Cedar schema language text.
func (s *CedarSchema) ToCedarText() string {
	var b strings.Builder

	fmt.Fprintf(&b, "namespace %s {\n", s.Namespace)

	// Entity types
	for _, et := range s.EntityTypes {
		if len(et.Parents) > 0 {
			fmt.Fprintf(&b, "  entity %s in [%s]", et.Name, strings.Join(et.Parents, ", "))
		} else {
			fmt.Fprintf(&b, "  entity %s", et.Name)
		}
		if len(et.Attributes) > 0 {
			b.WriteString(" = {\n")
			for attr, typ := range et.Attributes {
				fmt.Fprintf(&b, "    %s: %s,\n", attr, typ)
			}
			b.WriteString("  };\n")
		} else {
			b.WriteString(";\n")
		}
	}

	// Actions
	for _, act := range s.Actions {
		fmt.Fprintf(&b, "  action %q appliesTo {\n", act.Name)
		fmt.Fprintf(&b, "    principal: [%s],\n", joinQuoted(act.AppliesTo.Principals))
		fmt.Fprintf(&b, "    resource: [%s],\n", joinQuoted(act.AppliesTo.Resources))
		b.WriteString("  };\n")
	}

	b.WriteString("}\n")
	return b.String()
}

// joinQuoted joins strings with quotes for Cedar schema syntax.
func joinQuoted(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%s::%s", "AIP", item)
	}
	return strings.Join(quoted, ", ")
}
