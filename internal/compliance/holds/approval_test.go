package holds

import (
	"testing"
	"time"
)

// stubNotifier implements Notifier for testing.
type stubNotifier struct {
	messages []string
}

func (n *stubNotifier) Notify(channel, message string) error {
	n.messages = append(n.messages, channel+":"+message)
	return nil
}

// stubAuditLogger implements AuditLogger for testing.
type stubAuditLogger struct {
	events []string
}

func (l *stubAuditLogger) Log(event string, details map[string]string) error {
	l.events = append(l.events, event)
	return nil
}

func newTestApprovalService() (*ReleaseApprovalService, *HoldService) {
	store := NewMemoryStore()
	holdSvc := NewHoldService(store)
	holdSvc.now = fixedNow

	notifier := &stubNotifier{}
	audit := &stubAuditLogger{}
	approvalSvc := NewReleaseApprovalService(store, notifier, audit)
	approvalSvc.now = fixedNow

	return approvalSvc, holdSvc
}

func createPendingHold(t *testing.T, holdSvc *HoldService) *Hold {
	t.Helper()
	h, err := holdSvc.ApplyHold("legal-investigation", "admin@corp.com", validScope(), nil)
	if err != nil {
		t.Fatalf("ApplyHold failed: %v", err)
	}
	h, err = holdSvc.ReleaseHold(h.ID)
	if err != nil {
		t.Fatalf("ReleaseHold failed: %v", err)
	}
	return h
}

func TestReleaseApprovalRequestCreation(t *testing.T) {
	approvalSvc, holdSvc := newTestApprovalService()
	h := createPendingHold(t, holdSvc)

	req, err := approvalSvc.RequestRelease(h.ID, "officer@corp.com")
	if err != nil {
		t.Fatalf("RequestRelease failed: %v", err)
	}
	if req.ID == "" {
		t.Error("expected non-empty request ID")
	}
	if req.HoldID != h.ID {
		t.Errorf("expected holdID %q, got %q", h.ID, req.HoldID)
	}
	if req.Status != ApprovalStatusPending {
		t.Errorf("expected status %q, got %q", ApprovalStatusPending, req.Status)
	}
	if req.RequestedBy != "officer@corp.com" {
		t.Errorf("expected requestedBy %q, got %q", "officer@corp.com", req.RequestedBy)
	}
	if !req.RequestedAt.Equal(fixedNow()) {
		t.Errorf("expected requestedAt %v, got %v", fixedNow(), req.RequestedAt)
	}
	if len(req.Approvals) != 0 {
		t.Errorf("expected 0 approvals, got %d", len(req.Approvals))
	}
}

func TestReleaseApprovalFirstApprovalDoesNotRelease(t *testing.T) {
	approvalSvc, holdSvc := newTestApprovalService()
	h := createPendingHold(t, holdSvc)

	req, _ := approvalSvc.RequestRelease(h.ID, "officer@corp.com")

	req, err := approvalSvc.Approve(req.ID, "approver-1@corp.com")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if req.Status != ApprovalStatusPending {
		t.Errorf("expected status %q after first approval, got %q", ApprovalStatusPending, req.Status)
	}
	if len(req.Approvals) != 1 {
		t.Errorf("expected 1 approval, got %d", len(req.Approvals))
	}

	// Hold should still be pending_release.
	got, _ := holdSvc.GetHold(h.ID)
	if got.Status != StatusPendingRelease {
		t.Errorf("expected hold status %q, got %q", StatusPendingRelease, got.Status)
	}
}

func TestReleaseApprovalSecondApprovalTriggersRelease(t *testing.T) {
	approvalSvc, holdSvc := newTestApprovalService()
	h := createPendingHold(t, holdSvc)

	req, _ := approvalSvc.RequestRelease(h.ID, "officer@corp.com")

	approvalSvc.Approve(req.ID, "approver-1@corp.com")
	req, err := approvalSvc.Approve(req.ID, "approver-2@corp.com")
	if err != nil {
		t.Fatalf("second Approve failed: %v", err)
	}
	if req.Status != ApprovalStatusApproved {
		t.Errorf("expected status %q, got %q", ApprovalStatusApproved, req.Status)
	}
	if len(req.Approvals) != 2 {
		t.Errorf("expected 2 approvals, got %d", len(req.Approvals))
	}

	// Hold should be released.
	got, _ := holdSvc.GetHold(h.ID)
	if got.Status != StatusReleased {
		t.Errorf("expected hold status %q, got %q", StatusReleased, got.Status)
	}
}

func TestReleaseApprovalDuplicateApproverFails(t *testing.T) {
	approvalSvc, holdSvc := newTestApprovalService()
	h := createPendingHold(t, holdSvc)

	req, _ := approvalSvc.RequestRelease(h.ID, "officer@corp.com")
	approvalSvc.Approve(req.ID, "approver-1@corp.com")

	_, err := approvalSvc.Approve(req.ID, "approver-1@corp.com")
	if err != ErrDuplicateApprover {
		t.Errorf("expected ErrDuplicateApprover, got %v", err)
	}
}

func TestReleaseApprovalOnNonPendingHoldFails(t *testing.T) {
	approvalSvc, holdSvc := newTestApprovalService()

	// Create active hold (not pending_release).
	h, err := holdSvc.ApplyHold("legal", "admin@corp.com", validScope(), nil)
	if err != nil {
		t.Fatalf("ApplyHold failed: %v", err)
	}

	_, err = approvalSvc.RequestRelease(h.ID, "officer@corp.com")
	if err != ErrHoldNotPending {
		t.Errorf("expected ErrHoldNotPending, got %v", err)
	}
}

func TestReleaseApprovalInvalidHoldIDFails(t *testing.T) {
	approvalSvc, _ := newTestApprovalService()

	_, err := approvalSvc.RequestRelease("nonexistent-id", "officer@corp.com")
	if err != ErrInvalidHoldID {
		t.Errorf("expected ErrInvalidHoldID, got %v", err)
	}
}

func TestReleaseApprovalNotification(t *testing.T) {
	store := NewMemoryStore()
	holdSvc := NewHoldService(store)
	holdSvc.now = fixedNow

	notifier := &stubNotifier{}
	audit := &stubAuditLogger{}
	approvalSvc := NewReleaseApprovalService(store, notifier, audit)
	approvalSvc.now = fixedNow

	h := createPendingHold(t, holdSvc)
	req, _ := approvalSvc.RequestRelease(h.ID, "officer@corp.com")

	// Should have notification for request.
	if len(notifier.messages) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.messages))
	}

	approvalSvc.Approve(req.ID, "approver-1@corp.com")
	approvalSvc.Approve(req.ID, "approver-2@corp.com")

	// Should have notification for release.
	if len(notifier.messages) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(notifier.messages))
	}
}

func TestReleaseApprovalAuditEvents(t *testing.T) {
	store := NewMemoryStore()
	holdSvc := NewHoldService(store)
	holdSvc.now = fixedNow

	notifier := &stubNotifier{}
	audit := &stubAuditLogger{}
	approvalSvc := NewReleaseApprovalService(store, notifier, audit)
	approvalSvc.now = fixedNow

	h := createPendingHold(t, holdSvc)
	req, _ := approvalSvc.RequestRelease(h.ID, "officer@corp.com")

	approvalSvc.Approve(req.ID, "approver-1@corp.com")
	approvalSvc.Approve(req.ID, "approver-2@corp.com")

	// Expected events: release_requested, approval_granted, approval_granted, hold_released
	expected := []string{"release_requested", "approval_granted", "approval_granted", "hold_released"}
	if len(audit.events) != len(expected) {
		t.Fatalf("expected %d audit events, got %d: %v", len(expected), len(audit.events), audit.events)
	}
	for i, e := range expected {
		if audit.events[i] != e {
			t.Errorf("audit event[%d]: expected %q, got %q", i, e, audit.events[i])
		}
	}
}

func TestReleaseApprovalGetApproval(t *testing.T) {
	approvalSvc, holdSvc := newTestApprovalService()
	h := createPendingHold(t, holdSvc)

	req, _ := approvalSvc.RequestRelease(h.ID, "officer@corp.com")

	got, err := approvalSvc.GetApproval(req.ID)
	if err != nil {
		t.Fatalf("GetApproval failed: %v", err)
	}
	if got.ID != req.ID {
		t.Errorf("expected ID %q, got %q", req.ID, got.ID)
	}

	// Non-existent request.
	_, err = approvalSvc.GetApproval("nonexistent")
	if err != ErrApprovalNotFound {
		t.Errorf("expected ErrApprovalNotFound, got %v", err)
	}
}

// Ensure unused import doesn't break compilation.
var _ = time.Now
