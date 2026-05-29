package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aipv1 "github.com/ai-keeper/ai-keeper/proto/aip/v1"
	"github.com/ai-keeper/ai-keeper/internal/compiler"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDecide_NoBundleLoaded_FailClosed(t *testing.T) {
	srv := NewPDPServer()

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{
			TenantId:  "tenant-a",
			UserId:    "user-1",
			AgentName: "legal-copilot",
		},
		Action: &aipv1.Action{
			Verb:         "invoke",
			ResourceKind: "Skill",
			ResourceName: "contract-review",
		},
	}

	resp, err := srv.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Decision != aipv1.Decision_DECISION_DENY {
		t.Errorf("expected DENY when no bundle loaded, got %v", resp.Decision)
	}
	if resp.Reason != "no bundle loaded" {
		t.Errorf("expected reason 'no bundle loaded', got %q", resp.Reason)
	}
}

func TestDecide_WithBundle_Allow(t *testing.T) {
	srv := NewPDPServer()

	// Create a simple bundle that allows everything.
	bundle := buildTestBundle(t, "allow")
	loadBundle(t, srv, bundle)

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{
			TenantId:  "tenant-a",
			UserId:    "user-1",
			AgentName: "legal-copilot",
		},
		Action: &aipv1.Action{
			Verb:         "invoke",
			ResourceKind: "Skill",
			ResourceName: "contract-review",
		},
	}

	resp, err := srv.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Decision != aipv1.Decision_DECISION_ALLOW {
		t.Errorf("expected ALLOW, got %v (reason: %s)", resp.Decision, resp.Reason)
	}
}

func TestDecide_WithBundle_Deny(t *testing.T) {
	srv := NewPDPServer()

	// Create a bundle that denies everything.
	bundle := buildTestBundle(t, "deny")
	loadBundle(t, srv, bundle)

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{
			TenantId:  "tenant-a",
			UserId:    "user-1",
			AgentName: "legal-copilot",
		},
		Action: &aipv1.Action{
			Verb:         "invoke",
			ResourceKind: "Skill",
			ResourceName: "contract-review",
		},
	}

	resp, err := srv.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Decision != aipv1.Decision_DECISION_DENY {
		t.Errorf("expected DENY, got %v", resp.Decision)
	}
}

func TestDecide_HigherPriorityWins(t *testing.T) {
	srv := NewPDPServer()

	// Build a bundle with two policies: high-priority allow (900) and low-priority deny (100).
	bundle := buildMultiPolicyBundle(t, []testPolicy{
		{name: "high-allow", namespace: "default", effect: "allow", priority: 900},
		{name: "low-deny", namespace: "default", effect: "deny", priority: 100},
	})
	loadBundle(t, srv, bundle)

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{TenantId: "t", UserId: "u"},
		Action:    &aipv1.Action{Verb: "invoke", ResourceKind: "Skill"},
	}

	resp, err := srv.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Decision != aipv1.Decision_DECISION_ALLOW {
		t.Errorf("expected ALLOW (higher priority), got %v (reason: %s)", resp.Decision, resp.Reason)
	}
}

func TestDecide_SamePriorityDenyWins(t *testing.T) {
	srv := NewPDPServer()

	// Build a bundle with same-priority allow and deny → deny should win.
	bundle := buildMultiPolicyBundle(t, []testPolicy{
		{name: "pol-allow", namespace: "default", effect: "allow", priority: 500},
		{name: "pol-deny", namespace: "default", effect: "deny", priority: 500},
	})
	loadBundle(t, srv, bundle)

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{TenantId: "t", UserId: "u"},
		Action:    &aipv1.Action{Verb: "invoke", ResourceKind: "Skill"},
	}

	resp, err := srv.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Decision != aipv1.Decision_DECISION_DENY {
		t.Errorf("expected DENY (same priority deny wins), got %v", resp.Decision)
	}
}

func TestHandleBundleUpload_Success(t *testing.T) {
	srv := NewPDPServer()

	bundle := buildTestBundle(t, "allow")
	req := httptest.NewRequest(http.MethodPut, "/v1/bundle", bytes.NewReader(bundle))
	w := httptest.NewRecorder()

	srv.HandleBundleUpload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result["status"])
	}
	if result["bundle_hash"] == "" {
		t.Error("expected non-empty bundle_hash")
	}
}

func TestHandleBundleUpload_EmptyBody(t *testing.T) {
	srv := NewPDPServer()

	req := httptest.NewRequest(http.MethodPut, "/v1/bundle", nil)
	w := httptest.NewRecorder()

	srv.HandleBundleUpload(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", w.Result().StatusCode)
	}
}

func TestHandleStatus(t *testing.T) {
	srv := NewPDPServer()

	// Before loading a bundle.
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	w := httptest.NewRecorder()
	srv.HandleStatus(w, req)

	var result map[string]interface{}
	json.NewDecoder(w.Result().Body).Decode(&result)

	if result["bundle_loaded"] != false {
		t.Errorf("expected bundle_loaded=false before loading")
	}

	// Load a bundle.
	bundle := buildTestBundle(t, "allow")
	loadBundle(t, srv, bundle)

	w = httptest.NewRecorder()
	srv.HandleStatus(w, req)
	json.NewDecoder(w.Result().Body).Decode(&result)

	if result["bundle_loaded"] != true {
		t.Errorf("expected bundle_loaded=true after loading")
	}
	if result["bundle_hash"] == "" {
		t.Error("expected non-empty bundle_hash after loading")
	}
}

func TestBundleHotLoad_ReplacesOldBundle(t *testing.T) {
	srv := NewPDPServer()

	// Load allow bundle.
	bundle1 := buildTestBundle(t, "allow")
	loadBundle(t, srv, bundle1)

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{TenantId: "t"},
		Action:    &aipv1.Action{Verb: "invoke"},
	}
	resp, _ := srv.Decide(context.Background(), req)
	if resp.Decision != aipv1.Decision_DECISION_ALLOW {
		t.Fatalf("expected ALLOW with first bundle, got %v", resp.Decision)
	}

	// Hot-load deny bundle.
	bundle2 := buildTestBundle(t, "deny")
	loadBundle(t, srv, bundle2)

	resp, _ = srv.Decide(context.Background(), req)
	if resp.Decision != aipv1.Decision_DECISION_DENY {
		t.Errorf("expected DENY after hot-load, got %v", resp.Decision)
	}
}

func TestPEPClient_FailClosed_Timeout(t *testing.T) {
	// Start a gRPC server that delays response beyond timeout.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	srv := grpc.NewServer()
	pdp := &slowPDP{delay: 2 * time.Second}
	aipv1.RegisterPolicyDecisionServiceServer(srv, pdp)
	go srv.Serve(lis)
	defer srv.Stop()

	// Create PEP client with very short timeout.
	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := newPEPClientFromConn(conn, 100*time.Millisecond)

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{TenantId: "t"},
		Action:    &aipv1.Action{Verb: "invoke"},
	}

	resp := client.Decide(context.Background(), req)
	if resp.Decision != aipv1.Decision_DECISION_DENY {
		t.Errorf("expected DENY on timeout, got %v", resp.Decision)
	}
	if resp.Reason != "PolicyTimeout" {
		t.Errorf("expected reason PolicyTimeout, got %q", resp.Reason)
	}
}

func TestPEPClient_Success(t *testing.T) {
	// Start a PDP server with a bundle loaded.
	pdp := NewPDPServer()
	bundle := buildTestBundle(t, "allow")
	loadBundle(t, pdp, bundle)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	grpcSrv := grpc.NewServer()
	aipv1.RegisterPolicyDecisionServiceServer(grpcSrv, pdp)
	go grpcSrv.Serve(lis)
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := newPEPClientFromConn(conn, PEPTimeout)

	req := &aipv1.DecisionRequest{
		Principal: &aipv1.Principal{TenantId: "t", UserId: "u"},
		Action:    &aipv1.Action{Verb: "invoke", ResourceKind: "Skill"},
	}

	resp := client.Decide(context.Background(), req)
	if resp.Decision != aipv1.Decision_DECISION_ALLOW {
		t.Errorf("expected ALLOW, got %v (reason: %s)", resp.Decision, resp.Reason)
	}
}

func TestDriftDetection_BundleHash(t *testing.T) {
	srv := NewPDPServer()

	bundle := buildTestBundle(t, "allow")
	loadBundle(t, srv, bundle)

	hash := srv.BundleHash()
	if hash == "" {
		t.Error("expected non-empty bundle hash")
	}
	if hash[:7] != "sha256:" {
		t.Errorf("expected sha256: prefix, got %q", hash)
	}

	// Loading the same bundle should produce the same hash.
	loadBundle(t, srv, bundle)
	if srv.BundleHash() != hash {
		t.Error("same bundle should produce same hash")
	}

	// Loading a different bundle should produce a different hash.
	bundle2 := buildTestBundle(t, "deny")
	loadBundle(t, srv, bundle2)
	if srv.BundleHash() == hash {
		t.Error("different bundle should produce different hash")
	}
}

// --- Helpers ---

// slowPDP is a PDP server that delays before responding (for timeout tests).
type slowPDP struct {
	aipv1.UnimplementedPolicyDecisionServiceServer
	delay time.Duration
}

func (s *slowPDP) Decide(ctx context.Context, req *aipv1.DecisionRequest) (*aipv1.DecisionResponse, error) {
	select {
	case <-time.After(s.delay):
		return &aipv1.DecisionResponse{Decision: aipv1.Decision_DECISION_ALLOW}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func loadBundle(t *testing.T, srv *PDPServer, bundle []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/v1/bundle", bytes.NewReader(bundle))
	w := httptest.NewRecorder()
	srv.HandleBundleUpload(w, req)
	if w.Result().StatusCode != http.StatusOK {
		body, _ := io.ReadAll(w.Result().Body)
		t.Fatalf("bundle upload failed: %s", string(body))
	}
}

// buildTestBundle creates a minimal OPA bundle using the compiler package.
func buildTestBundle(t *testing.T, effect string) []byte {
	t.Helper()

	priority := int32(500)
	enabled := true
	policies := []policyv1alpha1.Policy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: "default",
			},
			Spec: policyv1alpha1.PolicySpec{
				Effect:   effect,
				Priority: &priority,
				Enabled:  &enabled,
				Subject: policyv1alpha1.SubjectSelector{},
				Action:  policyv1alpha1.PolicyAction{},
			},
		},
	}

	c := compiler.New()
	b, err := c.Compile(context.Background(), compiler.CompileInput{
		Policies: policies,
	})
	if err != nil {
		t.Fatalf("compile bundle: %v", err)
	}

	return b.Data
}

type testPolicy struct {
	name      string
	namespace string
	effect    string
	priority  int32
}

func buildMultiPolicyBundle(t *testing.T, pols []testPolicy) []byte {
	t.Helper()

	var policies []policyv1alpha1.Policy
	for _, p := range pols {
		priority := p.priority
		enabled := true
		policies = append(policies, policyv1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.name,
				Namespace: p.namespace,
			},
			Spec: policyv1alpha1.PolicySpec{
				Effect:   p.effect,
				Priority: &priority,
				Enabled:  &enabled,
				Subject:  policyv1alpha1.SubjectSelector{},
				Action:   policyv1alpha1.PolicyAction{},
			},
		})
	}

	c := compiler.New()
	b, err := c.Compile(context.Background(), compiler.CompileInput{
		Policies: policies,
	})
	if err != nil {
		t.Fatalf("compile bundle: %v", err)
	}

	return b.Data
}
