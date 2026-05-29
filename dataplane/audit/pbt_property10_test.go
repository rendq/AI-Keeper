//go:build pbt

// Feature: ai-platform, Property 10: eventHash Consistency
//
// Generator: Random valid AuditEvent
// Oracle: eventHash == sha256(canonical(spec)); multiple serializations produce same hash;
//         any single field change must produce a different hash
// Property: P10 / Validates: F22, B12.8

package audit

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// ---------------------------------------------------------------------------
// Generators for Property 10
// ---------------------------------------------------------------------------

func genOptionalString() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),
		gen.RegexMatch("[a-z0-9]{1,20}"),
	)
}

func genNonEmptyString() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9]{1,15}")
}

func genOptionalTrace() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*EventTrace)(nil)),
		gen.Struct(reflect.TypeOf(EventTrace{}), map[string]gopter.Gen{
			"TraceID":      gen.RegexMatch("[0-9a-f]{32}"),
			"SpanID":       gen.RegexMatch("[0-9a-f]{16}"),
			"ParentSpanID": genOptionalString(),
		}).Map(func(t EventTrace) *EventTrace { return &t }),
	)
}

func genEventPrincipal() gopter.Gen {
	return gen.Struct(reflect.TypeOf(EventPrincipal{}), map[string]gopter.Gen{
		"User": gen.Const((*EventPrincipalUser)(nil)).Map(func(_ *EventPrincipalUser) *EventPrincipalUser {
			return &EventPrincipalUser{ID: "user-1", TenantID: "t-1"}
		}),
		"Agent": gen.Struct(reflect.TypeOf(EventPrincipalAgent{}), map[string]gopter.Gen{
			"Name":      genNonEmptyString(),
			"Namespace": genOptionalString(),
			"Version":   genOptionalString(),
		}),
		"ServiceAccount": gen.Const((*EventPrincipalSA)(nil)),
		"OnBehalfOf":     genOptionalString(),
		"SourceIP":       gen.OneConstOf("10.0.0.1", "192.168.1.1", "172.16.0.5"),
		"UserAgent":      gen.OneConstOf("curl/7.0", "sdk/1.2", ""),
		"Channel":        gen.OneConstOf("feishu", "slack", "api", ""),
	})
}

func genEventAction() gopter.Gen {
	return gen.Struct(reflect.TypeOf(EventAction{}), map[string]gopter.Gen{
		"Verb":     gen.OneConstOf("invoke", "query", "execute", "create", "delete"),
		"Resource": gen.OneConstOf("skill://ns/review", "tool://ns/sign", "agent://ns/bot"),
		"Method":   gen.OneConstOf("POST", "GET", ""),
	})
}

func genOptionalPolicy() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*EventPolicy)(nil)),
		gen.Struct(reflect.TypeOf(EventPolicy{}), map[string]gopter.Gen{
			"Decision":           gen.OneConstOf("allow", "deny"),
			"MatchedPolicies":    gen.Const([]string{"policy-a"}),
			"Reason":             genOptionalString(),
			"ObligationsApplied": gen.Const([]string(nil)),
			"ApprovalID":         genOptionalString(),
		}).Map(func(p EventPolicy) *EventPolicy { return &p }),
	)
}

func genOptionalRequest() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*EventRequest)(nil)),
		gen.Struct(reflect.TypeOf(EventRequest{}), map[string]gopter.Gen{
			"InputHash":      genOptionalString(),
			"InputRef":       genOptionalString(),
			"Classification": gen.OneConstOf("public", "internal", "confidential", ""),
			"Redactions":     gen.Const([]string(nil)),
			"SizeBytes":      gen.Const((*int64)(nil)),
			"Language":       gen.OneConstOf("zh", "en", ""),
		}).Map(func(r EventRequest) *EventRequest { return &r }),
	)
}

func genOptionalResponse() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*EventResponse)(nil)),
		gen.Struct(reflect.TypeOf(EventResponse{}), map[string]gopter.Gen{
			"OutputHash":     genOptionalString(),
			"OutputRef":      genOptionalString(),
			"Confidence":     gen.Const((*float64)(nil)),
			"Citations":      gen.Const([]EventCitation(nil)),
			"Classification": gen.OneConstOf("public", "internal", ""),
			"SizeBytes":      gen.Const((*int64)(nil)),
		}).Map(func(r EventResponse) *EventResponse { return &r }),
	)
}

func genOptionalOutcome() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*EventOutcome)(nil)),
		gen.Struct(reflect.TypeOf(EventOutcome{}), map[string]gopter.Gen{
			"Status":       gen.OneConstOf("success", "error", "timeout", "deny"),
			"ErrorCode":    genOptionalString(),
			"ErrorMessage": genOptionalString(),
		}).Map(func(o EventOutcome) *EventOutcome { return &o }),
	)
}

func genOptionalCost() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*EventCost)(nil)),
		gen.Struct(reflect.TypeOf(EventCost{}), map[string]gopter.Gen{
			"Tokens":     gen.Const((*EventCostTokens)(nil)),
			"Usd":        gen.OneConstOf("0.001", "0.05", "1.23"),
			"DurationMs": gen.Const((*int64)(nil)),
		}).Map(func(c EventCost) *EventCost { return &c }),
	)
}

// genAuditEvent generates a random valid AuditEvent suitable for hash testing.
func genAuditEvent() gopter.Gen {
	return gen.Struct(reflect.TypeOf(Event{}), map[string]gopter.Gen{
		"InvocationID": gen.RegexMatch("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}"),
		"Timestamp":    gen.Int64Range(1000000000, 2000000000).Map(func(v int64) time.Time { return time.Unix(v, 0).UTC() }),
		"Trace":        genOptionalTrace(),
		"Principal":    genEventPrincipal(),
		"Action":       genEventAction(),
		"Policy":       genOptionalPolicy(),
		"Request":      genOptionalRequest(),
		"Response":     genOptionalResponse(),
		"Steps":        gen.Const([]EventStep(nil)),
		"Cost":         genOptionalCost(),
		"Guardrails":   gen.Const((*EventGuardrail)(nil)),
		"Compliance":   gen.Const((*EventCompliance)(nil)),
		"Outcome":      genOptionalOutcome(),
		"EventHash":    gen.Const(""),
		"RawObjectURI": genOptionalString(),
	})
}

// ---------------------------------------------------------------------------
// TestProperty10 — eventHash Consistency
//
// **Validates: Requirements F22, B12.8**
//
// Three sub-properties:
//   1. eventHash == sha256(canonical(spec)) — hash matches canonical form
//   2. Multiple serializations of same event produce same hash (deterministic)
//   3. Any single field change must produce a different hash (tamper-evident)
// ---------------------------------------------------------------------------

func TestProperty10(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Sub-property 1: eventHash matches sha256(canonical(spec))
	properties.Property("eventHash equals sha256 of canonical form", prop.ForAll(
		func(event Event) (bool, error) {
			// Compute canonical form
			canonical, err := Canonical(&event)
			if err != nil {
				return false, fmt.Errorf("canonical failed: %w", err)
			}

			// Compute expected hash directly
			sum := sha256.Sum256(canonical)
			expectedHash := fmt.Sprintf("sha256:%x", sum)

			// Compute hash via ComputeHash
			computedHash, err := ComputeHash(&event)
			if err != nil {
				return false, fmt.Errorf("ComputeHash failed: %w", err)
			}

			if computedHash != expectedHash {
				return false, fmt.Errorf(
					"hash mismatch: ComputeHash=%q, sha256(canonical)=%q",
					computedHash, expectedHash,
				)
			}
			return true, nil
		},
		genAuditEvent(),
	))

	// Sub-property 2: Multiple serializations produce the same hash (deterministic)
	properties.Property("multiple hash computations are deterministic", prop.ForAll(
		func(event Event) (bool, error) {
			hash1, err := ComputeHash(&event)
			if err != nil {
				return false, fmt.Errorf("first ComputeHash failed: %w", err)
			}

			hash2, err := ComputeHash(&event)
			if err != nil {
				return false, fmt.Errorf("second ComputeHash failed: %w", err)
			}

			hash3, err := ComputeHash(&event)
			if err != nil {
				return false, fmt.Errorf("third ComputeHash failed: %w", err)
			}

			if hash1 != hash2 || hash2 != hash3 {
				return false, fmt.Errorf(
					"non-deterministic hashes: %q, %q, %q",
					hash1, hash2, hash3,
				)
			}
			return true, nil
		},
		genAuditEvent(),
	))

	// Sub-property 3: Any single field change produces a different hash (tamper-evident)
	properties.Property("any field change produces different hash", prop.ForAll(
		func(event Event) (bool, error) {
			originalHash, err := ComputeHash(&event)
			if err != nil {
				return false, fmt.Errorf("original ComputeHash failed: %w", err)
			}

			// Mutate invocationId
			mutated := event
			mutated.InvocationID = event.InvocationID + "-x"
			mutatedHash, err := ComputeHash(&mutated)
			if err != nil {
				return false, fmt.Errorf("mutated ComputeHash failed: %w", err)
			}
			if mutatedHash == originalHash {
				return false, fmt.Errorf("changing invocationId did not change hash")
			}

			// Mutate action verb
			mutated2 := event
			mutated2.Action.Verb = event.Action.Verb + "-changed"
			mutatedHash2, err := ComputeHash(&mutated2)
			if err != nil {
				return false, fmt.Errorf("mutated2 ComputeHash failed: %w", err)
			}
			if mutatedHash2 == originalHash {
				return false, fmt.Errorf("changing action.verb did not change hash")
			}

			// Mutate principal agent name
			mutated3 := event
			mutated3.Principal.Agent.Name = event.Principal.Agent.Name + "-mod"
			mutatedHash3, err := ComputeHash(&mutated3)
			if err != nil {
				return false, fmt.Errorf("mutated3 ComputeHash failed: %w", err)
			}
			if mutatedHash3 == originalHash {
				return false, fmt.Errorf("changing principal.agent.name did not change hash")
			}

			// Mutate timestamp
			mutated4 := event
			mutated4.Timestamp = event.Timestamp.Add(time.Second)
			mutatedHash4, err := ComputeHash(&mutated4)
			if err != nil {
				return false, fmt.Errorf("mutated4 ComputeHash failed: %w", err)
			}
			if mutatedHash4 == originalHash {
				return false, fmt.Errorf("changing timestamp did not change hash")
			}

			// Mutate rawObjectUri
			mutated5 := event
			mutated5.RawObjectURI = event.RawObjectURI + "-new"
			mutatedHash5, err := ComputeHash(&mutated5)
			if err != nil {
				return false, fmt.Errorf("mutated5 ComputeHash failed: %w", err)
			}
			if mutatedHash5 == originalHash {
				return false, fmt.Errorf("changing rawObjectUri did not change hash")
			}

			return true, nil
		},
		genAuditEvent(),
	))

	properties.TestingRun(t)
}
