//go:build pbt

// Feature: ai-platform, Property 22: OBO 身份链路完整
//
// Generator: Random (user, agent, tool) invocations
// Oracle: representation.mode=on_behalf_of ⇒ tool receives token with
//         claim.onBehalfOf == user.id, never lost or replaced.
//         Token must also contain tenantId and agentName.
// Property: P22 / Validates: F10, B3.6, C7.1

package identity

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func pbtSeed() int64 {
	if env := os.Getenv("AIP_PBT_SEED"); env != "" {
		if v, err := strconv.ParseInt(env, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano()
}

// setupPBTBrokerAndExchanger creates a broker with a mock OIDC provider and
// a pre-registered service account that allows OBO.
func setupPBTBrokerAndExchanger(tenantID, saFQN string) *Exchanger {
	broker := NewBroker(BrokerConfig{
		Issuer: "https://aip.test/identity",
	})
	broker.RegisterProvider(&mockOIDCVerifier{
		issuer: "https://idp.test",
		verifyFn: func(_ context.Context, rawToken string, _ string) (*TokenClaims, error) {
			// The raw token IS the user ID in our test setup.
			return &TokenClaims{
				Subject:   rawToken,
				Issuer:    "https://idp.test",
				IssuedAt:  time.Now(),
				ExpiresAt: time.Now().Add(5 * time.Minute),
				TokenID:   "tok-" + rawToken,
			}, nil
		},
	})
	_ = broker.RegisterSA(ServiceAccountInfo{
		FQN:             saFQN,
		TenantID:        tenantID,
		TokenLifetime:   15 * time.Minute,
		AllowOnBehalfOf: true,
	})
	return NewExchanger(broker)
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// genUserID generates random user identifiers (simulates different end-users).
func genUserID() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9]{3,15}")
}

// genAgentName generates random agent names.
func genAgentName() gopter.Gen {
	return gen.RegexMatch("[a-z][a-z0-9-]{2,20}")
}

// genTenantID generates random tenant identifiers.
func genTenantID() gopter.Gen {
	return gen.RegexMatch("tenant-[a-z0-9]{3,10}")
}

// genSAFQN generates random service account fully-qualified names.
func genSAFQN() gopter.Gen {
	return gen.RegexMatch("[a-z]{3,10}/[a-z][a-z0-9-]{2,15}")
}

// genToolResource generates random tool resource URIs.
func genToolResource() gopter.Gen {
	return gen.RegexMatch("https://[a-z]{3,10}\\.example\\.com/api/[a-z]{3,10}")
}

// genAudience generates random audiences.
func genAudience() gopter.Gen {
	return gen.RegexMatch("[a-z]{3,10}-api")
}

// OBOInvocation represents a single (user, agent, tool) call for testing.
type OBOInvocation struct {
	UserID       string
	AgentName    string
	TenantID     string
	SAFQN        string
	ToolResource string
	ToolAudience string
}

func genOBOInvocation() gopter.Gen {
	return gopter.CombineGens(
		genUserID(),
		genAgentName(),
		genTenantID(),
		genSAFQN(),
		genToolResource(),
		genAudience(),
	).Map(func(values []interface{}) OBOInvocation {
		return OBOInvocation{
			UserID:       values[0].(string),
			AgentName:    values[1].(string),
			TenantID:     values[2].(string),
			SAFQN:        values[3].(string),
			ToolResource: values[4].(string),
			ToolAudience: values[5].(string),
		}
	})
}

// ---------------------------------------------------------------------------
// Property Test
// ---------------------------------------------------------------------------

// **Validates: Requirements F10, B3.6, C7.1**
func TestProperty22(t *testing.T) {
	seed := pbtSeed()
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Property 22a: OBO token preserves user identity (onBehalfOf == user.id)
	properties.Property("OBO token onBehalfOf equals original user ID", prop.ForAll(
		func(inv OBOInvocation) bool {
			exchanger := setupPBTBrokerAndExchanger(inv.TenantID, inv.SAFQN)
			ctx := context.Background()

			// The user's ID token is the user ID itself in our mock setup.
			toolAuth := ToolAuthConfig{
				Mode:     AuthModeOAuth2OBO,
				Resource: inv.ToolResource,
				Audience: inv.ToolAudience,
			}

			claims, err := exchanger.GetTokenForTool(ctx, toolAuth, inv.UserID, inv.SAFQN, inv.AgentName)
			if err != nil {
				t.Logf("unexpected error: %v (inv=%+v)", err, inv)
				return false
			}

			// Oracle: onBehalfOf MUST equal the original user ID
			if claims.OnBehalfOf != inv.UserID {
				t.Logf("FAIL: onBehalfOf=%q, want=%q", claims.OnBehalfOf, inv.UserID)
				return false
			}
			return true
		},
		genOBOInvocation(),
	))

	// Property 22b: OBO token contains required tenantId claim
	properties.Property("OBO token contains tenantId", prop.ForAll(
		func(inv OBOInvocation) bool {
			exchanger := setupPBTBrokerAndExchanger(inv.TenantID, inv.SAFQN)
			ctx := context.Background()

			toolAuth := ToolAuthConfig{
				Mode:     AuthModeOAuth2OBO,
				Resource: inv.ToolResource,
				Audience: inv.ToolAudience,
			}

			claims, err := exchanger.GetTokenForTool(ctx, toolAuth, inv.UserID, inv.SAFQN, inv.AgentName)
			if err != nil {
				t.Logf("unexpected error: %v", err)
				return false
			}

			// Oracle: tenantId must be present and match the SA's tenant
			if claims.TenantID != inv.TenantID {
				t.Logf("FAIL: tenantId=%q, want=%q", claims.TenantID, inv.TenantID)
				return false
			}
			return true
		},
		genOBOInvocation(),
	))

	// Property 22c: OBO token contains agentName claim
	properties.Property("OBO token contains agentName", prop.ForAll(
		func(inv OBOInvocation) bool {
			exchanger := setupPBTBrokerAndExchanger(inv.TenantID, inv.SAFQN)
			ctx := context.Background()

			toolAuth := ToolAuthConfig{
				Mode:     AuthModeOAuth2OBO,
				Resource: inv.ToolResource,
				Audience: inv.ToolAudience,
			}

			claims, err := exchanger.GetTokenForTool(ctx, toolAuth, inv.UserID, inv.SAFQN, inv.AgentName)
			if err != nil {
				t.Logf("unexpected error: %v", err)
				return false
			}

			// Oracle: agentName must match the requesting agent
			if claims.AgentName != inv.AgentName {
				t.Logf("FAIL: agentName=%q, want=%q", claims.AgentName, inv.AgentName)
				return false
			}
			return true
		},
		genOBOInvocation(),
	))

	// Property 22d: OBO identity chain is complete (user → agent → tool)
	// The token issued for tool invocation must carry all three identifiers
	// to prove unbroken chain: user (onBehalfOf), agent (agentName), tool (audience/resource).
	properties.Property("OBO identity chain is complete and unbroken", prop.ForAll(
		func(inv OBOInvocation) bool {
			exchanger := setupPBTBrokerAndExchanger(inv.TenantID, inv.SAFQN)
			ctx := context.Background()

			toolAuth := ToolAuthConfig{
				Mode:     AuthModeOAuth2OBO,
				Resource: inv.ToolResource,
				Audience: inv.ToolAudience,
			}

			claims, err := exchanger.GetTokenForTool(ctx, toolAuth, inv.UserID, inv.SAFQN, inv.AgentName)
			if err != nil {
				t.Logf("unexpected error: %v", err)
				return false
			}

			// Full chain invariant: all identity elements present and correct
			chainOK := claims.OnBehalfOf == inv.UserID &&
				claims.AgentName == inv.AgentName &&
				claims.TenantID == inv.TenantID &&
				claims.OnBehalfOf != "" &&
				claims.AgentName != "" &&
				claims.TenantID != ""

			if !chainOK {
				t.Logf("FAIL: identity chain broken: onBehalfOf=%q agentName=%q tenantId=%q",
					claims.OnBehalfOf, claims.AgentName, claims.TenantID)
				return false
			}
			return true
		},
		genOBOInvocation(),
	))

	// Property 22e: OBO token is required when mode=on_behalf_of (no SA fallback)
	// When user context is missing and mode is OBO, exchange MUST fail.
	properties.Property("OBO mode rejects missing user context", prop.ForAll(
		func(inv OBOInvocation) bool {
			exchanger := setupPBTBrokerAndExchanger(inv.TenantID, inv.SAFQN)
			ctx := context.Background()

			toolAuth := ToolAuthConfig{
				Mode:     AuthModeOAuth2OBO,
				Resource: inv.ToolResource,
				Audience: inv.ToolAudience,
			}

			// Pass empty user token — must fail with ErrMissingUserContext
			_, err := exchanger.GetTokenForTool(ctx, toolAuth, "", inv.SAFQN, inv.AgentName)
			if err == nil {
				t.Logf("FAIL: expected error when user context is missing")
				return false
			}
			return true
		},
		genOBOInvocation(),
	))

	properties.TestingRun(t)
}
