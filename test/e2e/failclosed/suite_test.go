//go:build e2e

// Package failclosed_test implements end-to-end tests for fail-closed PDP behavior
// and audit immutability (S3 Object Lock).
//
// Validates: Requirements F5, F6, A5.13, B12.6
package failclosed_test

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
	defaultGatewayURL = "http://localhost:30080"
	defaultPDPAddr    = "localhost:30081"
	defaultAuditURL   = "http://localhost:30090"
	defaultS3Endpoint = "http://localhost:30900"

	// S3 audit bucket with Object Lock enabled.
	auditBucket = "aik-audit-events"

	// Timeouts for polling / waiting.
	pollInterval = 2 * time.Second
	pollTimeout  = 60 * time.Second
)

// testEnv holds the resolved service endpoints for the e2e cluster.
type testEnv struct {
	GatewayURL string
	PDPAddr    string
	AuditURL   string
	S3Endpoint string
}

var env testEnv

// TestMain sets up the test environment and verifies the cluster is reachable.
func TestMain(m *testing.M) {
	env = testEnv{
		GatewayURL: envOrDefault("E2E_GATEWAY_URL", defaultGatewayURL),
		PDPAddr:    envOrDefault("E2E_PDP_ADDR", defaultPDPAddr),
		AuditURL:   envOrDefault("E2E_AUDIT_URL", defaultAuditURL),
		S3Endpoint: envOrDefault("E2E_S3_ENDPOINT", defaultS3Endpoint),
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
