package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// SchemaValidator compiles the input/output JSON Schema declared on a
// Skill and reports whether they parse successfully. The interface lets
// us swap in a stub in tests and keeps the reconcile path free of
// santhosh-tekuri-specific types.
type SchemaValidator interface {
	// Validate checks the JSON schemas declared on `spec.interface.input`
	// and `spec.interface.output`. A non-nil error means at least one
	// schema failed to compile; the controller maps that to
	// `SchemaValid=False, reason=InvalidSchema, phase=Failed` per
	// Requirement A3.2.
	Validate(skill *skillv1alpha1.Skill) error
}

// ErrInvalidSchema is returned by [DefaultSchemaValidator] when one of
// the schemas fails to compile. Callers should inspect the wrapped
// error for the underlying compiler message.
var ErrInvalidSchema = errors.New("skill: invalid JSON Schema")

// DefaultSchemaValidator implements [SchemaValidator] using
// santhosh-tekuri/jsonschema/v5. The compiler accepts Draft-7 and
// Draft 2020-12 schemas, which matches the OpenAPI v3 fragments AIP
// embeds in the Skill `interface`.
type DefaultSchemaValidator struct{}

// Validate compiles the input + output schemas. Both schemas are
// required by the CRD OpenAPI markers (`spec.interface.input.schema`
// is non-optional), so we treat missing payloads as failure.
func (DefaultSchemaValidator) Validate(skill *skillv1alpha1.Skill) error {
	if skill == nil {
		return fmt.Errorf("%w: nil skill", ErrInvalidSchema)
	}
	if skill.Spec.Interface.Input.Schema == nil || len(skill.Spec.Interface.Input.Schema.Raw) == 0 {
		return fmt.Errorf("%w: input.schema is empty", ErrInvalidSchema)
	}
	if skill.Spec.Interface.Output.Schema == nil || len(skill.Spec.Interface.Output.Schema.Raw) == 0 {
		return fmt.Errorf("%w: output.schema is empty", ErrInvalidSchema)
	}
	if err := compileSchema(skill.Spec.Interface.Input.Schema.Raw, "input"); err != nil {
		return err
	}
	if err := compileSchema(skill.Spec.Interface.Output.Schema.Raw, "output"); err != nil {
		return err
	}
	return nil
}

// compileSchema runs the supplied JSON bytes through the
// santhosh-tekuri compiler. The schema is registered under a synthetic
// URL so error messages reference it by purpose ("input" / "output")
// rather than by index.
func compileSchema(raw []byte, label string) error {
	c := jsonschema.NewCompiler()
	url := "mem:///skill/" + label + ".json"
	if err := c.AddResource(url, strings.NewReader(string(raw))); err != nil {
		return fmt.Errorf("%w: %s.schema: %v", ErrInvalidSchema, label, err)
	}
	if _, err := c.Compile(url); err != nil {
		return fmt.Errorf("%w: %s.schema: %v", ErrInvalidSchema, label, err)
	}
	return nil
}

// Compile-time interface assertion.
var _ SchemaValidator = DefaultSchemaValidator{}
