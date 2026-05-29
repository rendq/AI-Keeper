package report

import (
	"strings"
	"testing"
	"time"
)

func newSOC2TestData() SOC2ReportData {
	return SOC2ReportData{
		ReportData: ReportData{
			TenantID: "tenant-soc2-001",
			Period: Period{
				Start: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 7, 31, 23, 59, 59, 0, time.UTC),
			},
		},
		AccessControl: AccessControlAudit{
			UnauthorizedAttempts: 12,
			PrivilegeChanges:     5,
			MFAEnabled:           95,
			MFATotal:             100,
			ReviewDate:           time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC),
		},
		ChangeManagement: ChangeManagement{
			Deployments:  42,
			Rollbacks:    3,
			ApprovalRate: 97.5,
			Changes: []ChangeRecord{
				{
					ChangeID:    "CHG-001",
					Description: "API gateway update",
					ApprovedBy:  "admin@corp.com",
					DeployedAt:  time.Date(2024, 7, 10, 14, 0, 0, 0, time.UTC),
					RolledBack:  false,
				},
				{
					ChangeID:    "CHG-002",
					Description: "Database migration",
					ApprovedBy:  "dba@corp.com",
					DeployedAt:  time.Date(2024, 7, 18, 9, 0, 0, 0, time.UTC),
					RolledBack:  true,
				},
			},
		},
		Availability: AvailabilitySLO{
			UptimePercent: 99.95,
			SLOTarget:     99.9,
			Incidents:     2,
			MTTRMinutes:   15.5,
		},
		Encryption: EncryptionStatus{
			AtRestEnabled:    true,
			InTransitEnabled: true,
			KeyRotationDays:  90,
			LastRotation:     time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
			Algorithm:        "AES-256-GCM",
		},
		IncidentResponse: IncidentResponse{
			TotalIncidents:    3,
			ResolutionTimeAvg: 2.5,
			Escalations:       1,
			Incidents: []IncidentRecord{
				{
					IncidentID:  "INC-001",
					Severity:    "HIGH",
					DetectedAt:  time.Date(2024, 7, 5, 10, 30, 0, 0, time.UTC),
					ResolvedAt:  time.Date(2024, 7, 5, 12, 0, 0, 0, time.UTC),
					Escalated:   true,
					Description: "Service degradation in auth module",
				},
				{
					IncidentID:  "INC-002",
					Severity:    "LOW",
					DetectedAt:  time.Date(2024, 7, 20, 8, 0, 0, 0, time.UTC),
					ResolvedAt:  time.Date(2024, 7, 20, 9, 15, 0, 0, time.UTC),
					Escalated:   false,
					Description: "Temporary latency spike",
				},
			},
		},
	}
}

func TestSOC2ReportFullRender(t *testing.T) {
	engine := NewReportEngine()
	data := newSOC2TestData()

	result, err := NewSOC2Report(engine, data)
	if err != nil {
		t.Fatalf("NewSOC2Report failed: %v", err)
	}

	output := string(result)

	checks := []string{
		"# SOC2 Compliance Report",
		"tenant-soc2-001",
		"2024-07-01",
		"2024-07-31",
		"SOC2 Trust Services Criteria",
		// Section 1 - Security
		"Security (Access Control Audit)",
		"Unauthorized Access Attempts",
		"12",
		"Privilege Changes",
		"5",
		"95/100",
		"95.0%",
		"2024-07-15",
		// Section 2 - Availability
		"Availability (SLO Metrics)",
		"99.950%",
		"99.900%",
		"Within SLO",
		"15.5",
		// Section 3 - Processing Integrity
		"Processing Integrity (Change Management)",
		"42",
		"97.5%",
		"CHG-001",
		"API gateway update",
		"CHG-002",
		"Database migration",
		// Section 4 - Confidentiality
		"Confidentiality (Encryption Status)",
		"AES-256-GCM",
		"90 days",
		// Section 5 - Privacy
		"Privacy (Incident Response)",
		"2.5",
		"INC-001",
		"HIGH",
		"Service degradation in auth module",
		"INC-002",
		"Temporary latency spike",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("SOC2 report missing expected content: %q", check)
		}
	}
}

func TestSOC2ReportSLOBreach(t *testing.T) {
	engine := NewReportEngine()
	data := newSOC2TestData()

	// Set uptime below SLO target to trigger breach
	data.Availability.UptimePercent = 99.5
	data.Availability.SLOTarget = 99.9

	result, err := NewSOC2Report(engine, data)
	if err != nil {
		t.Fatalf("NewSOC2Report failed: %v", err)
	}

	output := string(result)

	if !strings.Contains(output, "⚠️ SLO BREACHED") {
		t.Error("expected SLO breach warning when uptime is below target")
	}

	// Verify non-breached case
	data.Availability.UptimePercent = 99.95
	result, err = NewSOC2Report(engine, data)
	if err != nil {
		t.Fatalf("NewSOC2Report failed: %v", err)
	}
	output = string(result)

	if !strings.Contains(output, "✓ Within SLO") {
		t.Error("expected 'Within SLO' when uptime meets target")
	}
}

func TestSOC2ReportEmptySections(t *testing.T) {
	engine := NewReportEngine()
	data := SOC2ReportData{
		ReportData: ReportData{
			TenantID: "tenant-empty",
			Period: Period{
				Start: time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 8, 31, 23, 59, 59, 0, time.UTC),
			},
		},
		// All SOC2-specific fields use zero values
	}

	result, err := NewSOC2Report(engine, data)
	if err != nil {
		t.Fatalf("NewSOC2Report with empty data failed: %v", err)
	}

	output := string(result)

	// Should still render all sections without errors
	expectedSections := []string{
		"Security (Access Control Audit)",
		"Availability (SLO Metrics)",
		"Processing Integrity (Change Management)",
		"Confidentiality (Encryption Status)",
		"Privacy (Incident Response)",
	}

	for _, section := range expectedSections {
		if !strings.Contains(output, section) {
			t.Errorf("empty SOC2 report missing section: %q", section)
		}
	}

	// Empty change records message
	if !strings.Contains(output, "No change records for this period.") {
		t.Error("expected empty change records message")
	}

	// Empty incidents message
	if !strings.Contains(output, "No incidents recorded for this period.") {
		t.Error("expected empty incidents message")
	}

	// With zero values, SLO should show breached (0 < 0 is false, so it should show Within SLO)
	// Actually 0.0 < 0.0 is false, so SLOBreached() returns false
	if !strings.Contains(output, "✓ Within SLO") {
		t.Error("expected 'Within SLO' for zero-value availability")
	}
}

func TestSOC2ReportNilEngine(t *testing.T) {
	data := SOC2ReportData{}
	_, err := NewSOC2Report(nil, data)
	if err == nil {
		t.Error("expected error when engine is nil")
	}
}
