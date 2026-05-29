package cedar

import (
	"strings"
	"testing"
)

func TestSchemaGeneration(t *testing.T) {
	schema := GenerateSchema()

	if schema == nil {
		t.Fatal("GenerateSchema() returned nil")
	}
	if schema.Namespace != "AIP" {
		t.Errorf("expected namespace AIP, got %s", schema.Namespace)
	}
}

func TestSchemaGenerationEntityTypes(t *testing.T) {
	schema := GenerateSchema()

	expectedPrincipals := []string{"User", "ServiceAccount", "Agent"}
	expectedResources := []string{"Skill", "Tool", "KnowledgeBase", "Model", "Data"}
	allExpected := append(expectedPrincipals, expectedResources...)

	if len(schema.EntityTypes) != len(allExpected) {
		t.Fatalf("expected %d entity types, got %d", len(allExpected), len(schema.EntityTypes))
	}

	entityNames := make(map[string]bool)
	for _, et := range schema.EntityTypes {
		entityNames[et.Name] = true
	}

	for _, name := range allExpected {
		if !entityNames[name] {
			t.Errorf("missing entity type: %s", name)
		}
	}
}

func TestSchemaGenerationActions(t *testing.T) {
	schema := GenerateSchema()

	expectedActions := []string{"invoke", "read", "write", "delete", "execute", "evaluate"}

	if len(schema.Actions) != len(expectedActions) {
		t.Fatalf("expected %d actions, got %d", len(expectedActions), len(schema.Actions))
	}

	actionNames := make(map[string]bool)
	for _, act := range schema.Actions {
		actionNames[act.Name] = true
	}

	for _, name := range expectedActions {
		if !actionNames[name] {
			t.Errorf("missing action: %s", name)
		}
	}
}

func TestSchemaGenerationCedarText(t *testing.T) {
	schema := GenerateSchema()
	text := schema.ToCedarText()

	if text == "" {
		t.Fatal("ToCedarText() returned empty string")
	}

	// Must start with namespace declaration
	if !strings.HasPrefix(text, "namespace AIP {") {
		t.Error("schema text must start with namespace declaration")
	}

	// Must contain entity declarations
	for _, name := range []string{"User", "ServiceAccount", "Agent", "Skill", "Tool", "KnowledgeBase", "Model", "Data"} {
		if !strings.Contains(text, "entity "+name) {
			t.Errorf("schema text missing entity %s", name)
		}
	}

	// Must contain action declarations
	for _, name := range []string{"invoke", "read", "write", "delete", "execute", "evaluate"} {
		if !strings.Contains(text, "action \""+name+"\"") {
			t.Errorf("schema text missing action %s", name)
		}
	}

	// Must end with closing brace
	if !strings.HasSuffix(strings.TrimSpace(text), "}") {
		t.Error("schema text must end with closing brace")
	}
}

func TestSchemaGenerationCompilePolicy(t *testing.T) {
	compiler := NewCompiler()

	policies := []PolicyInput{
		{
			Subject:  "User::\"alice\"",
			Action:   "invoke",
			Resource: "Skill::\"summarize\"",
			Effect:   "allow",
		},
	}

	result, err := compiler.Compile(policies)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !strings.Contains(result, "permit(") {
		t.Error("allow policy should produce 'permit' keyword")
	}
	if !strings.Contains(result, "AIK::User::\"alice\"") {
		t.Error("policy should contain principal reference")
	}
	if !strings.Contains(result, "AIK::Action::\"invoke\"") {
		t.Error("policy should contain action reference")
	}
	if !strings.Contains(result, "AIK::Skill::\"summarize\"") {
		t.Error("policy should contain resource reference")
	}
}

func TestSchemaGenerationCompileDenyPolicy(t *testing.T) {
	compiler := NewCompiler()

	policies := []PolicyInput{
		{
			Subject:  "Agent::\"bot1\"",
			Action:   "delete",
			Resource: "Data::\"sensitive\"",
			Effect:   "deny",
		},
	}

	result, err := compiler.Compile(policies)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !strings.Contains(result, "forbid(") {
		t.Error("deny policy should produce 'forbid' keyword")
	}
}

func TestSchemaGenerationCompileWithConditions(t *testing.T) {
	compiler := NewCompiler()

	policies := []PolicyInput{
		{
			Subject:    "User::\"bob\"",
			Action:     "read",
			Resource:   "KnowledgeBase::\"docs\"",
			Effect:     "allow",
			Conditions: []string{"resource.classification == \"public\""},
		},
	}

	result, err := compiler.Compile(policies)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !strings.Contains(result, "when {") {
		t.Error("policy with conditions should contain 'when' block")
	}
	if !strings.Contains(result, "resource.classification == \"public\"") {
		t.Error("policy should contain the condition expression")
	}
}

func TestSchemaGenerationCompileInvalidInput(t *testing.T) {
	compiler := NewCompiler()

	tests := []struct {
		name     string
		policies []PolicyInput
		wantErr  string
	}{
		{
			name:     "empty policies",
			policies: []PolicyInput{},
			wantErr:  "no policies to compile",
		},
		{
			name:     "missing subject",
			policies: []PolicyInput{{Action: "read", Resource: "Skill::\"x\"", Effect: "allow"}},
			wantErr:  "subject is required",
		},
		{
			name:     "missing action",
			policies: []PolicyInput{{Subject: "User::\"a\"", Resource: "Skill::\"x\"", Effect: "allow"}},
			wantErr:  "action is required",
		},
		{
			name:     "missing resource",
			policies: []PolicyInput{{Subject: "User::\"a\"", Action: "read", Effect: "allow"}},
			wantErr:  "resource is required",
		},
		{
			name:     "invalid effect",
			policies: []PolicyInput{{Subject: "User::\"a\"", Action: "read", Resource: "Skill::\"x\"", Effect: "maybe"}},
			wantErr:  "effect must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compiler.Compile(tt.policies)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
