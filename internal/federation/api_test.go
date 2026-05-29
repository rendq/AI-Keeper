package federation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFederationAPI_ListClusters(t *testing.T) {
	ctrl := NewFederationController()
	ctrl.Register(ClusterLink{
		Name: "us-east-1",
		Spec: ClusterLinkSpec{Endpoint: "https://us-east-1.example.com", Region: "us-east-1", SyncMode: SyncModePush},
	})
	ctrl.Register(ClusterLink{
		Name: "eu-west-1",
		Spec: ClusterLinkSpec{Endpoint: "https://eu-west-1.example.com", Region: "eu-west-1", SyncMode: SyncModePull},
	})

	syncer := NewPDPSyncer(ctrl, nil)
	api := NewFederationAPI(ctrl, syncer)

	req := httptest.NewRequest(http.MethodGet, "/api/federation/clusters", nil)
	rec := httptest.NewRecorder()
	api.HandleListClusters(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}

	var clusters []ClusterLink
	if err := json.NewDecoder(rec.Body).Decode(&clusters); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
}

func TestFederationAPI_Status(t *testing.T) {
	ctrl := NewFederationController()
	ctrl.Register(ClusterLink{
		Name: "us-east-1",
		Spec: ClusterLinkSpec{Endpoint: "https://us-east-1.example.com", Region: "us-east-1"},
	})
	ctrl.Register(ClusterLink{
		Name: "eu-west-1",
		Spec: ClusterLinkSpec{Endpoint: "https://eu-west-1.example.com", Region: "eu-west-1"},
	})

	// Set audit lag on one cluster directly
	ctrl.mu.Lock()
	ctrl.clusters["eu-west-1"].Status.AuditLag = 5 * time.Second
	ctrl.mu.Unlock()

	syncer := NewPDPSyncer(ctrl, nil)
	// Simulate a synced version for us-east-1
	syncer.mu.Lock()
	syncer.versions["us-east-1"] = BundleVersion{Version: 3, Hash: "abc123"}
	syncer.mu.Unlock()

	api := NewFederationAPI(ctrl, syncer)

	req := httptest.NewRequest(http.MethodGet, "/api/federation/status", nil)
	rec := httptest.NewRecorder()
	api.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status FederationStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if status.TotalClusters != 2 {
		t.Errorf("expected TotalClusters=2, got %d", status.TotalClusters)
	}
	if status.SyncedClusters != 1 {
		t.Errorf("expected SyncedClusters=1, got %d", status.SyncedClusters)
	}
	if status.MaxAuditLag != 5*time.Second {
		t.Errorf("expected MaxAuditLag=5s, got %v", status.MaxAuditLag)
	}
	if len(status.BundleVersionVector) != 1 {
		t.Errorf("expected 1 version entry, got %d", len(status.BundleVersionVector))
	}
}

func TestFederationAPI_Empty(t *testing.T) {
	ctrl := NewFederationController()
	syncer := NewPDPSyncer(ctrl, nil)
	api := NewFederationAPI(ctrl, syncer)

	// Test empty clusters list
	req := httptest.NewRequest(http.MethodGet, "/api/federation/clusters", nil)
	rec := httptest.NewRecorder()
	api.HandleListClusters(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var clusters []ClusterLink
	if err := json.NewDecoder(rec.Body).Decode(&clusters); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(clusters) != 0 {
		t.Fatalf("expected 0 clusters, got %d", len(clusters))
	}

	// Test empty status
	req = httptest.NewRequest(http.MethodGet, "/api/federation/status", nil)
	rec = httptest.NewRecorder()
	api.HandleStatus(rec, req)

	var status FederationStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if status.TotalClusters != 0 {
		t.Errorf("expected TotalClusters=0, got %d", status.TotalClusters)
	}
	if status.SyncedClusters != 0 {
		t.Errorf("expected SyncedClusters=0, got %d", status.SyncedClusters)
	}
	if status.MaxAuditLag != 0 {
		t.Errorf("expected MaxAuditLag=0, got %v", status.MaxAuditLag)
	}
}
