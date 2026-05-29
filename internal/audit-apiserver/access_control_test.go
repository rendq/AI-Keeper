package auditapiserver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWriteAccessChecker_SystemMastersAllowed(t *testing.T) {
	checker := &WriteAccessChecker{}
	err := checker.CheckWriteAccess(context.Background(), authnv1.UserInfo{
		Username: "admin",
		Groups:   []string{"system:masters"},
	})
	if err != nil {
		t.Errorf("system:masters should be allowed, got error: %v", err)
	}
}

func TestWriteAccessChecker_NonSADenied(t *testing.T) {
	checker := &WriteAccessChecker{}
	err := checker.CheckWriteAccess(context.Background(), authnv1.UserInfo{
		Username: "user@example.com",
		Groups:   []string{"developers"},
	})
	if err == nil {
		t.Error("non-SA user should be denied")
	}
}

func TestWriteAccessChecker_SAWithAnnotationAllowed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audit-sink",
			Namespace: "aik-system",
			Annotations: map[string]string{
				"ai-keeper.io/system": "true",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sa).
		Build()

	checker := NewWriteAccessChecker(client)
	err := checker.CheckWriteAccess(context.Background(), authnv1.UserInfo{
		Username: "system:serviceaccount:aik-system:audit-sink",
	})
	if err != nil {
		t.Errorf("SA with annotation should be allowed, got: %v", err)
	}
}

func TestWriteAccessChecker_SAWithoutAnnotationDenied(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "regular-sa",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sa).
		Build()

	checker := NewWriteAccessChecker(client)
	err := checker.CheckWriteAccess(context.Background(), authnv1.UserInfo{
		Username: "system:serviceaccount:default:regular-sa",
	})
	if err == nil {
		t.Error("SA without annotation should be denied")
	}
}

func TestWriteAccessChecker_SANotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	checker := NewWriteAccessChecker(client)
	err := checker.CheckWriteAccess(context.Background(), authnv1.UserInfo{
		Username: "system:serviceaccount:ghost-ns:ghost-sa",
	})
	if err == nil {
		t.Error("nonexistent SA should be denied")
	}
}

func TestWriteAccessChecker_SAWithWrongAnnotationValue(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "almost-system",
			Namespace: "aik-system",
			Annotations: map[string]string{
				"ai-keeper.io/system": "false",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sa).
		Build()

	checker := NewWriteAccessChecker(client)
	err := checker.CheckWriteAccess(context.Background(), authnv1.UserInfo{
		Username: "system:serviceaccount:aik-system:almost-system",
	})
	if err == nil {
		t.Error("SA with annotation=false should be denied")
	}
}

func TestParseServiceAccountUsername(t *testing.T) {
	tests := []struct {
		username  string
		wantNS    string
		wantName  string
		wantOK    bool
	}{
		{"system:serviceaccount:default:my-sa", "default", "my-sa", true},
		{"system:serviceaccount:aik-system:audit-sink", "aik-system", "audit-sink", true},
		{"user@example.com", "", "", false},
		{"system:serviceaccount:", "", "", false},
		{"system:serviceaccount:ns:", "", "", false},
		{"system:serviceaccount::name", "", "", false},
	}

	for _, tt := range tests {
		ns, name, ok := parseServiceAccountUsername(tt.username)
		if ok != tt.wantOK || ns != tt.wantNS || name != tt.wantName {
			t.Errorf("parseServiceAccountUsername(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.username, ns, name, ok, tt.wantNS, tt.wantName, tt.wantOK)
		}
	}
}
