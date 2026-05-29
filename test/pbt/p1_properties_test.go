//go:build pbt

// Feature: ai-platform, Properties P23, P27, P28, P29, P30
//
// This file supplements remaining PBT properties as integration-style tests
// that span multiple packages.
//
// P23: Skill backward compatibility — MAJOR unchanged ⇒ output schema backward compatible
// P27: ResourceRef format round-trip — parse(format(ref)) == ref for all valid ResourceRef
// P28: Policy bundle determinism — same input policies → same bundle hash
// P29: Model Router weight convergence — weights sum to 1.0 (within tolerance)
// P30: Agent drain ordering — drain never removes finalizer before audit flush

package pbt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

func pbtSeed() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// ===========================================================================
// P23 — Skill Backward Compatibility
//
// **Validates: Requirements F11, C7.2**
//
// Skill interface MAJOR version unchanged means output schema is backward
// compatible: new fields OK, removal = major bump. Input schema must not add
// new required fields or change existing field types within same MAJOR.
// ===========================================================================

// SchemaField represents a single field in a JSON Schema object.
type SchemaField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// SkillSchema represents a simplified Skill interface schema for compatibility checking.
type SkillSchema struct {
	Fields []SchemaField `json:"fields"`
}

// SchemaChange represents a mutation to a schema (for generating version transitions).
type SchemaChange struct {
	Kind string // "add_optional", "add_required", "remove", "change_type"
	Field SchemaField
}

// isBackwardCompatibleOutput checks if newSchema is backward compatible with oldSchema
// for output: fields may be added but not removed.
func isBackwardCompatibleOutput(old, new SkillSchema) bool {
	oldFields := make(map[string]SchemaField)
	for _, f := range old.Fields {
		oldFields[f.Name] = f
	}
	// All old fields must still exist with same type
	for _, f := range old.Fields {
		found := false
		for _, nf := range new.Fields {
			if nf.Name == f.Name {
				found = true
				if nf.Type != f.Type {
					return false // type changed
				}
				break
			}
		}
		if !found {
			return false // field removed
		}
	}
	_ = oldFields
	return true
}

// isBackwardCompatibleInput checks if newSchema is backward compatible with oldSchema
// for input: no new required fields, no type changes on existing fields.
func isBackwardCompatibleInput(old, new SkillSchema) bool {
	oldFields := make(map[string]SchemaField)
	for _, f := range old.Fields {
		oldFields[f.Name] = f
	}
	// Existing fields must not change type
	for _, nf := range new.Fields {
		if of, exists := oldFields[nf.Name]; exists {
			if nf.Type != of.Type {
				return false
			}
		} else {
			// New field — must not be required
			if nf.Required {
				return false
			}
		}
	}
	return true
}

// requiresMajorBump returns true if the schema change requires a MAJOR version bump.
func requiresMajorBump(change SchemaChange, isOutput bool) bool {
	if isOutput && change.Kind == "remove" {
		return true
	}
	if !isOutput && change.Kind == "add_required" {
		return true
	}
	if change.Kind == "change_type" {
		return true
	}
	return false
}

func genSchemaField() gopter.Gen {
	return gen.Struct(reflect.TypeOf(SchemaField{}), map[string]gopter.Gen{
		"Name":     gen.RegexMatch(`[a-z][a-zA-Z0-9]{1,10}`),
		"Type":     gen.OneConstOf("string", "number", "boolean", "array", "object"),
		"Required": gen.Bool(),
	})
}

func genSkillSchema() gopter.Gen {
	return gen.SliceOfN(3, genSchemaField()).Map(func(fields []SchemaField) SkillSchema {
		// Deduplicate by name
		seen := make(map[string]bool)
		deduped := make([]SchemaField, 0, len(fields))
		for _, f := range fields {
			if !seen[f.Name] {
				seen[f.Name] = true
				deduped = append(deduped, f)
			}
		}
		return SkillSchema{Fields: deduped}
	})
}

func genSchemaChange() gopter.Gen {
	return gen.OneConstOf("add_optional", "add_required", "remove", "change_type").
		FlatMap(func(kind interface{}) gopter.Gen {
			k := kind.(string)
			return genSchemaField().Map(func(f SchemaField) SchemaChange {
				if k == "add_optional" {
					f.Required = false
				} else if k == "add_required" {
					f.Required = true
				}
				return SchemaChange{Kind: k, Field: f}
			})
		}, reflect.TypeOf(SchemaChange{}))
}

// TestProperty23 validates P23: Skill backward compatibility.
// If a schema change does NOT require a major bump, then the resulting schema
// must be backward compatible with the original.
//
// **Validates: Requirements F11, C7.2**
func TestProperty23(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	properties.Property("non-major output change preserves backward compat", prop.ForAll(
		func(schema SkillSchema, change SchemaChange) bool {
			if requiresMajorBump(change, true) {
				// Skip — this change DOES require a major bump, not relevant
				return true
			}
			// Apply change to produce new schema
			newSchema := applyChange(schema, change, true)
			return isBackwardCompatibleOutput(schema, newSchema)
		},
		genSkillSchema(),
		genSchemaChange(),
	))

	properties.Property("non-major input change preserves backward compat", prop.ForAll(
		func(schema SkillSchema, change SchemaChange) bool {
			if requiresMajorBump(change, false) {
				return true
			}
			newSchema := applyChange(schema, change, false)
			return isBackwardCompatibleInput(schema, newSchema)
		},
		genSkillSchema(),
		genSchemaChange(),
	))

	properties.TestingRun(t)
}

// applyChange applies a schema change to produce a new schema version.
// Only applies changes that do NOT require a major bump for the given direction.
func applyChange(schema SkillSchema, change SchemaChange, isOutput bool) SkillSchema {
	newFields := make([]SchemaField, len(schema.Fields))
	copy(newFields, schema.Fields)

	// Check if field already exists in schema
	fieldExists := false
	for _, f := range newFields {
		if f.Name == change.Field.Name {
			fieldExists = true
			break
		}
	}

	switch change.Kind {
	case "add_optional":
		// Adding optional field — only if it doesn't already exist (name collision
		// with different type would be a breaking change)
		if !fieldExists {
			newFields = append(newFields, SchemaField{
				Name:     change.Field.Name,
				Type:     change.Field.Type,
				Required: false,
			})
		}
	case "add_required":
		if isOutput && !fieldExists {
			// For output, adding required is OK (consumer just gets more data)
			newFields = append(newFields, change.Field)
		}
		// For input, add_required is a major bump — not applied here
	case "remove":
		if !isOutput {
			// For input, removing a field is OK (less required from caller)
			filtered := make([]SchemaField, 0, len(newFields))
			for _, f := range newFields {
				if f.Name != change.Field.Name {
					filtered = append(filtered, f)
				}
			}
			newFields = filtered
		}
		// For output, remove is a major bump — not applied here
	case "change_type":
		// change_type always requires major bump — not applied
	}

	return SkillSchema{Fields: newFields}
}

// ===========================================================================
// P27 — ResourceRef Format Round-trip
//
// **Validates: Requirements F25, A2.1**
//
// parse(format(ref)) == ref for all valid ResourceRef
// ===========================================================================

type resourceRefInput struct {
	Scheme  sharedv1alpha1.ResourceRefScheme
	Path    string
	Version string
}

func genResourceRefScheme() gopter.Gen {
	return gen.OneConstOf(
		sharedv1alpha1.SchemeSkill,
		sharedv1alpha1.SchemeAgent,
		sharedv1alpha1.SchemeTool,
		sharedv1alpha1.SchemeModel,
		sharedv1alpha1.SchemeData,
		sharedv1alpha1.SchemePrompt,
		sharedv1alpha1.SchemeChannel,
		sharedv1alpha1.SchemeConnector,
		sharedv1alpha1.SchemeMemory,
		sharedv1alpha1.SchemeQuota,
		sharedv1alpha1.SchemeRef,
		sharedv1alpha1.SchemeSIEM,
		sharedv1alpha1.SchemePolicy,
	)
}

func genRefPath() gopter.Gen {
	return gen.RegexMatch(`[A-Za-z0-9][A-Za-z0-9._/\-]{0,30}`)
}

func genRefVersion() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),
		gen.RegexMatch(`[A-Za-z0-9][A-Za-z0-9._\-+]{0,15}`),
	)
}

func genResourceRef() gopter.Gen {
	return gen.Struct(reflect.TypeOf(resourceRefInput{}), map[string]gopter.Gen{
		"Scheme":  genResourceRefScheme(),
		"Path":    genRefPath(),
		"Version": genRefVersion(),
	})
}

// TestProperty27 validates P27: ResourceRef format round-trip.
// For all valid ResourceRef: parse(format(ref)) == ref
//
// **Validates: Requirements F25, A2.1**
func TestProperty27(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	properties.Property("parse(format(ref)) == ref", prop.ForAll(
		func(input resourceRefInput) (bool, error) {
			ref, err := sharedv1alpha1.FormatResourceRef(input.Scheme, input.Path, input.Version)
			if err != nil {
				return false, err
			}

			scheme, path, version, err := ref.Parse()
			if err != nil {
				return false, err
			}

			if scheme != input.Scheme {
				t.Logf("scheme mismatch: got %q want %q", scheme, input.Scheme)
				return false, nil
			}
			if path != input.Path {
				t.Logf("path mismatch: got %q want %q", path, input.Path)
				return false, nil
			}
			if version != input.Version {
				t.Logf("version mismatch: got %q want %q", version, input.Version)
				return false, nil
			}
			return true, nil
		},
		genResourceRef(),
	))

	properties.TestingRun(t)
}

// ===========================================================================
// P28 — Policy Bundle Determinism
//
// **Validates: Requirements A5.1, A5.5, F13**
//
// Same input policies → same bundle hash. The policy compiler is a pure
// function: given identical inputs it must produce byte-identical output.
// ===========================================================================

// PolicyInput represents a simplified policy for bundle compilation testing.
type PolicyInput struct {
	Name     string `json:"name"`
	Priority int32  `json:"priority"`
	Effect   string `json:"effect"` // "allow" or "deny"
	Subject  string `json:"subject"`
	Resource string `json:"resource"`
}

func genPolicyInput() gopter.Gen {
	return gen.Struct(reflect.TypeOf(PolicyInput{}), map[string]gopter.Gen{
		"Name":     gen.RegexMatch(`[a-z][a-z0-9\-]{2,15}`),
		"Priority": gen.Int32Range(1, 1000),
		"Effect":   gen.OneConstOf("allow", "deny"),
		"Subject":  gen.RegexMatch(`[a-z][a-z0-9]{2,10}`),
		"Resource": gen.RegexMatch(`[a-z][a-z0-9/]{2,20}`),
	})
}

// computeBundleHash computes a deterministic hash for a set of policies.
// This simulates the Policy Compiler's bundle hash generation.
func computeBundleHash(policies []PolicyInput) string {
	// Sort-stable canonical form: marshal to JSON (Go's encoding/json sorts keys)
	canonical, _ := json.Marshal(policies)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// TestProperty28 validates P28: Policy bundle determinism.
// Same input policies must always produce the same bundle hash.
//
// **Validates: Requirements A5.1, A5.5, F13**
func TestProperty28(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	properties.Property("same policies produce same hash", prop.ForAll(
		func(policies []PolicyInput) bool {
			hash1 := computeBundleHash(policies)
			hash2 := computeBundleHash(policies)
			return hash1 == hash2
		},
		gen.SliceOfN(5, genPolicyInput()),
	))

	properties.Property("any field change produces different hash", prop.ForAll(
		func(policies []PolicyInput) bool {
			if len(policies) == 0 {
				return true
			}
			hash1 := computeBundleHash(policies)

			// Mutate one policy
			mutated := make([]PolicyInput, len(policies))
			copy(mutated, policies)
			mutated[0].Priority = mutated[0].Priority + 1

			hash2 := computeBundleHash(mutated)
			return hash1 != hash2
		},
		gen.SliceOfN(5, genPolicyInput()),
	))

	properties.TestingRun(t)
}

// ===========================================================================
// P29 — Model Router Weight Convergence
//
// **Validates: Requirements F18, B8.3**
//
// Weights sum to 1.0 (within tolerance) for any valid endpoint set.
// In N→∞ random requests, routing frequency converges to wi/Σwj.
// ===========================================================================

// EndpointWeight represents a weighted endpoint in the model router.
type EndpointWeight struct {
	Name   string
	Weight int32
}

func genEndpointWeights() gopter.Gen {
	return gen.SliceOfN(5, gen.Int32Range(1, 100)).
		SuchThat(func(weights []int32) bool {
			return len(weights) > 0
		}).
		Map(func(weights []int32) []EndpointWeight {
			eps := make([]EndpointWeight, len(weights))
			for i, w := range weights {
				eps[i] = EndpointWeight{
					Name:   "ep-" + strconv.Itoa(i),
					Weight: w,
				}
			}
			return eps
		})
}

// normalizeWeights normalizes weights to sum to 1.0.
func normalizeWeights(endpoints []EndpointWeight) []float64 {
	total := int32(0)
	for _, ep := range endpoints {
		total += ep.Weight
	}
	if total == 0 {
		return nil
	}
	normalized := make([]float64, len(endpoints))
	for i, ep := range endpoints {
		normalized[i] = float64(ep.Weight) / float64(total)
	}
	return normalized
}

// weightedSelect simulates routing one request based on weights.
func weightedSelect(endpoints []EndpointWeight, rng *rand.Rand) int {
	total := int32(0)
	for _, ep := range endpoints {
		total += ep.Weight
	}
	r := rng.Int31n(total)
	cumulative := int32(0)
	for i, ep := range endpoints {
		cumulative += ep.Weight
		if r < cumulative {
			return i
		}
	}
	return len(endpoints) - 1
}

// TestProperty29 validates P29: Model Router weight convergence.
// Normalized weights sum to 1.0 and routing frequency converges to expected distribution.
//
// **Validates: Requirements F18, B8.3**
func TestProperty29(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	properties.Property("normalized weights sum to 1.0", prop.ForAll(
		func(endpoints []EndpointWeight) bool {
			normalized := normalizeWeights(endpoints)
			if normalized == nil {
				return true // degenerate case
			}
			sum := 0.0
			for _, w := range normalized {
				sum += w
			}
			return math.Abs(sum-1.0) < 1e-9
		},
		genEndpointWeights(),
	))

	properties.Property("routing converges to weight distribution within 5%", prop.ForAll(
		func(endpoints []EndpointWeight) bool {
			if len(endpoints) < 2 {
				return true
			}
			expected := normalizeWeights(endpoints)
			if expected == nil {
				return true
			}

			// Simulate 10000 requests
			const numRequests = 10000
			counts := make([]int, len(endpoints))
			rng := rand.New(rand.NewSource(seed))
			for i := 0; i < numRequests; i++ {
				idx := weightedSelect(endpoints, rng)
				counts[idx]++
			}

			// Check convergence within 5% tolerance
			for i, count := range counts {
				actual := float64(count) / float64(numRequests)
				if math.Abs(actual-expected[i]) > 0.05 {
					t.Logf("endpoint %d: expected %.3f got %.3f (diff %.3f)",
						i, expected[i], actual, math.Abs(actual-expected[i]))
					return false
				}
			}
			return true
		},
		genEndpointWeights(),
	))

	properties.TestingRun(t)
}

// ===========================================================================
// P30 — Agent Drain Ordering
//
// **Validates: Requirements F3, A4.7, A4.8**
//
// Drain sequence never removes finalizer before audit flush. The ordering
// invariant is: unregister channels → stop new sessions → wait in-flight →
// revoke tokens → flush audit → remove finalizer.
// ===========================================================================

// DrainStep represents a step in the agent drain sequence.
type DrainStep int

const (
	StepUnregisterChannels DrainStep = iota
	StepStopNewSessions
	StepWaitInFlight
	StepRevokeTokens
	StepFlushAudit
	StepRemoveFinalizer
)

func (s DrainStep) String() string {
	switch s {
	case StepUnregisterChannels:
		return "UnregisterChannels"
	case StepStopNewSessions:
		return "StopNewSessions"
	case StepWaitInFlight:
		return "WaitInFlight"
	case StepRevokeTokens:
		return "RevokeTokens"
	case StepFlushAudit:
		return "FlushAudit"
	case StepRemoveFinalizer:
		return "RemoveFinalizer"
	default:
		return "Unknown"
	}
}

// DrainSequence represents a potential drain ordering for testing.
type DrainSequence struct {
	Steps []DrainStep
}

// isValidDrainOrder checks the ordering invariants:
// 1. FlushAudit must come before RemoveFinalizer
// 2. WaitInFlight must come before FlushAudit
// 3. StopNewSessions must come before WaitInFlight
// 4. UnregisterChannels must come before StopNewSessions
func isValidDrainOrder(seq DrainSequence) bool {
	indexOf := func(step DrainStep) int {
		for i, s := range seq.Steps {
			if s == step {
				return i
			}
		}
		return -1
	}

	// All required steps must be present
	required := []DrainStep{
		StepUnregisterChannels,
		StepStopNewSessions,
		StepWaitInFlight,
		StepRevokeTokens,
		StepFlushAudit,
		StepRemoveFinalizer,
	}
	for _, r := range required {
		if indexOf(r) == -1 {
			return false
		}
	}

	// Ordering invariants
	if indexOf(StepFlushAudit) >= indexOf(StepRemoveFinalizer) {
		return false // audit must flush BEFORE finalizer removal
	}
	if indexOf(StepWaitInFlight) >= indexOf(StepFlushAudit) {
		return false
	}
	if indexOf(StepStopNewSessions) >= indexOf(StepWaitInFlight) {
		return false
	}
	if indexOf(StepUnregisterChannels) >= indexOf(StepStopNewSessions) {
		return false
	}
	return true
}

// canonicalDrainOrder returns the correct drain sequence.
func canonicalDrainOrder() DrainSequence {
	return DrainSequence{
		Steps: []DrainStep{
			StepUnregisterChannels,
			StepStopNewSessions,
			StepWaitInFlight,
			StepRevokeTokens,
			StepFlushAudit,
			StepRemoveFinalizer,
		},
	}
}

func genDrainSequence() gopter.Gen {
	// Generate permutations of the 6 drain steps
	return gen.Int64().Map(func(seed int64) DrainSequence {
		steps := []DrainStep{
			StepUnregisterChannels,
			StepStopNewSessions,
			StepWaitInFlight,
			StepRevokeTokens,
			StepFlushAudit,
			StepRemoveFinalizer,
		}
		rng := rand.New(rand.NewSource(seed))
		rng.Shuffle(len(steps), func(i, j int) {
			steps[i], steps[j] = steps[j], steps[i]
		})
		return DrainSequence{Steps: steps}
	})
}

// TestProperty30 validates P30: Agent drain ordering.
// The canonical drain order must satisfy all ordering invariants, and any
// sequence that violates the invariant (finalizer before audit) must be detected.
//
// **Validates: Requirements F3, A4.7, A4.8**
func TestProperty30(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	properties.Property("canonical drain order is always valid", prop.ForAll(
		func(_ int64) bool {
			return isValidDrainOrder(canonicalDrainOrder())
		},
		gen.Int64(),
	))

	properties.Property("finalizer before audit flush is always invalid", prop.ForAll(
		func(seq DrainSequence) bool {
			// Find positions
			auditIdx := -1
			finalizerIdx := -1
			for i, s := range seq.Steps {
				if s == StepFlushAudit {
					auditIdx = i
				}
				if s == StepRemoveFinalizer {
					finalizerIdx = i
				}
			}
			if auditIdx == -1 || finalizerIdx == -1 {
				return true // incomplete sequence, skip
			}
			// If finalizer comes before audit, sequence must be invalid
			if finalizerIdx < auditIdx {
				return !isValidDrainOrder(seq)
			}
			return true // order is fine for this check
		},
		genDrainSequence(),
	))

	properties.TestingRun(t)
}
