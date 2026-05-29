package cost

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// --- Mock Redis ---

type mockRedis struct {
	mu   sync.Mutex
	data map[string]float64
	// failOn can be set to a key prefix to simulate errors.
	failOn string
}

func newMockRedis() *mockRedis {
	return &mockRedis{data: make(map[string]float64)}
}

func (m *mockRedis) IncrByFloat(ctx context.Context, key string, value float64) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failOn != "" && len(key) >= len(m.failOn) && key[:len(m.failOn)] == m.failOn {
		return 0, fmt.Errorf("redis error: simulated failure")
	}

	m.data[key] += value
	return m.data[key], nil
}

func (m *mockRedis) get(key string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[key]
}

// --- Tests ---

func TestComputeCost_Basic(t *testing.T) {
	tests := []struct {
		name     string
		usage    Usage
		cost     EndpointCost
		expected float64
	}{
		{
			name:  "zero usage",
			usage: Usage{Input: 0, Output: 0, Cached: 0},
			cost:  EndpointCost{InputPerMillion: 3.0, OutputPerMillion: 15.0, CachedPerMillion: 1.5},
			expected: 0,
		},
		{
			name:  "typical GPT-4o usage",
			usage: Usage{Input: 1000, Output: 500, Cached: 200},
			cost:  EndpointCost{InputPerMillion: 5.0, OutputPerMillion: 15.0, CachedPerMillion: 2.5},
			// (1000*5 + 500*15 + 200*2.5) / 1e6 = (5000 + 7500 + 500) / 1e6 = 0.013
			expected: 0.013,
		},
		{
			name:  "only input tokens",
			usage: Usage{Input: 1000000, Output: 0, Cached: 0},
			cost:  EndpointCost{InputPerMillion: 3.0, OutputPerMillion: 15.0, CachedPerMillion: 1.5},
			// 1000000 * 3.0 / 1e6 = 3.0
			expected: 3.0,
		},
		{
			name:  "zero pricing",
			usage: Usage{Input: 1000, Output: 1000, Cached: 1000},
			cost:  EndpointCost{InputPerMillion: 0, OutputPerMillion: 0, CachedPerMillion: 0},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeCost(tt.usage, tt.cost)
			if result != tt.expected {
				t.Errorf("ComputeCost() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComputeCost_NonNegative(t *testing.T) {
	// Even with negative token counts (invalid but defensive), result should be >= 0.
	usage := Usage{Input: -100, Output: -100, Cached: -100}
	cost := EndpointCost{InputPerMillion: 5.0, OutputPerMillion: 15.0, CachedPerMillion: 2.5}
	result := ComputeCost(usage, cost)
	if result < 0 {
		t.Errorf("ComputeCost() = %v, want >= 0", result)
	}
}

func TestComputeCost_Deterministic(t *testing.T) {
	usage := Usage{Input: 1234, Output: 5678, Cached: 910}
	cost := EndpointCost{InputPerMillion: 5.0, OutputPerMillion: 15.0, CachedPerMillion: 2.5}

	first := ComputeCost(usage, cost)
	for i := 0; i < 100; i++ {
		result := ComputeCost(usage, cost)
		if result != first {
			t.Fatalf("ComputeCost not deterministic: got %v, want %v", result, first)
		}
	}
}

func TestTracker_Record(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()
	dim := Dimension{TenantID: "t1", AgentName: "agent1", SkillName: "skill1"}
	usage := Usage{Input: 1000, Output: 500, Cached: 200}
	endpointCost := EndpointCost{InputPerMillion: 5.0, OutputPerMillion: 15.0, CachedPerMillion: 2.5}

	cost, err := tracker.Record(ctx, dim, usage, endpointCost)
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if cost != 0.013 {
		t.Errorf("Record() cost = %v, want 0.013", cost)
	}

	// Verify Redis counters.
	if got := redis.get("aip:cost:tenant:t1"); got != 0.013 {
		t.Errorf("Redis tenant counter = %v, want 0.013", got)
	}
	if got := redis.get("aip:cost:agent:agent1"); got != 0.013 {
		t.Errorf("Redis agent counter = %v, want 0.013", got)
	}
	if got := redis.get("aip:cost:skill:skill1"); got != 0.013 {
		t.Errorf("Redis skill counter = %v, want 0.013", got)
	}
}

func TestTracker_Record_ZeroCost(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()
	dim := Dimension{TenantID: "t1", AgentName: "agent1", SkillName: "skill1"}
	usage := Usage{Input: 0, Output: 0, Cached: 0}
	endpointCost := EndpointCost{InputPerMillion: 5.0, OutputPerMillion: 15.0, CachedPerMillion: 2.5}

	cost, err := tracker.Record(ctx, dim, usage, endpointCost)
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if cost != 0 {
		t.Errorf("Record() cost = %v, want 0", cost)
	}

	// No Redis writes for zero cost.
	if got := redis.get("aip:cost:tenant:t1"); got != 0 {
		t.Errorf("Redis tenant counter = %v, want 0", got)
	}
}

func TestTracker_Record_Accumulates(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()
	dim := Dimension{TenantID: "t1", AgentName: "agent1", SkillName: "skill1"}
	usage := Usage{Input: 1000000, Output: 0, Cached: 0}
	endpointCost := EndpointCost{InputPerMillion: 1.0, OutputPerMillion: 0, CachedPerMillion: 0}

	// Each call costs $1.
	for i := 0; i < 5; i++ {
		_, err := tracker.Record(ctx, dim, usage, endpointCost)
		if err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	if got := redis.get("aip:cost:tenant:t1"); got != 5.0 {
		t.Errorf("Redis tenant counter after 5 calls = %v, want 5.0", got)
	}
}

func TestTracker_Record_RedisError(t *testing.T) {
	redis := newMockRedis()
	redis.failOn = "aip:cost:tenant:"
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()
	dim := Dimension{TenantID: "t1", AgentName: "agent1", SkillName: "skill1"}
	usage := Usage{Input: 1000000, Output: 0, Cached: 0}
	endpointCost := EndpointCost{InputPerMillion: 1.0, OutputPerMillion: 0, CachedPerMillion: 0}

	_, err := tracker.Record(ctx, dim, usage, endpointCost)
	if err == nil {
		t.Fatal("Record() expected error, got nil")
	}
}

func TestTracker_CheckHardCap_UnderLimit(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()

	// Pre-set tenant cost below limit.
	redis.data["aip:cost:tenant:t1"] = 50.0

	denied, err := tracker.CheckHardCap(ctx, BudgetCheck{TenantID: "t1", LimitUSD: 100.0})
	if err != nil {
		t.Fatalf("CheckHardCap() error = %v", err)
	}
	if denied {
		t.Error("CheckHardCap() = true, want false (under limit)")
	}
}

func TestTracker_CheckHardCap_AtLimit(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()

	// Pre-set tenant cost at exact limit.
	redis.data["aip:cost:tenant:t1"] = 100.0

	denied, err := tracker.CheckHardCap(ctx, BudgetCheck{TenantID: "t1", LimitUSD: 100.0})
	if err != nil {
		t.Fatalf("CheckHardCap() error = %v", err)
	}
	if !denied {
		t.Error("CheckHardCap() = false, want true (at limit)")
	}
}

func TestTracker_CheckHardCap_OverLimit(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()

	// Pre-set tenant cost over limit.
	redis.data["aip:cost:tenant:t1"] = 150.0

	denied, err := tracker.CheckHardCap(ctx, BudgetCheck{TenantID: "t1", LimitUSD: 100.0})
	if err != nil {
		t.Fatalf("CheckHardCap() error = %v", err)
	}
	if !denied {
		t.Error("CheckHardCap() = false, want true (over limit)")
	}
}

func TestTracker_CheckHardCap_ZeroLimit(t *testing.T) {
	redis := newMockRedis()
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()

	// Zero limit means no budget configured — allow.
	denied, err := tracker.CheckHardCap(ctx, BudgetCheck{TenantID: "t1", LimitUSD: 0})
	if err != nil {
		t.Fatalf("CheckHardCap() error = %v", err)
	}
	if denied {
		t.Error("CheckHardCap() = true, want false (no limit configured)")
	}
}

func TestTracker_CheckHardCap_RedisError_FailClosed(t *testing.T) {
	redis := newMockRedis()
	redis.failOn = "aip:cost:tenant:"
	reg := prometheus.NewRegistry()
	tracker := NewTracker(redis, reg)

	ctx := context.Background()

	denied, err := tracker.CheckHardCap(ctx, BudgetCheck{TenantID: "t1", LimitUSD: 100.0})
	if err == nil {
		t.Fatal("CheckHardCap() expected error, got nil")
	}
	if !denied {
		t.Error("CheckHardCap() = false, want true (fail-closed on Redis error)")
	}
}
