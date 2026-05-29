package federation

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// FederationController manages cluster registration, heartbeat, and status sync.
type FederationController struct {
	mu       sync.RWMutex
	clusters map[string]*ClusterLink
}

// NewFederationController creates a new FederationController with an in-memory registry.
func NewFederationController() *FederationController {
	return &FederationController{
		clusters: make(map[string]*ClusterLink),
	}
}

// Register adds a new cluster to the federation. Returns an error if already registered.
func (fc *FederationController) Register(link ClusterLink) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if _, exists := fc.clusters[link.Name]; exists {
		return errors.New("cluster already registered: " + link.Name)
	}
	link.Status.Connected = true
	link.Status.LastHeartbeat = time.Now()
	fc.clusters[link.Name] = &link
	return nil
}

// Heartbeat updates the last heartbeat timestamp for a registered cluster.
func (fc *FederationController) Heartbeat(clusterName string) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	cl, exists := fc.clusters[clusterName]
	if !exists {
		return errors.New("cluster not found: " + clusterName)
	}
	cl.Status.LastHeartbeat = time.Now()
	cl.Status.Connected = true
	return nil
}

// ListClusters returns all registered cluster links.
func (fc *FederationController) ListClusters() []ClusterLink {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	result := make([]ClusterLink, 0, len(fc.clusters))
	for _, cl := range fc.clusters {
		result = append(result, *cl)
	}
	return result
}

// updateBundleVersion updates the bundle version status for a cluster.
func (fc *FederationController) updateBundleVersion(clusterName string, bv BundleVersion) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if cl, exists := fc.clusters[clusterName]; exists {
		cl.Status.BundleVersion = fmt.Sprintf("v%d-%s", bv.Version, bv.Hash)
	}
}

// Deregister removes a cluster from the federation.
func (fc *FederationController) Deregister(clusterName string) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if _, exists := fc.clusters[clusterName]; !exists {
		return errors.New("cluster not found: " + clusterName)
	}
	delete(fc.clusters, clusterName)
	return nil
}
