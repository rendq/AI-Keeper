package report

import (
	"strings"
	"testing"
	"time"
)

func newDJSanjiTestData() DJSanjiReportData {
	return DJSanjiReportData{
		ReportData: ReportData{
			TenantID: "tenant-cn-001",
			Period: Period{
				Start: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC),
			},
		},
		ControlItems: []ControlItem{
			{
				ControlID:   "A.8.1.1",
				Category:    "物理安全",
				Description: "机房物理访问控制",
				Compliant:   true,
				Evidence:    "门禁系统日志",
			},
			{
				ControlID:   "A.10.2.3",
				Category:    "网络安全",
				Description: "网络边界防护",
				Compliant:   false,
				Evidence:    "防火墙策略审计",
			},
		},
		GuomiUsages: []GuomiUsage{
			{
				Algorithm:  "SM2",
				Purpose:    "数字签名",
				UsageCount: 15000,
				Compliant:  true,
			},
			{
				Algorithm:  "SM3",
				Purpose:    "消息摘要",
				UsageCount: 82000,
				Compliant:  true,
			},
			{
				Algorithm:  "SM4",
				Purpose:    "数据加密",
				UsageCount: 45000,
				Compliant:  false,
			},
		},
		AuditLogIntegrity: AuditLogIntegrity{
			TotalLogs:      100000,
			VerifiedCount:  99850,
			TamperDetected: true,
			HashAlgorithm:  "SM3",
		},
		AccessControlMatrix: []AccessControlMatrix{
			{
				Subject:      "admin-user",
				Resource:     "model-endpoint",
				Permission:   "read-write",
				Granted:      true,
				LastReviewed: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			},
			{
				Subject:      "readonly-user",
				Resource:     "audit-logs",
				Permission:   "read-only",
				Granted:      true,
				LastReviewed: time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC),
			},
		},
		SecurityEvents: []SecurityEvent{
			{
				EventID:     "SEC-2024-001",
				Severity:    "高",
				Category:    "未授权访问",
				Description: "多次登录失败尝试",
				DetectedAt:  time.Date(2024, 6, 12, 3, 45, 0, 0, time.UTC),
				Resolved:    true,
			},
			{
				EventID:     "SEC-2024-002",
				Severity:    "中",
				Category:    "异常流量",
				Description: "API调用频率异常",
				DetectedAt:  time.Date(2024, 6, 20, 14, 30, 0, 0, time.UTC),
				Resolved:    false,
			},
		},
	}
}

func TestDJSanjiReportFullRender(t *testing.T) {
	engine := NewReportEngine()
	data := newDJSanjiTestData()

	result, err := NewDJSanjiReport(engine, data)
	if err != nil {
		t.Fatalf("NewDJSanjiReport failed: %v", err)
	}

	output := string(result)

	checks := []string{
		"# 等保三级合规报告",
		"tenant-cn-001",
		"2024-06-01",
		"2024-06-30",
		// Section 1 - GB/T 22239 Controls
		"GB/T 22239 控制项对照",
		"A.8.1.1",
		"物理安全",
		"机房物理访问控制",
		"A.10.2.3",
		"网络安全",
		"网络边界防护",
		// Section 2 - 国密 Usage
		"国密算法使用情况",
		"SM2",
		"数字签名",
		"SM3",
		"SM4",
		"数据加密",
		// Section 3 - Audit Log Integrity
		"审计日志完整性",
		"100000",
		"99850",
		// Section 4 - Access Control Matrix
		"访问控制矩阵",
		"admin-user",
		"model-endpoint",
		"read-write",
		"readonly-user",
		"audit-logs",
		// Section 5 - Security Events
		"安全事件",
		"SEC-2024-001",
		"未授权访问",
		"SEC-2024-002",
		"异常流量",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("DJSanji report missing expected content: %q", check)
		}
	}
}

func TestDJSanjiReportNonCompliantControls(t *testing.T) {
	engine := NewReportEngine()
	data := newDJSanjiTestData()

	result, err := NewDJSanjiReport(engine, data)
	if err != nil {
		t.Fatalf("NewDJSanjiReport failed: %v", err)
	}

	output := string(result)

	// Compliant controls should show ✓
	if !strings.Contains(output, "✓ 合规") {
		t.Error("expected compliant control to show '✓ 合规'")
	}

	// Non-compliant controls should be highlighted with warning
	if !strings.Contains(output, "⚠️ 不合规") {
		t.Error("expected non-compliant control to show '⚠️ 不合规'")
	}
}

func TestDJSanjiReportAuditLogTamperDetection(t *testing.T) {
	engine := NewReportEngine()
	data := newDJSanjiTestData()

	result, err := NewDJSanjiReport(engine, data)
	if err != nil {
		t.Fatalf("NewDJSanjiReport failed: %v", err)
	}

	output := string(result)

	// Tamper detection should show warning
	if !strings.Contains(output, "⚠️ 检测到篡改") {
		t.Error("expected tamper detection warning '⚠️ 检测到篡改'")
	}

	// Test with no tamper detected
	data.AuditLogIntegrity.TamperDetected = false
	result2, err := NewDJSanjiReport(engine, data)
	if err != nil {
		t.Fatalf("NewDJSanjiReport (no tamper) failed: %v", err)
	}

	output2 := string(result2)
	if !strings.Contains(output2, "✓ 未检测到篡改") {
		t.Error("expected no-tamper message '✓ 未检测到篡改'")
	}
}

func TestDJSanjiReportEmptySections(t *testing.T) {
	engine := NewReportEngine()
	data := DJSanjiReportData{
		ReportData: ReportData{
			TenantID: "tenant-empty",
			Period: Period{
				Start: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 7, 31, 23, 59, 59, 0, time.UTC),
			},
		},
		// All DJSanji-specific slices are nil/empty
	}

	result, err := NewDJSanjiReport(engine, data)
	if err != nil {
		t.Fatalf("NewDJSanjiReport with empty data failed: %v", err)
	}

	output := string(result)

	emptyMessages := []string{
		"本周期内无控制项检查记录。",
		"本周期内无国密算法使用记录。",
		"本周期内无访问控制矩阵记录。",
		"本周期内无安全事件记录。",
	}

	for _, msg := range emptyMessages {
		if !strings.Contains(output, msg) {
			t.Errorf("expected empty-section message: %q", msg)
		}
	}

	// Audit log integrity section should still render
	if !strings.Contains(output, "审计日志完整性") {
		t.Error("expected audit log integrity section even with empty data")
	}

	// With no tamper detected (default false)
	if !strings.Contains(output, "✓ 未检测到篡改") {
		t.Error("expected no-tamper message with default empty data")
	}
}

func TestDJSanjiReportNilEngine(t *testing.T) {
	data := DJSanjiReportData{}
	_, err := NewDJSanjiReport(nil, data)
	if err == nil {
		t.Error("expected error when engine is nil")
	}
}
