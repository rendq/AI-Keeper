// Package dashboard provides a unified compliance status API.
package dashboard

import (
	"encoding/json"
	"net/http"
	"time"
)

// OverallStatus represents the traffic-light status.
type OverallStatus string

const (
	StatusGreen  OverallStatus = "green"
	StatusYellow OverallStatus = "yellow"
	StatusRed    OverallStatus = "red"
)

// FrameworkName identifies a compliance framework.
type FrameworkName string

const (
	FrameworkGDPR    FrameworkName = "GDPR"
	FrameworkSOC2    FrameworkName = "SOC2"
	FrameworkHIPAA   FrameworkName = "HIPAA"
	FrameworkDJSANJI FrameworkName = "DJSANJI"
)

// ComplianceLevel for individual frameworks.
type ComplianceLevel string

const (
	LevelCompliant    ComplianceLevel = "compliant"
	LevelPartial      ComplianceLevel = "partial"
	LevelNonCompliant ComplianceLevel = "non_compliant"
)

// FrameworkStatus holds compliance state for a single framework.
type FrameworkStatus struct {
	Name           FrameworkName   `json:"name"`
	Status         ComplianceLevel `json:"status"`
	ComplianceRate float64         `json:"complianceRate"`
	LastScanAt     time.Time       `json:"lastScanAt"`
}

// ComplianceStatus is the aggregated dashboard response.
type ComplianceStatus struct {
	Overall           OverallStatus   `json:"overall"`
	FrameworkStatuses []FrameworkStatus `json:"frameworkStatuses"`
	PendingHolds      int             `json:"pendingHolds"`
	LastReportAt      time.Time       `json:"lastReportAt"`
}

// DashboardService aggregates scanner results, hold counts, and report dates.
type DashboardService struct {
	frameworks   []FrameworkStatus
	pendingHolds int
	lastReport   time.Time
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(frameworks []FrameworkStatus, pendingHolds int, lastReport time.Time) *DashboardService {
	return &DashboardService{
		frameworks:   frameworks,
		pendingHolds: pendingHolds,
		lastReport:   lastReport,
	}
}

// GetStatus aggregates all compliance results into a unified status.
func (s *DashboardService) GetStatus() *ComplianceStatus {
	overall := computeOverall(s.frameworks)
	return &ComplianceStatus{
		Overall:           overall,
		FrameworkStatuses: s.frameworks,
		PendingHolds:      s.pendingHolds,
		LastReportAt:      s.lastReport,
	}
}

// computeOverall derives the traffic-light status from framework statuses.
func computeOverall(frameworks []FrameworkStatus) OverallStatus {
	if len(frameworks) == 0 {
		return StatusGreen
	}
	hasPartial := false
	for _, f := range frameworks {
		if f.Status == LevelNonCompliant {
			return StatusRed
		}
		if f.Status == LevelPartial {
			hasPartial = true
		}
	}
	if hasPartial {
		return StatusYellow
	}
	return StatusGreen
}

// DashboardAPI exposes the compliance dashboard over HTTP.
type DashboardAPI struct {
	service *DashboardService
}

// NewDashboardAPI creates a new HTTP API handler.
func NewDashboardAPI(service *DashboardService) *DashboardAPI {
	return &DashboardAPI{service: service}
}

// Handler returns an http.HandlerFunc for GET /api/compliance/status.
func (a *DashboardAPI) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := a.service.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}
