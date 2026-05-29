//go:build e2e

package airgapped_test

import (
	"testing"
)

// TestE2E_AirgappedInstallFromBundle verifies that the platform can be fully
// installed in an air-gapped (isolated) network from a pre-built tar bundle.
//
// Scenario:
//  1. Start with an isolated cluster with no internet access.
//  2. Load the air-gap bundle (tar archive with all images and charts).
//  3. Run the installer against the isolated cluster.
//  4. Verify all platform components start successfully.
//  5. Execute a basic agent invocation to confirm functionality.
//
// Validates: Requirements C6.5, C6.1
func TestE2E_AirgappedInstallFromBundle(t *testing.T) {
	t.Skip("requires isolated cluster environment with air-gap bundle available")

	// TODO: verify cluster has no external network access (curl to public endpoint fails)
	// TODO: load air-gap tar bundle into local registry
	// TODO: run helm install from bundle-provided chart and values
	// TODO: wait for all deployments to reach Ready state (timeout 10m)
	// TODO: list all pods and assert none are in ImagePullBackOff
	// TODO: assert core controllers are running (tenant, policy, model, agent)
	// TODO: assert CRDs are installed (kubectl get crd | grep aip)
}

// TestE2E_AirgappedBasicFunctionality verifies core platform operations work
// after an air-gapped installation without any external dependencies.
//
// Scenario:
//  1. After air-gapped install, create a Tenant.
//  2. Deploy a Skill and Agent within the tenant.
//  3. Invoke the Agent and verify response.
//  4. Verify audit events are recorded locally.
//
// Validates: Requirements C6.5, C6.1
func TestE2E_AirgappedBasicFunctionality(t *testing.T) {
	t.Skip("requires completed air-gapped installation")

	// TODO: create Tenant CR and wait for Ready
	// TODO: deploy Skill CR referencing bundled model image
	// TODO: deploy Agent CR referencing the Skill
	// TODO: invoke Agent via internal service endpoint
	// TODO: assert response is valid and non-empty
	// TODO: query audit events and assert invocation was recorded
	// TODO: verify no external network calls were attempted (check network policy logs)
}

// TestE2E_AirgappedBundleIntegrity verifies that the air-gap bundle contains
// all required images and no components fail due to missing artifacts.
//
// Scenario:
//  1. Extract and inspect the air-gap bundle manifest.
//  2. Verify all image references in Helm values exist in the bundle.
//  3. Verify chart dependencies are self-contained.
//
// Validates: Requirements C6.5, C6.1
func TestE2E_AirgappedBundleIntegrity(t *testing.T) {
	t.Skip("requires air-gap bundle tar file")

	// TODO: extract bundle manifest.json
	// TODO: parse all image references from Helm values
	// TODO: assert each image reference exists in bundle's images/ directory
	// TODO: verify Helm chart has no external repository dependencies
	// TODO: verify total bundle size is within documented limits
}
