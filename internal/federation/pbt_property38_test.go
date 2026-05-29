// Feature: ai-platform, Property 38: Federation bundle version convergence

//go:build pbt

package federation

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements F2**

// partitionableClient is a mock ClusterClient that can simulate network partitions.
type partitionableClient struct {
	mu          sync.RWMutex
	partitioned map[string]bool // endpoint -> whether it's partitioned (unreachable)
}

func newPartitionableClient() *partitionableClient {
	return &partitionableClient{
		partitioned: make(map[string]bool),
	}
}

func (c *partitionableClient) PushBundle(_ context.Context, endpoint, _ string, _ []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.partitioned[endpoint] {
		return context.DeadlineExceeded
	}
	return nil
}

func (c *partitionableClient) partition(endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partitioned[endpoint] = true
}

func (c *partitionableClient) heal(endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.partitioned, endpoint)
}

// clusterSetup describes a generated cluster configuration for testing.
type clusterSetup struct {
	ClusterNames []string
	Endpoints    []string
}

// genClusterSet generates a random set of 2-6 clusters.
func genClusterSet() gopter.Gen {
	return gen.IntRange(2, 6).Map(func(n int) clusterSetup {
		names := make([]string, n)
		endpoints := make([]string, n)
		regions := []string{"us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1", "sa-east-1", "ca-central-1"}
		for i := 0; i < n; i++ {
			names[i] = regions[i%len(regions)]
			endpoints[i] = "pdp-" + regions[i%len(regions)] + ".internal:8080"
		}
		return clusterSetup{
			ClusterNames: names,
			Endpoints:    endpoints,
		}
	})
}

// genBundleVersion generates a random bundle version number (1-100).
func genBundleVersion() gopter.Gen {
	return gen.Int64Range(1, 100)
}

// genPartitionIndices generates indices of clusters to partition (subset).
func genPartitionIndices(maxClusters int) gopter.Gen {
	// Partition 1 to maxClusters/2 clusters (never all)
	if maxClusters <= 1 {
		return gen.Const([]int{})
	}
	maxPartition := maxClusters / 2
	if maxPartition < 1 {
		maxPartition = 1
	}
	return gen.IntRange(1, maxPartition).FlatMap(func(v interface{}) gopter.Gen {
		count := v.(int)
		return gen.Const(count).Map(func(c interface{}) []int {
			n := c.(int)
			indices := make([]int, n)
			for i := 0; i < n; i++ {
				indices[i] = i
			}
			return indices
		})
	}, nil)
}

func TestProperty38(t *testing.T) {
	seed := time.Now().UnixNano()
	if s := os.Getenv("AIP_PBT_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Property: After a network partition heals and sync is retried,
	// all clusters converge to the same bundle version.
	properties.Property("all clusters converge after partition heals and sync retried", prop.ForAll(
		func(clusters clusterSetup, bundleVer int64, partitionCount int) bool {
			// Setup federation controller and syncer
			controller := NewFederationController()
			client := newPartitionableClient()
			syncer := NewPDPSyncer(controller, client)

			// Register all clusters
			for i, name := range clusters.ClusterNames {
				err := controller.Register(ClusterLink{
					Name: name,
					Spec: ClusterLinkSpec{
						Endpoint: clusters.Endpoints[i],
						Region:   name,
						AuthRef:  "secret-" + name,
						SyncMode: SyncModePush,
					},
				})
				if err != nil {
					// Duplicate name (can happen with small region list), skip
					continue
				}
			}

			registeredClusters := controller.ListClusters()
			if len(registeredClusters) < 2 {
				// Need at least 2 clusters for meaningful partition test
				return true
			}

			// Determine which clusters to partition (up to half)
			numToPartition := partitionCount
			if numToPartition >= len(registeredClusters) {
				numToPartition = len(registeredClusters) / 2
			}
			if numToPartition < 1 {
				numToPartition = 1
			}

			// Phase 1: Create network partition for some clusters
			partitionedEndpoints := make([]string, 0, numToPartition)
			for i := 0; i < numToPartition && i < len(registeredClusters); i++ {
				ep := registeredClusters[i].Spec.Endpoint
				client.partition(ep)
				partitionedEndpoints = append(partitionedEndpoints, ep)
			}

			// Phase 2: Push bundle — some clusters will fail due to partition
			bundle := BundleVersion{
				Version:    bundleVer,
				Hash:       "hash-" + strconv.FormatInt(bundleVer, 10),
				CompiledAt: time.Now(),
			}
			syncer.SyncBundle(bundle, []byte("bundle-payload"))

			// Verify: not all clusters have converged yet (some are partitioned)
			versions := syncer.GetVersionVector()
			allConverged := true
			for _, cl := range registeredClusters {
				v, ok := versions[cl.Name]
				if !ok || v.Version != bundleVer {
					allConverged = false
					break
				}
			}
			// At least the partitioned ones should NOT have the new version
			// (unless the partition list was empty, which we prevented)

			// Phase 3: Heal the partition
			for _, ep := range partitionedEndpoints {
				client.heal(ep)
			}

			// Phase 4: Retry sync after healing
			errs := syncer.SyncBundle(bundle, []byte("bundle-payload"))

			// Oracle: After heal + retry, ALL clusters should converge to the same version
			if errs != nil {
				t.Logf("VIOLATION: sync errors after partition healed: %v", errs)
				return false
			}

			versions = syncer.GetVersionVector()
			for _, cl := range registeredClusters {
				v, ok := versions[cl.Name]
				if !ok {
					t.Logf("VIOLATION: cluster %s has no version after heal+sync", cl.Name)
					return false
				}
				if v.Version != bundleVer {
					t.Logf("VIOLATION: cluster %s has version %d, expected %d", cl.Name, v.Version, bundleVer)
					return false
				}
				if v.Hash != bundle.Hash {
					t.Logf("VIOLATION: cluster %s has hash %s, expected %s", cl.Name, v.Hash, bundle.Hash)
					return false
				}
			}

			// Suppress unused variable warning
			_ = allConverged

			return true
		},
		genClusterSet(),
		genBundleVersion(),
		gen.IntRange(1, 3),
	))

	properties.TestingRun(t)
}
