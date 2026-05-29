package report

import (
	"strings"
	"testing"
	"time"
)

func newHIPAATestData() HIPAAReportData {
	return HIPAAReportData{
		ReportData: ReportData{
			TenantID: "tenant-health-001",
			Period: Period{
				Start: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC),
			},
		},
		PHIAccessLogs: []PHIAccessLog{
			{
				User:             "dr.smith",
				Resource:         "patient-record-123",
				Timestamp:        time.Date(2024, 6, 5, 9, 30, 0, 0, time.UTC),
				Justification:    "Treatment",
				MinimumNecessary: true,
			},
			{
				User:             "nurse.jones",
				Resource:         "patient-record-456",
				Timestamp:        time.Date(2024, 6, 10, 14, 0, 0, 0, time.UTC),
				Justification:    "None provided",
				MinimumNecessary: false,
			},
		},
		Encryption: EncryptionCompliance{
			AtRest:        true,
			InTransit:     true,
			Algorithm:     "AES-256-GCM",
			KeyManagement: "AWS KMS",
		},
		BAATracking: []BAAEntry{
			{
				VendorName: "CloudHealthInc",
				SignedDate:  time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC),
				ExpiryDate: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
				Status:     "Active",
			},
			{
				VendorName: "OldLabCo",
				SignedDate:  time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC),
				ExpiryDate: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Status:     "Expired",
			},
		},
		MinimumNecessary: MinimumNecessaryCheck{
			TotalAccesses:    100,
			JustifiedCount:   92,
			UnjustifiedCount: 8,
			Violators:        []string{"nurse.jones", "admin.temp"},
		},
		BreachNotifications: []BreachNotification{
			{
				IncidentID:          "HIPAA-BREACH-001",
				DetectedAt:          time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC),
				NotifiedHHS:         true,
				NotifiedIndividuals: true,
				AffectedCount:       500,
			},
			{
				IncidentID:          "HIPAA-BREACH-002",
				DetectedAt:          time.Date(2024, 6, 20, 16, 30, 0, 0, time.UTC),
				NotifiedHHS:         false,
				NotifiedIndividuals: false,
				AffectedCount:       25,
			},
		},
	}
}

func TestHIPAAReportFullRender(t *testing.T) {
	engine := NewReportEngine()
	data := newHIPAATestData()

	result, err := NewHIPAAReport(engine, data)
	if err != nil {
		t.Fatalf("NewHIPAAReport failed: %v", err)
	}

	output := string(result)

	checks := []string{
		"# HIPAA Compliance Report",
		"tenant-health-001",
		"2024-06-01",
		"2024-06-30",
		// Section 1 - PHI Access Log
		"PHI Access Log",
		"dr.smith",
		"patient-record-123",
		"Treatment",
		"nurse.jones",
		"None provided",
		// Section 2 - Encryption
		"Encryption Status",
		"AES-256-GCM",
		"AWS KMS",
		// Section 3 - BAA
		"Business Associate Agreements",
		"CloudHealthInc",
		"Active",
		"OldLabCo",
		// Section 4 - Minimum Necessary
		"Minimum Necessary Principle",
		"92",
		"8",
		"92.0%",
		// Section 5 - Breach Notification
		"Breach Notification Status",
		"HIPAA-BREACH-001",
		"500",
		"HIPAA-BREACH-002",
		"25",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("HIPAA report missing expected content: %q", check)
		}
	}
}

func TestHIPAAReportBreachNotificationCompliance(t *testing.T) {
	engine := NewReportEngine()
	data := newHIPAATestData()

	result, err := NewHIPAAReport(engine, data)
	if err != nil {
		t.Fatalf("NewHIPAAReport failed: %v", err)
	}

	output := string(result)

	// Breach 001 has both notifications done
	if !strings.Contains(output, "✓ Yes") {
		t.Error("expected compliant breach notification to show '✓ Yes'")
	}

	// Breach 002 is missing notifications
	if !strings.Contains(output, "⚠️ No") {
		t.Error("expected non-compliant breach notification to show '⚠️ No'")
	}
}

func TestHIPAAReportMinimumNecessaryViolations(t *testing.T) {
	engine := NewReportEngine()
	data := newHIPAATestData()

	result, err := NewHIPAAReport(engine, data)
	if err != nil {
		t.Fatalf("NewHIPAAReport failed: %v", err)
	}

	output := string(result)

	// Violators should be highlighted
	if !strings.Contains(output, "⚠️ Violators:") {
		t.Error("expected violators section to be highlighted with warning")
	}
	if !strings.Contains(output, "nurse.jones") {
		t.Error("expected violator 'nurse.jones' to be listed")
	}
	if !strings.Contains(output, "admin.temp") {
		t.Error("expected violator 'admin.temp' to be listed")
	}

	// PHI access log should show minimum necessary violations
	if !strings.Contains(output, "⚠️ No") {
		t.Error("expected minimum necessary violation to show '⚠️ No' in PHI access log")
	}
}

func TestHIPAAReportEmptySections(t *testing.T) {
	engine := NewReportEngine()
	data := HIPAAReportData{
		ReportData: ReportData{
			TenantID: "tenant-empty",
			Period: Period{
				Start: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 7, 31, 23, 59, 59, 0, time.UTC),
			},
		},
		// All HIPAA-specific slices are nil/empty
	}

	result, err := NewHIPAAReport(engine, data)
	if err != nil {
		t.Fatalf("NewHIPAAReport with empty data failed: %v", err)
	}

	output := string(result)

	emptyMessages := []string{
		"No PHI access logs recorded for this period.",
		"No BAA records for this period.",
		"No breach notifications for this period.",
	}

	for _, msg := range emptyMessages {
		if !strings.Contains(output, msg) {
			t.Errorf("expected empty-section message: %q", msg)
		}
	}

	// Even with empty data, encryption section should render
	if !strings.Contains(output, "Encryption Status") {
		t.Error("expected encryption section even with empty data")
	}

	// Minimum necessary with zero accesses should show 100% compliance
	if !strings.Contains(output, "100.0%") {
		t.Error("expected 100.0% compliance rate with zero accesses")
	}
}

func TestHIPAAReportNilEngine(t *testing.T) {
	data := HIPAAReportData{}
	_, err := NewHIPAAReport(nil, data)
	if err == nil {
		t.Error("expected error when engine is nil")
	}
}
