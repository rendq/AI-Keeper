package main

import (
	"context"
	"sync"
	"time"
)

// PDPClient is the interface for pushing bundles to remote PDP instances.
type PDPClient interface {
	PushBundle(ctx context.Context, endpoint string, bundle []byte) error
}

// RegionPDP represents a PDP instance in a specific region with its sync state.
type RegionPDP struct {
	Region     string
	Endpoint   string
	BundleHash string
	Version    int64
	LastSyncAt time.Time
}

// SyncStatus reports the current sync state across all regions.
type SyncStatus struct {
	Regions    []RegionPDP
	FullySynced bool
}

// BundleSyncer pushes compiled policy bundles to all region PDP instances
// and tracks per-region sync status. On network partition, regions retain
// their stale bundle (fail-closed, not fail-open).
type BundleSyncer struct {
	mu      sync.RWMutex
	regions []RegionPDP
	client  PDPClient
}

// NewBundleSyncer creates a BundleSyncer for the given regions.
func NewBundleSyncer(regions []RegionPDP, client PDPClient) *BundleSyncer {
	copied := make([]RegionPDP, len(regions))
	copy(copied, regions)
	return &BundleSyncer{
		regions: copied,
		client:  client,
	}
}

// Push pushes the bundle to all regions concurrently. Regions that fail
// retain their previous hash/version (stale bundle). Returns the resulting
// SyncStatus after the push attempt.
func (s *BundleSyncer) Push(ctx context.Context, bundleHash string, version int64, bundle []byte) *SyncStatus {
	type result struct {
		index int
		err   error
	}

	s.mu.RLock()
	regionCount := len(s.regions)
	endpoints := make([]string, regionCount)
	for i := 0; i < regionCount; i++ {
		endpoints[i] = s.regions[i].Endpoint
	}
	s.mu.RUnlock()

	results := make(chan result, regionCount)
	for i := 0; i < regionCount; i++ {
		go func(idx int, ep string) {
			err := s.client.PushBundle(ctx, ep, bundle)
			results <- result{index: idx, err: err}
		}(i, endpoints[i])
	}

	now := time.Now()
	s.mu.Lock()
	for i := 0; i < regionCount; i++ {
		r := <-results
		if r.err == nil {
			s.regions[r.index].BundleHash = bundleHash
			s.regions[r.index].Version = version
			s.regions[r.index].LastSyncAt = now
		}
		// On failure: retain old hash/version (stale bundle, not fail-open)
	}
	s.mu.Unlock()

	return s.Status()
}

// Status returns the current sync status of all regions.
func (s *BundleSyncer) Status() *SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	regions := make([]RegionPDP, len(s.regions))
	copy(regions, s.regions)

	fullySynced := true
	if len(regions) == 0 {
		fullySynced = false
	} else {
		target := regions[0].BundleHash
		for _, r := range regions {
			if r.BundleHash != target || r.BundleHash == "" {
				fullySynced = false
				break
			}
		}
	}

	return &SyncStatus{
		Regions:     regions,
		FullySynced: fullySynced,
	}
}
