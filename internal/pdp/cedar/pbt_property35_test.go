//go:build pbt

// Feature: ai-platform, Property 35: Cedar vs OPA Decision Consistency
//
// Generator: Random (policy set, request context) — limited to subset expressible by both engines
// Oracle: CedarEngine.Decide(ctx) == OPAEngine.Decide(ctx) for equivalent policies
// Property: P35 / Validates: F13

package cedar

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func pbtSeed35() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// ---------------------------------------------------------------------------
// Domain types for the property test
// ---------------------------------------------------------------------------

// testPolicy represents a single policy used in both engines.
type testPolicy struct {
	Effect    string // "permit" or "forbid"
	Principal string // entity or "*" for wildcard
	Action    string // entity or "*" for wildcard
	Resource  string // entity or "*" for wildcard
}

// testRequest represents an authorization request.
type testRequest struct {
	Principal string
	Action    string
	Resource  string
}

// testCase bundles a set of policies and a request for the property test.
type testCase struct {
	Policies []testPolicy
	Request  testRequest
}

// ---------------------------------------------------------------------------
// Reference OPA-like evaluator
//
// Implements the same decision algorithm as Cedar:
// 1. Match policies against the request
// 2. If any forbid policy matches → deny (forbid takes precedence)
// 3. If any permit policy matches (and no forbid) → allow
// 4. Default → deny (no matching policy)
// ---------------------------------------------------------------------------

func referenceOPADecision(policies []testPolicy, req testRequest) string {
	hasPermit := false

	for _, p := range policies {
		if !refEntityMatches(p.Principal, req.Principal) {
			continue
		}
		if !refEntityMatches(p.Action, req.Action) {
			continue
		}
		if !refEntityMatches(p.Resource, req.Resource) {
			continue
		}

		// Policy matches
		if p.Effect == "forbid" {
			return "deny" // forbid takes precedence immediately
		}
		if p.Effect == "permit" {
			hasPermit = true
		}
	}

	if hasPermit {
		return "allow"
	}
	return "deny" // default deny
}

func refEntityMatches(pattern, value string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return pattern == value
}

// ---------------------------------------------------------------------------
// Cedar policy text generation from testPolicy slice
// ---------------------------------------------------------------------------

func policiesToCedarText(policies []testPolicy) string {
	var sb strings.Builder
	for _, p := range policies {
		sb.WriteString(p.Effect)
		sb.WriteString("(")

		parts := []string{}
		if p.Principal != "" && p.Principal != "*" {
			parts = append(parts, fmt.Sprintf("principal == %s", p.Principal))
		} else {
			parts = append(parts, "principal")
		}
		if p.Action != "" && p.Action != "*" {
			parts = append(parts, fmt.Sprintf("action == %s", p.Action))
		} else {
			parts = append(parts, "action")
		}
		if p.Resource != "" && p.Resource != "*" {
			parts = append(parts, fmt.Sprintf("resource == %s", p.Resource))
		} else {
			parts = append(parts, "resource")
		}

		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteString(");\n")
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func genEffect() gopter.Gen {
	return gen.OneConstOf("permit", "forbid")
}

func genEntity() gopter.Gen {
	entities := []string{
		"AIK::User::alice",
		"AIK::User::bob",
		"AIK::User::charlie",
		"AIK::Service::gateway",
		"AIK::Agent::copilot",
	}
	return gen.OneConstOf(entities[0], entities[1], entities[2], entities[3], entities[4])
}

func genActionEntity() gopter.Gen {
	actions := []string{
		"AIK::Action::invoke",
		"AIK::Action::read",
		"AIK::Action::write",
		"AIK::Action::admin",
	}
	return gen.OneConstOf(actions[0], actions[1], actions[2], actions[3])
}

func genResourceEntity() gopter.Gen {
	resources := []string{
		"AIK::Skill::contract-review",
		"AIK::Skill::search",
		"AIK::KB::legal",
		"AIK::Tool::docusign",
	}
	return gen.OneConstOf(resources[0], resources[1], resources[2], resources[3])
}

// genPolicyPrincipal generates either a specific entity or wildcard.
func genPolicyPrincipal() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		{Weight: 3, Gen: genEntity()},
		{Weight: 1, Gen: gen.Const("*")},
	})
}

// genPolicyAction generates either a specific action or wildcard.
func genPolicyAction() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		{Weight: 3, Gen: genActionEntity()},
		{Weight: 1, Gen: gen.Const("*")},
	})
}

// genPolicyResource generates either a specific resource or wildcard.
func genPolicyResource() gopter.Gen {
	return gen.Weighted([]gen.WeightedGen{
		{Weight: 3, Gen: genResourceEntity()},
		{Weight: 1, Gen: gen.Const("*")},
	})
}

func genTestPolicy() gopter.Gen {
	return gopter.CombineGens(
		genEffect(),
		genPolicyPrincipal(),
		genPolicyAction(),
		genPolicyResource(),
	).Map(func(vals []interface{}) testPolicy {
		return testPolicy{
			Effect:    vals[0].(string),
			Principal: vals[1].(string),
			Action:    vals[2].(string),
			Resource:  vals[3].(string),
		}
	})
}

func genTestRequest() gopter.Gen {
	return gopter.CombineGens(
		genEntity(),
		genActionEntity(),
		genResourceEntity(),
	).Map(func(vals []interface{}) testRequest {
		return testRequest{
			Principal: vals[0].(string),
			Action:    vals[1].(string),
			Resource:  vals[2].(string),
		}
	})
}

func genTestCase() gopter.Gen {
	return gopter.CombineGens(
		gen.SliceOfN(5, genTestPolicy()),  // 1-5 policies
		genTestRequest(),
	).Map(func(vals []interface{}) testCase {
		policies := vals[0].([]testPolicy)
		req := vals[1].(testRequest)
		// Ensure at least 1 policy
		if len(policies) == 0 {
			policies = []testPolicy{{Effect: "permit", Principal: "*", Action: "*", Resource: "*"}}
		}
		return testCase{
			Policies: policies,
			Request:  req,
		}
	})
}

// ---------------------------------------------------------------------------
// TestProperty35 — Cedar vs OPA Decision Consistency
//
// **Validates: Requirements F13**
//
// For the same policy semantics and the same request, Cedar and OPA (reference
// implementation) must produce the same authorization decision. Both engines
// implement forbid-takes-precedence (deny-wins-over-allow) with default-deny.
// ---------------------------------------------------------------------------

func TestProperty35(t *testing.T) {
	seed := pbtSeed35()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	properties.Property("Cedar and OPA reference produce identical decisions for equivalent policies", prop.ForAll(
		func(tc testCase) (bool, error) {
			// 1. Load policies into Cedar engine
			engine := NewCedarEngine()
			cedarText := policiesToCedarText(tc.Policies)
			if err := engine.LoadPolicies(cedarText); err != nil {
				return false, fmt.Errorf("failed to load policies into Cedar: %v\npolicies:\n%s", err, cedarText)
			}

			// 2. Evaluate with Cedar
			cedarResp, err := engine.Evaluate(context.Background(), DecisionRequest{
				Principal: tc.Request.Principal,
				Action:    tc.Request.Action,
				Resource:  tc.Request.Resource,
			})
			if err != nil {
				return false, fmt.Errorf("Cedar evaluation error: %v", err)
			}

			// 3. Evaluate with reference OPA-like evaluator
			opaDecision := referenceOPADecision(tc.Policies, tc.Request)

			// 4. Verify both produce the same decision
			if cedarResp.Decision != opaDecision {
				return false, fmt.Errorf(
					"decision mismatch:\n  Cedar: %s\n  OPA ref: %s\n  Policies: %v\n  Request: {principal=%s, action=%s, resource=%s}\n  Cedar text:\n%s",
					cedarResp.Decision, opaDecision,
					tc.Policies,
					tc.Request.Principal, tc.Request.Action, tc.Request.Resource,
					cedarText,
				)
			}

			return true, nil
		},
		genTestCase(),
	))

	properties.TestingRun(t)
}
