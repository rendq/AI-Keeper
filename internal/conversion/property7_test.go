//go:build pbt

// Feature: ai-platform, Property 7: Conversion round-trip 等价
//
// Generator: Random valid Skill/Agent v1alpha1 specs
// Oracle: alpha→beta→alpha preserves all v1alpha1 fields exactly (round-trip equivalence)
// Property: P7 / Validates: A11.2

package conversion

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// ---------------------------------------------------------------------------
// TestProperty7 — Conversion Round-trip Equivalence
//
// **Validates: Requirements A11.2**
//
// For any valid v1alpha1 Skill s: ConvertSkillBetaToAlpha(ConvertSkillAlphaToBeta(s)) == s
// For any valid v1alpha1 Agent a: ConvertAgentBetaToAlpha(ConvertAgentAlphaToBeta(a)) == a
// ---------------------------------------------------------------------------

func pbtSeed7() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// --- Generators ---

func genSemVer() gopter.Gen {
	return gen.RegexMatch(`[1-9]\.[0-9]\.[0-9]`).Map(func(s string) shared.SemVer {
		return shared.SemVer(s)
	})
}

func genStage() gopter.Gen {
	return gen.OneConstOf(
		shared.StageExperimental,
		shared.StageBeta,
		shared.StageStable,
		shared.StageDeprecated,
	)
}

func genImplType() gopter.Gen {
	return gen.OneConstOf("function", "workflow", "agentic", "mcp_tool", "external_api")
}

func genOptionalString() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),
		gen.RegexMatch(`[a-z][a-z0-9]{0,14}`),
	)
}

func genResourceRef() gopter.Gen {
	schemes := []string{"skill", "model", "tool", "data", "prompt"}
	return gen.OneConstOf(schemes[0], schemes[1], schemes[2], schemes[3], schemes[4]).FlatMap(func(v interface{}) gopter.Gen {
		scheme := v.(string)
		return gen.RegexMatch(`[a-z][a-z0-9]{1,10}`).Map(func(path string) shared.ResourceRef {
			return shared.ResourceRef(scheme + "://" + path)
		})
	}, reflect.TypeOf(""))
}

func genOptionalRuntime() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*skillv1alpha1.SkillRuntime)(nil)),
		gen.Struct(reflect.TypeOf(skillv1alpha1.SkillRuntime{}), map[string]gopter.Gen{
			"Engine":     genOptionalString(),
			"Entrypoint": genOptionalString(),
			"Image":      genOptionalString(),
		}).Map(func(r skillv1alpha1.SkillRuntime) *skillv1alpha1.SkillRuntime {
			return &r
		}),
	)
}

func genSkillImplementation() gopter.Gen {
	return gen.Struct(reflect.TypeOf(skillv1alpha1.SkillImplementation{}), map[string]gopter.Gen{
		"Type":           genImplType(),
		"Runtime":        genOptionalRuntime(),
		"PromptTemplate": gen.Const((*skillv1alpha1.SkillPromptTemplate)(nil)),
		"Requires":       gen.Const((*skillv1alpha1.SkillRequires)(nil)),
	})
}

func genSkillSpec() gopter.Gen {
	return gopter.CombineGens(
		genSemVer(),
		genStage(),
		genSkillImplementation(),
	).Map(func(vals []interface{}) skillv1alpha1.SkillSpec {
		return skillv1alpha1.SkillSpec{
			Version:        vals[0].(shared.SemVer),
			Stability:      vals[1].(shared.Stage),
			Implementation: vals[2].(skillv1alpha1.SkillImplementation),
		}
	})
}

func genSkillAlpha() gopter.Gen {
	return gopter.CombineGens(
		gen.RegexMatch(`[a-z][a-z0-9]{1,10}`),
		genSkillSpec(),
	).Map(func(vals []interface{}) *skillv1alpha1.Skill {
		name := vals[0].(string)
		spec := vals[1].(skillv1alpha1.SkillSpec)
		return &skillv1alpha1.Skill{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "skill.ai-keeper.io/v1alpha1",
				Kind:       "Skill",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: spec,
		}
	})
}

// --- Agent generators ---

func genPattern() gopter.Gen {
	return gen.OneConstOf("react", "plan_execute", "reflection", "workflow", "tool_calling", "multi_agent")
}

func genOptionalInt32() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*int32)(nil)),
		gen.Int32Range(1, 100).Map(func(v int32) *int32 { return &v }),
	)
}

func genAgentSkillBindings() gopter.Gen {
	return gen.SliceOfN(3, genResourceRef()).Map(func(refs []shared.ResourceRef) []agentv1alpha1.AgentSkillBinding {
		bindings := make([]agentv1alpha1.AgentSkillBinding, 0, len(refs))
		for _, ref := range refs {
			bindings = append(bindings, agentv1alpha1.AgentSkillBinding{
				Ref: ref,
			})
		}
		if len(bindings) == 0 {
			bindings = append(bindings, agentv1alpha1.AgentSkillBinding{
				Ref: shared.ResourceRef("skill://default"),
			})
		}
		return bindings
	})
}

func genAgentChannels() gopter.Gen {
	kinds := []string{"feishu", "wecom", "dingtalk", "slack", "teams", "web", "api", "sdk"}
	return gen.SliceOfN(2, gen.OneConstOf(kinds[0], kinds[1], kinds[2], kinds[3], kinds[4], kinds[5], kinds[6], kinds[7])).Map(func(ks []string) []agentv1alpha1.AgentChannel {
		channels := make([]agentv1alpha1.AgentChannel, len(ks))
		for i, k := range ks {
			channels[i] = agentv1alpha1.AgentChannel{Kind: k}
		}
		return channels
	})
}

func genAgentAlpha() gopter.Gen {
	return gopter.CombineGens(
		gen.RegexMatch(`[a-z][a-z0-9]{1,10}`),
		gen.RegexMatch(`[A-Z][a-zA-Z ]{1,20}`),
		genPattern(),
		genOptionalInt32(),
		genAgentSkillBindings(),
		genAgentChannels(),
	).Map(func(vals []interface{}) *agentv1alpha1.Agent {
		name := vals[0].(string)
		displayName := vals[1].(string)
		pattern := vals[2].(string)
		maxSteps := vals[3].(*int32)
		skills := vals[4].([]agentv1alpha1.AgentSkillBinding)
		channels := vals[5].([]agentv1alpha1.AgentChannel)

		return &agentv1alpha1.Agent{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "agent.ai-keeper.io/v1alpha1",
				Kind:       "Agent",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: agentv1alpha1.AgentSpec{
				DisplayName: displayName,
				Identity:    agentv1alpha1.AgentIdentity{ServiceAccount: name + "-sa"},
				Skills:      skills,
				Runtime: agentv1alpha1.AgentRuntime{
					Pattern:  pattern,
					MaxSteps: maxSteps,
				},
				Channels: channels,
			},
		}
	})
}

// --- Property test ---

func TestProperty7(t *testing.T) {
	seed := pbtSeed7()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Sub-property 1: Skill round-trip
	properties.Property("skill: alpha→beta→alpha preserves all v1alpha1 fields", prop.ForAll(
		func(original *skillv1alpha1.Skill) (bool, error) {
			beta, _ := ConvertSkillAlphaToBeta(original)
			roundtripped, _ := ConvertSkillBetaToAlpha(beta)

			// TypeMeta should be restored to alpha
			if roundtripped.APIVersion != "skill.ai-keeper.io/v1alpha1" {
				t.Logf("APIVersion mismatch: got %q", roundtripped.APIVersion)
				return false, nil
			}
			if roundtripped.Kind != "Skill" {
				t.Logf("Kind mismatch: got %q", roundtripped.Kind)
				return false, nil
			}
			// ObjectMeta
			if roundtripped.Name != original.Name {
				t.Logf("Name mismatch: got %q want %q", roundtripped.Name, original.Name)
				return false, nil
			}
			if roundtripped.Namespace != original.Namespace {
				t.Logf("Namespace mismatch: got %q want %q", roundtripped.Namespace, original.Namespace)
				return false, nil
			}
			// Spec fields
			if roundtripped.Spec.Version != original.Spec.Version {
				t.Logf("Version mismatch: got %q want %q", roundtripped.Spec.Version, original.Spec.Version)
				return false, nil
			}
			if roundtripped.Spec.Stability != original.Spec.Stability {
				t.Logf("Stability mismatch: got %q want %q", roundtripped.Spec.Stability, original.Spec.Stability)
				return false, nil
			}
			if roundtripped.Spec.Implementation.Type != original.Spec.Implementation.Type {
				t.Logf("Implementation.Type mismatch: got %q want %q",
					roundtripped.Spec.Implementation.Type, original.Spec.Implementation.Type)
				return false, nil
			}
			// Runtime
			if original.Spec.Implementation.Runtime == nil && roundtripped.Spec.Implementation.Runtime != nil {
				t.Logf("Runtime should be nil but got %+v", roundtripped.Spec.Implementation.Runtime)
				return false, nil
			}
			if original.Spec.Implementation.Runtime != nil {
				if roundtripped.Spec.Implementation.Runtime == nil {
					t.Logf("Runtime lost in round-trip")
					return false, nil
				}
				if roundtripped.Spec.Implementation.Runtime.Engine != original.Spec.Implementation.Runtime.Engine {
					t.Logf("Runtime.Engine mismatch")
					return false, nil
				}
				if roundtripped.Spec.Implementation.Runtime.Entrypoint != original.Spec.Implementation.Runtime.Entrypoint {
					t.Logf("Runtime.Entrypoint mismatch")
					return false, nil
				}
				if roundtripped.Spec.Implementation.Runtime.Image != original.Spec.Implementation.Runtime.Image {
					t.Logf("Runtime.Image mismatch")
					return false, nil
				}
			}
			return true, nil
		},
		genSkillAlpha(),
	))

	// Sub-property 2: Agent round-trip
	properties.Property("agent: alpha→beta→alpha preserves all v1alpha1 fields", prop.ForAll(
		func(original *agentv1alpha1.Agent) (bool, error) {
			beta, _ := ConvertAgentAlphaToBeta(original)
			roundtripped, _ := ConvertAgentBetaToAlpha(beta)

			// TypeMeta
			if roundtripped.APIVersion != "agent.ai-keeper.io/v1alpha1" {
				t.Logf("APIVersion mismatch: got %q", roundtripped.APIVersion)
				return false, nil
			}
			if roundtripped.Kind != "Agent" {
				t.Logf("Kind mismatch: got %q", roundtripped.Kind)
				return false, nil
			}
			// ObjectMeta
			if roundtripped.Name != original.Name {
				t.Logf("Name mismatch: got %q want %q", roundtripped.Name, original.Name)
				return false, nil
			}
			if roundtripped.Namespace != original.Namespace {
				t.Logf("Namespace mismatch")
				return false, nil
			}
			// Spec
			if roundtripped.Spec.DisplayName != original.Spec.DisplayName {
				t.Logf("DisplayName mismatch: got %q want %q",
					roundtripped.Spec.DisplayName, original.Spec.DisplayName)
				return false, nil
			}
			if roundtripped.Spec.Identity.ServiceAccount != original.Spec.Identity.ServiceAccount {
				t.Logf("ServiceAccount mismatch")
				return false, nil
			}
			if roundtripped.Spec.Runtime.Pattern != original.Spec.Runtime.Pattern {
				t.Logf("Pattern mismatch: got %q want %q",
					roundtripped.Spec.Runtime.Pattern, original.Spec.Runtime.Pattern)
				return false, nil
			}
			// MaxSteps
			if (original.Spec.Runtime.MaxSteps == nil) != (roundtripped.Spec.Runtime.MaxSteps == nil) {
				t.Logf("MaxSteps nil mismatch")
				return false, nil
			}
			if original.Spec.Runtime.MaxSteps != nil &&
				*roundtripped.Spec.Runtime.MaxSteps != *original.Spec.Runtime.MaxSteps {
				t.Logf("MaxSteps value mismatch")
				return false, nil
			}
			// Skills count and refs
			if len(roundtripped.Spec.Skills) != len(original.Spec.Skills) {
				t.Logf("Skills count mismatch: got %d want %d",
					len(roundtripped.Spec.Skills), len(original.Spec.Skills))
				return false, nil
			}
			for i, s := range original.Spec.Skills {
				if roundtripped.Spec.Skills[i].Ref != s.Ref {
					t.Logf("Skill[%d].Ref mismatch", i)
					return false, nil
				}
			}
			// Channels count and kinds
			if len(roundtripped.Spec.Channels) != len(original.Spec.Channels) {
				t.Logf("Channels count mismatch: got %d want %d",
					len(roundtripped.Spec.Channels), len(original.Spec.Channels))
				return false, nil
			}
			for i, c := range original.Spec.Channels {
				if roundtripped.Spec.Channels[i].Kind != c.Kind {
					t.Logf("Channel[%d].Kind mismatch", i)
					return false, nil
				}
			}
			return true, nil
		},
		genAgentAlpha(),
	))

	properties.TestingRun(t)
}
