package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Server is the marketplace HTTP server.
type Server struct {
	store       Store
	reviewStore ReviewStore
	workflow    *PublishWorkflow
	mux         *http.ServeMux
	server      *http.Server
}

// NewServer creates a marketplace server with the given store.
func NewServer(store Store) *Server {
	s := &Server{store: store, reviewStore: NewMemoryReviewStore(), workflow: NewPublishWorkflow(store), mux: http.NewServeMux()}
	s.routes()
	return s
}

// NewServerWithReviewStore creates a marketplace server with explicit review store (for testing).
func NewServerWithReviewStore(store Store, reviewStore ReviewStore) *Server {
	s := &Server{store: store, reviewStore: reviewStore, workflow: NewPublishWorkflow(store), mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/listings", s.handleListings)
	s.mux.HandleFunc("/listings/", s.handleListingByID)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	s.server = &http.Server{Addr: addr, Handler: s.mux}
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// ServeHTTP implements http.Handler for testing.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleListings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createListing(w, r)
	case http.MethodGet:
		s.searchListings(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleListingByID(w http.ResponseWriter, r *http.Request) {
	// Extract path after /listings/
	path := strings.TrimPrefix(r.URL.Path, "/listings/")
	if path == "" {
		http.Error(w, "listing ID required", http.StatusBadRequest)
		return
	}

	// Check for sub-resource: /listings/{id}/reviews, /listings/{id}/submit, /listings/{id}/approve, /listings/{id}/reject
	if parts := strings.SplitN(path, "/", 2); len(parts) == 2 {
		id := parts[0]
		subResource := parts[1]
		switch subResource {
		case "reviews":
			switch r.Method {
			case http.MethodPost:
				s.createReview(w, r, id)
			default:
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		case "submit":
			if r.Method == http.MethodPost {
				s.submitForReview(w, r, id)
			} else {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		case "approve":
			if r.Method == http.MethodPost {
				s.approveReview(w, r, id)
			} else {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		case "reject":
			if r.Method == http.MethodPost {
				s.rejectReview(w, r, id)
			} else {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
		return
	}

	id := path
	switch r.Method {
	case http.MethodGet:
		s.getListing(w, r, id)
	case http.MethodPut:
		s.updateListing(w, r, id)
	case http.MethodDelete:
		s.deleteListing(w, r, id)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// CreateListingRequest is the request body for POST /listings.
type CreateListingRequest struct {
	TenantID  string   `json:"tenantId"`
	Scope     Scope    `json:"scope"`
	Name      string   `json:"name"`
	SkillRef  string   `json:"skillRef"`
	Publisher string   `json:"publisher"`
	Category  string   `json:"category"`
	Tags      []string `json:"tags,omitempty"`
	Readme    string   `json:"readme,omitempty"`
}

// UpdateListingRequest is the request body for PUT /listings/:id.
type UpdateListingRequest struct {
	Scope     *Scope   `json:"scope,omitempty"`
	Name      *string  `json:"name,omitempty"`
	SkillRef  *string  `json:"skillRef,omitempty"`
	Publisher *string  `json:"publisher,omitempty"`
	Category  *string  `json:"category,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Readme    *string  `json:"readme,omitempty"`
	Rating    *float64 `json:"rating,omitempty"`
	Downloads *int64   `json:"downloads,omitempty"`
}

func (s *Server) createListing(w http.ResponseWriter, r *http.Request) {
	var req CreateListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.SkillRef == "" || req.Publisher == "" || req.Category == "" {
		http.Error(w, "name, skillRef, publisher, and category are required", http.StatusBadRequest)
		return
	}

	if req.Scope == "" {
		req.Scope = ScopeTenant
	}
	if req.Scope != ScopeTenant && req.Scope != ScopeGlobal {
		http.Error(w, "scope must be 'tenant' or 'global'", http.StatusBadRequest)
		return
	}

	listing := &Listing{
		TenantID:  req.TenantID,
		Scope:     req.Scope,
		Phase:     PhaseDraft,
		Name:      req.Name,
		SkillRef:  req.SkillRef,
		Publisher: req.Publisher,
		Category:  req.Category,
		Tags:      req.Tags,
		Readme:    req.Readme,
	}

	if err := s.store.Create(r.Context(), listing); err != nil {
		log.Printf("aik-marketplace: create error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(listing)
}

func (s *Server) getListing(w http.ResponseWriter, r *http.Request, id string) {
	listing, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(listing)
}

func (s *Server) updateListing(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var req UpdateListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Scope != nil {
		if *req.Scope != ScopeTenant && *req.Scope != ScopeGlobal {
			http.Error(w, "scope must be 'tenant' or 'global'", http.StatusBadRequest)
			return
		}
		existing.Scope = *req.Scope
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.SkillRef != nil {
		existing.SkillRef = *req.SkillRef
	}
	if req.Publisher != nil {
		existing.Publisher = *req.Publisher
	}
	if req.Category != nil {
		existing.Category = *req.Category
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}
	if req.Readme != nil {
		existing.Readme = *req.Readme
	}
	if req.Rating != nil {
		existing.Rating = *req.Rating
	}
	if req.Downloads != nil {
		existing.Downloads = *req.Downloads
	}

	if err := s.store.Update(r.Context(), existing); err != nil {
		log.Printf("aik-marketplace: update error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

func (s *Server) deleteListing(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.store.Delete(r.Context(), id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) searchListings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	params := SearchParams{
		TenantID: q.Get("tenantId"),
		Category: q.Get("category"),
		Query:    q.Get("q"),
	}

	if tags := q.Get("tags"); tags != "" {
		params.Tags = strings.Split(tags, ",")
	}

	if minRating := q.Get("minRating"); minRating != "" {
		var rating float64
		if _, err := fmt.Sscanf(minRating, "%f", &rating); err == nil {
			params.MinRating = rating
		}
	}

	listings, err := s.store.Search(r.Context(), params)
	if err != nil {
		log.Printf("aik-marketplace: search error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if listings == nil {
		listings = []*Listing{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(listings)
}

func (s *Server) createReview(w http.ResponseWriter, r *http.Request, listingID string) {
	// Verify listing exists
	_, err := s.store.Get(r.Context(), listingID)
	if err != nil {
		http.Error(w, "listing not found", http.StatusNotFound)
		return
	}

	var req CreateReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate star rating
	if req.Star < 1 || req.Star > 5 {
		http.Error(w, "star must be between 1 and 5", http.StatusBadRequest)
		return
	}

	if req.TenantID == "" {
		http.Error(w, "tenantId is required", http.StatusBadRequest)
		return
	}

	// Anti-spam: same tenant can only rate once within 24 hours
	hasRecent, err := s.reviewStore.HasRecentReview(r.Context(), listingID, req.TenantID, 24*time.Hour)
	if err != nil {
		log.Printf("aik-marketplace: review check error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if hasRecent {
		http.Error(w, ErrRateLimited.Error(), http.StatusTooManyRequests)
		return
	}

	review := &Review{
		ListingID: listingID,
		TenantID:  req.TenantID,
		Star:      req.Star,
		Text:      req.Text,
		Usage:     req.Usage,
	}

	if err := s.reviewStore.CreateReview(r.Context(), review); err != nil {
		log.Printf("aik-marketplace: create review error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Update listing's average rating
	if err := s.updateListingRating(r.Context(), listingID); err != nil {
		log.Printf("aik-marketplace: update rating error: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(review)
}

func (s *Server) updateListingRating(ctx context.Context, listingID string) error {
	reviews, err := s.reviewStore.ListReviews(ctx, listingID)
	if err != nil {
		return err
	}

	listing, err := s.store.Get(ctx, listingID)
	if err != nil {
		return err
	}

	listing.Rating = computeWeightedAverage(reviews)
	return s.store.Update(ctx, listing)
}

// submitForReview handles POST /listings/:id/submit.
func (s *Server) submitForReview(w http.ResponseWriter, r *http.Request, id string) {
	result, err := s.workflow.SubmitForReview(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrInvalidTransition) {
			http.Error(w, "invalid transition: listing must be in Draft phase", http.StatusConflict)
			return
		}
		if errors.Is(err, ErrSecurityScanFail) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(result)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// approveReview handles POST /listings/:id/approve.
func (s *Server) approveReview(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.workflow.ApproveReview(r.Context(), id); err != nil {
		if errors.Is(err, ErrInvalidTransition) {
			http.Error(w, "invalid transition: listing must be in PendingReview phase", http.StatusConflict)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"published"}`))
}

// rejectReview handles POST /listings/:id/reject.
func (s *Server) rejectReview(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Allow empty body — reason is optional.
		body.Reason = ""
	}

	if err := s.workflow.RejectReview(r.Context(), id, body.Reason); err != nil {
		if errors.Is(err, ErrInvalidTransition) {
			http.Error(w, "invalid transition: listing must be in PendingReview phase", http.StatusConflict)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"rejected"}`))
}
