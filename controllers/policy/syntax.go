package policy

import (
	"fmt"
	"net"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/util/validation/field"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// celEnv is the global CEL environment used for parse-only validation.
// Building it once is safe because we never execute the resulting
// programs — we just want the parser to accept or reject syntax.
var celEnv = mustCELEnv()

func mustCELEnv() *cel.Env {
	env, err := cel.NewEnv()
	if err != nil {
		// cel.NewEnv with no options never errors in practice; if it
		// ever does we want to surface the failure loudly during
		// process start.
		panic(fmt.Sprintf("policy: cel.NewEnv: %v", err))
	}
	return env
}

// cronParser accepts the full set of robfig/cron v3 features —
// 5-field standard cron, optional seconds and the @-shortcuts —
// matching the schedule strings users typically copy from existing
// systems.
var cronParser = cron.NewParser(
	cron.SecondOptional |
		cron.Minute |
		cron.Hour |
		cron.Dom |
		cron.Month |
		cron.Dow |
		cron.Descriptor,
)

// validateCEL reports whether `expr` is syntactically a valid CEL
// expression. The function only invokes `cel.Env.Parse` — it never
// executes the program — so it is safe to call against arbitrary
// user-supplied input.
//
// An empty expression returns nil because the surrounding policy
// types treat the field as optional; callers that require a non-empty
// string should check that before calling.
func validateCEL(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	if _, issues := celEnv.Parse(expr); issues != nil && issues.Err() != nil {
		return fmt.Errorf("CEL parse error: %w", issues.Err())
	}
	return nil
}

// validateCron reports whether `expr` is a valid cron schedule under
// the 6-field (with optional seconds) grammar used by robfig/cron v3.
// An empty string returns nil — schedules are optional on
// `ConditionTimeWindow`.
func validateCron(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	if _, err := cronParser.Parse(expr); err != nil {
		return fmt.Errorf("cron parse error: %w", err)
	}
	return nil
}

// validateCIDR reports whether `s` is a valid CIDR block. Empty
// strings return nil because the surrounding `ipAllowList`/`ipDenyList`
// fields are optional.
func validateCIDR(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if _, _, err := net.ParseCIDR(s); err != nil {
		return fmt.Errorf("CIDR parse error: %w", err)
	}
	return nil
}

// validatePolicy walks every CEL / cron / CIDR field declared on a
// Policy and aggregates their parse errors into a [field.ErrorList].
// The returned list mirrors the shape used by Kubernetes admission
// webhooks so the controller can promote it to a single
// `SyntaxValid=False` condition message.
//
// Validates: Requirement A5.2.
func validatePolicy(p *policyv1alpha1.Policy) field.ErrorList {
	var errs field.ErrorList
	if p == nil {
		return errs
	}
	specPath := field.NewPath("spec")

	if cs := p.Spec.Conditions; cs != nil {
		condsPath := specPath.Child("conditions")
		errs = append(errs, validateConditionItems(condsPath.Child("allOf"), cs.AllOf)...)
		errs = append(errs, validateConditionItems(condsPath.Child("anyOf"), cs.AnyOf)...)
		errs = append(errs, validateConditionItems(condsPath.Child("noneOf"), cs.NoneOf)...)
	}

	if len(p.Spec.Approvals) > 0 {
		appPath := specPath.Child("approvals")
		for i, a := range p.Spec.Approvals {
			ePath := appPath.Index(i).Child("when", "expression")
			if err := validateCEL(a.When.Expression); err != nil {
				errs = append(errs, field.Invalid(ePath, a.When.Expression, err.Error()))
			}
		}
	}

	if p.Spec.Obligations != nil && p.Spec.Obligations.Notify != nil {
		notifyPath := specPath.Child("obligations", "notify", "onMatch")
		for i, m := range p.Spec.Obligations.Notify.OnMatch {
			cPath := notifyPath.Index(i).Child("condition")
			if err := validateCEL(m.Condition); err != nil {
				errs = append(errs, field.Invalid(cPath, m.Condition, err.Error()))
			}
		}
	}

	return errs
}

// validateConditionItems walks one branch of `conditions.{allOf|anyOf|noneOf}`.
func validateConditionItems(parent *field.Path, items []policyv1alpha1.ConditionItem) field.ErrorList {
	var errs field.ErrorList
	for i, item := range items {
		itemPath := parent.Index(i)
		if err := validateCEL(item.Expression); err != nil {
			errs = append(errs, field.Invalid(itemPath.Child("expression"), item.Expression, err.Error()))
		}
		if item.TimeWindow != nil {
			if err := validateCron(item.TimeWindow.Schedule); err != nil {
				errs = append(errs, field.Invalid(
					itemPath.Child("timeWindow", "schedule"),
					item.TimeWindow.Schedule, err.Error()))
			}
		}
		if item.Location != nil {
			for j, c := range item.Location.IPAllowList {
				if err := validateCIDR(c); err != nil {
					errs = append(errs, field.Invalid(
						itemPath.Child("location", "ipAllowList").Index(j),
						c, err.Error()))
				}
			}
			for j, c := range item.Location.IPDenyList {
				if err := validateCIDR(c); err != nil {
					errs = append(errs, field.Invalid(
						itemPath.Child("location", "ipDenyList").Index(j),
						c, err.Error()))
				}
			}
		}
	}
	return errs
}
