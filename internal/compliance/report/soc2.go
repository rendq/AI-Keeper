package report

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// AccessControlAudit represents access control audit data for SOC2 reporting.
type AccessControlAudit struct {
	UnauthorizedAttempts int
	PrivilegeChanges     int
	MFAEnabled           int
	MFATotal             int
	ReviewDate           time.Time
}

// MFAPercentage returns the percentage of users with MFA enabled.
func (a AccessControlAudit) MFAPercentage() float64 {
	if a.MFATotal == 0 {
		return 0
	}
	return float64(a.MFAEnabled) / float64(a.MFATotal) * 100
}

// ChangeManagement represents change management metrics for SOC2 reporting.
type ChangeManagement struct {
	Deployments  int
	Rollbacks    int
	ApprovalRate float64 // percentage 0-100
	Changes      []ChangeRecord
}

// ChangeRecord represents a single change management entry.
type ChangeRecord struct {
	ChangeID    string
	Description string
	ApprovedBy  string
	DeployedAt  time.Time
	RolledBack  bool
}

// AvailabilitySLO represents availability SLO metrics for SOC2 reporting.
type AvailabilitySLO struct {
	UptimePercent float64 // e.g., 99.95
	SLOTarget     float64 // e.g., 99.9
	Incidents     int
	MTTRMinutes   float64 // Mean Time To Resolve in minutes
}

// SLOBreached returns true if uptime is below the SLO target.
func (a AvailabilitySLO) SLOBreached() bool {
	return a.UptimePercent < a.SLOTarget
}

// EncryptionStatus represents encryption posture for SOC2 reporting.
type EncryptionStatus struct {
	AtRestEnabled    bool
	InTransitEnabled bool
	KeyRotationDays  int
	LastRotation     time.Time
	Algorithm        string
}

// IncidentResponse represents incident response metrics for SOC2 reporting.
type IncidentResponse struct {
	TotalIncidents     int
	ResolutionTimeAvg  float64 // average resolution time in hours
	Escalations        int
	Incidents          []IncidentRecord
}

// IncidentRecord represents a single incident entry.
type IncidentRecord struct {
	IncidentID     string
	Severity       string
	DetectedAt     time.Time
	ResolvedAt     time.Time
	Escalated      bool
	Description    string
}

// SOC2ReportData extends ReportData with SOC2-specific fields.
type SOC2ReportData struct {
	ReportData
	AccessControl    AccessControlAudit
	ChangeManagement ChangeManagement
	Availability     AvailabilitySLO
	Encryption       EncryptionStatus
	IncidentResponse IncidentResponse
}

// SOC2Template is the Go template string for the SOC2 monthly compliance report.
// It covers all 5 SOC2 Trust Service Categories: Security, Availability,
// Processing Integrity, Confidentiality, and Privacy.
const SOC2Template = `# SOC2 Compliance Report

**Tenant:** {{.TenantID}}
**Period:** {{.Period.Start.Format "2006-01-02"}} to {{.Period.End.Format "2006-01-02"}}
**Framework:** SOC2 Trust Services Criteria

---

## 1. Security (Access Control Audit)

| Metric | Value |
|--------|-------|
| Unauthorized Access Attempts | {{.AccessControl.UnauthorizedAttempts}} |
| Privilege Changes | {{.AccessControl.PrivilegeChanges}} |
| MFA Enabled | {{.AccessControl.MFAEnabled}}/{{.AccessControl.MFATotal}} ({{printf "%.1f" .AccessControl.MFAPercentage}}%) |
{{if not .AccessControl.ReviewDate.IsZero}}| Last Access Review | {{.AccessControl.ReviewDate.Format "2006-01-02"}} |
{{end}}
## 2. Availability (SLO Metrics)

| Metric | Value |
|--------|-------|
| Uptime | {{printf "%.3f" .Availability.UptimePercent}}% |
| SLO Target | {{printf "%.3f" .Availability.SLOTarget}}% |
| Status | {{if .Availability.SLOBreached}}⚠️ SLO BREACHED{{else}}✓ Within SLO{{end}} |
| Incidents | {{.Availability.Incidents}} |
| MTTR (minutes) | {{printf "%.1f" .Availability.MTTRMinutes}} |

## 3. Processing Integrity (Change Management)

| Metric | Value |
|--------|-------|
| Total Deployments | {{.ChangeManagement.Deployments}} |
| Rollbacks | {{.ChangeManagement.Rollbacks}} |
| Approval Rate | {{printf "%.1f" .ChangeManagement.ApprovalRate}}% |

{{if .ChangeManagement.Changes}}| Change ID | Description | Approved By | Deployed | Rolled Back |
|-----------|-------------|-------------|----------|-------------|
{{range .ChangeManagement.Changes}}| {{.ChangeID}} | {{.Description}} | {{.ApprovedBy}} | {{.DeployedAt.Format "2006-01-02"}} | {{if .RolledBack}}⚠️ Yes{{else}}No{{end}} |
{{end}}{{else}}No change records for this period.
{{end}}
## 4. Confidentiality (Encryption Status)

| Metric | Value |
|--------|-------|
| Encryption at Rest | {{if .Encryption.AtRestEnabled}}✓ Enabled{{else}}⚠️ Disabled{{end}} |
| Encryption in Transit | {{if .Encryption.InTransitEnabled}}✓ Enabled{{else}}⚠️ Disabled{{end}} |
| Algorithm | {{if .Encryption.Algorithm}}{{.Encryption.Algorithm}}{{else}}N/A{{end}} |
| Key Rotation Interval | {{.Encryption.KeyRotationDays}} days |
{{if not .Encryption.LastRotation.IsZero}}| Last Key Rotation | {{.Encryption.LastRotation.Format "2006-01-02"}} |
{{end}}
## 5. Privacy (Incident Response)

| Metric | Value |
|--------|-------|
| Total Incidents | {{.IncidentResponse.TotalIncidents}} |
| Avg Resolution Time | {{printf "%.1f" .IncidentResponse.ResolutionTimeAvg}} hours |
| Escalations | {{.IncidentResponse.Escalations}} |

{{if .IncidentResponse.Incidents}}| Incident ID | Severity | Detected | Resolved | Escalated | Description |
|-------------|----------|----------|----------|-----------|-------------|
{{range .IncidentResponse.Incidents}}| {{.IncidentID}} | {{.Severity}} | {{.DetectedAt.Format "2006-01-02 15:04"}} | {{if .ResolvedAt.IsZero}}-{{else}}{{.ResolvedAt.Format "2006-01-02 15:04"}}{{end}} | {{if .Escalated}}Yes{{else}}No{{end}} | {{.Description}} |
{{end}}{{else}}No incidents recorded for this period.
{{end}}
---

*Report generated automatically by AIP Compliance Engine — SOC2 Trust Services Criteria.*
`

// NewSOC2Report renders a SOC2 compliance report using the report engine.
func NewSOC2Report(engine *ReportEngine, data SOC2ReportData) ([]byte, error) {
	if engine == nil {
		return nil, fmt.Errorf("report engine must not be nil")
	}

	t, err := template.New("soc2-report").Funcs(engine.funcMap).Parse(SOC2Template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SOC2 template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute SOC2 template: %w", err)
	}

	return buf.Bytes(), nil
}
