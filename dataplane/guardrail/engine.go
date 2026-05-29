package guardrail

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Engine is the guardrail evaluation engine.
// It executes rules serially by stage (input → output → behavior),
// concurrently within each stage, and aggregates results.
type Engine struct {
	mu       sync.RWMutex
	rules    []Rule
	registry *ProviderRegistry
}

// NewEngine creates a guardrail engine with the given provider registry and initial rules.
func NewEngine(registry *ProviderRegistry, rules []Rule) *Engine {
	return &Engine{
		rules:    rules,
		registry: registry,
	}
}

// SetRules atomically replaces the current rule set (used by hot reload).
func (e *Engine) SetRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = rules
}

// Rules returns a snapshot of current rules.
func (e *Engine) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	dst := make([]Rule, len(e.rules))
	copy(dst, e.rules)
	return dst
}

// Evaluate runs all guardrail rules in stage order: input → output → behavior.
// Within each stage, rules execute concurrently.
// If input stage blocks, output and behavior stages are skipped.
// Returns the aggregated result with all hits and their scores.
func (e *Engine) Evaluate(ctx context.Context, req EvalRequest) (*EvalResult, error) {
	e.mu.RLock()
	rules := make([]Rule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	// Group rules by stage
	stageRules := map[Stage][]Rule{
		StageInput:    {},
		StageOutput:   {},
		StageBehavior: {},
	}
	for _, r := range rules {
		stageRules[r.Stage] = append(stageRules[r.Stage], r)
	}

	result := &EvalResult{
		FinalAction: ActionAllow,
	}

	// Execute stages in order
	stages := []Stage{StageInput, StageOutput, StageBehavior}
	for _, stage := range stages {
		stageHits, err := e.evaluateStage(ctx, stage, stageRules[stage], req)
		if err != nil {
			return nil, fmt.Errorf("guardrail stage %s failed: %w", stage, err)
		}

		result.Hits = append(result.Hits, stageHits...)
		result.StagesExecuted = append(result.StagesExecuted, stage)

		// Update final action based on hits from this stage
		for _, hit := range stageHits {
			if ActionPriority(hit.Action) > ActionPriority(result.FinalAction) {
				result.FinalAction = hit.Action
			}
		}

		// If input stage blocks, skip remaining stages (don't call LLM)
		if stage == StageInput && result.FinalAction == ActionBlock {
			break
		}
	}

	result.Blocked = result.FinalAction == ActionBlock
	return result, nil
}

// evaluateStage runs all rules for a given stage concurrently and returns triggered hits.
func (e *Engine) evaluateStage(ctx context.Context, stage Stage, rules []Rule, req EvalRequest) ([]Hit, error) {
	if len(rules) == 0 {
		return nil, nil
	}

	type evalResult struct {
		hit *Hit
		err error
	}

	results := make([]evalResult, len(rules))
	var wg sync.WaitGroup

	for i, rule := range rules {
		wg.Add(1)
		go func(idx int, r Rule) {
			defer wg.Done()

			provider, err := e.registry.Get(r.Provider)
			if err != nil {
				results[idx] = evalResult{err: err}
				return
			}

			score, triggered, reason, err := provider.Evaluate(ctx, r, req)
			if err != nil {
				results[idx] = evalResult{err: err}
				return
			}

			if triggered {
				results[idx] = evalResult{
					hit: &Hit{
						Rule:      r,
						Score:     score,
						Action:    r.Action,
						Reason:    reason,
						Timestamp: time.Now(),
					},
				}
			}
		}(i, rule)
	}

	wg.Wait()

	var hits []Hit
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		if r.hit != nil {
			hits = append(hits, *r.hit)
		}
	}
	return hits, nil
}
