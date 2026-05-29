package report

import (
	"strings"
	"testing"
	"time"
)

func newGDPRTestData() GDPRReportData {
	return GDPRReportData{
		ReportData: ReportData{
			TenantID: "tenant-eu-001",
			Period: Period{
				Start: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC),
			},
		},
		DataProcessingActivities: []DataProcessingActivity{
			{
				Purpose:       "User Analytics",
				LegalBasis:    "Consent",
				DataCategory:  "Behavioral",
				DataSubjects:  5000,
				RetentionDays: 365,
				Processor:     "AnalyticsCo",
			},
			{
				Purpose:       "Payment Processing",
				LegalBasis:    "Contract",
				DataCategory:  "Financial",
				DataSubjects:  1200,
				RetentionDays: 2555,
				Processor:     "PaymentInc",
			},
		},
		CrossBorderTransfers: []CrossBorderTransfer{
			{
				SourceRegion:      "EU-West",
				DestinationRegion: "US-East",
				DataCategory:      "PII",
				TransferCount:     340,
				LegalMechanism:    "SCC",
				Flagged:           false,
			},
			{
				SourceRegion:      "EU-West",
				DestinationRegion: "CN-North",
				DataCategory:      "Behavioral",
				TransferCount:     15,
				LegalMechanism:    "None",
				Flagged:           true,
			},
		},
		DPAStatus: []DPAEntry{
			{
				ProcessorName: "AnalyticsCo",
				SignedDate:     time.Date(2023, 3, 15, 0, 0, 0, 0, time.UTC),
				ExpiryDate:    time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC),
				Status:        "Active",
			},
			{
				ProcessorName: "OldVendor",
				SignedDate:     time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				ExpiryDate:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Status:        "Expired",
			},
		},
		RightToBeForgettenRequests: []ForgetRequest{
			{
				RequestID:   "RTBF-001",
				SubjectID:   "user-42",
				RequestDate: time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC),
				CompletedAt: time.Date(2024, 6, 8, 0, 0, 0, 0, time.UTC),
				Status:      "Completed",
			},
			{
				RequestID:   "RTBF-002",
				SubjectID:   "user-99",
				RequestDate: time.Date(2024, 6, 25, 0, 0, 0, 0, time.UTC),
				CompletedAt: time.Time{},
				Status:      "InProgress",
			},
		},
		DataBreachEvents: []DataBreach{
			{
				IncidentID:        "BREACH-2024-001",
				DetectedAt:        time.Date(2024, 6, 10, 14, 30, 0, 0, time.UTC),
				ReportedAt:        time.Date(2024, 6, 10, 20, 0, 0, 0, time.UTC),
				Severity:          "HIGH",
				AffectedCount:     250,
				Description:       "Unauthorized access to user profiles",
				NotifiedDPA:       true,
				NotifiedWithin72h: true,
			},
		},
	}
}

func TestGDPRReportFullRender(t *testing.T) {
	engine := NewReportEngine()
	data := newGDPRTestData()

	result, err := NewGDPRReport(engine, data)
	if err != nil {
		t.Fatalf("NewGDPRReport failed: %v", err)
	}

	output := string(result)

	checks := []string{
		"# GDPR Compliance Report",
		"tenant-eu-001",
		"2024-06-01",
		"2024-06-30",
		// Section 1 - Data Processing Activities
		"Data Processing Activities",
		"User Analytics",
		"Consent",
		"Payment Processing",
		"Contract",
		// Section 2 - Cross-Border Transfers
		"Cross-Border Transfers",
		"EU-West",
		"US-East",
		"SCC",
		// Section 3 - DPA Status
		"DPA Signing Status",
		"AnalyticsCo",
		"Active",
		"OldVendor",
		"Expired",
		// Section 4 - RTBF
		"Right to Be Forgotten",
		"RTBF-001",
		"Completed",
		"RTBF-002",
		"InProgress",
		// Section 5 - Data Breach
		"Data Breach Events",
		"BREACH-2024-001",
		"HIGH",
		"Unauthorized access to user profiles",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("GDPR report missing expected content: %q", check)
		}
	}
}

func TestGDPRReportCrossBorderHighlight(t *testing.T) {
	engine := NewReportEngine()
	data := newGDPRTestData()

	result, err := NewGDPRReport(engine, data)
	if err != nil {
		t.Fatalf("NewGDPRReport failed: %v", err)
	}

	output := string(result)

	// The flagged CN-North transfer should be highlighted
	if !strings.Contains(output, "⚠️ FLAGGED") {
		t.Error("expected flagged cross-border transfer to show '⚠️ FLAGGED'")
	}
	// The valid SCC transfer should show OK
	if !strings.Contains(output, "✓ OK") {
		t.Error("expected compliant transfer to show '✓ OK'")
	}
}

func TestGDPRReportDataBreachFormatted(t *testing.T) {
	engine := NewReportEngine()
	data := newGDPRTestData()

	result, err := NewGDPRReport(engine, data)
	if err != nil {
		t.Fatalf("NewGDPRReport failed: %v", err)
	}

	output := string(result)

	// Breach event details
	if !strings.Contains(output, "BREACH-2024-001") {
		t.Error("expected breach incident ID in report")
	}
	if !strings.Contains(output, "250") {
		t.Error("expected affected count in report")
	}
	if !strings.Contains(output, "2024-06-10 14:30") {
		t.Error("expected detection time in report")
	}

	// Test breach that was NOT reported within 72h
	data.DataBreachEvents = append(data.DataBreachEvents, DataBreach{
		IncidentID:        "BREACH-2024-002",
		DetectedAt:        time.Date(2024, 6, 20, 8, 0, 0, 0, time.UTC),
		ReportedAt:        time.Date(2024, 6, 24, 8, 0, 0, 0, time.UTC),
		Severity:          "MEDIUM",
		AffectedCount:     50,
		Description:       "Email leak",
		NotifiedDPA:       false,
		NotifiedWithin72h: false,
	})

	result, err = NewGDPRReport(engine, data)
	if err != nil {
		t.Fatalf("NewGDPRReport failed: %v", err)
	}
	output = string(result)

	if !strings.Contains(output, "⚠️ OVERDUE") {
		t.Error("expected overdue notification warning for late-reported breach")
	}
	if !strings.Contains(output, "⚠️ No") {
		t.Error("expected warning for breach not reported to DPA")
	}
}

func TestGDPRReportEmptySections(t *testing.T) {
	engine := NewReportEngine()
	data := GDPRReportData{
		ReportData: ReportData{
			TenantID: "tenant-empty",
			Period: Period{
				Start: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 7, 31, 23, 59, 59, 0, time.UTC),
			},
		},
		// All GDPR-specific slices are nil/empty
	}

	result, err := NewGDPRReport(engine, data)
	if err != nil {
		t.Fatalf("NewGDPRReport with empty data failed: %v", err)
	}

	output := string(result)

	emptyMessages := []string{
		"No data processing activities recorded for this period.",
		"No cross-border transfers recorded for this period.",
		"No DPA records for this period.",
		"No Right to Be Forgotten requests for this period.",
		"No data breach events for this period.",
	}

	for _, msg := range emptyMessages {
		if !strings.Contains(output, msg) {
			t.Errorf("expected empty-section message: %q", msg)
		}
	}
}

func TestGDPRReportNilEngine(t *testing.T) {
	data := GDPRReportData{}
	_, err := NewGDPRReport(nil, data)
	if err == nil {
		t.Error("expected error when engine is nil")
	}
}
