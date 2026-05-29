package scanner

import (
	"testing"
)

func TestGDPRInventory_GenerateFromAnnotations(t *testing.T) {
	gen := NewGDPRInventoryGenerator()
	resources := []K8sResource{
		{
			Kind:      "Deployment",
			Name:      "user-service",
			Namespace: "default",
			Annotations: map[string]string{
				"ai-keeper.io/processing-purpose": "user authentication",
				"ai-keeper.io/legal-basis":        "consent",
				"ai-keeper.io/data-category":      "personal,identity",
				"ai-keeper.io/recipients":         "auth-service,logging",
				"ai-keeper.io/retention-days":     "365",
				"ai-keeper.io/cross-border":       "false",
			},
		},
	}

	activities := gen.Generate(resources, nil)
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}

	a := activities[0]
	if a.Purpose != "user authentication" {
		t.Errorf("purpose = %q, want %q", a.Purpose, "user authentication")
	}
	if a.LegalBasis != "consent" {
		t.Errorf("legal basis = %q, want %q", a.LegalBasis, "consent")
	}
	if len(a.DataCategories) != 2 || a.DataCategories[0] != "personal" || a.DataCategories[1] != "identity" {
		t.Errorf("data categories = %v, want [personal identity]", a.DataCategories)
	}
	if len(a.Recipients) != 2 || a.Recipients[0] != "auth-service" || a.Recipients[1] != "logging" {
		t.Errorf("recipients = %v, want [auth-service logging]", a.Recipients)
	}
	if a.RetentionDays != 365 {
		t.Errorf("retention days = %d, want 365", a.RetentionDays)
	}
	if a.CrossBorderTransfer {
		t.Error("cross border transfer should be false")
	}
}

func TestGDPRInventory_EmptyResources(t *testing.T) {
	gen := NewGDPRInventoryGenerator()
	activities := gen.Generate(nil, nil)
	if len(activities) != 0 {
		t.Fatalf("expected 0 activities for nil resources, got %d", len(activities))
	}

	activities = gen.Generate([]K8sResource{}, nil)
	if len(activities) != 0 {
		t.Fatalf("expected 0 activities for empty resources, got %d", len(activities))
	}

	// Resource without processing-purpose annotation should be skipped.
	resources := []K8sResource{
		{
			Kind:        "ConfigMap",
			Name:        "app-config",
			Namespace:   "default",
			Annotations: map[string]string{"other": "value"},
		},
	}
	activities = gen.Generate(resources, nil)
	if len(activities) != 0 {
		t.Fatalf("expected 0 activities for resource without purpose, got %d", len(activities))
	}
}

func TestGDPRInventory_CrossBorderDetection(t *testing.T) {
	gen := NewGDPRInventoryGenerator()
	resources := []K8sResource{
		{
			Kind:      "StatefulSet",
			Name:      "analytics-db",
			Namespace: "data",
			Annotations: map[string]string{
				"ai-keeper.io/processing-purpose":    "analytics processing",
				"ai-keeper.io/legal-basis":           "legitimate interest",
				"ai-keeper.io/data-category":         "behavioral,usage",
				"ai-keeper.io/retention-days":        "90",
				"ai-keeper.io/cross-border":          "true",
				"ai-keeper.io/destination-countries":  "US,DE,JP",
			},
		},
	}

	activities := gen.Generate(resources, nil)
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}

	a := activities[0]
	if !a.CrossBorderTransfer {
		t.Error("cross border transfer should be true")
	}
	if len(a.DestinationCountries) != 3 {
		t.Fatalf("expected 3 destination countries, got %d", len(a.DestinationCountries))
	}
	expected := []string{"US", "DE", "JP"}
	for i, c := range expected {
		if a.DestinationCountries[i] != c {
			t.Errorf("destination country[%d] = %q, want %q", i, a.DestinationCountries[i], c)
		}
	}
}
