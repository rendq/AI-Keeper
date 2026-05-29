package cost

import (
	"fmt"
	"sync"
)

// QuotaConfig defines a quota limit for a specific resource type within a scope.
type QuotaConfig struct {
	Scope        string
	ResourceType string
	Limit        int
}

// QuotaService tracks resource usage and enforces quota limits at admission time.
// In production this would integrate with the K8s admission webhook.
type QuotaService struct {
	mu     sync.RWMutex
	limits map[string]int // key: "scope/resourceType" -> limit
	usage  map[string]int // key: "scope/resourceType" -> current count
}

// NewQuotaService creates a QuotaService with the given quota configurations.
func NewQuotaService(configs []QuotaConfig) *QuotaService {
	qs := &QuotaService{
		limits: make(map[string]int),
		usage:  make(map[string]int),
	}
	for _, c := range configs {
		key := quotaKey(c.Scope, c.ResourceType)
		qs.limits[key] = c.Limit
	}
	return qs
}

// CheckAdmission checks if creating another resource of the given type in
// the given scope would exceed the configured quota. Returns allowed=true if
// there is no quota configured or the current count is below the limit.
func (qs *QuotaService) CheckAdmission(resourceType, scope string, currentCount int) (allowed bool, reason string) {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	key := quotaKey(scope, resourceType)
	limit, exists := qs.limits[key]
	if !exists {
		// No quota configured means no limit
		return true, ""
	}
	if currentCount >= limit {
		return false, fmt.Sprintf("quota exceeded: %s in scope %q has %d/%d", resourceType, scope, currentCount, limit)
	}
	return true, ""
}

// UpdateUsage updates the tracked resource count for the given type and scope.
func (qs *QuotaService) UpdateUsage(resourceType, scope string, count int) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	key := quotaKey(scope, resourceType)
	qs.usage[key] = count
}

// GetUsage returns the current usage and configured limit for a resource type
// in a given scope. If no quota is configured, limit is returned as -1.
func (qs *QuotaService) GetUsage(resourceType, scope string) (used, limit int) {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	key := quotaKey(scope, resourceType)
	used = qs.usage[key]
	l, exists := qs.limits[key]
	if !exists {
		return used, -1
	}
	return used, l
}

func quotaKey(scope, resourceType string) string {
	return scope + "/" + resourceType
}
