package report

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// DataProcessingActivity represents a GDPR data processing activity record.
type DataProcessingActivity struct {
	Purpose       string
	LegalBasis    string
	DataCategory  string
	DataSubjects  int
	RetentionDays int
	Processor     string
}

// CrossBorderTransfer represents a cross-border data transfer record.
type CrossBorderTransfer struct {
	SourceRegion      string
	DestinationRegion string
	DataCategory      string
	TransferCount     int64
	LegalMechanism    string // e.g., "SCC", "BCR", "Adequacy Decision"
	Flagged           bool   // true if transfer lacks proper legal basis
}

// DPAEntry represents the status of a Data Processing Agreement.
type DPAEntry struct {
	ProcessorName string
	SignedDate    time.Time
	ExpiryDate   time.Time
	Status       string // "Active", "Expired", "Pending"
}

// ForgetRequest represents a Right to Be Forgotten (erasure) request.
type ForgetRequest struct {
	RequestID   string
	SubjectID   string
	RequestDate time.Time
	CompletedAt time.Time
	Status      string // "Completed", "InProgress", "Overdue"
}

// DataBreach represents a data breach event for GDPR reporting.
type DataBreach struct {
	IncidentID     string
	DetectedAt     time.Time
	ReportedAt     time.Time
	Severity       string // "HIGH", "MEDIUM", "LOW"
	AffectedCount  int
	Description    string
	NotifiedDPA    bool
	NotifiedWithin72h bool
}

// GDPRReportData extends ReportData with GDPR-specific fields.
type GDPRReportData struct {
	ReportData
	DataProcessingActivities   []DataProcessingActivity
	CrossBorderTransfers       []CrossBorderTransfer
	DPAStatus                  []DPAEntry
	RightToBeForgettenRequests []ForgetRequest
	DataBreachEvents           []DataBreach
}

// GDPRTemplate is the Go template string for the GDPR monthly compliance report.
const GDPRTemplate = `# GDPR Compliance Report

**Tenant:** {{.TenantID}}
**Period:** {{.Period.Start.Format "2006-01-02"}} to {{.Period.End.Format "2006-01-02"}}

---

## 1. Data Processing Activities

{{if .DataProcessingActivities}}| Purpose | Legal Basis | Data Category | Data Subjects | Retention (days) | Processor |
|---------|-------------|---------------|---------------|------------------|-----------|
{{range .DataProcessingActivities}}| {{.Purpose}} | {{.LegalBasis}} | {{.DataCategory}} | {{.DataSubjects}} | {{.RetentionDays}} | {{.Processor}} |
{{end}}{{else}}No data processing activities recorded for this period.
{{end}}
## 2. Cross-Border Transfers

{{if .CrossBorderTransfers}}| Source | Destination | Category | Count | Legal Mechanism | Status |
|--------|-------------|----------|-------|-----------------|--------|
{{range .CrossBorderTransfers}}| {{.SourceRegion}} | {{.DestinationRegion}} | {{.DataCategory}} | {{.TransferCount}} | {{.LegalMechanism}} | {{if .Flagged}}⚠️ FLAGGED{{else}}✓ OK{{end}} |
{{end}}{{else}}No cross-border transfers recorded for this period.
{{end}}
## 3. DPA Signing Status

{{if .DPAStatus}}| Processor | Signed Date | Expiry Date | Status |
|-----------|-------------|-------------|--------|
{{range .DPAStatus}}| {{.ProcessorName}} | {{.SignedDate.Format "2006-01-02"}} | {{.ExpiryDate.Format "2006-01-02"}} | {{.Status}} |
{{end}}{{else}}No DPA records for this period.
{{end}}
## 4. Right to Be Forgotten Requests

{{if .RightToBeForgettenRequests}}| Request ID | Subject | Request Date | Completed | Status |
|------------|---------|--------------|-----------|--------|
{{range .RightToBeForgettenRequests}}| {{.RequestID}} | {{.SubjectID}} | {{.RequestDate.Format "2006-01-02"}} | {{if .CompletedAt.IsZero}}-{{else}}{{.CompletedAt.Format "2006-01-02"}}{{end}} | {{.Status}} |
{{end}}{{else}}No Right to Be Forgotten requests for this period.
{{end}}
## 5. Data Breach Events

{{if .DataBreachEvents}}| Incident ID | Detected | Severity | Affected | Reported to DPA | Within 72h | Description |
|-------------|----------|----------|----------|-----------------|------------|-------------|
{{range .DataBreachEvents}}| {{.IncidentID}} | {{.DetectedAt.Format "2006-01-02 15:04"}} | {{.Severity}} | {{.AffectedCount}} | {{if .NotifiedDPA}}Yes{{else}}⚠️ No{{end}} | {{if .NotifiedWithin72h}}✓{{else}}⚠️ OVERDUE{{end}} | {{.Description}} |
{{end}}{{else}}No data breach events for this period.
{{end}}
---

*Report generated automatically by AIP Compliance Engine.*
`

// NewGDPRReport renders a GDPR compliance report using the report engine.
func NewGDPRReport(engine *ReportEngine, data GDPRReportData) ([]byte, error) {
	if engine == nil {
		return nil, fmt.Errorf("report engine must not be nil")
	}

	t, err := template.New("gdpr-report").Funcs(engine.funcMap).Parse(GDPRTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GDPR template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute GDPR template: %w", err)
	}

	return buf.Bytes(), nil
}
