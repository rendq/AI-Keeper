// Package modelrouter provides routing logic for model endpoints including
// cross-border compliance checks.
package modelrouter

// CrossBorderConfig holds the cross-border routing configuration for a tenant.
type CrossBorderConfig struct {
	TenantRegion      string
	ForbidCrossBorder bool
}

// CheckCrossBorder determines whether a request is allowed to route to an
// endpoint in a different region. If forbid is true and tenantRegion differs
// from endpointRegion, the route is rejected.
func CheckCrossBorder(tenantRegion, endpointRegion string, forbid bool) (allowed bool, reason string) {
	// If cross-border is not forbidden, always allow.
	if !forbid {
		return true, ""
	}

	// If either region is empty, we cannot determine cross-border status — allow.
	if tenantRegion == "" || endpointRegion == "" {
		return true, ""
	}

	// Same region is always allowed.
	if tenantRegion == endpointRegion {
		return true, ""
	}

	// Different region with forbid=true → reject.
	return false, "cross-border routing denied: tenant region " + tenantRegion +
		" cannot route to endpoint region " + endpointRegion
}
