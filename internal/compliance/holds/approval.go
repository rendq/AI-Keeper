package holds

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ApprovalStatus represents the state of a release approval request.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending_approval"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"

	// RequiredApprovals is the minimum number of approvals needed to release a hold.
	RequiredApprovals = 2
)

// Approval records a single approval action.
type Approval struct {
	ApprovedBy string    `json:"approvedBy"`
	ApprovedAt time.Time `json:"approvedAt"`
}

// ApprovalRequest tracks the dual-approval workflow for releasing a hold.
type ApprovalRequest struct {
	ID          string         `json:"id"`
	HoldID      string         `json:"holdID"`
	RequestedBy string         `json:"requestedBy"`
	RequestedAt time.Time      `json:"requestedAt"`
	Approvals   []Approval     `json:"approvals"`
	Status      ApprovalStatus `json:"status"`
}

// Notifier sends notifications for approval events.
type Notifier interface {
	Notify(channel, message string) error
}

// AuditLogger records audit events for approval actions.
type AuditLogger interface {
	Log(event string, details map[string]string) error
}

// Approval errors.
var (
	ErrApprovalNotFound   = errors.New("approval request not found")
	ErrDuplicateApprover  = errors.New("same person cannot approve twice")
	ErrNotPendingApproval = errors.New("approval request is not in pending state")
	ErrHoldNotPending     = errors.New("hold is not in pending_release state")
	ErrInvalidHoldID      = errors.New("invalid hold ID")
)

// ReleaseApprovalService manages the dual-approval workflow for hold releases.
type ReleaseApprovalService struct {
	store    HoldStore
	requests map[string]*ApprovalRequest
	mu       sync.RWMutex
	now      func() time.Time
	notifier Notifier
	audit    AuditLogger
}

// NewReleaseApprovalService creates a new approval service.
func NewReleaseApprovalService(store HoldStore, notifier Notifier, audit AuditLogger) *ReleaseApprovalService {
	return &ReleaseApprovalService{
		store:    store,
		requests: make(map[string]*ApprovalRequest),
		now:      time.Now,
		notifier: notifier,
		audit:    audit,
	}
}

// RequestRelease initiates a release approval workflow for the given hold.
func (s *ReleaseApprovalService) RequestRelease(holdID, requestedBy string) (*ApprovalRequest, error) {
	if holdID == "" {
		return nil, ErrInvalidHoldID
	}

	h, err := s.store.Get(holdID)
	if err != nil {
		return nil, ErrInvalidHoldID
	}

	if h.Status != StatusPendingRelease {
		return nil, ErrHoldNotPending
	}

	req := &ApprovalRequest{
		ID:          uuid.New().String(),
		HoldID:      holdID,
		RequestedBy: requestedBy,
		RequestedAt: s.now(),
		Approvals:   []Approval{},
		Status:      ApprovalStatusPending,
	}

	s.mu.Lock()
	s.requests[req.ID] = req
	s.mu.Unlock()

	// Notify compliance-officer channel.
	if s.notifier != nil {
		_ = s.notifier.Notify("compliance-officer", "Release requested for hold "+holdID)
	}

	// Audit log.
	if s.audit != nil {
		_ = s.audit.Log("release_requested", map[string]string{
			"holdID":      holdID,
			"requestedBy": requestedBy,
			"requestID":   req.ID,
		})
	}

	return req, nil
}

// Approve adds an approval to the request. When RequiredApprovals is reached,
// the hold status is updated to released.
func (s *ReleaseApprovalService) Approve(requestID, approvedBy string) (*ApprovalRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.requests[requestID]
	if !ok {
		return nil, ErrApprovalNotFound
	}

	if req.Status != ApprovalStatusPending {
		return nil, ErrNotPendingApproval
	}

	// Check duplicate approver.
	for _, a := range req.Approvals {
		if a.ApprovedBy == approvedBy {
			return nil, ErrDuplicateApprover
		}
	}

	req.Approvals = append(req.Approvals, Approval{
		ApprovedBy: approvedBy,
		ApprovedAt: s.now(),
	})

	// Audit log the approval.
	if s.audit != nil {
		_ = s.audit.Log("approval_granted", map[string]string{
			"requestID":  requestID,
			"approvedBy": approvedBy,
			"holdID":     req.HoldID,
		})
	}

	// Check if we have enough approvals.
	if len(req.Approvals) >= RequiredApprovals {
		req.Status = ApprovalStatusApproved

		// Update hold to released.
		h, err := s.store.Get(req.HoldID)
		if err == nil {
			h.Status = StatusReleased
			_ = s.store.Update(h)
		}

		// Notify.
		if s.notifier != nil {
			_ = s.notifier.Notify("compliance-officer", "Hold "+req.HoldID+" has been released")
		}

		if s.audit != nil {
			_ = s.audit.Log("hold_released", map[string]string{
				"requestID": requestID,
				"holdID":    req.HoldID,
			})
		}
	}

	return req, nil
}

// GetApproval retrieves an approval request by ID.
func (s *ReleaseApprovalService) GetApproval(requestID string) (*ApprovalRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	req, ok := s.requests[requestID]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	return req, nil
}
