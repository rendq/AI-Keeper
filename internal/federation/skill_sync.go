package federation

import (
	"sync"
	"time"
)

// SkillEntry represents a skill registered in the federation.
type SkillEntry struct {
	Name      string
	Version   string
	Cluster   string
	Stability string
	UpdatedAt time.Time
}

// SkillFilter defines criteria for discovering skills.
type SkillFilter struct {
	Name      string
	Stability string
	Cluster   string
}

// SkillRegistrySyncer synchronizes skill registry across federated clusters.
// Primary cluster publishes changes; linked clusters discover skills.
// Conflict resolution: same name+version → primary cluster wins.
type SkillRegistrySyncer struct {
	mu             sync.RWMutex
	primaryCluster string
	// registry key: "name/version/cluster"
	entries map[string]SkillEntry
}

// NewSkillRegistrySyncer creates a syncer with the given primary cluster name.
func NewSkillRegistrySyncer(primaryCluster string) *SkillRegistrySyncer {
	return &SkillRegistrySyncer{
		primaryCluster: primaryCluster,
		entries:        make(map[string]SkillEntry),
	}
}

// Publish stores skills from a cluster, resolving conflicts when needed.
func (s *SkillRegistrySyncer) Publish(entries []SkillEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range entries {
		key := e.Name + "/" + e.Version + "/" + e.Cluster
		conflictKey := s.findConflictKey(e)
		if conflictKey != "" {
			existing := s.entries[conflictKey]
			winner := s.resolveConflict(existing, e)
			if winner.Cluster != existing.Cluster {
				delete(s.entries, conflictKey)
			} else {
				continue
			}
		}
		s.entries[key] = e
	}
	return nil
}

// findConflictKey returns the key of an existing entry with same name+version but different cluster.
func (s *SkillRegistrySyncer) findConflictKey(e SkillEntry) string {
	for k, existing := range s.entries {
		if existing.Name == e.Name && existing.Version == e.Version && existing.Cluster != e.Cluster {
			return k
		}
	}
	return ""
}

// resolveConflict returns the winner: primary cluster always wins on same name+version.
func (s *SkillRegistrySyncer) resolveConflict(a, b SkillEntry) SkillEntry {
	return s.ResolveConflict(a, b)
}

// ResolveConflict resolves a conflict between two entries with the same name+version.
// The entry from the primary cluster wins.
func (s *SkillRegistrySyncer) ResolveConflict(a, b SkillEntry) SkillEntry {
	if a.Cluster == s.primaryCluster {
		return a
	}
	if b.Cluster == s.primaryCluster {
		return b
	}
	// Neither is primary; keep the newer one.
	if a.UpdatedAt.After(b.UpdatedAt) {
		return a
	}
	return b
}

// Discover returns skills matching the filter. Empty filter fields match all.
func (s *SkillRegistrySyncer) Discover(filter SkillFilter) []SkillEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []SkillEntry
	for _, e := range s.entries {
		if filter.Name != "" && e.Name != filter.Name {
			continue
		}
		if filter.Stability != "" && e.Stability != filter.Stability {
			continue
		}
		if filter.Cluster != "" && e.Cluster != filter.Cluster {
			continue
		}
		result = append(result, e)
	}
	return result
}
