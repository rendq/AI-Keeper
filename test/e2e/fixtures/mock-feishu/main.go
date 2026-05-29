// Package main implements a mock Feishu (Lark) webhook echo service for testing.
// It handles event subscription verification and echoes back received messages.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
)

// VerificationRequest is the Feishu event subscription verification challenge.
type VerificationRequest struct {
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
	Type      string `json:"type"`
}

// EventRequest represents a generic Feishu event callback.
type EventRequest struct {
	Schema    string          `json:"schema"`
	Header    *EventHeader    `json:"header"`
	Event     json.RawMessage `json:"event"`
	Challenge string          `json:"challenge"`
	Token     string          `json:"token"`
	Type      string          `json:"type"`
}

// EventHeader is the header section of a Feishu event.
type EventHeader struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	CreateTime string `json:"create_time"`
	Token      string `json:"token"`
	AppID      string `json:"app_id"`
	TenantKey  string `json:"tenant_key"`
}

// EchoResponse wraps the echoed event for inspection.
type EchoResponse struct {
	Status  string          `json:"status"`
	EventID string          `json:"event_id,omitempty"`
	Event   json.RawMessage `json:"event,omitempty"`
}

// Store received events for later inspection.
var (
	eventsMu sync.RWMutex
	events   []json.RawMessage
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Handle URL verification challenge
	if req.Type == "url_verification" || req.Challenge != "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"challenge": req.Challenge,
		})
		return
	}

	// Store and echo the event
	eventsMu.Lock()
	events = append(events, req.Event)
	eventsMu.Unlock()

	resp := EchoResponse{
		Status: "ok",
		Event:  req.Event,
	}
	if req.Header != nil {
		resp.EventID = req.Header.EventID
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleEvents(w http.ResponseWriter, _ *http.Request) {
	eventsMu.RLock()
	defer eventsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(events),
		"events": events,
	})
}

func handleReset(w http.ResponseWriter, _ *http.Request) {
	eventsMu.Lock()
	events = nil
	eventsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
}

// handleSign computes the Feishu event signature for testing.
func handleSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Timestamp        string `json:"timestamp"`
		Nonce            string `json:"nonce"`
		EncryptKey       string `json:"encrypt_key"`
		Body             string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Feishu signature: sha256(timestamp + nonce + encrypt_key + body)
	content := req.Timestamp + req.Nonce + req.EncryptKey + req.Body
	hash := sha256.Sum256([]byte(content))
	signature := fmt.Sprintf("%x", hash)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"signature": signature})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", handleWebhook)
	mux.HandleFunc("/events", handleEvents)
	mux.HandleFunc("/reset", handleReset)
	mux.HandleFunc("/sign", handleSign)
	mux.HandleFunc("/healthz", handleHealth)

	log.Printf("mock-feishu listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
