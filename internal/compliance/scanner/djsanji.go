// Package scanner provides compliance scanning tools for K8s resources.
package scanner

// ControlCategory represents a GB/T 22239 Level-3 control category.
type ControlCategory string

const (
	IdentityAuth         ControlCategory = "identity_auth"
	AccessControl        ControlCategory = "access_control"
	SecurityAudit        ControlCategory = "security_audit"
	IntrusionPrevention  ControlCategory = "intrusion_prevention"
	DataIntegrity        ControlCategory = "data_integrity"
	DataConfidentiality  ControlCategory = "data_confidentiality"
)

// K8sResource represents a Kubernetes resource to be scanned.
type K8sResource struct {
	Kind        string
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
}

// ScanRule defines a single compliance check rule.
type ScanRule struct {
	ID          string
	Category    ControlCategory
	Description string
	CheckFunc   func(K8sResource) bool
}

// ScanResult holds the outcome of a single rule check against a resource.
type ScanResult struct {
	RuleID    string
	Compliant bool
	Details   string
}

// DJSanjiScanner scans K8s resources against GB/T 22239 Level-3 requirements.
type DJSanjiScanner struct {
	rules []ScanRule
}

// NewDJSanjiScanner creates a scanner initialized with default rules.
func NewDJSanjiScanner() *DJSanjiScanner {
	return &DJSanjiScanner{rules: defaultRules()}
}

// Scan runs all rules against each resource and returns results.
func (s *DJSanjiScanner) Scan(resources []K8sResource) []ScanResult {
	var results []ScanResult
	for _, r := range resources {
		for _, rule := range s.rules {
			compliant := rule.CheckFunc(r)
			detail := rule.Description + " — compliant"
			if !compliant {
				detail = rule.Description + " — non-compliant"
			}
			results = append(results, ScanResult{
				RuleID:    rule.ID,
				Compliant: compliant,
				Details:   detail,
			})
		}
	}
	return results
}

// GetRules returns the current set of scan rules.
func (s *DJSanjiScanner) GetRules() []ScanRule {
	return s.rules
}

func defaultRules() []ScanRule {
	return []ScanRule{
		{
			ID:          "DJ-001",
			Category:    IdentityAuth,
			Description: "ServiceAccount must use short-lived tokens",
			CheckFunc: func(r K8sResource) bool {
				if r.Kind != "ServiceAccount" {
					return true
				}
				return r.Annotations["ai-keeper.io/token-expiry"] != ""
			},
		},
		{
			ID:          "DJ-002",
			Category:    AccessControl,
			Description: "ClusterRoleBinding must not grant cluster-admin",
			CheckFunc: func(r K8sResource) bool {
				if r.Kind != "ClusterRoleBinding" {
					return true
				}
				return r.Labels["rbac.ai-keeper.io/role"] != "cluster-admin"
			},
		},
		{
			ID:          "DJ-003",
			Category:    SecurityAudit,
			Description: "Resource must have audit logging enabled",
			CheckFunc: func(r K8sResource) bool {
				return r.Annotations["ai-keeper.io/audit-logging"] == "enabled"
			},
		},
		{
			ID:          "DJ-004",
			Category:    IntrusionPrevention,
			Description: "Namespace must have NetworkPolicy defined",
			CheckFunc: func(r K8sResource) bool {
				if r.Kind != "Namespace" {
					return true
				}
				return r.Labels["ai-keeper.io/network-policy"] == "enforced"
			},
		},
		{
			ID:          "DJ-005",
			Category:    DataIntegrity,
			Description: "Resource must have SM3 hash label",
			CheckFunc: func(r K8sResource) bool {
				return r.Labels["ai-keeper.io/sm3-hash"] != ""
			},
		},
		{
			ID:          "DJ-006",
			Category:    DataConfidentiality,
			Description: "Resource must have encryption annotation",
			CheckFunc: func(r K8sResource) bool {
				return r.Annotations["ai-keeper.io/encryption"] != ""
			},
		},
	}
}
