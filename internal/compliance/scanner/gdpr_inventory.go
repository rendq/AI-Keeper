package scanner

import "strconv"

// ProcessingActivity represents a GDPR data processing activity extracted from K8s resources.
type ProcessingActivity struct {
	Purpose              string
	LegalBasis           string
	DataCategories       []string
	Recipients           []string
	RetentionDays        int
	CrossBorderTransfer  bool
	DestinationCountries []string
}

// GDPRInventoryGenerator generates GDPR processing activity inventories
// from K8s resources and audit events.
type GDPRInventoryGenerator struct{}

// NewGDPRInventoryGenerator creates a new generator instance.
func NewGDPRInventoryGenerator() *GDPRInventoryGenerator {
	return &GDPRInventoryGenerator{}
}

// Generate extracts processing activities from K8s resources and audit statistics.
// It infers activities from well-known annotations:
//   - ai-keeper.io/processing-purpose
//   - ai-keeper.io/legal-basis
//   - ai-keeper.io/data-category
//   - ai-keeper.io/retention-days
//   - ai-keeper.io/cross-border
func (g *GDPRInventoryGenerator) Generate(resources []K8sResource, auditStats interface{}) []ProcessingActivity {
	var activities []ProcessingActivity
	for _, r := range resources {
		purpose := r.Annotations["ai-keeper.io/processing-purpose"]
		if purpose == "" {
			continue
		}

		activity := ProcessingActivity{
			Purpose:    purpose,
			LegalBasis: r.Annotations["ai-keeper.io/legal-basis"],
		}

		if cat := r.Annotations["ai-keeper.io/data-category"]; cat != "" {
			activity.DataCategories = splitCSV(cat)
		}

		if recv := r.Annotations["ai-keeper.io/recipients"]; recv != "" {
			activity.Recipients = splitCSV(recv)
		}

		if days := r.Annotations["ai-keeper.io/retention-days"]; days != "" {
			if d, err := strconv.Atoi(days); err == nil {
				activity.RetentionDays = d
			}
		}

		if cb := r.Annotations["ai-keeper.io/cross-border"]; cb == "true" {
			activity.CrossBorderTransfer = true
			if dest := r.Annotations["ai-keeper.io/destination-countries"]; dest != "" {
				activity.DestinationCountries = splitCSV(dest)
			}
		}

		activities = append(activities, activity)
	}
	return activities
}

// splitCSV splits a comma-separated string into trimmed tokens.
func splitCSV(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			token := trimSpace(s[start:i])
			if token != "" {
				result = append(result, token)
			}
			start = i + 1
		}
	}
	return result
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && s[i] == ' ' {
		i++
	}
	for j > i && s[j-1] == ' ' {
		j--
	}
	return s[i:j]
}
