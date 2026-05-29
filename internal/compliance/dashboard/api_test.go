package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDashboardAPI_GetStatus(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	frameworks := []FrameworkStatus{
		{Name: FrameworkGDPR, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
		{Name: FrameworkDJSANJI, Status: LevelPartial, ComplianceRate: 0.8, LastScanAt: now},
	}
	svc := NewDashboardService(frameworks, 3, now)
	api := NewDashboardAPI(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/compliance/status", nil)
	rec := httptest.NewRecorder()
	api.Handler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var got ComplianceStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.Overall != StatusYellow {
		t.Errorf("expected yellow overall, got %s", got.Overall)
	}
	if got.PendingHolds != 3 {
		t.Errorf("expected 3 pending holds, got %d", got.PendingHolds)
	}
	if len(got.FrameworkStatuses) != 2 {
		t.Errorf("expected 2 frameworks, got %d", len(got.FrameworkStatuses))
	}
}

func TestDashboardAPI_AllCompliant(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	frameworks := []FrameworkStatus{
		{Name: FrameworkGDPR, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
		{Name: FrameworkSOC2, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
		{Name: FrameworkHIPAA, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
		{Name: FrameworkDJSANJI, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
	}
	svc := NewDashboardService(frameworks, 0, now)
	status := svc.GetStatus()

	if status.Overall != StatusGreen {
		t.Errorf("expected green overall, got %s", status.Overall)
	}
	if status.PendingHolds != 0 {
		t.Errorf("expected 0 pending holds, got %d", status.PendingHolds)
	}
}

func TestDashboardAPI_Partial(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	frameworks := []FrameworkStatus{
		{Name: FrameworkGDPR, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
		{Name: FrameworkSOC2, Status: LevelPartial, ComplianceRate: 0.75, LastScanAt: now},
		{Name: FrameworkHIPAA, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
	}
	svc := NewDashboardService(frameworks, 1, now)
	status := svc.GetStatus()

	if status.Overall != StatusYellow {
		t.Errorf("expected yellow overall, got %s", status.Overall)
	}
}

func TestDashboardAPI_NonCompliant(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	frameworks := []FrameworkStatus{
		{Name: FrameworkGDPR, Status: LevelCompliant, ComplianceRate: 1.0, LastScanAt: now},
		{Name: FrameworkSOC2, Status: LevelPartial, ComplianceRate: 0.75, LastScanAt: now},
		{Name: FrameworkDJSANJI, Status: LevelNonCompliant, ComplianceRate: 0.3, LastScanAt: now},
	}
	svc := NewDashboardService(frameworks, 5, now)
	status := svc.GetStatus()

	if status.Overall != StatusRed {
		t.Errorf("expected red overall, got %s", status.Overall)
	}
	if status.PendingHolds != 5 {
		t.Errorf("expected 5 pending holds, got %d", status.PendingHolds)
	}
}
