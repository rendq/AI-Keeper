package approval

import (
	"context"
	"testing"
	"time"
)

func TestSubmitAndGetStatus(t *testing.T) {
	svc := NewApprovalService()
	ctx := context.Background()

	req := ApprovalRequest{
		Requester: "agent-1",
		Action:    "deploy",
		Resource:  "prod-cluster",
		Reason:    "scheduled release",
	}

	id, err := svc.Submit(ctx, req)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty request ID")
	}

	result, err := svc.GetStatus(ctx, id)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if result.Status != StatusPending {
		t.Fatalf("expected Pending, got %s", result.Status)
	}
}

func TestApprove(t *testing.T) {
	svc := NewApprovalService()
	ctx := context.Background()

	id, _ := svc.Submit(ctx, ApprovalRequest{
		Requester: "agent-1",
		Action:    "delete-resource",
		Resource:  "dataset-abc",
	})

	err := svc.Approve(ctx, id, "admin@example.com", "looks good")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	result, _ := svc.GetStatus(ctx, id)
	if result.Status != StatusApproved {
		t.Fatalf("expected Approved, got %s", result.Status)
	}
	if result.Approver != "admin@example.com" {
		t.Fatalf("expected approver admin@example.com, got %s", result.Approver)
	}
	if result.Comment != "looks good" {
		t.Fatalf("expected comment 'looks good', got %s", result.Comment)
	}

	// Cannot approve again
	err = svc.Approve(ctx, id, "other@example.com", "also me")
	if err != ErrAlreadyDecided {
		t.Fatalf("expected ErrAlreadyDecided, got %v", err)
	}
}

func TestDeny(t *testing.T) {
	svc := NewApprovalService()
	ctx := context.Background()

	id, _ := svc.Submit(ctx, ApprovalRequest{
		Requester: "agent-2",
		Action:    "escalate-privilege",
		Resource:  "service-account-x",
	})

	err := svc.Deny(ctx, id, "security@example.com", "too risky")
	if err != nil {
		t.Fatalf("Deny failed: %v", err)
	}

	result, _ := svc.GetStatus(ctx, id)
	if result.Status != StatusDenied {
		t.Fatalf("expected Denied, got %s", result.Status)
	}
	if result.Approver != "security@example.com" {
		t.Fatalf("expected approver security@example.com, got %s", result.Approver)
	}

	// Cannot deny again
	err = svc.Deny(ctx, id, "other@example.com", "nope")
	if err != ErrAlreadyDecided {
		t.Fatalf("expected ErrAlreadyDecided, got %v", err)
	}
}

func TestCheckTimeout_Expired(t *testing.T) {
	svc := NewApprovalService()
	ctx := context.Background()

	// Submit with a very short timeout that has already passed
	id, _ := svc.Submit(ctx, ApprovalRequest{
		Requester: "agent-3",
		Action:    "dangerous-op",
		Resource:  "critical-system",
		Timeout:   1 * time.Millisecond,
		CreatedAt: time.Now().Add(-1 * time.Hour), // created an hour ago
	})

	result, err := svc.CheckTimeout(ctx, id)
	if err != nil {
		t.Fatalf("CheckTimeout failed: %v", err)
	}
	if result.Status != StatusExpired {
		t.Fatalf("expected Expired, got %s", result.Status)
	}
}

func TestCheckTimeout_NotExpired(t *testing.T) {
	svc := NewApprovalService()
	ctx := context.Background()

	id, _ := svc.Submit(ctx, ApprovalRequest{
		Requester: "agent-4",
		Action:    "safe-op",
		Resource:  "test-env",
		Timeout:   24 * time.Hour,
	})

	result, err := svc.CheckTimeout(ctx, id)
	if err != nil {
		t.Fatalf("CheckTimeout failed: %v", err)
	}
	if result.Status != StatusPending {
		t.Fatalf("expected Pending, got %s", result.Status)
	}
}
