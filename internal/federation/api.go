package federation

import (
	"encoding/json"
	"net/http"
	"time"
)

// FederationStatus represents the summary status of the federation.
type FederationStatus struct {
	TotalClusters      int                      `json:"totalClusters"`
	SyncedClusters     int                      `json:"syncedClusters"`
	MaxAuditLag        time.Duration            `json:"maxAuditLag"`
	BundleVersionVector map[string]BundleVersion `json:"bundleVersionVector"`
}

// FederationAPI provides HTTP handlers for federation dashboard and CLI.
type FederationAPI struct {
	controller *FederationController
	syncer     *PDPSyncer
}

// NewFederationAPI creates a FederationAPI wrapping the given controller and syncer.
func NewFederationAPI(controller *FederationController, syncer *PDPSyncer) *FederationAPI {
	return &FederationAPI{
		controller: controller,
		syncer:     syncer,
	}
}

// HandleListClusters handles GET /api/federation/clusters.
func (a *FederationAPI) HandleListClusters(w http.ResponseWriter, r *http.Request) {
	clusters := a.controller.ListClusters()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clusters)
}

// HandleStatus handles GET /api/federation/status.
func (a *FederationAPI) HandleStatus(w http.ResponseWriter, r *http.Request) {
	clusters := a.controller.ListClusters()
	versions := a.syncer.GetVersionVector()

	status := FederationStatus{
		TotalClusters:       len(clusters),
		BundleVersionVector: versions,
	}

	var maxLag time.Duration
	for _, cl := range clusters {
		if cl.Status.Connected && versions[cl.Name].Version > 0 {
			status.SyncedClusters++
		}
		if cl.Status.AuditLag > maxLag {
			maxLag = cl.Status.AuditLag
		}
	}
	status.MaxAuditLag = maxLag

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
