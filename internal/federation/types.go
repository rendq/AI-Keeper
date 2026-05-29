// Package federation implements multi-cluster federation via ClusterLink CRD.
package federation

import "time"

// SyncMode defines how policy bundles are synchronized between clusters.
type SyncMode string

const (
	SyncModePush SyncMode = "push"
	SyncModePull SyncMode = "pull"
)

// ClusterLinkSpec defines the desired state of a ClusterLink.
type ClusterLinkSpec struct {
	// Endpoint is the API endpoint of the remote cluster.
	Endpoint string
	// Region is the geographic region of the remote cluster.
	Region string
	// AuthRef references a Secret containing credentials for the remote cluster.
	AuthRef string
	// SyncMode specifies how bundles are synchronized (push or pull).
	SyncMode SyncMode
}

// ClusterLinkStatus defines the observed state of a ClusterLink.
type ClusterLinkStatus struct {
	// Connected indicates whether the cluster is reachable.
	Connected bool
	// LastHeartbeat is the timestamp of the last successful heartbeat.
	LastHeartbeat time.Time
	// BundleVersion is the current policy bundle version on the remote cluster.
	BundleVersion string
	// AuditLag is the replication lag for audit events.
	AuditLag time.Duration
}

// ClusterLink represents a federated cluster registration.
type ClusterLink struct {
	// Name uniquely identifies the cluster link.
	Name   string
	Spec   ClusterLinkSpec
	Status ClusterLinkStatus
}
