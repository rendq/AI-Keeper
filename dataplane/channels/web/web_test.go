package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleHTTPRequest_ValidPost(t *testing.T) {
	adapter := NewWebAdapter()

	body := `{"request_id":"r1","tenant_id":"t1","agent_id":"a1","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	adapter.HandleHTTPRequest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"request_id":"r1"`) {
		t.Fatalf("expected request_id in response, got %s", w.Body.String())
	}
}

func TestHandleHTTPRequest_MethodNotAllowed(t *testing.T) {
	adapter := NewWebAdapter()

	req := httptest.NewRequest(http.MethodGet, "/invoke", nil)
	w := httptest.NewRecorder()

	adapter.HandleHTTPRequest(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", w.Code)
	}
}

func TestHandleHTTPRequest_InvalidJSON(t *testing.T) {
	adapter := NewWebAdapter()

	req := httptest.NewRequest(http.MethodPost, "/invoke", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	adapter.HandleHTTPRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestHandleHTTPRequest_MissingFields(t *testing.T) {
	adapter := NewWebAdapter()

	body := `{"request_id":"r1","tenant_id":"t1"}`
	req := httptest.NewRequest(http.MethodPost, "/invoke", strings.NewReader(body))
	w := httptest.NewRecorder()

	adapter.HandleHTTPRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestParseRequest_Valid(t *testing.T) {
	body := []byte(`{"tenant_id":"t1","agent_id":"a1","input":"hello"}`)
	req, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.TenantID != "t1" {
		t.Fatalf("expected tenant_id t1, got %s", req.TenantID)
	}
	if req.AgentID != "a1" {
		t.Fatalf("expected agent_id a1, got %s", req.AgentID)
	}
}

func TestParseRequest_MissingInput(t *testing.T) {
	body := []byte(`{"tenant_id":"t1","agent_id":"a1"}`)
	_, err := ParseRequest(body)
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestHandleWebSocket_Valid(t *testing.T) {
	adapter := NewWebAdapter()

	conn := &WSConn{ID: "ws-1", TenantID: "t1"}
	if err := adapter.HandleWebSocket(conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.ActiveConnections() != 1 {
		t.Fatalf("expected 1 active connection, got %d", adapter.ActiveConnections())
	}
}

func TestHandleWebSocket_Nil(t *testing.T) {
	adapter := NewWebAdapter()

	if err := adapter.HandleWebSocket(nil); err == nil {
		t.Fatal("expected error for nil connection")
	}
}

func TestHandleWebSocket_EmptyID(t *testing.T) {
	adapter := NewWebAdapter()

	conn := &WSConn{TenantID: "t1"}
	if err := adapter.HandleWebSocket(conn); err == nil {
		t.Fatal("expected error for empty connection ID")
	}
}

func TestDisconnectWebSocket(t *testing.T) {
	adapter := NewWebAdapter()

	conn := &WSConn{ID: "ws-1", TenantID: "t1"}
	adapter.HandleWebSocket(conn)
	adapter.DisconnectWebSocket("ws-1")

	if adapter.ActiveConnections() != 0 {
		t.Fatalf("expected 0 active connections, got %d", adapter.ActiveConnections())
	}
}
