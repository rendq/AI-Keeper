package router

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// CELEvaluator evaluates CEL expressions against a RequestContext.
type CELEvaluator struct {
	env *cel.Env
}

// NewCELEvaluator creates a CEL environment with the standard request
// context variables available for routing rules.
func NewCELEvaluator() (*CELEvaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable("user", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("classification", cel.StringType),
		cel.Variable("costSensitive", cel.BoolType),
		cel.Variable("tenant", cel.StringType),
		cel.Variable("taskType", cel.StringType),
		cel.Variable("contextLength", cel.IntType),
	)
	if err != nil {
		return nil, fmt.Errorf("cel: failed to create env: %w", err)
	}
	return &CELEvaluator{env: env}, nil
}

// Evaluate evaluates a CEL expression against the given request context.
// An empty expression always returns true (catch-all rule).
func (e *CELEvaluator) Evaluate(expression string, ctx RequestContext) (bool, error) {
	if expression == "" {
		return true, nil
	}

	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("cel: compile error: %w", issues.Err())
	}

	prg, err := e.env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("cel: program error: %w", err)
	}

	// Build the activation map from RequestContext.
	userMap := map[string]interface{}{
		"country": ctx.UserCountry,
	}
	// Merge extra attributes into user map.
	if ctx.Extra != nil {
		for k, v := range ctx.Extra {
			userMap[k] = v
		}
	}

	vars := map[string]interface{}{
		"user":           userMap,
		"classification": ctx.Classification,
		"costSensitive":  ctx.CostSensitive,
		"tenant":         ctx.TenantID,
		"taskType":       ctx.TaskType,
		"contextLength":  int64(ctx.ContextLength),
	}

	out, _, err := prg.Eval(vars)
	if err != nil {
		return false, fmt.Errorf("cel: eval error: %w", err)
	}

	// Ensure the result is a boolean.
	if out.Type() != types.BoolType {
		return false, fmt.Errorf("cel: expression must return bool, got %s", out.Type())
	}

	return out.(ref.Val).Value().(bool), nil
}
