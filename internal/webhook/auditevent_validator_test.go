package webhook

import (
	"context"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	auditv1alpha1 "github.com/ai-keeper/ai-keeper/api/audit/v1alpha1"
)

// systemSA returns a ServiceAccount object with the system annotation
// set to the given value (use "true" for happy path, anything else for
// negative tests).
func systemSA(name, namespace, val string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{AIPSystemAnnotation: val},
		},
	}
}

// auditCtx returns a context that carries an admission.Request whose
// UserInfo equals `u` — matches the way controller-runtime invokes a
// CustomValidator at runtime.
func auditCtx(u authnv1.UserInfo) context.Context {
	return admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: u},
	})
}

// minimalAuditEvent constructs a structurally valid AuditEvent.
func minimalAuditEvent() *auditv1alpha1.AuditEvent {
	return &auditv1alpha1.AuditEvent{
		ObjectMeta: metav1.ObjectMeta{Name: "audit-1", Namespace: "default"},
		Spec: auditv1alpha1.AuditEventSpec{
			InvocationID: "11111111-2222-3333-4444-555555555555",
			Timestamp:    metav1.Now(),
			Principal: auditv1alpha1.AuditPrincipal{
				Agent: auditv1alpha1.AuditPrincipalAgent{Name: "legal-copilot"},
			},
			Action: auditv1alpha1.AuditAction{
				Verb:     "invoke",
				Resource: "skill://contract-review@1.0.0",
			},
		},
	}
}

// TestAuditEventValidator_SystemSA covers Requirement A1.5: the
// AuditEvent admission webhook must allow only ServiceAccounts that
// carry the `ai-keeper.io/system=true` annotation.
//
// Validates: Requirements A1.5.
func TestAuditEventValidator_SystemSA(t *testing.T) {
	t.Parallel()
	scheme := NewScheme()

	t.Run("system SA allowed on CREATE", func(t *testing.T) {
		t.Parallel()
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(systemSA("aik-audit", "aik-system", "true")).Build()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(c)}
		ctx := auditCtx(authnv1.UserInfo{Username: "system:serviceaccount:aik-system:aik-audit"})
		if _, err := v.ValidateCreate(ctx, minimalAuditEvent()); err != nil {
			t.Fatalf("expected allow, got %v", err)
		}
	})

	t.Run("missing annotation denied", func(t *testing.T) {
		t.Parallel()
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(systemSA("aik-audit", "aik-system", "false")).Build()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(c)}
		ctx := auditCtx(authnv1.UserInfo{Username: "system:serviceaccount:aik-system:aik-audit"})
		_, err := v.ValidateCreate(ctx, minimalAuditEvent())
		if err == nil || !strings.Contains(err.Error(), "lacks the annotation") {
			t.Fatalf("expected annotation rejection, got %v", err)
		}
	})

	t.Run("non-existent SA denied", func(t *testing.T) {
		t.Parallel()
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(c)}
		ctx := auditCtx(authnv1.UserInfo{Username: "system:serviceaccount:aik-system:ghost"})
		_, err := v.ValidateCreate(ctx, minimalAuditEvent())
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found rejection, got %v", err)
		}
	})

	t.Run("regular user denied", func(t *testing.T) {
		t.Parallel()
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(c)}
		ctx := auditCtx(authnv1.UserInfo{Username: "alice@example.com"})
		_, err := v.ValidateCreate(ctx, minimalAuditEvent())
		if err == nil || !strings.Contains(err.Error(), "is not a ServiceAccount") {
			t.Fatalf("expected non-SA rejection, got %v", err)
		}
	})

	t.Run("system:masters bypass", func(t *testing.T) {
		t.Parallel()
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(c)}
		ctx := auditCtx(authnv1.UserInfo{
			Username: "kubernetes-admin",
			Groups:   []string{"system:masters"},
		})
		if _, err := v.ValidateCreate(ctx, minimalAuditEvent()); err != nil {
			t.Fatalf("expected system:masters bypass to succeed, got %v", err)
		}
	})

	t.Run("UPDATE also gated", func(t *testing.T) {
		t.Parallel()
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(c)}
		ctx := auditCtx(authnv1.UserInfo{Username: "alice@example.com"})
		_, err := v.ValidateUpdate(ctx, minimalAuditEvent(), minimalAuditEvent())
		if err == nil || !strings.Contains(err.Error(), "is not a ServiceAccount") {
			t.Fatalf("expected UPDATE rejection, got %v", err)
		}
	})

	t.Run("DELETE also gated", func(t *testing.T) {
		t.Parallel()
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(c)}
		ctx := auditCtx(authnv1.UserInfo{Username: "alice@example.com"})
		_, err := v.ValidateDelete(ctx, minimalAuditEvent())
		if err == nil || !strings.Contains(err.Error(), "is not a ServiceAccount") {
			t.Fatalf("expected DELETE rejection, got %v", err)
		}
	})

	t.Run("missing context returns error", func(t *testing.T) {
		t.Parallel()
		v := &AuditEventValidator{SystemSAChecker: NewSystemSAChecker(nil)}
		_, err := v.ValidateCreate(context.Background(), minimalAuditEvent())
		if err == nil {
			t.Fatalf("expected error when admission context missing")
		}
	})
}

// TestSplitServiceAccountUsername sanity-checks the helper used by the
// SystemSAChecker.
func TestSplitServiceAccountUsername(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    string
		ns, sa   string
		expectOk bool
	}{
		{"happy path", "system:serviceaccount:aik-system:aik-audit", "aik-system", "aik-audit", true},
		{"non-SA prefix", "alice@example.com", "", "", false},
		{"missing namespace", "system:serviceaccount::aik-audit", "", "", false},
		{"missing name", "system:serviceaccount:aik-system:", "", "", false},
		{"missing colon", "system:serviceaccount:aik-system", "", "", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ns, name, ok := splitServiceAccountUsername(tc.input)
			if ok != tc.expectOk || ns != tc.ns || name != tc.sa {
				t.Fatalf("got (%q,%q,%v), want (%q,%q,%v)", ns, name, ok, tc.ns, tc.sa, tc.expectOk)
			}
		})
	}
}
