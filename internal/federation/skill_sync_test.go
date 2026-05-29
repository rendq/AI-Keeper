package federation

import (
	"testing"
	"time"
)

func TestSkillRegistrySync_Publish(t *testing.T) {
	syncer := NewSkillRegistrySyncer("cluster-a")

	entries := []SkillEntry{
		{Name: "summarize", Version: "v1", Cluster: "cluster-a", Stability: "stable", UpdatedAt: time.Now()},
		{Name: "translate", Version: "v2", Cluster: "cluster-a", Stability: "beta", UpdatedAt: time.Now()},
	}

	if err := syncer.Publish(entries); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	all := syncer.Discover(SkillFilter{})
	if len(all) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(all))
	}
}

func TestSkillRegistrySync_Discover(t *testing.T) {
	syncer := NewSkillRegistrySyncer("cluster-a")
	now := time.Now()

	_ = syncer.Publish([]SkillEntry{
		{Name: "summarize", Version: "v1", Cluster: "cluster-a", Stability: "stable", UpdatedAt: now},
		{Name: "translate", Version: "v1", Cluster: "cluster-a", Stability: "beta", UpdatedAt: now},
		{Name: "summarize", Version: "v2", Cluster: "cluster-b", Stability: "stable", UpdatedAt: now},
	})

	// Filter by name
	results := syncer.Discover(SkillFilter{Name: "summarize"})
	if len(results) != 2 {
		t.Fatalf("expected 2 skills named summarize, got %d", len(results))
	}

	// Filter by stability
	results = syncer.Discover(SkillFilter{Stability: "beta"})
	if len(results) != 1 {
		t.Fatalf("expected 1 beta skill, got %d", len(results))
	}
	if results[0].Name != "translate" {
		t.Fatalf("expected translate, got %s", results[0].Name)
	}
}

func TestSkillRegistrySync_ConflictResolution(t *testing.T) {
	syncer := NewSkillRegistrySyncer("cluster-a")
	now := time.Now()

	// Publish from secondary cluster first
	_ = syncer.Publish([]SkillEntry{
		{Name: "summarize", Version: "v1", Cluster: "cluster-b", Stability: "beta", UpdatedAt: now},
	})

	// Publish same name+version from primary cluster
	_ = syncer.Publish([]SkillEntry{
		{Name: "summarize", Version: "v1", Cluster: "cluster-a", Stability: "stable", UpdatedAt: now.Add(-time.Hour)},
	})

	results := syncer.Discover(SkillFilter{Name: "summarize", Cluster: ""})
	if len(results) != 1 {
		t.Fatalf("expected 1 skill after conflict resolution, got %d", len(results))
	}
	if results[0].Cluster != "cluster-a" {
		t.Fatalf("expected primary cluster-a to win, got %s", results[0].Cluster)
	}
	if results[0].Stability != "stable" {
		t.Fatalf("expected stability=stable from primary, got %s", results[0].Stability)
	}
}

func TestSkillRegistrySync_MultiCluster(t *testing.T) {
	syncer := NewSkillRegistrySyncer("cluster-a")
	now := time.Now()

	_ = syncer.Publish([]SkillEntry{
		{Name: "summarize", Version: "v1", Cluster: "cluster-a", Stability: "stable", UpdatedAt: now},
		{Name: "translate", Version: "v1", Cluster: "cluster-b", Stability: "stable", UpdatedAt: now},
		{Name: "classify", Version: "v1", Cluster: "cluster-c", Stability: "beta", UpdatedAt: now},
	})

	all := syncer.Discover(SkillFilter{})
	if len(all) != 3 {
		t.Fatalf("expected 3 skills from multiple clusters, got %d", len(all))
	}

	// Each cluster's skill is discoverable
	for _, cluster := range []string{"cluster-a", "cluster-b", "cluster-c"} {
		results := syncer.Discover(SkillFilter{Cluster: cluster})
		if len(results) != 1 {
			t.Fatalf("expected 1 skill from %s, got %d", cluster, len(results))
		}
	}
}
