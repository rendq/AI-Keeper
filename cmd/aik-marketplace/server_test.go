package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer() *Server {
	return NewServer(NewMemoryStore())
}

func createTestListing(t *testing.T, srv *Server, req CreateListingRequest) *Listing {
	t.Helper()
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/listings", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var listing Listing
	json.NewDecoder(w.Body).Decode(&listing)
	return &listing
}

// --- CRUD Tests ---

func TestCreateListing(t *testing.T) {
	srv := newTestServer()

	req := CreateListingRequest{
		TenantID:  "tenant-1",
		Scope:     ScopeTenant,
		Name:      "code-review-skill",
		SkillRef:  "skill://code-review@1.0.0",
		Publisher: "DevTeam",
		Category:  "development",
		Tags:      []string{"code", "review", "ai"},
		Readme:    "# Code Review Skill",
	}

	listing := createTestListing(t, srv, req)

	if listing.ID == "" {
		t.Error("listing should have an ID")
	}
	if listing.Name != "code-review-skill" {
		t.Errorf("expected name=code-review-skill, got %s", listing.Name)
	}
	if listing.Scope != ScopeTenant {
		t.Errorf("expected scope=tenant, got %s", listing.Scope)
	}
	if listing.TenantID != "tenant-1" {
		t.Errorf("expected tenantId=tenant-1, got %s", listing.TenantID)
	}
}

func TestCreateListing_MissingFields(t *testing.T) {
	srv := newTestServer()
	body := `{"name": ""}`
	r := httptest.NewRequest(http.MethodPost, "/listings", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateListing_InvalidScope(t *testing.T) {
	srv := newTestServer()
	body := `{"name":"x","skillRef":"skill://x","publisher":"p","category":"c","scope":"invalid"}`
	r := httptest.NewRequest(http.MethodPost, "/listings", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetListing(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID:  "tenant-1",
		Name:      "test-skill",
		SkillRef:  "skill://test@1.0.0",
		Publisher: "Publisher",
		Category:  "testing",
	})

	r := httptest.NewRequest(http.MethodGet, "/listings/"+listing.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var got Listing
	json.NewDecoder(w.Body).Decode(&got)
	if got.ID != listing.ID {
		t.Errorf("expected id=%s, got %s", listing.ID, got.ID)
	}
}

func TestGetListing_NotFound(t *testing.T) {
	srv := newTestServer()
	r := httptest.NewRequest(http.MethodGet, "/listings/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateListing(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID:  "tenant-1",
		Name:      "original-name",
		SkillRef:  "skill://original@1.0.0",
		Publisher: "Publisher",
		Category:  "dev",
	})

	newName := "updated-name"
	updateBody, _ := json.Marshal(UpdateListingRequest{Name: &newName})
	r := httptest.NewRequest(http.MethodPut, "/listings/"+listing.ID, bytes.NewReader(updateBody))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated Listing
	json.NewDecoder(w.Body).Decode(&updated)
	if updated.Name != "updated-name" {
		t.Errorf("expected name=updated-name, got %s", updated.Name)
	}
}

func TestUpdateListing_NotFound(t *testing.T) {
	srv := newTestServer()
	body := `{"name":"x"}`
	r := httptest.NewRequest(http.MethodPut, "/listings/nonexistent", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteListing(t *testing.T) {
	srv := newTestServer()
	listing := createTestListing(t, srv, CreateListingRequest{
		TenantID:  "tenant-1",
		Name:      "to-delete",
		SkillRef:  "skill://del@1.0.0",
		Publisher: "Publisher",
		Category:  "misc",
	})

	r := httptest.NewRequest(http.MethodDelete, "/listings/"+listing.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	// Verify it's gone
	r = httptest.NewRequest(http.MethodGet, "/listings/"+listing.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

func TestDeleteListing_NotFound(t *testing.T) {
	srv := newTestServer()
	r := httptest.NewRequest(http.MethodDelete, "/listings/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Scope Tests ---

func TestSearchListings_TenantScope(t *testing.T) {
	srv := newTestServer()

	// Create tenant-scoped listing for tenant-1
	createTestListing(t, srv, CreateListingRequest{
		TenantID:  "tenant-1",
		Scope:     ScopeTenant,
		Name:      "internal-skill",
		SkillRef:  "skill://internal@1.0.0",
		Publisher: "Team1",
		Category:  "internal",
	})

	// Create global listing
	createTestListing(t, srv, CreateListingRequest{
		TenantID:  "tenant-1",
		Scope:     ScopeGlobal,
		Name:      "public-skill",
		SkillRef:  "skill://public@1.0.0",
		Publisher: "Team1",
		Category:  "public",
	})

	// tenant-1 should see both
	r := httptest.NewRequest(http.MethodGet, "/listings?tenantId=tenant-1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var results []*Listing
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 2 {
		t.Errorf("tenant-1 should see 2 listings, got %d", len(results))
	}

	// tenant-2 should only see global listing
	r = httptest.NewRequest(http.MethodGet, "/listings?tenantId=tenant-2", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 1 {
		t.Errorf("tenant-2 should see 1 listing, got %d", len(results))
	}
	if results[0].Name != "public-skill" {
		t.Errorf("tenant-2 should only see public-skill, got %s", results[0].Name)
	}
}

// --- Search/Filter Tests ---

func TestSearchListings_ByCategory(t *testing.T) {
	srv := newTestServer()

	createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "skill-a",
		SkillRef: "skill://a@1.0.0", Publisher: "P", Category: "finance",
	})
	createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "skill-b",
		SkillRef: "skill://b@1.0.0", Publisher: "P", Category: "healthcare",
	})

	r := httptest.NewRequest(http.MethodGet, "/listings?category=finance", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var results []*Listing
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for category=finance, got %d", len(results))
	}
	if results[0].Category != "finance" {
		t.Errorf("expected category=finance, got %s", results[0].Category)
	}
}

func TestSearchListings_ByTags(t *testing.T) {
	srv := newTestServer()

	createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "skill-tagged",
		SkillRef: "skill://tagged@1.0.0", Publisher: "P", Category: "dev",
		Tags: []string{"nlp", "summarization"},
	})
	createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "skill-other",
		SkillRef: "skill://other@1.0.0", Publisher: "P", Category: "dev",
		Tags: []string{"vision"},
	})

	r := httptest.NewRequest(http.MethodGet, "/listings?tags=nlp", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var results []*Listing
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for tags=nlp, got %d", len(results))
	}
	if results[0].Name != "skill-tagged" {
		t.Errorf("expected skill-tagged, got %s", results[0].Name)
	}
}

func TestSearchListings_ByMinRating(t *testing.T) {
	srv := newTestServer()

	l1 := createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "high-rated",
		SkillRef: "skill://high@1.0.0", Publisher: "P", Category: "dev",
	})
	// Update rating via PUT
	rating := 4.5
	updateBody, _ := json.Marshal(UpdateListingRequest{Rating: &rating})
	r := httptest.NewRequest(http.MethodPut, "/listings/"+l1.ID, bytes.NewReader(updateBody))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "low-rated",
		SkillRef: "skill://low@1.0.0", Publisher: "P", Category: "dev",
	})

	r = httptest.NewRequest(http.MethodGet, "/listings?minRating=4.0", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var results []*Listing
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for minRating=4.0, got %d", len(results))
	}
	if results[0].Name != "high-rated" {
		t.Errorf("expected high-rated, got %s", results[0].Name)
	}
}

func TestSearchListings_FullText(t *testing.T) {
	srv := newTestServer()

	createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "code-review-bot",
		SkillRef: "skill://cr@1.0.0", Publisher: "P", Category: "dev",
		Readme: "Automated code review using AI to detect bugs",
	})
	createTestListing(t, srv, CreateListingRequest{
		TenantID: "t1", Scope: ScopeGlobal, Name: "translation-skill",
		SkillRef: "skill://translate@1.0.0", Publisher: "P", Category: "language",
		Readme: "Translate documents between languages",
	})

	r := httptest.NewRequest(http.MethodGet, "/listings?q=code+review", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var results []*Listing
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for q=code review, got %d", len(results))
	}
	if results[0].Name != "code-review-bot" {
		t.Errorf("expected code-review-bot, got %s", results[0].Name)
	}
}

func TestSearchListings_EmptyResult(t *testing.T) {
	srv := newTestServer()

	r := httptest.NewRequest(http.MethodGet, "/listings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var results []*Listing
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 0 {
		t.Errorf("expected empty list, got %d items", len(results))
	}
}

func TestHealthz(t *testing.T) {
	srv := newTestServer()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %s", w.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	r := httptest.NewRequest(http.MethodPatch, "/listings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
