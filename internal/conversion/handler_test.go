package conversion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
)

// minimalSkill / minimalAgent / minimalPolicy mirror the fixtures
// used by `internal/webhook` so the conversion tests exercise real
// AIP types end-to-end.
func minimalSkill() *skillv1alpha1.Skill {
	return &skillv1alpha1.Skill{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "skill.ai-keeper.io/v1alpha1",
			Kind:       "Skill",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "contract-review", Namespace: "default"},
		Spec: skillv1alpha1.SkillSpec{
			Version:   shared.SemVer("1.0.0"),
			Stability: shared.StageBeta,
			Implementation: skillv1alpha1.SkillImplementation{
				Type: "function",
			},
		},
	}
}

func minimalAgent() *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "agent.ai-keeper.io/v1alpha1",
			Kind:       "Agent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "legal-copilot", Namespace: "default"},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: "Legal Copilot",
			Identity:    agentv1alpha1.AgentIdentity{ServiceAccount: "legal-bot"},
			Skills: []agentv1alpha1.AgentSkillBinding{
				{Ref: shared.ResourceRef("skill://contract-review")},
			},
			Runtime: agentv1alpha1.AgentRuntime{Pattern: "tool_calling"},
		},
	}
}

func minimalPolicy() *policyv1alpha1.Policy {
	pri := int32(100)
	return &policyv1alpha1.Policy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.ai-keeper.io/v1alpha1",
			Kind:       "Policy",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "legal-acl", Namespace: "default"},
		Spec: policyv1alpha1.PolicySpec{
			Effect:   "allow",
			Priority: &pri,
			Subject: policyv1alpha1.SubjectSelector{
				AnyOf: []policyv1alpha1.SubjectEntry{{Kind: "Agent"}},
			},
			Action: policyv1alpha1.PolicyAction{
				Verbs: []string{"invoke"},
				Resources: policyv1alpha1.PolicyActionResources{
					AnyOf: []policyv1alpha1.ResourceSelector{{Kind: "Skill"}},
				},
			},
		},
	}
}

// rawExtensionFor JSON-encodes an arbitrary value into the
// runtime.RawExtension form the API server uses on the wire.
func rawExtensionFor(t *testing.T, v interface{}) runtime.RawExtension {
	t.Helper()
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return runtime.RawExtension{Raw: buf}
}

// TestEchoIdentity feeds three random AIP CRs (Skill / Agent /
// Policy) through the handler with `desiredAPIVersion=skill.ai-keeper.io/v1alpha1`
// and asserts the bytes come back unchanged.
//
// While the API server actually expects every object in the request
// to share the same group as the desiredAPIVersion (the ConversionReview
// is per-CRD), the echo handler is group-agnostic — it only checks the
// desired version is one P0 ships. We exploit that to assert in a
// single test that the byte sequence is preserved across all served
// Kinds.
//
// Validates: Requirements A11.1, A11.2 (echo identity P0).
func TestEchoIdentity(t *testing.T) {
	t.Parallel()
	h := NewHandler()
	objects := []runtime.RawExtension{
		rawExtensionFor(t, minimalSkill()),
		rawExtensionFor(t, minimalAgent()),
		rawExtensionFor(t, minimalPolicy()),
	}
	uid := types.UID("test-uid-echo-identity")
	in := &apiextensionsv1.ConversionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "ConversionReview",
		},
		Request: &apiextensionsv1.ConversionRequest{
			UID:               uid,
			DesiredAPIVersion: "skill.ai-keeper.io/v1alpha1",
			Objects:           objects,
		},
	}

	out := h.Convert(in)

	if out.Response == nil {
		t.Fatalf("response is nil")
	}
	if out.Response.UID != uid {
		t.Fatalf("uid not echoed: want %q got %q", uid, out.Response.UID)
	}
	if out.Response.Result.Status != metav1.StatusSuccess {
		t.Fatalf("expected Success, got status=%q msg=%q",
			out.Response.Result.Status, out.Response.Result.Message)
	}
	if got, want := len(out.Response.ConvertedObjects), len(objects); got != want {
		t.Fatalf("converted objects count: got %d want %d", got, want)
	}
	for i, in := range objects {
		got := out.Response.ConvertedObjects[i]
		if !bytes.Equal(in.Raw, got.Raw) {
			t.Fatalf("object[%d] bytes differ:\nin  = %s\nout = %s",
				i, string(in.Raw), string(got.Raw))
		}
	}
}

// TestUnknownTargetReturnsFailed feeds a desiredAPIVersion that does
// not yet exist (v1beta1 is reserved for P1). The handler must return
// status=Failure with a human-readable message that documents the
// future P1 wiring.
//
// Validates: Requirements A11.1, A11.2 (placeholder).
func TestUnknownTargetReturnsFailed(t *testing.T) {
	t.Parallel()
	h := NewHandler()
	in := &apiextensionsv1.ConversionReview{
		Request: &apiextensionsv1.ConversionRequest{
			UID:               "test-uid-unknown-target",
			DesiredAPIVersion: "skill.ai-keeper.io/v1beta1",
			Objects: []runtime.RawExtension{
				rawExtensionFor(t, minimalSkill()),
			},
		},
	}

	out := h.Convert(in)
	if out.Response == nil {
		t.Fatalf("response is nil")
	}
	if out.Response.Result.Status != metav1.StatusFailure {
		t.Fatalf("expected Failure, got %q", out.Response.Result.Status)
	}
	if !strings.Contains(out.Response.Result.Message, "v1beta1") {
		t.Fatalf("expected message to mention v1beta1, got %q",
			out.Response.Result.Message)
	}
	if !strings.Contains(out.Response.Result.Message, "not yet supported") {
		t.Fatalf("expected message to mention 'not yet supported', got %q",
			out.Response.Result.Message)
	}
	if len(out.Response.ConvertedObjects) != 0 {
		t.Fatalf("expected no converted objects on failure, got %d",
			len(out.Response.ConvertedObjects))
	}
}

// TestServeHTTP exercises the JSON wire format end-to-end via a
// httptest.Server, confirming the handler can be mounted at `/convert`
// under controller-runtime's webhook server without further glue.
func TestServeHTTP(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(NewHandler())
	t.Cleanup(srv.Close)

	in := &apiextensionsv1.ConversionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "ConversionReview",
		},
		Request: &apiextensionsv1.ConversionRequest{
			UID:               "http-test",
			DesiredAPIVersion: "skill.ai-keeper.io/v1alpha1",
			Objects:           []runtime.RawExtension{rawExtensionFor(t, minimalSkill())},
		},
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL+ConvertPath, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	out := &apiextensionsv1.ConversionReview{}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Response == nil || out.Response.Result.Status != metav1.StatusSuccess {
		t.Fatalf("expected success, got %+v", out.Response)
	}
	if len(out.Response.ConvertedObjects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(out.Response.ConvertedObjects))
	}
}

// TestServeHTTPMethodNotAllowed asserts non-POST requests are rejected
// without invoking Convert (defence-in-depth against probe traffic).
func TestServeHTTPMethodNotAllowed(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ConvertPath, nil)
	NewHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// TestNilRequest defends against a malformed ConversionReview with no
// `request` field (e.g. accidental smoke probe). The handler must
// return a Failure status, never panic.
func TestNilRequest(t *testing.T) {
	t.Parallel()
	out := NewHandler().Convert(&apiextensionsv1.ConversionReview{})
	if out.Response == nil {
		t.Fatalf("response is nil")
	}
	if out.Response.Result.Status != metav1.StatusFailure {
		t.Fatalf("expected Failure, got %q", out.Response.Result.Status)
	}
}
