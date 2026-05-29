package cedar

import (
	"fmt"
	"strings"
)

// PolicyInput represents a single policy rule to be compiled into Cedar policy text.
type PolicyInput struct {
	Subject    string // Principal entity, e.g. "User::alice"
	Action     string // Action name, e.g. "invoke"
	Resource   string // Resource entity, e.g. "Skill::summarize"
	Effect     string // "allow" or "deny"
	Conditions []string // Optional Cedar condition expressions
}

// CedarPolicyCompiler compiles PolicyInput slices into Cedar policy text.
type CedarPolicyCompiler struct{}

// NewCompiler creates a new CedarPolicyCompiler.
func NewCompiler() *CedarPolicyCompiler {
	return &CedarPolicyCompiler{}
}

// Compile takes a slice of PolicyInput and returns the combined Cedar policy text.
func (c *CedarPolicyCompiler) Compile(policies []PolicyInput) (string, error) {
	if len(policies) == 0 {
		return "", fmt.Errorf("no policies to compile")
	}

	var b strings.Builder
	for i, p := range policies {
		text, err := compileSingle(p)
		if err != nil {
			return "", fmt.Errorf("policy[%d]: %w", i, err)
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(text)
	}
	return b.String(), nil
}

// compileSingle compiles one PolicyInput into Cedar policy text.
func compileSingle(p PolicyInput) (string, error) {
	if err := validatePolicyInput(p); err != nil {
		return "", err
	}

	var b strings.Builder

	// Effect keyword: permit or forbid
	switch p.Effect {
	case "allow":
		b.WriteString("permit(\n")
	case "deny":
		b.WriteString("forbid(\n")
	}

	fmt.Fprintf(&b, "  principal == AIK::%s,\n", p.Subject)
	fmt.Fprintf(&b, "  action == AIK::Action::\"%s\",\n", p.Action)
	fmt.Fprintf(&b, "  resource == AIK::%s\n", p.Resource)
	b.WriteString(")")

	// Append conditions if present
	if len(p.Conditions) > 0 {
		b.WriteString("\nwhen {\n")
		for _, cond := range p.Conditions {
			fmt.Fprintf(&b, "  %s\n", cond)
		}
		b.WriteString("}")
	}

	b.WriteString(";\n")
	return b.String(), nil
}

// validatePolicyInput checks that required fields are present and valid.
func validatePolicyInput(p PolicyInput) error {
	if p.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	if p.Action == "" {
		return fmt.Errorf("action is required")
	}
	if p.Resource == "" {
		return fmt.Errorf("resource is required")
	}
	if p.Effect != "allow" && p.Effect != "deny" {
		return fmt.Errorf("effect must be \"allow\" or \"deny\", got %q", p.Effect)
	}
	return nil
}
