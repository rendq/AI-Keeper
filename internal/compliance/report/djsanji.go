package report

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// ControlItem represents a GB/T 22239 control item compliance check.
type ControlItem struct {
	ControlID   string
	Category    string
	Description string
	Compliant   bool
	Evidence    string
}

// GuomiUsage represents national cryptographic algorithm (国密) usage record.
type GuomiUsage struct {
	Algorithm  string // SM2, SM3, SM4
	Purpose    string
	UsageCount int
	Compliant  bool
}

// AuditLogIntegrity represents audit log integrity verification results.
type AuditLogIntegrity struct {
	TotalLogs      int
	VerifiedCount  int
	TamperDetected bool
	HashAlgorithm  string
}

// AccessControlMatrix represents an access control matrix entry.
type AccessControlMatrix struct {
	Subject      string
	Resource     string
	Permission   string
	Granted      bool
	LastReviewed time.Time
}

// SecurityEvent represents a security event record.
type SecurityEvent struct {
	EventID     string
	Severity    string
	Category    string
	Description string
	DetectedAt  time.Time
	Resolved    bool
}

// DJSanjiReportData extends ReportData with 等保三级 specific fields.
type DJSanjiReportData struct {
	ReportData
	ControlItems        []ControlItem
	GuomiUsages         []GuomiUsage
	AuditLogIntegrity   AuditLogIntegrity
	AccessControlMatrix []AccessControlMatrix
	SecurityEvents      []SecurityEvent
}

// DJSanjiTemplate is the Go template string for the 等保三级 monthly compliance report.
const DJSanjiTemplate = `# 等保三级合规报告

**租户:** {{.TenantID}}
**报告周期:** {{.Period.Start.Format "2006-01-02"}} 至 {{.Period.End.Format "2006-01-02"}}

---

## 1. GB/T 22239 控制项对照

{{if .ControlItems}}| 控制项编号 | 类别 | 描述 | 合规状态 | 证据 |
|-----------|------|------|---------|------|
{{range .ControlItems}}| {{.ControlID}} | {{.Category}} | {{.Description}} | {{if .Compliant}}✓ 合规{{else}}⚠️ 不合规{{end}} | {{.Evidence}} |
{{end}}{{else}}本周期内无控制项检查记录。
{{end}}
## 2. 国密算法使用情况

{{if .GuomiUsages}}| 算法 | 用途 | 调用次数 | 合规状态 |
|------|------|---------|---------|
{{range .GuomiUsages}}| {{.Algorithm}} | {{.Purpose}} | {{.UsageCount}} | {{if .Compliant}}✓ 合规{{else}}⚠️ 不合规{{end}} |
{{end}}{{else}}本周期内无国密算法使用记录。
{{end}}
## 3. 审计日志完整性

| 指标 | 值 |
|------|-----|
| 日志总数 | {{.AuditLogIntegrity.TotalLogs}} |
| 已验证数 | {{.AuditLogIntegrity.VerifiedCount}} |
| 篡改检测 | {{if .AuditLogIntegrity.TamperDetected}}⚠️ 检测到篡改{{else}}✓ 未检测到篡改{{end}} |
| 哈希算法 | {{if .AuditLogIntegrity.HashAlgorithm}}{{.AuditLogIntegrity.HashAlgorithm}}{{else}}N/A{{end}} |

## 4. 访问控制矩阵

{{if .AccessControlMatrix}}| 主体 | 资源 | 权限 | 授权状态 | 最近审核 |
|------|------|------|---------|---------|
{{range .AccessControlMatrix}}| {{.Subject}} | {{.Resource}} | {{.Permission}} | {{if .Granted}}✓ 已授权{{else}}✗ 未授权{{end}} | {{.LastReviewed.Format "2006-01-02"}} |
{{end}}{{else}}本周期内无访问控制矩阵记录。
{{end}}
## 5. 安全事件

{{if .SecurityEvents}}| 事件ID | 严重程度 | 类别 | 描述 | 检测时间 | 已解决 |
|--------|---------|------|------|---------|--------|
{{range .SecurityEvents}}| {{.EventID}} | {{.Severity}} | {{.Category}} | {{.Description}} | {{.DetectedAt.Format "2006-01-02 15:04"}} | {{if .Resolved}}✓ 是{{else}}⚠️ 否{{end}} |
{{end}}{{else}}本周期内无安全事件记录。
{{end}}
---

*报告由 AIP 合规引擎自动生成 — 等保三级。*
`

// NewDJSanjiReport renders a 等保三级 compliance report using the report engine.
func NewDJSanjiReport(engine *ReportEngine, data DJSanjiReportData) ([]byte, error) {
	if engine == nil {
		return nil, fmt.Errorf("report engine must not be nil")
	}

	t, err := template.New("djsanji-report").Funcs(engine.funcMap).Parse(DJSanjiTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DJSanji template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute DJSanji template: %w", err)
	}

	return buf.Bytes(), nil
}
