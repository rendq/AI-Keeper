package main

import (
	"context"
	"fmt"
	"testing"
)

// mockPDPClient implements PDPClient for testing.
type mockPDPClient struct {
	failEndpoints map[string]bool
}

func (m *mockPDPClient) PushBundle(_ context.Context, endpoint string, _ []byte) error {
	if m.failEndpoints[endpoint] {
		return fmt.Errorf("network partition: unreachable %s", endpoint)
	}
	return nil
}

func TestCrossRegionSync_AllSuccess(t *testing.T) {
	regions := []RegionPDP{
		{Region: "cn-north", Endpoint: "pdp-cn.internal:8080"},
		{Region: "eu-west", Endpoint: "pdp-eu.internal:8080"},
		{Region: "us-east", Endpoint: "pdp-us.internal:8080"},
	}
	client := &mockPDPClient{failEndpoints: map[string]bool{}}
	syncer := NewBundleSyncer(regions, client)

	status := syncer.Push(context.Background(), "sha256:abc123", 1, []byte("bundle-data"))

	if !status.FullySynced {
		t.Fatal("expected FullySynced=true when all regions succeed")
	}
	for _, r := range status.Regions {
		if r.BundleHash != "sha256:abc123" {
			t.Errorf("region %s: got hash %q, want sha256:abc123", r.Region, r.BundleHash)
		}
		if r.Version != 1 {
			t.Errorf("region %s: got version %d, want 1", r.Region, r.Version)
		}
		if r.LastSyncAt.IsZero() {
			t.Errorf("region %s: LastSyncAt should not be zero", r.Region)
		}
	}
}

func TestCrossRegionSync_PartialFailure(t *testing.T) {
	regions := []RegionPDP{
		{Region: "cn-north", Endpoint: "pdp-cn.internal:8080"},
		{Region: "eu-west", Endpoint: "pdp-eu.internal:8080"},
		{Region: "us-east", Endpoint: "pdp-us.internal:8080"},
	}
	client := &mockPDPClient{failEndpoints: map[string]bool{
		"pdp-eu.internal:8080": true, // eu-west unreachable
	}}
	syncer := NewBundleSyncer(regions, client)

	status := syncer.Push(context.Background(), "sha256:def456", 2, []byte("bundle-v2"))

	if status.FullySynced {
		t.Fatal("expected FullySynced=false when one region fails")
	}

	for _, r := range status.Regions {
		switch r.Region {
		case "cn-north", "us-east":
			if r.BundleHash != "sha256:def456" {
				t.Errorf("region %s: got hash %q, want sha256:def456", r.Region, r.BundleHash)
			}
			if r.Version != 2 {
				t.Errorf("region %s: got version %d, want 2", r.Region, r.Version)
			}
		case "eu-west":
			// Stale: should retain empty hash (never synced)
			if r.BundleHash != "" {
				t.Errorf("region eu-west: expected stale empty hash, got %q", r.BundleHash)
			}
			if r.Version != 0 {
				t.Errorf("region eu-west: expected stale version 0, got %d", r.Version)
			}
		}
	}
}

func TestCrossRegionSync_StatusTracking(t *testing.T) {
	regions := []RegionPDP{
		{Region: "cn-north", Endpoint: "pdp-cn.internal:8080"},
		{Region: "eu-west", Endpoint: "pdp-eu.internal:8080"},
	}
	client := &mockPDPClient{failEndpoints: map[string]bool{}}
	syncer := NewBundleSyncer(regions, client)

	// Initial status — nothing synced
	status := syncer.Status()
	if status.FullySynced {
		t.Fatal("expected FullySynced=false before any push")
	}

	// Push v1
	syncer.Push(context.Background(), "sha256:v1", 1, []byte("v1"))
	status = syncer.Status()
	if !status.FullySynced {
		t.Fatal("expected FullySynced=true after successful push")
	}
	for _, r := range status.Regions {
		if r.BundleHash != "sha256:v1" || r.Version != 1 {
			t.Errorf("region %s: unexpected state hash=%s version=%d", r.Region, r.BundleHash, r.Version)
		}
	}

	// Push v2 with eu-west failing
	client.failEndpoints["pdp-eu.internal:8080"] = true
	syncer.Push(context.Background(), "sha256:v2", 2, []byte("v2"))
	status = syncer.Status()
	if status.FullySynced {
		t.Fatal("expected FullySynced=false when regions diverge")
	}

	// cn-north should be v2, eu-west stays at v1
	for _, r := range status.Regions {
		switch r.Region {
		case "cn-north":
			if r.BundleHash != "sha256:v2" || r.Version != 2 {
				t.Errorf("cn-north: expected v2, got hash=%s version=%d", r.BundleHash, r.Version)
			}
		case "eu-west":
			if r.BundleHash != "sha256:v1" || r.Version != 1 {
				t.Errorf("eu-west: expected stale v1, got hash=%s version=%d", r.BundleHash, r.Version)
			}
		}
	}
}

func TestCrossRegionSync_NetworkPartitionNoFailOpen(t *testing.T) {
	regions := []RegionPDP{
		{Region: "cn-north", Endpoint: "pdp-cn.internal:8080"},
		{Region: "eu-west", Endpoint: "pdp-eu.internal:8080"},
	}
	// All regions fail — simulates full network partition
	client := &mockPDPClient{failEndpoints: map[string]bool{
		"pdp-cn.internal:8080": true,
		"pdp-eu.internal:8080": true,
	}}
	syncer := NewBundleSyncer(regions, client)

	status := syncer.Push(context.Background(), "sha256:new", 5, []byte("new-bundle"))

	// No region should have the new bundle — they keep stale state
	if status.FullySynced {
		t.Fatal("expected FullySynced=false on full network partition")
	}
	for _, r := range status.Regions {
		if r.BundleHash == "sha256:new" {
			t.Errorf("region %s: should NOT have new bundle on network partition (fail-open detected)", r.Region)
		}
		if r.Version == 5 {
			t.Errorf("region %s: should NOT have new version on network partition", r.Region)
		}
	}
}
