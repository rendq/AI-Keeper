//go:build pbt && gm

// Feature: ai-platform, Property 34: 国密 hash 一致性
//
// Generator: Random AuditEvent (varying fields: eventID, action, principal, resource, timestamp)
// Oracle: SM3(canonical(event)) == event.eventHash when build tag=gm;
//         deterministic and tamper-evident
// Property: P34 / Validates: D7.3, F22

package gm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
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

func pbtSeed() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// AuditEvent is a simplified audit event for GM hash property testing.
type AuditEvent struct {
	EventID   string `json:"eventId"`
	Action    string `json:"action"`
	Principal string `json:"principal"`
	Resource  string `json:"resource"`
	Timestamp string `json:"timestamp"`
}

// canonicalJSON serializes an AuditEvent to canonical JSON (sorted keys, no extra whitespace).
func canonicalJSON(evt *AuditEvent) ([]byte, error) {
	// Marshal to generic map to sort keys
	raw, err := json.Marshal(evt)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return canonicalObject(m)
}

// canonicalObject produces canonical JSON for a map with sorted keys (RFC 8785 style).
func canonicalObject(obj map[string]interface{}) ([]byte, error) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, _ := json.Marshal(k)
		buf.Write(keyJSON)
		buf.WriteByte(':')
		valJSON, err := json.Marshal(obj[k])
		if err != nil {
			return nil, err
		}
		buf.Write(valJSON)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

// computeSM3Hash computes the SM3 hash of a canonical audit event, returning the hex-encoded hash.
func computeSM3Hash(evt *AuditEvent) ([]byte, error) {
	canonical, err := canonicalJSON(evt)
	if err != nil {
		return nil, err
	}
	return SM3Hash(canonical), nil
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func genEventID() gopter.Gen {
	return gen.RegexMatch("evt-[a-f0-9]{8}")
}

func genAction() gopter.Gen {
	return gen.OneConstOf(
		"model.invoke",
		"data.access",
		"agent.execute",
		"policy.evaluate",
		"knowledge.query",
	)
}

func genPrincipal() gopter.Gen {
	return gen.AlphaString().Map(func(s string) string {
		if s == "" {
			s = "default"
		}
		return "user:" + s
	})
}

func genResource() gopter.Gen {
	prefixes := []string{"model:", "agent:", "data:", "kb:", "policy:"}
	return gen.IntRange(0, len(prefixes)-1).FlatMap(func(v interface{}) gopter.Gen {
		prefix := prefixes[v.(int)]
		return gen.AlphaString().Map(func(s string) string {
			if s == "" {
				s = "default"
			}
			return prefix + s
		})
	}, reflect.TypeOf(""))
}

func genTimestamp() gopter.Gen {
	return gen.Int64Range(1000000000, 2000000000).Map(func(v int64) string {
		return time.Unix(v, 0).UTC().Format(time.RFC3339)
	})
}

func genAuditEvent() gopter.Gen {
	return gopter.CombineGens(
		genEventID(),
		genAction(),
		genPrincipal(),
		genResource(),
		genTimestamp(),
	).Map(func(values []interface{}) *AuditEvent {
		return &AuditEvent{
			EventID:   values[0].(string),
			Action:    values[1].(string),
			Principal: values[2].(string),
			Resource:  values[3].(string),
			Timestamp: values[4].(string),
		}
	})
}

// ---------------------------------------------------------------------------
// TestProperty34 — 国密 hash 一致性
//
// **Validates: Requirements D7.3, F22**
//
// Sub-properties:
//   1. SM3(canonical(event)) is deterministic: same event → same hash
//   2. Any field change → different hash (tamper-evident)
// ---------------------------------------------------------------------------

func TestProperty34(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Sub-property 1: SM3 hash is deterministic — same event always produces same hash
	properties.Property("SM3(canonical(event)) is deterministic", prop.ForAll(
		func(evt *AuditEvent) (bool, error) {
			hash1, err := computeSM3Hash(evt)
			if err != nil {
				return false, fmt.Errorf("first hash failed: %w", err)
			}

			hash2, err := computeSM3Hash(evt)
			if err != nil {
				return false, fmt.Errorf("second hash failed: %w", err)
			}

			hash3, err := computeSM3Hash(evt)
			if err != nil {
				return false, fmt.Errorf("third hash failed: %w", err)
			}

			if !bytes.Equal(hash1, hash2) || !bytes.Equal(hash2, hash3) {
				return false, fmt.Errorf(
					"non-deterministic SM3 hashes: %x, %x, %x",
					hash1, hash2, hash3,
				)
			}

			// Verify hash length is 32 bytes (SM3 output)
			if len(hash1) != 32 {
				return false, fmt.Errorf("expected 32-byte hash, got %d bytes", len(hash1))
			}

			return true, nil
		},
		genAuditEvent(),
	))

	// Sub-property 2: GMEventHasher.Hash produces same result as SM3Hash on canonical form
	properties.Property("GMEventHasher.Hash matches SM3Hash on canonical", prop.ForAll(
		func(evt *AuditEvent) (bool, error) {
			canonical, err := canonicalJSON(evt)
			if err != nil {
				return false, fmt.Errorf("canonical failed: %w", err)
			}

			directHash := SM3Hash(canonical)

			hasher := &GMEventHasher{}
			hasherHash := hasher.Hash(canonical)

			if !bytes.Equal(directHash, hasherHash) {
				return false, fmt.Errorf(
					"GMEventHasher.Hash != SM3Hash: %x vs %x",
					hasherHash, directHash,
				)
			}
			return true, nil
		},
		genAuditEvent(),
	))

	// Sub-property 3: Any single field change produces a different hash (tamper-evident)
	properties.Property("any field change produces different SM3 hash", prop.ForAll(
		func(evt *AuditEvent) (bool, error) {
			originalHash, err := computeSM3Hash(evt)
			if err != nil {
				return false, fmt.Errorf("original hash failed: %w", err)
			}

			// Mutate eventId
			mutated := *evt
			mutated.EventID = evt.EventID + "-x"
			mutatedHash, err := computeSM3Hash(&mutated)
			if err != nil {
				return false, fmt.Errorf("mutated eventId hash failed: %w", err)
			}
			if bytes.Equal(mutatedHash, originalHash) {
				return false, fmt.Errorf("changing eventId did not change hash")
			}

			// Mutate action
			mutated2 := *evt
			mutated2.Action = evt.Action + "-changed"
			mutatedHash2, err := computeSM3Hash(&mutated2)
			if err != nil {
				return false, fmt.Errorf("mutated action hash failed: %w", err)
			}
			if bytes.Equal(mutatedHash2, originalHash) {
				return false, fmt.Errorf("changing action did not change hash")
			}

			// Mutate principal
			mutated3 := *evt
			mutated3.Principal = evt.Principal + "-mod"
			mutatedHash3, err := computeSM3Hash(&mutated3)
			if err != nil {
				return false, fmt.Errorf("mutated principal hash failed: %w", err)
			}
			if bytes.Equal(mutatedHash3, originalHash) {
				return false, fmt.Errorf("changing principal did not change hash")
			}

			// Mutate resource
			mutated4 := *evt
			mutated4.Resource = evt.Resource + "-alt"
			mutatedHash4, err := computeSM3Hash(&mutated4)
			if err != nil {
				return false, fmt.Errorf("mutated resource hash failed: %w", err)
			}
			if bytes.Equal(mutatedHash4, originalHash) {
				return false, fmt.Errorf("changing resource did not change hash")
			}

			// Mutate timestamp
			mutated5 := *evt
			mutated5.Timestamp = "2099-12-31T23:59:59Z"
			mutatedHash5, err := computeSM3Hash(&mutated5)
			if err != nil {
				return false, fmt.Errorf("mutated timestamp hash failed: %w", err)
			}
			if bytes.Equal(mutatedHash5, originalHash) {
				return false, fmt.Errorf("changing timestamp did not change hash")
			}

			return true, nil
		},
		genAuditEvent(),
	))

	properties.TestingRun(t)
}
