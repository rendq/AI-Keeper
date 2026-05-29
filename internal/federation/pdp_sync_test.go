package federation

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockClusterClient implements ClusterClient for testing.
type mockClusterClient struct {
	pushErr map[string]error // endpoint -> error
}

func (m *mockClusterClient) PushBundle(_ context.Context, endpoint, _ string, _ []byte) error {
	if err, ok := m.pushErr[endpoint]; ok {
		return err
	}
	return nil
}

func setupSyncer(clusters []ClusterLink, client ClusterClient) *PDPSyncer {
	fc := NewFederationController()
	for _, cl := range clusters {
		_ = fc.Register(cl)
	}
	return NewPDPSyncer(fc, client)
}

func TestPDPSync_AllClustersSuccess(t *testing.T) {
	clusters := []ClusterLink{
		{Name: "us-east", Spec: ClusterLinkSpec{Endpoint: "https://us-east.example.com", AuthRef: "secret-1"}},
		{Name: "eu-west", Spec: ClusterLinkSpec{Endpoint: "https://eu-west.example.com", AuthRef: "secret-2"}},
	}
	client := &mockClusterClient{pushErr: map[string]error{}}
	syncer := setupSyncer(clusters, client)

	bundle := BundleVersion{Version: 1, Hash: "abc123", CompiledAt: time.Now()}
	errs := syncer.SyncBundle(bundle, []byte("payload"))

	if errs != nil {
		t.Fatalf("expected no errors, got %v", errs)
	}

	vv := syncer.GetVersionVector()
	if len(vv) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(vv))
	}
	for _, name := range []string{"us-east", "eu-west"} {
		v, ok := vv[name]
		if !ok {
			t.Errorf("missing version for %s", name)
		}
		if v.Version != 1 || v.Hash != "abc123" {
			t.Errorf("unexpected version for %s: %+v", name, v)
		}
	}
}

func TestPDPSync_PartialFailure(t *testing.T) {
	clusters := []ClusterLink{
		{Name: "us-east", Spec: ClusterLinkSpec{Endpoint: "https://us-east.example.com", AuthRef: "secret-1"}},
		{Name: "eu-west", Spec: ClusterLinkSpec{Endpoint: "https://eu-west.example.com", AuthRef: "secret-2"}},
	}
	client := &mockClusterClient{pushErr: map[string]error{
		"https://eu-west.example.com": errors.New("connection refused"),
	}}
	syncer := setupSyncer(clusters, client)

	bundle := BundleVersion{Version: 2, Hash: "def456", CompiledAt: time.Now()}
	errs := syncer.SyncBundle(bundle, []byte("payload"))

	if errs == nil {
		t.Fatal("expected errors for partial failure")
	}
	if _, ok := errs["eu-west"]; !ok {
		t.Error("expected error for eu-west")
	}
	if _, ok := errs["us-east"]; ok {
		t.Error("unexpected error for us-east")
	}

	vv := syncer.GetVersionVector()
	if _, ok := vv["us-east"]; !ok {
		t.Error("us-east should have version updated")
	}
	if _, ok := vv["eu-west"]; ok {
		t.Error("eu-west should NOT have version updated")
	}
}

func TestPDPSync_VersionVector(t *testing.T) {
	clusters := []ClusterLink{
		{Name: "ap-south", Spec: ClusterLinkSpec{Endpoint: "https://ap-south.example.com", AuthRef: "secret-3"}},
	}
	client := &mockClusterClient{pushErr: map[string]error{}}
	syncer := setupSyncer(clusters, client)

	// Initially empty
	vv := syncer.GetVersionVector()
	if len(vv) != 0 {
		t.Fatalf("expected empty version vector, got %d entries", len(vv))
	}

	// After first sync
	b1 := BundleVersion{Version: 1, Hash: "v1hash", CompiledAt: time.Now()}
	syncer.SyncBundle(b1, []byte("p1"))

	vv = syncer.GetVersionVector()
	if vv["ap-south"].Version != 1 {
		t.Errorf("expected version 1, got %d", vv["ap-south"].Version)
	}

	// After second sync — version advances
	b2 := BundleVersion{Version: 2, Hash: "v2hash", CompiledAt: time.Now()}
	syncer.SyncBundle(b2, []byte("p2"))

	vv = syncer.GetVersionVector()
	if vv["ap-south"].Version != 2 {
		t.Errorf("expected version 2, got %d", vv["ap-south"].Version)
	}
}

func TestPDPSync_NoClusters(t *testing.T) {
	client := &mockClusterClient{pushErr: map[string]error{}}
	syncer := setupSyncer(nil, client)

	bundle := BundleVersion{Version: 1, Hash: "abc", CompiledAt: time.Now()}
	errs := syncer.SyncBundle(bundle, []byte("payload"))

	if errs != nil {
		t.Fatalf("expected nil errors for no clusters, got %v", errs)
	}

	vv := syncer.GetVersionVector()
	if len(vv) != 0 {
		t.Fatalf("expected empty version vector, got %d", len(vv))
	}
}
