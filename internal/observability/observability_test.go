package observability

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewProvider_RegistersMetrics(t *testing.T) {
	p := NewProvider(Config{
		MetricsAddr:    ":0", // OS-assigned port
		ServiceName:    "test-service",
		ServiceVersion: "0.0.1",
	})

	if p.Metrics == nil {
		t.Fatal("expected non-nil Metrics")
	}
	if p.Metrics.ReconcileDuration == nil {
		t.Fatal("expected non-nil ReconcileDuration")
	}
	if p.Metrics.PDPDecisionDuration == nil {
		t.Fatal("expected non-nil PDPDecisionDuration")
	}
	if p.Metrics.RouterRouteTotal == nil {
		t.Fatal("expected non-nil RouterRouteTotal")
	}
	if p.Metrics.AuditEmitTotal == nil {
		t.Fatal("expected non-nil AuditEmitTotal")
	}
	if p.Metrics.CostUSDTotal == nil {
		t.Fatal("expected non-nil CostUSDTotal")
	}
}

func TestProvider_MetricsEndpoint(t *testing.T) {
	// Use a dynamic port to avoid conflicts.
	p := NewProvider(Config{
		MetricsAddr:    ":19090",
		ServiceName:    "test-service",
		ServiceVersion: "0.0.1",
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Shutdown(ctx)
	}()

	// Give the server a moment to start.
	time.Sleep(100 * time.Millisecond)

	// Emit some metrics.
	p.ObserveReconcile("skill", 50*time.Millisecond)
	p.ObservePDPDecision("allow", 10*time.Millisecond)
	p.IncRouterRoute("gpt-4o-eu", "eu-west-1")
	p.IncAuditEmit("success")
	p.Metrics.CostUSDTotal.WithLabelValues("tenant-a", "legal-copilot", "contract-review").Add(0.05)

	// Query the metrics endpoint.
	resp, err := http.Get("http://localhost:19090/metrics")
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	bodyStr := string(body)

	// Verify all 5 key metrics are present.
	expectedMetrics := []string{
		"aip_reconcile_duration_seconds",
		"aip_pdp_decision_duration_seconds",
		"aip_router_route_total",
		"aip_audit_emit_total",
		"aip_cost_usd_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(bodyStr, metric) {
			t.Errorf("expected metric %q in response, not found", metric)
		}
	}

	// Verify at least 5 distinct aip_ metrics show up.
	aipCount := 0
	for _, line := range strings.Split(bodyStr, "\n") {
		if strings.HasPrefix(line, "# HELP aip_") {
			aipCount++
		}
	}
	if aipCount < 5 {
		t.Errorf("expected at least 5 aip_ metric families, got %d", aipCount)
	}

	fmt.Printf("✓ Found %d aip_ metric families\n", aipCount)
}

func TestProvider_HealthzEndpoint(t *testing.T) {
	p := NewProvider(Config{
		MetricsAddr:    ":19091",
		ServiceName:    "test-service",
		ServiceVersion: "0.0.1",
	})

	if err := p.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:19091/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestObserveReconcile_MultipleControllers(t *testing.T) {
	p := NewProvider(Config{MetricsAddr: ":0"})

	// Should not panic with different controllers.
	p.ObserveReconcile("skill", 10*time.Millisecond)
	p.ObserveReconcile("agent", 20*time.Millisecond)
	p.ObserveReconcile("policy", 30*time.Millisecond)
	p.ObserveReconcile("tenant", 5*time.Millisecond)
}
