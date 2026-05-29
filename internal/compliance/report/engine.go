// Package report provides a compliance report template engine that renders
// Go templates with audit and resource scan data into markdown and PDF formats.
package report

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// ReportFormat defines the output format of a report.
type ReportFormat string

const (
	FormatMarkdown ReportFormat = "markdown"
	FormatPDF      ReportFormat = "pdf"
)

// ReportTemplate holds metadata and content for a compliance report template.
type ReportTemplate struct {
	Name            string
	Description     string
	TemplateContent string // Go template string
	Format          ReportFormat
}

// Period represents a time range for report data collection.
type Period struct {
	Start time.Time
	End   time.Time
}

// CustomSection allows adding arbitrary named sections to a report.
type CustomSection struct {
	Title   string
	Content string
}

// ReportData contains all data needed to render a compliance report.
type ReportData struct {
	TenantID       string
	Period         Period
	AuditStats     *AuditStats
	ResourceScan   *ResourceScanResult
	Violations     []Violation
	CustomSections []CustomSection
}

// Violation represents a compliance violation found during audit.
type Violation struct {
	Rule        string
	Severity    string
	Description string
	Resource    string
	Timestamp   time.Time
}

// ReportEngine renders compliance report templates with data.
type ReportEngine struct {
	funcMap template.FuncMap
}

// NewReportEngine creates a new ReportEngine with default template functions.
func NewReportEngine() *ReportEngine {
	return &ReportEngine{
		funcMap: template.FuncMap{
			"formatTime": func(t time.Time) string {
				return t.Format("2006-01-02 15:04:05 UTC")
			},
			"upper": func(s string) string {
				return s
			},
		},
	}
}

// Render renders a template with data and returns the output bytes in the
// format specified by the template.
func (e *ReportEngine) Render(tmpl ReportTemplate, data ReportData) ([]byte, error) {
	switch tmpl.Format {
	case FormatPDF:
		return e.RenderPDF(tmpl, data)
	case FormatMarkdown:
		md, err := e.RenderMarkdown(tmpl, data)
		if err != nil {
			return nil, err
		}
		return []byte(md), nil
	default:
		md, err := e.RenderMarkdown(tmpl, data)
		if err != nil {
			return nil, err
		}
		return []byte(md), nil
	}
}

// RenderMarkdown renders a template with data and returns the markdown string.
func (e *ReportEngine) RenderMarkdown(tmpl ReportTemplate, data ReportData) (string, error) {
	if tmpl.TemplateContent == "" {
		return "", fmt.Errorf("template content is empty")
	}

	t, err := template.New(tmpl.Name).Funcs(e.funcMap).Parse(tmpl.TemplateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %q: %w", tmpl.Name, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %q: %w", tmpl.Name, err)
	}

	return buf.String(), nil
}

// RenderPDF renders a template to PDF format. Currently returns markdown content
// with a PDF header as a stub implementation. In production this would use
// headless Chrome or wkhtmltopdf for conversion.
func (e *ReportEngine) RenderPDF(tmpl ReportTemplate, data ReportData) ([]byte, error) {
	md, err := e.RenderMarkdown(tmpl, data)
	if err != nil {
		return nil, err
	}

	// Stub: prepend PDF header marker. In production, this would invoke
	// headless Chrome or wkhtmltopdf to convert markdown to actual PDF.
	header := "%PDF-1.4 (stub)\n"
	return []byte(header + md), nil
}
