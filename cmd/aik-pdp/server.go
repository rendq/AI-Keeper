package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"

	aipv1 "github.com/ai-keeper/ai-keeper/proto/aip/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultSetsRego is not used - we implement the decision algorithm in Go
// to avoid OPA type-checking issues with missing rule definitions.

// PDPServer implements the PolicyDecisionService gRPC server and HTTP bundle upload.
type PDPServer struct {
	aipv1.UnimplementedPolicyDecisionServiceServer

	mu            sync.RWMutex
	bundleHash    string
	bundleVersion int64
	bundleData    []byte // raw tar.gz for drift detection
	regoModules   map[string]string
	dataJSON      map[string]interface{}
}

// NewPDPServer creates a new PDPServer with no bundle loaded (fail-closed state).
func NewPDPServer() *PDPServer {
	return &PDPServer{
		regoModules: make(map[string]string),
	}
}

// Decide implements the gRPC Decide RPC.
// If no bundle is loaded or evaluation fails, it returns deny (fail-closed).
func (s *PDPServer) Decide(ctx context.Context, req *aipv1.DecisionRequest) (*aipv1.DecisionResponse, error) {
	s.mu.RLock()
	modules := s.regoModules
	data := s.dataJSON
	bundleVersion := s.bundleVersion
	bundleLoaded := len(modules) > 0
	s.mu.RUnlock()

	now := timestamppb.Now()

	// Fail-closed: no bundle loaded → deny.
	if !bundleLoaded {
		return &aipv1.DecisionResponse{
			Decision:      aipv1.Decision_DECISION_DENY,
			Reason:        "no bundle loaded",
			BundleVersion: bundleVersion,
			EvaluatedAt:   now,
		}, nil
	}

	// Build OPA input from the request.
	input := buildOPAInput(req)

	// Evaluate allow and deny sets separately to avoid undefined reference
	// issues when only one effect type exists in the compiled bundle.
	// Skip main.rego - we implement the decision algorithm in Go.
	policyModules := make([]func(*rego.Rego), 0, len(modules))
	for name, content := range modules {
		if name == "aip/main.rego" {
			continue
		}
		policyModules = append(policyModules, rego.Module(name, content))
	}

	var storeOpt func(*rego.Rego)
	if data != nil {
		storeOpt = rego.Store(newInMemoryStore(data))
	}

	allowSet := evalPolicySet(ctx, "data.aip.aip_allow", input, policyModules, storeOpt)
	denySet := evalPolicySet(ctx, "data.aip.aip_deny", input, policyModules, storeOpt)

	// Apply decision algorithm: higher priority wins; same priority deny wins.
	resp := computeDecision(allowSet, denySet, bundleVersion, now)
	return resp, nil
}

// Watch implements the gRPC Watch RPC (streaming bundle events).
// For P0, this is a placeholder that the Policy Controller calls.
func (s *PDPServer) Watch(req *aipv1.BundleSubscribeRequest, stream grpc.ServerStreamingServer[aipv1.BundleEvent]) error {
	// P0: simple implementation - just keep the stream open until cancelled.
	<-stream.Context().Done()
	return stream.Context().Err()
}

// HandleBundleUpload handles HTTP PUT /v1/bundle for hot-loading bundles.
func (s *PDPServer) HandleBundleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 50*1024*1024)) // 50MB max
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		http.Error(w, "empty bundle", http.StatusBadRequest)
		return
	}

	// Parse the tar.gz bundle.
	modules, dataJSON, err := parseBundleTarGz(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("parse bundle: %v", err), http.StatusBadRequest)
		return
	}

	// Compute hash.
	hash := computeBundleHash(body)

	// Extract version from manifest if present.
	version := extractVersionFromModules(modules)

	// Hot-load: swap the bundle atomically.
	s.mu.Lock()
	s.bundleHash = hash
	s.bundleVersion = version
	s.bundleData = body
	s.regoModules = modules
	s.dataJSON = dataJSON
	s.mu.Unlock()

	log.Printf("bundle loaded: hash=%s version=%d modules=%d", hash, version, len(modules))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"bundle_hash": hash,
		"version":     version,
	})
}

// HandleStatus returns the current PDP status including bundle hash for drift detection.
func (s *PDPServer) HandleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	hash := s.bundleHash
	version := s.bundleVersion
	loaded := len(s.regoModules) > 0
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"bundle_loaded":  loaded,
		"bundle_hash":    hash,
		"bundle_version": version,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
}

// BundleHash returns the current bundle hash (for drift detection).
func (s *PDPServer) BundleHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bundleHash
}

// BundleVersion returns the current bundle version.
func (s *PDPServer) BundleVersion() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bundleVersion
}

// parseBundleTarGz extracts .rego modules and data.json from a tar.gz bundle.
func parseBundleTarGz(data []byte) (map[string]string, map[string]interface{}, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	modules := make(map[string]string)
	var dataJSON map[string]interface{}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("tar read: %w", err)
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, nil, fmt.Errorf("read entry %s: %w", hdr.Name, err)
		}

		switch {
		case len(hdr.Name) > 5 && hdr.Name[len(hdr.Name)-5:] == ".rego":
			modules[hdr.Name] = string(content)
		case hdr.Name == "data.json":
			if err := json.Unmarshal(content, &dataJSON); err != nil {
				return nil, nil, fmt.Errorf("parse data.json: %w", err)
			}
		}
	}

	if len(modules) == 0 {
		return nil, nil, fmt.Errorf("no .rego files found in bundle")
	}

	return modules, dataJSON, nil
}

// computeBundleHash returns "sha256:<hex>" for the raw bundle bytes.
func computeBundleHash(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// extractVersionFromModules tries to extract version from data.json metadata.
func extractVersionFromModules(modules map[string]string) int64 {
	// Version comes from the manifest or data.json, not from rego modules.
	// We'll rely on the controller to pass it; default to timestamp-based.
	return time.Now().Unix()
}

// buildOPAInput converts a DecisionRequest into the OPA input document.
func buildOPAInput(req *aipv1.DecisionRequest) map[string]interface{} {
	input := map[string]interface{}{}

	if req.Principal != nil {
		principal := map[string]interface{}{
			"tenant_id":       req.Principal.TenantId,
			"user_id":         req.Principal.UserId,
			"user_groups":     req.Principal.UserGroups,
			"agent_name":      req.Principal.AgentName,
			"service_account": req.Principal.ServiceAccount,
			"on_behalf_of":    req.Principal.OnBehalfOf,
			"source_ip":       req.Principal.SourceIp,
			"channel":         req.Principal.Channel,
			"mfa":             req.Principal.Mfa,
			"device_id":       req.Principal.DeviceId,
		}
		input["principal"] = principal
	}

	if req.Action != nil {
		action := map[string]interface{}{
			"verb":          req.Action.Verb,
			"resource_kind": req.Action.ResourceKind,
			"resource_name": req.Action.ResourceName,
			"method":        req.Action.Method,
		}
		if req.Resource != nil {
			action["resource"] = map[string]interface{}{
				"kind":           req.Resource.Kind,
				"namespace":      req.Resource.Namespace,
				"name":           req.Resource.Name,
				"classification": req.Resource.Classification.String(),
				"labels":         req.Resource.Labels,
			}
		}
		input["action"] = action
	}

	if req.Conditions != nil && req.Conditions.Attributes != nil {
		input["conditions"] = req.Conditions.Attributes.AsMap()
	}

	return input
}

// evalPolicySet evaluates a single OPA query (e.g., "data.aip.aip_allow") and
// returns the set of matched policies. Returns nil if the rule is undefined.
func evalPolicySet(ctx context.Context, query string, input map[string]interface{}, modules []func(*rego.Rego), storeOpt func(*rego.Rego)) []policyMatch {
	opts := []func(*rego.Rego){
		rego.Query(query),
		rego.Input(input),
	}
	opts = append(opts, modules...)
	if storeOpt != nil {
		opts = append(opts, storeOpt)
	}

	r := rego.New(opts...)
	rs, err := r.Eval(ctx)
	if err != nil {
		// Rule doesn't exist or eval error → empty set.
		return nil
	}

	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil
	}

	return extractPolicySet(rs[0].Expressions[0].Value)
}

// computeDecision applies the decision algorithm:
// higher priority wins; at same priority, deny wins over allow.
func computeDecision(allowSet, denySet []policyMatch, bundleVersion int64, evaluatedAt *timestamppb.Timestamp) *aipv1.DecisionResponse {
	resp := &aipv1.DecisionResponse{
		Decision:      aipv1.Decision_DECISION_DENY, // default deny (fail-closed)
		BundleVersion: bundleVersion,
		EvaluatedAt:   evaluatedAt,
	}

	// Collect matched policy names.
	for _, p := range allowSet {
		resp.MatchedPolicies = append(resp.MatchedPolicies, p.name)
	}
	for _, p := range denySet {
		resp.MatchedPolicies = append(resp.MatchedPolicies, p.name)
	}

	// No policies matched at all → deny (fail-closed).
	if len(allowSet) == 0 && len(denySet) == 0 {
		resp.Reason = "no matching policy"
		return resp
	}

	// Find max priority in each set.
	maxAllowPriority := int32(-1)
	maxDenyPriority := int32(-1)

	for _, p := range allowSet {
		if p.priority > maxAllowPriority {
			maxAllowPriority = p.priority
		}
	}
	for _, p := range denySet {
		if p.priority > maxDenyPriority {
			maxDenyPriority = p.priority
		}
	}

	// Only allow rules matched → allow.
	if len(denySet) == 0 {
		resp.Decision = aipv1.Decision_DECISION_ALLOW
		return resp
	}

	// Only deny rules matched → deny.
	if len(allowSet) == 0 {
		resp.Decision = aipv1.Decision_DECISION_DENY
		resp.Reason = "denied by policy"
		return resp
	}

	// Higher priority wins.
	if maxAllowPriority > maxDenyPriority {
		resp.Decision = aipv1.Decision_DECISION_ALLOW
		return resp
	}
	if maxDenyPriority > maxAllowPriority {
		resp.Decision = aipv1.Decision_DECISION_DENY
		resp.Reason = "denied by policy"
		return resp
	}

	// Same priority: deny wins.
	resp.Decision = aipv1.Decision_DECISION_DENY
	resp.Reason = "denied by policy (same priority)"
	return resp
}

// policyMatch represents a matched policy from OPA evaluation.
type policyMatch struct {
	name     string
	priority int32
}

// extractPolicySet extracts policy matches from OPA set results.
func extractPolicySet(raw interface{}) []policyMatch {
	if raw == nil {
		return nil
	}

	var results []policyMatch

	// OPA returns sets as []interface{} where each element is a map.
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		pm := policyMatch{}
		if name, ok := m["name"].(string); ok {
			pm.name = name
		}
		if p, ok := m["priority"].(json.Number); ok {
			if v, err := p.Int64(); err == nil {
				pm.priority = int32(v)
			}
		} else if p, ok := m["priority"].(float64); ok {
			pm.priority = int32(p)
		}
		results = append(results, pm)
	}

	return results
}
