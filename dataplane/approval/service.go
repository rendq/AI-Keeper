// Package approval provides a human-in-the-loop approval workflow engine.
// It manages approval requests for sensitive AI agent actions, allowing
// designated approvers to approve or deny pending requests within a
// configurable timeout window.
//
// Requirements: B14, D5
package approval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// Types
// ──────────────────────────────────────────────────────────────────────────────

// ApprovalStatus represents the current state of an approval request.
type ApprovalStatus string

const (
	StatusPending  ApprovalStatus = "Pending"
	StatusApproved ApprovalStatus = "Approved"
	StatusDenied   ApprovalStatus = "Denied"
	StatusExpired  ApprovalStatus = "Expired"
)

// DefaultTimeout is the default duration before a pending request expires.
const DefaultTimeout = 4 * time.Hour

// ApprovalRequest represents a request for human approval of an action.
type ApprovalRequest struct {
	ID        string
	Requester string
	Action    string
	Resource  string
	Reason    string
	Timeout   time.Duration
	CreatedAt time.Time
}

// ApprovalResult represents the outcome of an approval request.
type ApprovalResult struct {
	Status     ApprovalStatus
	Approver   string
	ApprovedAt time.Time
	Comment    string
}

// ──────────────────────────────────────────────────────────────────────────────
// Errors
// ──────────────────────────────────────────────────────────────────────────────

var (
	ErrNotFound       = errors.New("approval request not found")
	ErrAlreadyDecided = errors.New("approval request already decided")
)

// ──────────────────────────────────────────────────────────────────────────────
// Internal record
// ──────────────────────────────────────────────────────────────────────────────

type record struct {
	request ApprovalRequest
	result  ApprovalResult
}

// ──────────────────────────────────────────────────────────────────────────────
// Service
// ──────────────────────────────────────────────────────────────────────────────

// ApprovalService manages the lifecycle of approval requests in-memory.
// Production implementations would persist to PostgreSQL and integrate with
// messaging channels (Slack, Teams, etc).
type ApprovalService struct {
	mu      sync.RWMutex
	records map[string]*record
}

// NewApprovalService creates a new in-memory approval service.
func NewApprovalService() *ApprovalService {
	return &ApprovalService{
		records: make(map[string]*record),
	}
}

// Submit creates a new approval request and returns the generated request ID.
func (s *ApprovalService) Submit(_ context.Context, req ApprovalRequest) (string, error) {
	if req.Requester == "" {
		return "", fmt.Errorf("requester is required")
	}
	if req.Action == "" {
		return "", fmt.Errorf("action is required")
	}

	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}

	r := &record{
		request: req,
		result: ApprovalResult{
			Status: StatusPending,
		},
	}

	s.mu.Lock()
	s.records[req.ID] = r
	s.mu.Unlock()

	return req.ID, nil
}

// GetStatus returns the current result for an approval request.
func (s *ApprovalService) GetStatus(_ context.Context, requestID string) (*ApprovalResult, error) {
	s.mu.RLock()
	r, ok := s.records[requestID]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	result := r.result
	return &result, nil
}

// Approve marks an approval request as approved.
func (s *ApprovalService) Approve(_ context.Context, requestID, approver, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[requestID]
	if !ok {
		return ErrNotFound
	}
	if r.result.Status != StatusPending {
		return ErrAlreadyDecided
	}

	r.result = ApprovalResult{
		Status:     StatusApproved,
		Approver:   approver,
		ApprovedAt: time.Now(),
		Comment:    comment,
	}
	return nil
}

// Deny marks an approval request as denied.
func (s *ApprovalService) Deny(_ context.Context, requestID, approver, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[requestID]
	if !ok {
		return ErrNotFound
	}
	if r.result.Status != StatusPending {
		return ErrAlreadyDecided
	}

	r.result = ApprovalResult{
		Status:     StatusDenied,
		Approver:   approver,
		ApprovedAt: time.Now(),
		Comment:    comment,
	}
	return nil
}

// CheckTimeout checks whether a pending request has exceeded its timeout.
// If expired, it transitions the status to Expired. Returns the current result.
func (s *ApprovalService) CheckTimeout(_ context.Context, requestID string) (*ApprovalResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[requestID]
	if !ok {
		return nil, ErrNotFound
	}

	if r.result.Status == StatusPending {
		deadline := r.request.CreatedAt.Add(r.request.Timeout)
		if time.Now().After(deadline) {
			r.result = ApprovalResult{
				Status:     StatusExpired,
				ApprovedAt: time.Now(),
				Comment:    "request timed out",
			}
		}
	}

	result := r.result
	return &result, nil
}
