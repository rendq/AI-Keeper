package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Unit tests for the publish workflow state machine ---

func TestPublishWorkflow_SubmitForReview_Success(t *testing.T) {
	store := NewMemoryStore()
	wf := NewPublishWorkflow(store)

	listing := &Listing{
		Name:     "safe-skill",
		SkillRef: "my-skill-v1",
		Phase:    PhaseDraft,
	}
	if err := store.Create(t.Context(), listing); err != nil {
		t.Fatal(err)
	}

	result, err := wf.SubmitForReview(t.Context(), listing.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected scan to pass, got issues: %v", result.Issues)
	}

	updated, _ := store.Get(t.Context(), listing.ID)
	if updated.Phase != PhasePendingReview {
		t.Fatalf("expected phase PendingReview, got %s", updated.Phase)
	}
}

func TestPublishWorkflow_SubmitForReview_SecurityFail(t *testing.T) {
	store := NewMemoryStore()
	wf := NewPublishWorkflow(store)

	listing := &Listing{
		Name:     "bad-skill",
		SkillRef: "exec(evil)",
		Phase:    PhaseDraft,
	}
	if err := store.Create(t.Context(), listing); err != nil {
		t.Fatal(err)
	}

	result, err := wf.SubmitForReview(t.Context(), listing.ID)
	if err != ErrSecurityScanFail {
		t.Fatalf("expected ErrSecurityScanFail, got %v", err)
	}
	if result.Passed {
		t.Fatal("expected scan to fail")
	}

	// Phase should remain Draft.
	updated, _ := store.Get(t.Context(), listing.ID)
	if updated.Phase != PhaseDraft {
		t.Fatalf("expected phase Draft after scan failure, got %s", updated.Phase)
	}
}

func TestPublishWorkflow_SubmitForReview_InvalidTransition(t *testing.T) {
	store := NewMemoryStore()
	wf := NewPublishWorkflow(store)

	listing := &Listing{
		Name:     "already-published",
		SkillRef: "my-skill-v1",
		Phase:    PhasePublished,
	}
	if err := store.Create(t.Context(), listing); err != nil {
		t.Fatal(err)
	}

	_, err := wf.SubmitForReview(t.Context(), listing.ID)
	if err != ErrInvalidTransition {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestPublishWorkflow_ApproveReview_Success(t *testing.T) {
	store := NewMemoryStore()
	wf := NewPublishWorkflow(store)

	listing := &Listing{
		Name:     "pending-skill",
		SkillRef: "my-skill-v1",
		Phase:    PhasePendingReview,
	}
	if err := store.Create(t.Context(), listing); err != nil {
		t.Fatal(err)
	}

	if err := wf.ApproveReview(t.Context(), listing.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	updated, _ := store.Get(t.Context(), listing.ID)
	if updated.Phase != PhasePublished {
		t.Fatalf("expected phase Published, got %s", updated.Phase)
	}
}

func TestPublishWorkflow_ApproveReview_InvalidTransition(t *testing.T) {
	store := NewMemoryStore()
	wf := NewPublishWorkflow(store)

	listing := &Listing{
		Name:     "draft-skill",
		SkillRef: "my-skill-v1",
		Phase:    PhaseDraft,
	}
	if err := store.Create(t.Context(), listing); err != nil {
		t.Fatal(err)
	}

	err := wf.ApproveReview(t.Context(), listing.ID)
	if err != ErrInvalidTransition {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestPublishWorkflow_RejectReview_Success(t *testing.T) {
	store := NewMemoryStore()
	wf := NewPublishWorkflow(store)

	listing := &Listing{
		Name:     "pending-skill",
		SkillRef: "my-skill-v1",
		Phase:    PhasePendingReview,
	}
	if err := store.Create(t.Context(), listing); err != nil {
		t.Fatal(err)
	}

	if err := wf.RejectReview(t.Context(), listing.ID, "policy violation"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	updated, _ := store.Get(t.Context(), listing.ID)
	if updated.Phase != PhaseRejected {
		t.Fatalf("expected phase Rejected, got %s", updated.Phase)
	}
}

func TestPublishWorkflow_RejectReview_InvalidTransition(t *testing.T) {
	store := NewMemoryStore()
	wf := NewPublishWorkflow(store)

	listing := &Listing{
		Name:     "draft-skill",
		SkillRef: "my-skill-v1",
		Phase:    PhaseDraft,
	}
	if err := store.Create(t.Context(), listing); err != nil {
		t.Fatal(err)
	}

	err := wf.RejectReview(t.Context(), listing.ID, "reason")
	if err != ErrInvalidTransition {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

// --- HTTP endpoint tests ---

func TestPublishWorkflow_HTTP_FullFlow(t *testing.T) {
	store := NewMemoryStore()
	srv := NewServer(store)

	// Create a listing via API.
	body := `{"tenantId":"t1","scope":"global","name":"my-skill","skillRef":"safe-ref","publisher":"pub","category":"ai"}`
	req := httptest.NewRequest(http.MethodPost, "/listings", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created Listing
	json.NewDecoder(w.Body).Decode(&created)
	if created.Phase != PhaseDraft {
		t.Fatalf("expected Draft phase on create, got %s", created.Phase)
	}

	// Submit for review.
	req = httptest.NewRequest(http.MethodPost, "/listings/"+created.ID+"/submit", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify phase is PendingReview.
	req = httptest.NewRequest(http.MethodGet, "/listings/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var fetched Listing
	json.NewDecoder(w.Body).Decode(&fetched)
	if fetched.Phase != PhasePendingReview {
		t.Fatalf("expected PendingReview after submit, got %s", fetched.Phase)
	}

	// Approve.
	req = httptest.NewRequest(http.MethodPost, "/listings/"+created.ID+"/approve", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify phase is Published.
	req = httptest.NewRequest(http.MethodGet, "/listings/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	json.NewDecoder(w.Body).Decode(&fetched)
	if fetched.Phase != PhasePublished {
		t.Fatalf("expected Published after approve, got %s", fetched.Phase)
	}
}

func TestPublishWorkflow_HTTP_RejectFlow(t *testing.T) {
	store := NewMemoryStore()
	srv := NewServer(store)

	// Create and submit a listing.
	body := `{"tenantId":"t1","scope":"global","name":"my-skill","skillRef":"safe-ref","publisher":"pub","category":"ai"}`
	req := httptest.NewRequest(http.MethodPost, "/listings", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var created Listing
	json.NewDecoder(w.Body).Decode(&created)

	req = httptest.NewRequest(http.MethodPost, "/listings/"+created.ID+"/submit", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Reject.
	rejectBody := `{"reason":"violates policy"}`
	req = httptest.NewRequest(http.MethodPost, "/listings/"+created.ID+"/reject", bytes.NewBufferString(rejectBody))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("reject: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify phase is Rejected.
	req = httptest.NewRequest(http.MethodGet, "/listings/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var fetched Listing
	json.NewDecoder(w.Body).Decode(&fetched)
	if fetched.Phase != PhaseRejected {
		t.Fatalf("expected Rejected after reject, got %s", fetched.Phase)
	}
}

func TestPublishWorkflow_HTTP_InvalidTransition(t *testing.T) {
	store := NewMemoryStore()
	srv := NewServer(store)

	// Create a listing (starts in Draft).
	body := `{"tenantId":"t1","scope":"global","name":"my-skill","skillRef":"safe-ref","publisher":"pub","category":"ai"}`
	req := httptest.NewRequest(http.MethodPost, "/listings", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var created Listing
	json.NewDecoder(w.Body).Decode(&created)

	// Try to approve a Draft listing — should fail.
	req = httptest.NewRequest(http.MethodPost, "/listings/"+created.ID+"/approve", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("approve draft: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishWorkflow_HTTP_SecurityScanReject(t *testing.T) {
	store := NewMemoryStore()
	srv := NewServer(store)

	// Create a listing with dangerous skillRef.
	body := `{"tenantId":"t1","scope":"global","name":"bad-skill","skillRef":"exec(hack)","publisher":"pub","category":"ai"}`
	req := httptest.NewRequest(http.MethodPost, "/listings", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var created Listing
	json.NewDecoder(w.Body).Decode(&created)

	// Submit — security scan should fail.
	req = httptest.NewRequest(http.MethodPost, "/listings/"+created.ID+"/submit", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("submit bad listing: expected 422, got %d: %s", w.Code, w.Body.String())
	}

	var scanResult SecurityScanResult
	json.NewDecoder(w.Body).Decode(&scanResult)
	if scanResult.Passed {
		t.Fatal("expected scan to fail")
	}
	if len(scanResult.Issues) == 0 {
		t.Fatal("expected at least one issue reported")
	}
}
