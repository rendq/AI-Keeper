package report

import (
	"strings"
	"testing"
	"time"
)

func newTestPeriod() Period {
	return Period{
		Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC),
	}
}

func newTestData() ReportData {
	return ReportData{
		TenantID: "tenant-001",
		Period:   newTestPeriod(),
		AuditStats: &AuditStats{
			TotalEvents:  1500,
			SuccessCount: 1400,
			FailureCount: 100,
			UniqueUsers:  25,
			TopActions: []ActionCount{
				{Action: "model.invoke", Count: 800},
				{Action: "data.read", Count: 400},
			},
			PeriodStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:   time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC),
		},
		ResourceScan: &ResourceScanResult{
			TotalResources:    50,
			CompliantCount:    45,
			NonCompliantCount: 5,
			Resources: []ScannedResource{
				{Kind: "Pod", Name: "api-server", Namespace: "default", Compliant: true},
				{Kind: "Pod", Name: "db-unencrypted", Namespace: "data", Compliant: false, Issues: []string{"missing encryption"}},
			},
			ScanTime: time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC),
		},
		Violations: []Violation{
			{
				Rule:        "encryption-at-rest",
				Severity:    "HIGH",
				Description: "Data stored without encryption",
				Resource:    "pod/db-unencrypted",
				Timestamp:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			},
		},
		CustomSections: []CustomSection{
			{Title: "Recommendations", Content: "Enable encryption for all data stores."},
		},
	}
}

const testTemplate = `# Compliance Report: {{.TenantID}}

**Period:** {{.Period.Start.Format "2006-01-02"}} to {{.Period.End.Format "2006-01-02"}}

## Audit Summary
- Total Events: {{.AuditStats.TotalEvents}}
- Success: {{.AuditStats.SuccessCount}}
- Failures: {{.AuditStats.FailureCount}}
- Unique Users: {{.AuditStats.UniqueUsers}}

## Resource Scan
- Total Resources: {{.ResourceScan.TotalResources}}
- Compliant: {{.ResourceScan.CompliantCount}}
- Non-Compliant: {{.ResourceScan.NonCompliantCount}}

## Violations
{{range .Violations}}- [{{.Severity}}] {{.Rule}}: {{.Description}} ({{.Resource}})
{{end}}
## Custom Sections
{{range .CustomSections}}### {{.Title}}
{{.Content}}
{{end}}`

func TestRenderMarkdown(t *testing.T) {
	engine := NewReportEngine()
	tmpl := ReportTemplate{
		Name:            "test-compliance",
		Description:     "Test compliance report",
		TemplateContent: testTemplate,
		Format:          FormatMarkdown,
	}

	result, err := engine.RenderMarkdown(tmpl, newTestData())
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	// Verify key content is present
	checks := []string{
		"# Compliance Report: tenant-001",
		"Total Events: 1500",
		"Success: 1400",
		"Failures: 100",
		"Unique Users: 25",
		"Total Resources: 50",
		"Compliant: 45",
		"Non-Compliant: 5",
		"[HIGH] encryption-at-rest",
		"### Recommendations",
		"Enable encryption for all data stores.",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("markdown output missing expected content: %q", check)
		}
	}
}

func TestRenderPDFStub(t *testing.T) {
	engine := NewReportEngine()
	tmpl := ReportTemplate{
		Name:            "test-pdf",
		Description:     "Test PDF report",
		TemplateContent: testTemplate,
		Format:          FormatPDF,
	}

	result, err := engine.RenderPDF(tmpl, newTestData())
	if err != nil {
		t.Fatalf("RenderPDF failed: %v", err)
	}

	content := string(result)
	if !strings.HasPrefix(content, "%PDF-1.4 (stub)") {
		t.Error("PDF stub should start with PDF header")
	}
	if !strings.Contains(content, "# Compliance Report: tenant-001") {
		t.Error("PDF stub should contain rendered markdown content")
	}
}

func TestRenderWithFormat(t *testing.T) {
	engine := NewReportEngine()
	data := newTestData()

	// Test markdown format via Render
	mdTmpl := ReportTemplate{
		Name:            "md-test",
		TemplateContent: `# {{.TenantID}}`,
		Format:          FormatMarkdown,
	}
	result, err := engine.Render(mdTmpl, data)
	if err != nil {
		t.Fatalf("Render markdown failed: %v", err)
	}
	if string(result) != "# tenant-001" {
		t.Errorf("unexpected markdown output: %s", result)
	}

	// Test PDF format via Render
	pdfTmpl := ReportTemplate{
		Name:            "pdf-test",
		TemplateContent: `# {{.TenantID}}`,
		Format:          FormatPDF,
	}
	result, err = engine.Render(pdfTmpl, data)
	if err != nil {
		t.Fatalf("Render PDF failed: %v", err)
	}
	if !strings.HasPrefix(string(result), "%PDF-1.4 (stub)") {
		t.Error("PDF output should have PDF header")
	}
}

func TestRenderEmptyTemplate(t *testing.T) {
	engine := NewReportEngine()
	tmpl := ReportTemplate{
		Name:            "empty",
		TemplateContent: "",
		Format:          FormatMarkdown,
	}

	_, err := engine.RenderMarkdown(tmpl, ReportData{})
	if err == nil {
		t.Error("expected error for empty template content")
	}
	if !strings.Contains(err.Error(), "template content is empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRenderMissingData(t *testing.T) {
	engine := NewReportEngine()

	// Template that accesses nil fields gracefully
	tmpl := ReportTemplate{
		Name: "partial-data",
		TemplateContent: `# Report: {{.TenantID}}
Period: {{.Period.Start.Format "2006-01-02"}}
{{if .AuditStats}}Audit: {{.AuditStats.TotalEvents}}{{else}}No audit data{{end}}
{{if .ResourceScan}}Scan: {{.ResourceScan.TotalResources}}{{else}}No scan data{{end}}
{{if .Violations}}Violations: {{len .Violations}}{{else}}No violations{{end}}
{{range .CustomSections}}{{.Title}}{{end}}`,
		Format: FormatMarkdown,
	}

	data := ReportData{
		TenantID: "tenant-empty",
		Period:   newTestPeriod(),
		// AuditStats, ResourceScan, Violations all nil/empty
	}

	result, err := engine.RenderMarkdown(tmpl, data)
	if err != nil {
		t.Fatalf("RenderMarkdown with missing data failed: %v", err)
	}

	if !strings.Contains(result, "No audit data") {
		t.Error("expected 'No audit data' for nil AuditStats")
	}
	if !strings.Contains(result, "No scan data") {
		t.Error("expected 'No scan data' for nil ResourceScan")
	}
	if !strings.Contains(result, "No violations") {
		t.Error("expected 'No violations' for nil Violations")
	}
}

func TestRenderCustomSections(t *testing.T) {
	engine := NewReportEngine()
	tmpl := ReportTemplate{
		Name: "custom-sections",
		TemplateContent: `{{range .CustomSections}}## {{.Title}}
{{.Content}}
{{end}}`,
		Format: FormatMarkdown,
	}

	data := ReportData{
		CustomSections: []CustomSection{
			{Title: "Section A", Content: "Content for section A"},
			{Title: "Section B", Content: "Content for section B"},
		},
	}

	result, err := engine.RenderMarkdown(tmpl, data)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	if !strings.Contains(result, "## Section A") {
		t.Error("missing Section A")
	}
	if !strings.Contains(result, "Content for section A") {
		t.Error("missing content for Section A")
	}
	if !strings.Contains(result, "## Section B") {
		t.Error("missing Section B")
	}
}

func TestRenderInvalidTemplate(t *testing.T) {
	engine := NewReportEngine()
	tmpl := ReportTemplate{
		Name:            "invalid",
		TemplateContent: `{{.Invalid`,
		Format:          FormatMarkdown,
	}

	_, err := engine.RenderMarkdown(tmpl, ReportData{})
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}
