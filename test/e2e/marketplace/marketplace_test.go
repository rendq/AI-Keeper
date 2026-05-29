//go:build e2e

package marketplace_test

import (
	"testing"
)

// TestE2E_MarketplacePublishInstallBilling verifies the full Marketplace lifecycle:
// a Skill is published, installed by another tenant, invoked, and billing is settled.
//
// Scenario:
//  1. Tenant A publishes a SkillListing to the Marketplace (version, pricing, metadata).
//  2. Marketplace controller validates the listing and sets status to "Published".
//  3. Tenant B discovers and installs the SkillListing via Marketplace API.
//  4. Tenant B invokes the installed Skill successfully.
//  5. Billing controller records the usage event against Tenant B's budget.
//  6. Verify billing settlement: Tenant B is charged, Tenant A receives revenue share.
//
// Validates: Requirements C5
func TestE2E_MarketplacePublishInstallBilling(t *testing.T) {
	t.Skip("requires kind cluster with marketplace, billing, and skill controllers deployed")

	// TODO: create Tenant A and Tenant B with separate namespaces
	// TODO: deploy a Skill owned by Tenant A
	// TODO: create SkillListing CR with pricing (e.g., pay-per-call)
	// TODO: assert SkillListing.status.phase == "Published"
	// TODO: as Tenant B, call marketplace install API for the listing
	// TODO: assert Skill is available in Tenant B's namespace
	// TODO: invoke the installed Skill and verify successful response
	// TODO: assert BillingEvent CR is created with correct tenant and amount
	// TODO: verify Tenant B's Budget.status.consumed is incremented
	// TODO: verify revenue share record exists for Tenant A
}

// TestE2E_MarketplaceListingVersionUpgrade verifies that upgrading a published
// SkillListing propagates updates to all tenants who installed it.
//
// Scenario:
//  1. Tenant A publishes SkillListing v1.
//  2. Tenant B installs SkillListing v1.
//  3. Tenant A publishes SkillListing v2 (updated image/config).
//  4. Tenant B receives notification or auto-upgrade depending on policy.
//  5. Verify Tenant B's installed Skill reflects v2 after upgrade.
//
// Validates: Requirements C5
func TestE2E_MarketplaceListingVersionUpgrade(t *testing.T) {
	t.Skip("requires kind cluster with marketplace controller and upgrade policy")

	// TODO: publish SkillListing v1 as Tenant A
	// TODO: install as Tenant B, verify v1 is active
	// TODO: publish SkillListing v2 as Tenant A
	// TODO: trigger or wait for auto-upgrade on Tenant B
	// TODO: assert Tenant B's installed Skill image/tag matches v2
	// TODO: assert upgrade event is recorded in audit log
}
