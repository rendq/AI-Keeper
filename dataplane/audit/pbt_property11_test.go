//go:build pbt

// Feature: ai-platform, Property 11: AuditEvent Round-trip Serialization
//
// Generator: Random valid AuditEvent
// Oracle: parse(serialize(e)) == e, JSON / JSONL round-trip
// Property: P11 / Validates: F22, B12.8

package audit

import (
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/prop"
)

// ---------------------------------------------------------------------------
// TestProperty11 — AuditEvent Round-trip Serialization
//
// **Validates: Requirements F22, B12.8**
//
// Two sub-properties:
//   1. JSON round-trip: ParseJSON(SerializeJSON(e)) == e
//   2. JSONL round-trip: ParseJSONL(SerializeJSONL(e))[0] == e
// ---------------------------------------------------------------------------

func TestProperty11(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Sub-property 1: JSON round-trip — ParseJSON(SerializeJSON(e)) == e
	properties.Property("JSON round-trip: ParseJSON(SerializeJSON(e)) equals original", prop.ForAll(
		func(event Event) (bool, error) {
			// Normalize: truncate timestamp to second precision (JSON marshals to second)
			event.Timestamp = event.Timestamp.Truncate(time.Second).UTC()

			data, err := SerializeJSON(&event)
			if err != nil {
				return false, err
			}

			parsed, err := ParseJSON(data)
			if err != nil {
				return false, err
			}

			// Use time.Equal for timestamp comparison, then compare the rest
			if !event.Timestamp.Equal(parsed.Timestamp) {
				return false, nil
			}

			// Set timestamps to same value for DeepEqual on remaining fields
			parsed.Timestamp = event.Timestamp

			if !reflect.DeepEqual(event, *parsed) {
				return false, nil
			}

			return true, nil
		},
		genAuditEvent(),
	))

	// Sub-property 2: JSONL round-trip — ParseJSONL(SerializeJSONL(e))[0] == e
	properties.Property("JSONL round-trip: ParseJSONL(SerializeJSONL(e))[0] equals original", prop.ForAll(
		func(event Event) (bool, error) {
			// Normalize: truncate timestamp to second precision
			event.Timestamp = event.Timestamp.Truncate(time.Second).UTC()

			data, err := SerializeJSONL(&event)
			if err != nil {
				return false, err
			}

			events, err := ParseJSONL(data)
			if err != nil {
				return false, err
			}

			if len(events) != 1 {
				return false, nil
			}

			parsed := events[0]

			// Use time.Equal for timestamp comparison
			if !event.Timestamp.Equal(parsed.Timestamp) {
				return false, nil
			}

			// Set timestamps to same value for DeepEqual on remaining fields
			parsed.Timestamp = event.Timestamp

			if !reflect.DeepEqual(event, *parsed) {
				return false, nil
			}

			return true, nil
		},
		genAuditEvent(),
	))

	properties.TestingRun(t)
}
