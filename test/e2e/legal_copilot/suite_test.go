//go:build e2e

// Package legal_copilot_test implements end-to-end tests for the Legal Copilot scenario.
// These tests run against a fully deployed AIP stack in a kind cluster (via `make e2e-up`).
//
// Validates: Requirements A1, A3, A4, A5, B1, B2, B3, B4, B5, B8, B12, B13, C8, D1, E1
package legal_copilot_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	// Default service endpoints (overridable via env vars).
	defaultGatewayURL    = "http://localhost:30080"
	defaultFeishuURL     = "http://localhost:30082"
	defaultAuditURL      = "http://localhost:30090"
	defaultClickHouseURL = "http://localhost:30123"
	defaultRedisAddr     = "localhost:30379"
	defaultS3Endpoint    = "http://localhost:30900"

	// Timeouts for polling / waiting.
	pollInterval = 2 * time.Second
	pollTimeout  = 60 * time.Second
)

// testEnv holds the resolved service endpoints for the e2e cluster.
type testEnv struct {
	GatewayURL    string
	FeishuURL     string
	AuditURL      string
	ClickHouseURL string
	RedisAddr     string
	S3Endpoint    string
}

var env testEnv

// TestMain sets up the test environment and verifies the kind cluster is reachable.
func TestMain(m *testing.M) {
	env = testEnv{
		GatewayURL:    envOrDefault("E2E_GATEWAY_URL", defaultGatewayURL),
		FeishuURL:     envOrDefault("E2E_FEISHU_URL", defaultFeishuURL),
		AuditURL:      envOrDefault("E2E_AUDIT_URL", defaultAuditURL),
		ClickHouseURL: envOrDefault("E2E_CLICKHOUSE_URL", defaultClickHouseURL),
		RedisAddr:     envOrDefault("E2E_REDIS_ADDR", defaultRedisAddr),
		S3Endpoint:    envOrDefault("E2E_S3_ENDPOINT", defaultS3Endpoint),
	}

	// Verify cluster services are reachable before running tests.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := waitForServices(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: cluster services not ready: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// waitForServices polls the health endpoints of critical services.
func waitForServices(ctx context.Context) error {
	endpoints := map[string]string{
		"gateway": env.GatewayURL + "/healthz",
		"feishu":  env.FeishuURL + "/healthz",
		"audit":   env.AuditURL + "/healthz",
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for name, url := range endpoints {
		if err := pollHealth(ctx, client, name, url); err != nil {
			return err
		}
	}
	return nil
}

// pollHealth waits for a service to return HTTP 200.
func pollHealth(ctx context.Context, client *http.Client, name, url string) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s at %s", name, url)
		case <-ticker.C:
			resp, err := client.Get(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
