// Feature: ai-platform, Property 33: Multi-Region data residency invariant

//go:build pbt

package router

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements C2, F7**

// allRegions is the set of known cloud regions used for generation.
var allRegions = []string{
	"cn-north", "ap-northeast", "ap-southeast", "ap-south",
	"us-east", "us-west", "sa-east", "eu-west", "eu-central",
}

// allCountries is the set of known country codes used for generation.
var allCountries = []string{
	"CN", "JP", "KR", "SG", "AU", "IN", "US", "CA", "BR", "DE", "FR", "GB", "NL",
}

// genRegion generates a random region from the known set.
func genRegion() gopter.Gen {
	return gen.OneConstOf(
		"cn-north", "ap-northeast", "ap-southeast", "ap-south",
		"us-east", "us-west", "sa-east", "eu-west", "eu-central",
	)
}

// genCountry generates a random country code.
func genCountry() gopter.Gen {
	return gen.OneConstOf(
		"CN", "JP", "KR", "SG", "AU", "IN", "US", "CA", "BR", "DE", "FR", "GB", "NL",
	)
}

// genEndpoints generates a non-empty slice of ModelEndpointRegion with random regions.
func genEndpoints() gopter.Gen {
	epGen := genRegion().Map(func(region string) ModelEndpointRegion {
		continent := RegionToContinent(region)
		return ModelEndpointRegion{
			Endpoint:  "https://" + region + ".api.example.com",
			Region:    region,
			Continent: continent,
		}
	})
	return gen.SliceOfN(6, epGen).SuchThat(func(v interface{}) bool {
		eps, ok := v.([]ModelEndpointRegion)
		return ok && len(eps) > 0
	})
}

// genAllowedRegions generates a non-empty subset of known regions.
func genAllowedRegions() gopter.Gen {
	return gen.SliceOfN(5, genRegion()).SuchThat(func(v interface{}) bool {
		rs, ok := v.([]string)
		return ok && len(rs) > 0
	})
}

// p33Input holds the generated inputs for property 33.
type p33Input struct {
	UserCountry       string
	Endpoints         []ModelEndpointRegion
	AllowedRegions    []string
	ForbidCrossBorder bool
}

func TestProperty33(t *testing.T) {
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

	// Property P33: When forbidCrossBorder=true, the router must NEVER route to a
	// different continent than the user's. It either returns an endpoint in the same
	// continent (or matching region), or returns ErrCrossBorderDenied.
	properties.Property("forbidCrossBorder=true => route stays in user continent or denied", prop.ForAll(
		func(country string, endpoints []ModelEndpointRegion, allowedRegions []string, forbid bool) bool {
			userRegion := CountryToRegion(country)
			userContinent := RegionToContinent(userRegion)

			rr := NewRegionRouter()
			config := RegionRoutingConfig{
				AllowedRegions:    allowedRegions,
				ForbidCrossBorder: forbid,
			}

			result, err := rr.RouteByRegion(userRegion, endpoints, config)

			if !forbid {
				// When cross-border is allowed, we only check that we get a valid result
				// (either an endpoint or ErrNoEndpoints for empty list).
				if err != nil && err != ErrNoEndpoints {
					return false
				}
				return true
			}

			// forbidCrossBorder=true case:
			// Must either deny or route within the same continent.
			if err != nil {
				// Only ErrCrossBorderDenied and ErrNoEndpoints are acceptable errors.
				return err == ErrCrossBorderDenied || err == ErrNoEndpoints
			}

			// If a route was returned, it must be in the user's region or continent.
			if result.Region == userRegion {
				return true
			}
			if result.Continent == userContinent {
				return true
			}

			// Routed to a different continent — invariant violated!
			return false
		},
		genCountry(),
		genEndpoints(),
		genAllowedRegions(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}
