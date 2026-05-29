package cost

import (
	"context"
	"testing"
	"time"
)

func TestBudgetEnforcer_BelowAllThresholds_Allow(t *testing.T) {
	alerts := []AlertConfig{
		{Threshold: 0.5, Channel: "slack", Action: ActionNotify},
		{Threshold: 0.8, Channel: "email", Action: ActionThrottle},
	}
	enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)

	ctx := context.Background()
	decision := enforcer.Check(ctx, "tenant-1", 30.0, 100.0)

	if !decision.Allowed {
		t.Errorf("expected Allowed=true, got false")
	}
	if decision.Action != "allow" {
		t.Errorf("expected Action='allow', got %q", decision.Action)
	}
	if decision.ThrottleDelay != 0 {
		t.Errorf("expected ThrottleDelay=0, got %v", decision.ThrottleDelay)
	}
}

func TestBudgetEnforcer_AtAlertThreshold_Notify(t *testing.T) {
	alerts := []AlertConfig{
		{Threshold: 0.5, Channel: "slack", Action: ActionNotify},
		{Threshold: 0.8, Channel: "email", Action: ActionThrottle},
	}
	enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)

	ctx := context.Background()
	// 50% usage — exactly at notify threshold.
	decision := enforcer.Check(ctx, "tenant-1", 50.0, 100.0)

	if !decision.Allowed {
		t.Errorf("expected Allowed=true, got false")
	}
	if decision.Action != "notify" {
		t.Errorf("expected Action='notify', got %q", decision.Action)
	}
	if decision.ThrottleDelay != 0 {
		t.Errorf("expected ThrottleDelay=0, got %v", decision.ThrottleDelay)
	}

	// 60% usage — above notify but below throttle.
	decision = enforcer.Check(ctx, "tenant-1", 60.0, 100.0)
	if !decision.Allowed {
		t.Errorf("expected Allowed=true at 60%%, got false")
	}
	if decision.Action != "notify" {
		t.Errorf("expected Action='notify' at 60%%, got %q", decision.Action)
	}
}

func TestBudgetEnforcer_AtThrottleThreshold_Throttle(t *testing.T) {
	alerts := []AlertConfig{
		{Threshold: 0.5, Channel: "slack", Action: ActionNotify},
		{Threshold: 0.8, Channel: "email", Action: ActionThrottle},
	}
	enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)

	ctx := context.Background()
	// 80% usage — exactly at throttle threshold.
	decision := enforcer.Check(ctx, "tenant-1", 80.0, 100.0)

	if !decision.Allowed {
		t.Errorf("expected Allowed=true, got false")
	}
	if decision.Action != "throttle" {
		t.Errorf("expected Action='throttle', got %q", decision.Action)
	}
	if decision.ThrottleDelay <= 0 {
		t.Errorf("expected ThrottleDelay > 0, got %v", decision.ThrottleDelay)
	}

	// 95% usage — well past throttle threshold, delay should be higher.
	decision2 := enforcer.Check(ctx, "tenant-1", 95.0, 100.0)
	if !decision2.Allowed {
		t.Errorf("expected Allowed=true at 95%%, got false")
	}
	if decision2.Action != "throttle" {
		t.Errorf("expected Action='throttle' at 95%%, got %q", decision2.Action)
	}
	if decision2.ThrottleDelay <= decision.ThrottleDelay {
		t.Errorf("expected higher delay at 95%% (%v) than at 80%% (%v)", decision2.ThrottleDelay, decision.ThrottleDelay)
	}
}

func TestBudgetEnforcer_AtHardCap_Block(t *testing.T) {
	alerts := []AlertConfig{
		{Threshold: 0.5, Channel: "slack", Action: ActionNotify},
		{Threshold: 0.8, Channel: "email", Action: ActionThrottle},
	}
	enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)

	ctx := context.Background()

	// Exactly at limit.
	decision := enforcer.Check(ctx, "tenant-1", 100.0, 100.0)
	if decision.Allowed {
		t.Errorf("expected Allowed=false at hardCap, got true")
	}
	if decision.Action != "block" {
		t.Errorf("expected Action='block', got %q", decision.Action)
	}

	// Over limit.
	decision = enforcer.Check(ctx, "tenant-1", 150.0, 100.0)
	if decision.Allowed {
		t.Errorf("expected Allowed=false over hardCap, got true")
	}
	if decision.Action != "block" {
		t.Errorf("expected Action='block' over hardCap, got %q", decision.Action)
	}
}

func TestBudgetEnforcer_Rollover_CarriesUnused(t *testing.T) {
	enforcer := NewBudgetEnforcer(nil, 0)
	ctx := context.Background()

	tests := []struct {
		name      string
		unused    float64
		nextLimit float64
		expected  float64
	}{
		{
			name:      "carries unused budget forward",
			unused:    20.0,
			nextLimit: 100.0,
			expected:  120.0,
		},
		{
			name:      "caps at 2x next limit",
			unused:    150.0,
			nextLimit: 100.0,
			expected:  200.0,
		},
		{
			name:      "no rollover on overspend",
			unused:    -10.0,
			nextLimit: 100.0,
			expected:  100.0,
		},
		{
			name:      "zero unused means no change",
			unused:    0.0,
			nextLimit: 100.0,
			expected:  100.0,
		},
		{
			name:      "small rollover",
			unused:    5.0,
			nextLimit: 50.0,
			expected:  55.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enforcer.Rollover(ctx, "tenant-1", tt.unused, tt.nextLimit)
			if result != tt.expected {
				t.Errorf("Rollover() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBudgetEnforcer_NoAlerts_AlwaysAllow(t *testing.T) {
	enforcer := NewBudgetEnforcer(nil, 0)
	ctx := context.Background()

	// Below limit with no alert configs — should allow.
	decision := enforcer.Check(ctx, "tenant-1", 90.0, 100.0)
	if !decision.Allowed {
		t.Errorf("expected Allowed=true with no alerts, got false")
	}
	if decision.Action != "allow" {
		t.Errorf("expected Action='allow', got %q", decision.Action)
	}

	// At limit — still blocked by hardCap even without alerts.
	decision = enforcer.Check(ctx, "tenant-1", 100.0, 100.0)
	if decision.Allowed {
		t.Errorf("expected Allowed=false at hardCap, got true")
	}
}

func TestBudgetEnforcer_NoLimit_AlwaysAllow(t *testing.T) {
	alerts := []AlertConfig{
		{Threshold: 0.5, Channel: "slack", Action: ActionNotify},
	}
	enforcer := NewBudgetEnforcer(alerts, 500*time.Millisecond)
	ctx := context.Background()

	// Zero limit means no budget configured.
	decision := enforcer.Check(ctx, "tenant-1", 999.0, 0)
	if !decision.Allowed {
		t.Errorf("expected Allowed=true with no limit, got false")
	}
	if decision.Action != "allow" {
		t.Errorf("expected Action='allow', got %q", decision.Action)
	}
}

func TestBudgetEnforcer_BlockAlertAction(t *testing.T) {
	// Test that an alert with action=block denies the request even before hardCap.
	alerts := []AlertConfig{
		{Threshold: 0.9, Channel: "pagerduty", Action: ActionBlock},
	}
	enforcer := NewBudgetEnforcer(alerts, 0)
	ctx := context.Background()

	decision := enforcer.Check(ctx, "tenant-1", 92.0, 100.0)
	if decision.Allowed {
		t.Errorf("expected Allowed=false with block alert at 90%%, got true")
	}
	if decision.Action != "block" {
		t.Errorf("expected Action='block', got %q", decision.Action)
	}
}
