//go:build pbt

package modelrouter

import (
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements C2**
// Property P17: Cross-border rejection invariant.
// When forbidCrossBorder=true and tenantRegion != endpointRegion (both non-empty),
// CheckCrossBorder always rejects. When regions are the same, it always allows.

var regionPool = []string{"us-east-1", "eu-west-1", "ap-southeast-1", "cn-north-1", "me-south-1"}

func genRegion() gopter.Gen {
	return gen.OneConstOf(regionPool[0], regionPool[1], regionPool[2], regionPool[3], regionPool[4])
}

func genForbid() gopter.Gen {
	return gen.Bool()
}

func TestProperty17(t *testing.T) {
	seed := time.Now().UnixNano()
	if s, ok := os.LookupEnv("AIP_PBT_SEED"); ok {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200
	params.Rng = rand.New(rand.NewSource(seed))

	properties := gopter.NewProperties(params)

	// P17a: forbid=true AND different non-empty regions → always rejected
	properties.Property("P17a: cross-border with forbid=true and different regions is always rejected", prop.ForAll(
		func(tenantRegion, endpointRegion string) bool {
			if tenantRegion == endpointRegion {
				// Skip same-region pairs for this sub-property
				return true
			}
			allowed, reason := CheckCrossBorder(tenantRegion, endpointRegion, true)
			return !allowed && reason != ""
		},
		genRegion(),
		genRegion(),
	))

	// P17b: same region → always allowed regardless of forbid flag
	properties.Property("P17b: same region is always allowed regardless of forbid", prop.ForAll(
		func(region string, forbid bool) bool {
			allowed, reason := CheckCrossBorder(region, region, forbid)
			return allowed && reason == ""
		},
		genRegion(),
		genForbid(),
	))

	// P17c: forbid=false → always allowed regardless of regions
	properties.Property("P17c: forbid=false always allows routing", prop.ForAll(
		func(tenantRegion, endpointRegion string) bool {
			allowed, reason := CheckCrossBorder(tenantRegion, endpointRegion, false)
			return allowed && reason == ""
		},
		genRegion(),
		genRegion(),
	))

	properties.TestingRun(t)
}
