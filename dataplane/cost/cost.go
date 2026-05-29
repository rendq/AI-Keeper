// Package cost provides the Cost Tracker for AIP data plane.
// It computes per-call costs in USD based on token usage, maintains
// Redis counters per Tenant/Agent/Skill, emits Prometheus metrics,
// and enforces hardCap budget blocking.
package cost

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Usage represents the token counts for a single model call.
type Usage struct {
	Input  int64 // number of input tokens
	Output int64 // number of output tokens
	Cached int64 // number of cached tokens
}

// EndpointCost holds per-million-token pricing for a model endpoint.
type EndpointCost struct {
	InputPerMillion  float64 // USD per 1M input tokens
	OutputPerMillion float64 // USD per 1M output tokens
	CachedPerMillion float64 // USD per 1M cached tokens
}

// Dimension identifies the scope of a cost counter.
type Dimension struct {
	TenantID  string
	AgentName string
	SkillName string
}

// ComputeCost is a pure, deterministic function that calculates the USD cost
// for a single model call. The result is always >= 0.
//
// Formula: (usage.Input * cost.InputPerMillion + usage.Output * cost.OutputPerMillion + usage.Cached * cost.CachedPerMillion) / 1e6
func ComputeCost(usage Usage, cost EndpointCost) float64 {
	result := (float64(usage.Input)*cost.InputPerMillion +
		float64(usage.Output)*cost.OutputPerMillion +
		float64(usage.Cached)*cost.CachedPerMillion) / 1e6
	if result < 0 {
		return 0
	}
	return result
}

// RedisClient is the minimal interface for Redis operations needed by cost tracking.
type RedisClient interface {
	// IncrByFloat increments a key by a float64 value and returns the new value.
	IncrByFloat(ctx context.Context, key string, value float64) (float64, error)
}

// Tracker tracks costs per call: increments Redis counters, emits Prometheus metrics,
// and checks hardCap budgets.
type Tracker struct {
	redis   RedisClient
	metrics *metrics
}

type metrics struct {
	costTotal *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		costTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "aip_cost_usd_total",
			Help: "Total cost in USD accumulated per tenant/agent/skill.",
		}, []string{"tenant", "agent", "skill"}),
	}
	return m
}

// NewTracker creates a new cost Tracker.
func NewTracker(redis RedisClient, reg prometheus.Registerer) *Tracker {
	return &Tracker{
		redis:   redis,
		metrics: newMetrics(reg),
	}
}

const (
	redisKeyPrefix = "aip:cost:"
)

// redisKey builds a Redis key for a given dimension component.
func redisKey(dimension, name string) string {
	return redisKeyPrefix + dimension + ":" + name
}

// Record computes the cost for the given usage and endpoint pricing,
// increments Redis counters for the specified dimension, and emits
// the Prometheus metric. It returns the computed cost.
func (t *Tracker) Record(ctx context.Context, dim Dimension, usage Usage, endpointCost EndpointCost) (float64, error) {
	cost := ComputeCost(usage, endpointCost)
	if cost == 0 {
		return 0, nil
	}

	// Increment per-dimension Redis counters.
	if dim.TenantID != "" {
		if _, err := t.redis.IncrByFloat(ctx, redisKey("tenant", dim.TenantID), cost); err != nil {
			return cost, fmt.Errorf("cost: redis incr tenant: %w", err)
		}
	}
	if dim.AgentName != "" {
		if _, err := t.redis.IncrByFloat(ctx, redisKey("agent", dim.AgentName), cost); err != nil {
			return cost, fmt.Errorf("cost: redis incr agent: %w", err)
		}
	}
	if dim.SkillName != "" {
		if _, err := t.redis.IncrByFloat(ctx, redisKey("skill", dim.SkillName), cost); err != nil {
			return cost, fmt.Errorf("cost: redis incr skill: %w", err)
		}
	}

	// Emit Prometheus metric.
	t.metrics.costTotal.WithLabelValues(dim.TenantID, dim.AgentName, dim.SkillName).Add(cost)

	return cost, nil
}

// BudgetCheck holds the parameters for a hardCap budget check.
type BudgetCheck struct {
	TenantID string
	LimitUSD float64 // hardCap budget limit
}

// CheckHardCap verifies whether the current accumulated cost for the tenant
// exceeds the budget limit. Returns true if the call should be denied.
func (t *Tracker) CheckHardCap(ctx context.Context, check BudgetCheck) (bool, error) {
	if check.LimitUSD <= 0 {
		// No budget configured or invalid limit — allow.
		return false, nil
	}

	key := redisKey("tenant", check.TenantID)
	current, err := t.redis.IncrByFloat(ctx, key, 0) // read without incrementing
	if err != nil {
		// Fail-closed: if we cannot read the budget, deny.
		return true, fmt.Errorf("cost: redis read budget: %w", err)
	}

	return current >= check.LimitUSD, nil
}
