//go:build pbt

package guardrail

import (
	"context"
	"sync"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements B6.9**
// Property P4: Guardrail evaluation order invariant
// - StagesExecuted is always in order: input before output before behavior
// - If any input rule has action=block AND is triggered, then StagesExecuted only contains "input"
// - All hits from stage N come before hits from stage N+1 in the result

// orderTrackingProvider records the order in which it is called.
type orderTrackingProvider struct {
	name    ProviderName
	mu      sync.Mutex
	callLog []Stage // records the stage of each Evaluate call in order
}

func (p *orderTrackingProvider) Name() ProviderName { return p.name }

func (p *orderTrackingProvider) Evaluate(_ context.Context, rule Rule, _ EvalRequest) (float64, bool, string, error) {
	p.mu.Lock()
	p.callLog = append(p.callLog, rule.Stage)
	p.mu.Unlock()
	// Always trigger so we can observe hits
	return 0.8, true, "triggered", nil
}

func (p *orderTrackingProvider) reset() {
	p.mu.Lock()
	p.callLog = nil
	p.mu.Unlock()
}

// blockingProvider always triggers — used for input-block scenarios.
type blockingProvider struct {
	name    ProviderName
	mu      sync.Mutex
	callLog []Stage
}

func (bp *blockingProvider) Name() ProviderName { return bp.name }

func (bp *blockingProvider) Evaluate(_ context.Context, rule Rule, _ EvalRequest) (float64, bool, string, error) {
	bp.mu.Lock()
	bp.callLog = append(bp.callLog, rule.Stage)
	bp.mu.Unlock()
	return 1.0, true, "blocked", nil
}

func (bp *blockingProvider) reset() {
	bp.mu.Lock()
	bp.callLog = nil
	bp.mu.Unlock()
}

// --- Generators ---

var allStages = []Stage{StageInput, StageOutput, StageBehavior}
var allActions = []Action{ActionAllow, ActionWarn, ActionMask, ActionEscalate, ActionBlock}
var testProviders = []ProviderName{"tracking-provider", "blocking-provider"}

func genStage() gopter.Gen {
	return gen.IntRange(0, len(allStages)-1).Map(func(i int) Stage {
		return allStages[i]
	})
}

func genAction() gopter.Gen {
	return gen.IntRange(0, len(allActions)-1).Map(func(i int) Action {
		return allActions[i]
	})
}

func genProviderName() gopter.Gen {
	return gen.IntRange(0, len(testProviders)-1).Map(func(i int) ProviderName {
		return testProviders[i]
	})
}

func genRule() gopter.Gen {
	return gopter.CombineGens(
		genStage(),
		genProviderName(),
		genAction(),
	).Map(func(v []interface{}) Rule {
		return Rule{
			Kind:     RuleCustom,
			Stage:    v[0].(Stage),
			Provider: v[1].(ProviderName),
			Action:   v[2].(Action),
		}
	})
}

func genRuleSlice() gopter.Gen {
	return gen.SliceOfN(10, genRule())
}

func TestProperty4(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 1000
	parameters.MaxSize = 10
	properties := gopter.NewProperties(parameters)

	// Set up providers
	trackingProv := &orderTrackingProvider{name: "tracking-provider"}
	blockingProv := &blockingProvider{name: "blocking-provider"}

	registry := NewProviderRegistry()
	registry.Register(trackingProv)
	registry.Register(blockingProv)

	properties.Property("stages execute in order: input < output < behavior", prop.ForAll(
		func(rules []Rule) bool {
			trackingProv.reset()
			blockingProv.reset()

			engine := NewEngine(registry, rules)
			result, err := engine.Evaluate(context.Background(), EvalRequest{
				Input:  "test input",
				Output: "test output",
			})
			if err != nil {
				return false
			}

			// Assertion 1: StagesExecuted is in correct order
			for i := 1; i < len(result.StagesExecuted); i++ {
				if StageOrder(result.StagesExecuted[i]) <= StageOrder(result.StagesExecuted[i-1]) {
					t.Logf("FAIL: StagesExecuted not in order: %v", result.StagesExecuted)
					return false
				}
			}

			// Assertion 2: If input stage has a blocking rule that triggered,
			// then only "input" is in StagesExecuted
			hasInputBlock := false
			for _, r := range rules {
				if r.Stage == StageInput && r.Action == ActionBlock {
					hasInputBlock = true
					break
				}
			}
			// Both providers always trigger, so if there's an input-block rule, it will fire
			if hasInputBlock {
				if len(result.StagesExecuted) != 1 || result.StagesExecuted[0] != StageInput {
					t.Logf("FAIL: input block present but StagesExecuted = %v", result.StagesExecuted)
					return false
				}
				// Also verify no output/behavior hits exist
				for _, hit := range result.Hits {
					if hit.Rule.Stage != StageInput {
						t.Logf("FAIL: input block but hit from stage %s", hit.Rule.Stage)
						return false
					}
				}
			}

			// Assertion 3: All hits from stage N come before hits from stage N+1
			lastStageOrder := -1
			for _, hit := range result.Hits {
				order := StageOrder(hit.Rule.Stage)
				if order < lastStageOrder {
					t.Logf("FAIL: hits not in stage order: got stage %s after stage order %d", hit.Rule.Stage, lastStageOrder)
					return false
				}
				lastStageOrder = order
			}

			return true
		},
		genRuleSlice(),
	))

	properties.TestingRun(t)
}
