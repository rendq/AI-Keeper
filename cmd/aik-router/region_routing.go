package router

import "errors"

// Errors returned by region-aware routing.
var (
	ErrCrossBorderDenied = errors.New("router: cross-border routing denied by policy")
	ErrNoEndpoints       = errors.New("router: no endpoints available")
)

// RegionRoutingConfig controls region-aware routing behavior.
type RegionRoutingConfig struct {
	AllowedRegions    []string
	ForbidCrossBorder bool
}

// ModelEndpointRegion describes an endpoint with its region metadata.
type ModelEndpointRegion struct {
	Endpoint  string
	Region    string
	Continent string
}

// RegionRouter provides region-aware endpoint selection.
type RegionRouter struct{}

// NewRegionRouter creates a RegionRouter.
func NewRegionRouter() *RegionRouter {
	return &RegionRouter{}
}

// RouteByRegion selects an endpoint based on region affinity.
// Priority: exact region match → same continent → deny if forbidCrossBorder, else pick any.
func (r *RegionRouter) RouteByRegion(userRegion string, endpoints []ModelEndpointRegion, config RegionRoutingConfig) (*ModelEndpointRegion, error) {
	if len(endpoints) == 0 {
		return nil, ErrNoEndpoints
	}

	userContinent := RegionToContinent(userRegion)

	// 1. Exact region match.
	for i := range endpoints {
		if endpoints[i].Region == userRegion {
			return &endpoints[i], nil
		}
	}

	// 2. Same continent fallback.
	for i := range endpoints {
		if endpoints[i].Continent == userContinent {
			return &endpoints[i], nil
		}
	}

	// 3. Cross-border check.
	if config.ForbidCrossBorder {
		return nil, ErrCrossBorderDenied
	}

	// 4. Pick any available endpoint.
	return &endpoints[0], nil
}

// CountryToRegion maps ISO country codes to cloud regions.
func CountryToRegion(country string) string {
	m := map[string]string{
		"CN": "cn-north",
		"JP": "ap-northeast",
		"KR": "ap-northeast",
		"SG": "ap-southeast",
		"AU": "ap-southeast",
		"IN": "ap-south",
		"US": "us-east",
		"CA": "us-east",
		"BR": "sa-east",
		"DE": "eu-west",
		"FR": "eu-west",
		"GB": "eu-west",
		"NL": "eu-west",
	}
	if r, ok := m[country]; ok {
		return r
	}
	return "us-east" // default
}

// RegionToContinent maps cloud regions to continents.
func RegionToContinent(region string) string {
	m := map[string]string{
		"cn-north":     "asia",
		"ap-northeast": "asia",
		"ap-southeast": "asia",
		"ap-south":     "asia",
		"us-east":      "americas",
		"us-west":      "americas",
		"sa-east":      "americas",
		"eu-west":      "europe",
		"eu-central":   "europe",
	}
	if c, ok := m[region]; ok {
		return c
	}
	return "unknown"
}
