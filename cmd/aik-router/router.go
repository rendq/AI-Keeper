// Package router implements the Model Router decision engine.
//
// It evaluates CEL-based routing rules against a request context,
// performs weighted random endpoint selection, executes fallback chains,
// enforces tenant modelAllowlist, and records audit metadata.
package router

import (
	"errors"
	"fmt"
	"math/rand"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// Errors returned by the router.
var (
	ErrNoMatchingRule    = errors.New("router: no matching rule")
	ErrAllEndpointsFailed = errors.New("router: all endpoints failed (502)")
	ErrModelNotAllowed   = errors.New("router: model not in tenant allowlist")
)

// RequestContext holds the attributes available to CEL rule evaluation.
type RequestContext struct {
	UserCountry    string
	Classification string
	CostSensitive  bool
	TenantID       string
	TaskType       string
	ContextLength  int32
	// Extra allows arbitrary attributes for CEL expressions.
	Extra map[string]interface{}
}

// RouteEndpoint represents a routable model endpoint with its metadata.
type RouteEndpoint struct {
	// Ref is the ResourceRef identifier (e.g. "model://gpt-4o-eu").
	Ref      shared.ResourceRef
	Provider string // "openai" | "azure_openai" | etc.
	Endpoint string // URL
	Region   string
	// Fallback is an ordered list of fallback endpoint refs.
	Fallback []shared.ResourceRef
}

// RouteRule is a compiled routing rule from the ModelRouter CR.
type RouteRule struct {
	// CELExpression is the raw CEL expression. Empty means always-true.
	CELExpression string
	// Endpoints are the weighted endpoints for this rule.
	Endpoints []WeightedEndpoint
}

// WeightedEndpoint pairs an endpoint ref with a routing weight.
type WeightedEndpoint struct {
	Ref    shared.ResourceRef
	Weight int32
}

// RouteTable is the compiled routing configuration pushed by the
// ModelRouter controller (task 4.3).
type RouteTable struct {
	Alias    string
	Rules    []RouteRule
	// DefaultEndpoint is used if no rule matches and is non-nil.
	DefaultEndpoint *shared.ResourceRef
}

// RoutingDecision captures the output of a routing decision for audit.
type RoutingDecision struct {
	Endpoint     shared.ResourceRef
	Region       string
	FallbackUsed bool
}

// EndpointRegistry provides endpoint metadata lookup.
type EndpointRegistry interface {
	// Get returns the RouteEndpoint for the given ref, or false if not found.
	Get(ref shared.ResourceRef) (RouteEndpoint, bool)
}

// EndpointCaller attempts to call a model endpoint. Returns an error if
// the call fails (triggering fallback).
type EndpointCaller interface {
	Call(endpoint RouteEndpoint, request interface{}) (interface{}, error)
}

// Router is the main routing engine.
type Router struct {
	table      *RouteTable
	registry   EndpointRegistry
	celEval    *CELEvaluator
	allowlist  []shared.ResourceRef // tenant modelAllowlist
	randSource *rand.Rand
}

// NewRouter creates a Router with the given configuration.
func NewRouter(table *RouteTable, registry EndpointRegistry, allowlist []shared.ResourceRef, seed int64) (*Router, error) {
	celEval, err := NewCELEvaluator()
	if err != nil {
		return nil, fmt.Errorf("router: failed to create CEL evaluator: %w", err)
	}
	return &Router{
		table:      table,
		registry:   registry,
		celEval:    celEval,
		allowlist:  allowlist,
		randSource: rand.New(rand.NewSource(seed)),
	}, nil
}

// Route evaluates the route table against the request context and returns
// the selected endpoint ref plus audit metadata. It does NOT call the
// endpoint — the caller is responsible for invoking the endpoint and
// handling fallback via RouteWithFallback.
func (r *Router) Route(ctx RequestContext) (RoutingDecision, error) {
	// 1. Evaluate CEL rules — first match wins.
	selectedEndpoints, err := r.matchRule(ctx)
	if err != nil {
		return RoutingDecision{}, err
	}

	// 2. Weighted random selection among matched endpoints.
	ref := r.weightedSelect(selectedEndpoints)

	// 3. Tenant allowlist check.
	if err := r.checkAllowlist(ref); err != nil {
		return RoutingDecision{}, err
	}

	// 4. Lookup endpoint metadata for audit.
	ep, ok := r.registry.Get(ref)
	if !ok {
		return RoutingDecision{}, fmt.Errorf("router: endpoint %s not found in registry", ref)
	}

	return RoutingDecision{
		Endpoint:     ref,
		Region:       ep.Region,
		FallbackUsed: false,
	}, nil
}

// RouteWithFallback performs routing and, if the primary endpoint call
// fails, walks the fallback chain. Returns the response, routing decision
// (for audit), and any error.
func (r *Router) RouteWithFallback(ctx RequestContext, caller EndpointCaller, request interface{}) (interface{}, RoutingDecision, error) {
	// 1. Get primary route.
	decision, err := r.Route(ctx)
	if err != nil {
		return nil, RoutingDecision{}, err
	}

	// 2. Try primary endpoint.
	ep, _ := r.registry.Get(decision.Endpoint)
	resp, err := caller.Call(ep, request)
	if err == nil {
		return resp, decision, nil
	}

	// 3. Walk fallback chain.
	for _, fbRef := range ep.Fallback {
		// Check allowlist for fallback endpoint too.
		if aErr := r.checkAllowlist(fbRef); aErr != nil {
			continue
		}
		fbEp, ok := r.registry.Get(fbRef)
		if !ok {
			continue
		}
		resp, err = caller.Call(fbEp, request)
		if err == nil {
			return resp, RoutingDecision{
				Endpoint:     fbRef,
				Region:       fbEp.Region,
				FallbackUsed: true,
			}, nil
		}
	}

	// 4. All failed → 502.
	return nil, RoutingDecision{FallbackUsed: true}, ErrAllEndpointsFailed
}

// matchRule evaluates CEL rules in order, returning the endpoints of
// the first matching rule.
func (r *Router) matchRule(ctx RequestContext) ([]WeightedEndpoint, error) {
	for _, rule := range r.table.Rules {
		match, err := r.celEval.Evaluate(rule.CELExpression, ctx)
		if err != nil {
			// Skip rules with evaluation errors.
			continue
		}
		if match {
			return rule.Endpoints, nil
		}
	}
	// No rule matched — use default if available.
	if r.table.DefaultEndpoint != nil {
		return []WeightedEndpoint{{Ref: *r.table.DefaultEndpoint, Weight: 1}}, nil
	}
	return nil, ErrNoMatchingRule
}

// weightedSelect picks one endpoint from the weighted list using
// weighted random selection.
func (r *Router) weightedSelect(endpoints []WeightedEndpoint) shared.ResourceRef {
	if len(endpoints) == 1 {
		return endpoints[0].Ref
	}

	totalWeight := int32(0)
	for _, ep := range endpoints {
		w := ep.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	pick := r.randSource.Int31n(totalWeight)
	cumulative := int32(0)
	for _, ep := range endpoints {
		w := ep.Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if pick < cumulative {
			return ep.Ref
		}
	}
	// Should not reach here, but fallback to last.
	return endpoints[len(endpoints)-1].Ref
}

// checkAllowlist enforces the tenant modelAllowlist. If the allowlist
// is empty, all endpoints are permitted.
func (r *Router) checkAllowlist(ref shared.ResourceRef) error {
	if len(r.allowlist) == 0 {
		return nil
	}
	for _, allowed := range r.allowlist {
		if allowed == ref {
			return nil
		}
	}
	return ErrModelNotAllowed
}
