package federation

import (
	"testing"
	"time"
)

func newTestLink(name string) ClusterLink {
	return ClusterLink{
		Name: name,
		Spec: ClusterLinkSpec{
			Endpoint: "https://" + name + ".example.com",
			Region:   "us-west-2",
			AuthRef:  "secret-" + name,
			SyncMode: SyncModePush,
		},
	}
}

func TestRegisterCluster(t *testing.T) {
	fc := NewFederationController()
	link := newTestLink("cluster-a")

	if err := fc.Register(link); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clusters := fc.ListClusters()
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Name != "cluster-a" {
		t.Errorf("expected name cluster-a, got %s", clusters[0].Name)
	}
	if !clusters[0].Status.Connected {
		t.Error("expected cluster to be connected after register")
	}
}

func TestHeartbeatUpdatesLastHeartbeat(t *testing.T) {
	fc := NewFederationController()
	link := newTestLink("cluster-b")
	_ = fc.Register(link)

	// Get initial heartbeat time.
	clusters := fc.ListClusters()
	initial := clusters[0].Status.LastHeartbeat

	time.Sleep(2 * time.Millisecond)

	if err := fc.Heartbeat("cluster-b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clusters = fc.ListClusters()
	if !clusters[0].Status.LastHeartbeat.After(initial) {
		t.Error("expected LastHeartbeat to be updated after heartbeat")
	}
}

func TestHeartbeatUnknownCluster(t *testing.T) {
	fc := NewFederationController()
	if err := fc.Heartbeat("nonexistent"); err == nil {
		t.Error("expected error for unknown cluster heartbeat")
	}
}

func TestListClusters(t *testing.T) {
	fc := NewFederationController()
	_ = fc.Register(newTestLink("cluster-1"))
	_ = fc.Register(newTestLink("cluster-2"))
	_ = fc.Register(newTestLink("cluster-3"))

	clusters := fc.ListClusters()
	if len(clusters) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(clusters))
	}
}

func TestDeregisterRemovesCluster(t *testing.T) {
	fc := NewFederationController()
	_ = fc.Register(newTestLink("cluster-x"))

	if err := fc.Deregister("cluster-x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clusters := fc.ListClusters()
	if len(clusters) != 0 {
		t.Fatalf("expected 0 clusters after deregister, got %d", len(clusters))
	}
}

func TestDeregisterUnknownCluster(t *testing.T) {
	fc := NewFederationController()
	if err := fc.Deregister("ghost"); err == nil {
		t.Error("expected error for deregistering unknown cluster")
	}
}

func TestDuplicateRegisterReturnsError(t *testing.T) {
	fc := NewFederationController()
	link := newTestLink("cluster-dup")
	_ = fc.Register(link)

	err := fc.Register(link)
	if err == nil {
		t.Error("expected error for duplicate register")
	}
}
