package report

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// PHIAccessLog represents a Protected Health Information access log entry.
type PHIAccessLog struct {
	User             string
	Resource         string
	Timestamp        time.Time
	Justification    string
	MinimumNecessary bool // true if access met minimum necessary standard
}

// EncryptionCompliance represents HIPAA encryption compliance status.
type EncryptionCompliance struct {
	AtRest        bool
	InTransit     bool
	Algorithm     string
	KeyManagement string // e.g., "AWS KMS", "HashiCorp Vault"
}

// BAAEntry represents a Business Associate Agreement tracking entry.
type BAAEntry struct {
	VendorName string
	SignedDate  time.Time
	ExpiryDate time.Time
	Status     string // "Active", "Expired", "Pending"
}

// MinimumNecessaryCheck represents the results of minimum necessary principle auditing.
type MinimumNecessaryCheck struct {
	TotalAccesses   int
	JustifiedCount  int
	UnjustifiedCount int
	Violators       []string
}

// ComplianceRate returns the percentage of accesses that met minimum necessary.
func (m MinimumNecessaryCheck) ComplianceRate() float64 {
	if m.TotalAccesses == 0 {
		return 100
	}
	return float64(m.JustifiedCount) / float64(m.TotalAccesses) * 100
}

// BreachNotification represents a HIPAA breach notification record.
type BreachNotification struct {
	IncidentID          string
	DetectedAt          time.Time
	NotifiedHHS         bool
	NotifiedIndividuals bool
	AffectedCount       int
}

// HIPAAReportData extends ReportData with HIPAA-specific fields.
type HIPAAReportData struct {
	ReportData
	PHIAccessLogs         []PHIAccessLog
	Encryption            EncryptionCompliance
	BAATracking           []BAAEntry
	MinimumNecessary      MinimumNecessaryCheck
	BreachNotifications   []BreachNotification
}

// HIPAATemplate is the Go template string for the HIPAA monthly compliance report.
const HIPAATemplate = `# HIPAA Compliance Report

**Tenant:** {{.TenantID}}
**Period:** {{.Period.Start.Format "2006-01-02"}} to {{.Period.End.Format "2006-01-02"}}

---

## 1. PHI Access Log

{{if .PHIAccessLogs}}| User | Resource | Timestamp | Justification | Minimum Necessary |
|------|----------|-----------|---------------|-------------------|
{{range .PHIAccessLogs}}| {{.User}} | {{.Resource}} | {{.Timestamp.Format "2006-01-02 15:04"}} | {{.Justification}} | {{if .MinimumNecessary}}✓ Yes{{else}}⚠️ No{{end}} |
{{end}}{{else}}No PHI access logs recorded for this period.
{{end}}
## 2. Encryption Status

| Metric | Value |
|--------|-------|
| Encryption at Rest | {{if .Encryption.AtRest}}✓ Enabled{{else}}⚠️ Disabled{{end}} |
| Encryption in Transit | {{if .Encryption.InTransit}}✓ Enabled{{else}}⚠️ Disabled{{end}} |
| Algorithm | {{if .Encryption.Algorithm}}{{.Encryption.Algorithm}}{{else}}N/A{{end}} |
| Key Management | {{if .Encryption.KeyManagement}}{{.Encryption.KeyManagement}}{{else}}N/A{{end}} |

## 3. Business Associate Agreements (BAA)

{{if .BAATracking}}| Vendor | Signed Date | Expiry Date | Status |
|--------|-------------|-------------|--------|
{{range .BAATracking}}| {{.VendorName}} | {{.SignedDate.Format "2006-01-02"}} | {{.ExpiryDate.Format "2006-01-02"}} | {{if eq .Status "Expired"}}⚠️ {{.Status}}{{else}}{{.Status}}{{end}} |
{{end}}{{else}}No BAA records for this period.
{{end}}
## 4. Minimum Necessary Principle

| Metric | Value |
|--------|-------|
| Total Accesses | {{.MinimumNecessary.TotalAccesses}} |
| Justified | {{.MinimumNecessary.JustifiedCount}} |
| Unjustified | {{.MinimumNecessary.UnjustifiedCount}} |
| Compliance Rate | {{printf "%.1f" .MinimumNecessary.ComplianceRate}}% |

{{if .MinimumNecessary.Violators}}**⚠️ Violators:**
{{range .MinimumNecessary.Violators}}- {{.}}
{{end}}{{end}}
## 5. Breach Notification Status

{{if .BreachNotifications}}| Incident ID | Detected | Affected Count | Notified HHS | Notified Individuals |
|-------------|----------|----------------|--------------|----------------------|
{{range .BreachNotifications}}| {{.IncidentID}} | {{.DetectedAt.Format "2006-01-02 15:04"}} | {{.AffectedCount}} | {{if .NotifiedHHS}}✓ Yes{{else}}⚠️ No{{end}} | {{if .NotifiedIndividuals}}✓ Yes{{else}}⚠️ No{{end}} |
{{end}}{{else}}No breach notifications for this period.
{{end}}
---

*Report generated automatically by AIP Compliance Engine — HIPAA.*
`

// NewHIPAAReport renders a HIPAA compliance report using the report engine.
func NewHIPAAReport(engine *ReportEngine, data HIPAAReportData) ([]byte, error) {
	if engine == nil {
		return nil, fmt.Errorf("report engine must not be nil")
	}

	t, err := template.New("hipaa-report").Funcs(engine.funcMap).Parse(HIPAATemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HIPAA template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute HIPAA template: %w", err)
	}

	return buf.Bytes(), nil
}
