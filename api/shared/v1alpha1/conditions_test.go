package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSetCondition exercises the contract:
//
//   - first call: appends with LastTransitionTime ≈ now;
//   - same status+reason+message: no-op (idempotent);
//   - status changes: LastTransitionTime advances;
//   - only reason/message change: LastTransitionTime preserved.
func TestSetCondition(t *testing.T) {
	var conds []metav1.Condition

	// 1) initial set
	if !SetCondition(&conds, "SchemaValid", "False", "InvalidSchema", "missing required") {
		t.Fatalf("expected initial SetCondition to mutate slice")
	}
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	t0 := conds[0].LastTransitionTime
	if t0.IsZero() {
		t.Fatalf("LastTransitionTime should be set, got zero")
	}

	// 2) idempotent — exact same status/reason/message must not mutate.
	time.Sleep(time.Millisecond) // ensure metav1.Now() would advance
	if SetCondition(&conds, "SchemaValid", "False", "InvalidSchema", "missing required") {
		t.Fatalf("expected SetCondition no-op when nothing changed")
	}
	if !conds[0].LastTransitionTime.Equal(&t0) {
		t.Fatalf("LastTransitionTime must be preserved on no-op, got %v vs %v",
			conds[0].LastTransitionTime, t0)
	}

	// 3) reason/message change without status change: we expect the
	//    function to mutate (return true) but to KEEP LastTransitionTime
	//    because only status transitions reset the timer.
	if !SetCondition(&conds, "SchemaValid", "False", "InvalidSchema", "still missing") {
		t.Fatalf("expected SetCondition to mutate when message changes")
	}
	if !conds[0].LastTransitionTime.Equal(&t0) {
		t.Fatalf("LastTransitionTime must NOT advance on message-only change")
	}
	if conds[0].Message != "still missing" {
		t.Fatalf("message not updated, got %q", conds[0].Message)
	}

	// 4) status flip — LastTransitionTime should advance.
	time.Sleep(2 * time.Millisecond)
	if !SetCondition(&conds, "SchemaValid", "True", "SchemaParsed", "ok") {
		t.Fatalf("expected SetCondition to mutate when status flips")
	}
	if conds[0].Status != metav1.ConditionTrue {
		t.Fatalf("status not flipped, got %q", conds[0].Status)
	}
	if !conds[0].LastTransitionTime.After(t0.Time) {
		t.Fatalf("LastTransitionTime must advance on status flip, got %v vs %v",
			conds[0].LastTransitionTime, t0)
	}

	// 5) adding a different condition type appends a new entry.
	if !SetCondition(&conds, "Ready", "True", "", "") {
		t.Fatalf("expected SetCondition to append new condition type")
	}
	if len(conds) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(conds))
	}
}

func TestGetAndIsConditionTrue(t *testing.T) {
	var conds []metav1.Condition
	SetCondition(&conds, "A", "True", "", "")
	SetCondition(&conds, "B", "False", "Why", "because")

	if got := GetCondition(conds, "A"); got == nil || got.Status != metav1.ConditionTrue {
		t.Fatalf("expected A=True, got %#v", got)
	}
	if got := GetCondition(conds, "missing"); got != nil {
		t.Fatalf("expected nil for missing condition")
	}
	if !IsConditionTrue(conds, "A") {
		t.Fatalf("expected IsConditionTrue(A)")
	}
	if IsConditionTrue(conds, "B") {
		t.Fatalf("expected !IsConditionTrue(B)")
	}
}

func TestRemoveCondition(t *testing.T) {
	var conds []metav1.Condition
	SetCondition(&conds, "A", "True", "", "")
	SetCondition(&conds, "B", "True", "", "")

	if !RemoveCondition(&conds, "A") {
		t.Fatalf("expected RemoveCondition to return true")
	}
	if len(conds) != 1 || conds[0].Type != "B" {
		t.Fatalf("expected only B to remain, got %#v", conds)
	}
	if RemoveCondition(&conds, "missing") {
		t.Fatalf("expected RemoveCondition to return false for missing type")
	}
}

func TestSetCondition_NilGuard(t *testing.T) {
	if SetCondition(nil, "X", "True", "", "") {
		t.Fatalf("nil pointer must not panic and must report no mutation")
	}
	if RemoveCondition(nil, "X") {
		t.Fatalf("nil pointer must not panic and must report no mutation")
	}
}
