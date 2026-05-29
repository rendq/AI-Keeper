//go:build pbt

// Feature: ai-platform, Property 26: ResourceRef round-trip
//
// Generator: Random valid ResourceRef (13 schemes × paths × @version)
// Oracle: parse(format(r)) == r
// Property: P26 / Validates: A2.1

package v1alpha1

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// ---------------------------------------------------------------------------
// TestProperty26 — ResourceRef Parse/Format Round-trip
//
// **Validates: Requirements A2.1**
//
// For any well-formed ResourceRef r: parse(format(r)) == r
// ---------------------------------------------------------------------------

func pbtSeed() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// genScheme generates one of the 13 valid ResourceRef schemes.
func genScheme() gopter.Gen {
	return gen.OneConstOf(
		SchemeSkill,
		SchemeAgent,
		SchemeTool,
		SchemeModel,
		SchemeData,
		SchemePrompt,
		SchemeChannel,
		SchemeConnector,
		SchemeMemory,
		SchemeQuota,
		SchemeRef,
		SchemeSIEM,
		SchemePolicy,
	)
}

// genPath generates a valid path segment matching [A-Za-z0-9._/\-]+.
func genPath() gopter.Gen {
	return gen.RegexMatch(`[A-Za-z0-9][A-Za-z0-9._/\-]{0,30}`)
}

// genVersion generates a valid optional version matching [A-Za-z0-9._\-+]+.
func genVersion() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""), // no version
		gen.RegexMatch(`[A-Za-z0-9][A-Za-z0-9._\-+]{0,15}`),
	)
}

type resourceRefInput struct {
	Scheme  ResourceRefScheme
	Path    string
	Version string
}

func genResourceRefInput() gopter.Gen {
	return gen.Struct(reflect.TypeOf(resourceRefInput{}), map[string]gopter.Gen{
		"Scheme":  genScheme(),
		"Path":    genPath(),
		"Version": genVersion(),
	})
}

func TestProperty26(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	properties.Property("parse(format(r)) == r", prop.ForAll(
		func(input resourceRefInput) (bool, error) {
			// Format the ResourceRef from components
			ref, err := FormatResourceRef(input.Scheme, input.Path, input.Version)
			if err != nil {
				t.Logf("FormatResourceRef failed: scheme=%q path=%q version=%q err=%v",
					input.Scheme, input.Path, input.Version, err)
				return false, err
			}

			// Parse it back
			scheme, path, version, err := ref.Parse()
			if err != nil {
				t.Logf("Parse failed: ref=%q err=%v", ref, err)
				return false, err
			}

			// Verify round-trip equality
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
		genResourceRefInput(),
	))

	properties.TestingRun(t)
}
