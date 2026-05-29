package federation

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BundleVersion represents the version metadata of a compiled policy bundle.
type BundleVersion struct {
	Version    int64
	Hash       string
	CompiledAt time.Time
}

// ClusterClient is the interface for pushing bundles to remote cluster PDPs.
type ClusterClient interface {
	PushBundle(ctx context.Context, endpoint, authRef string, bundle []byte) error
}

// PDPSyncer synchronizes compiled policy bundles to all linked clusters.
type PDPSyncer struct {
	controller *FederationController
	client     ClusterClient

	mu       sync.RWMutex
	versions map[string]BundleVersion
}

// NewPDPSyncer creates a PDPSyncer backed by the given controller and client.
func NewPDPSyncer(controller *FederationController, client ClusterClient) *PDPSyncer {
	return &PDPSyncer{
		controller: controller,
		client:     client,
		versions:   make(map[string]BundleVersion),
	}
}

// SyncBundle pushes the bundle to all linked clusters and returns per-cluster errors.
// Successful clusters have their BundleVersion status updated.
func (s *PDPSyncer) SyncBundle(bundle BundleVersion, payload []byte) map[string]error {
	clusters := s.controller.ListClusters()
	if len(clusters) == 0 {
		return nil
	}

	errs := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cl := range clusters {
		wg.Add(1)
		go func(cl ClusterLink) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := s.client.PushBundle(ctx, cl.Spec.Endpoint, cl.Spec.AuthRef, payload)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[cl.Name] = fmt.Errorf("push to %s: %w", cl.Name, err)
			} else {
				// Update version vector
				s.mu.Lock()
				s.versions[cl.Name] = bundle
				s.mu.Unlock()
				// Update cluster link status
				s.controller.updateBundleVersion(cl.Name, bundle)
			}
		}(cl)
	}
	wg.Wait()

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// GetVersionVector returns the current per-cluster bundle version map.
func (s *PDPSyncer) GetVersionVector() map[string]BundleVersion {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]BundleVersion, len(s.versions))
	for k, v := range s.versions {
		result[k] = v
	}
	return result
}
