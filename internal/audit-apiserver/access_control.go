package auditapiserver

import (
	"context"
	"fmt"
	"strings"

	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// SystemAnnotation is the annotation key that marks a ServiceAccount
	// as allowed to write AuditEvent resources.
	SystemAnnotation = "ai-keeper.io/system"
	// SystemAnnotationValue is the required value.
	SystemAnnotationValue = "true"
	// saPrefix is the K8s username prefix for ServiceAccount tokens.
	saPrefix = "system:serviceaccount:"
	// mastersGroup bypasses access control (cluster-admin).
	mastersGroup = "system:masters"
)

// WriteAccessChecker enforces write restrictions on AuditEvent resources.
// Only ServiceAccounts annotated with ai-keeper.io/system=true (or cluster
// admins in system:masters group) are allowed to CREATE/UPDATE/DELETE.
type WriteAccessChecker struct {
	Client client.Reader
}

// NewWriteAccessChecker creates a WriteAccessChecker.
func NewWriteAccessChecker(c client.Reader) *WriteAccessChecker {
	return &WriteAccessChecker{Client: c}
}

// CheckWriteAccess verifies that the given user identity is allowed to
// perform write operations on AuditEvent resources. Returns nil if
// allowed, or a descriptive error if denied.
func (w *WriteAccessChecker) CheckWriteAccess(ctx context.Context, userInfo authnv1.UserInfo) error {
	// system:masters bypass.
	for _, g := range userInfo.Groups {
		if g == mastersGroup {
			return nil
		}
	}

	// Must be a ServiceAccount.
	ns, name, ok := parseServiceAccountUsername(userInfo.Username)
	if !ok {
		return fmt.Errorf(
			"AuditEvent write operations are restricted to system ServiceAccounts with annotation %s=%s; %q is not a ServiceAccount",
			SystemAnnotation, SystemAnnotationValue, userInfo.Username,
		)
	}

	if w.Client == nil {
		return fmt.Errorf("WriteAccessChecker: client not configured")
	}

	// Look up the ServiceAccount and check annotation.
	sa := &corev1.ServiceAccount{}
	if err := w.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sa); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf(
				"AuditEvent write denied: ServiceAccount %s/%s not found",
				ns, name,
			)
		}
		return fmt.Errorf("failed to look up ServiceAccount %s/%s: %w", ns, name, err)
	}

	if sa.Annotations[SystemAnnotation] != SystemAnnotationValue {
		return fmt.Errorf(
			"AuditEvent write denied: ServiceAccount %s/%s does not have annotation %s=%s",
			ns, name, SystemAnnotation, SystemAnnotationValue,
		)
	}

	return nil
}

// parseServiceAccountUsername extracts namespace and name from a K8s
// ServiceAccount username of the form "system:serviceaccount:<ns>:<name>".
func parseServiceAccountUsername(username string) (namespace, name string, ok bool) {
	if !strings.HasPrefix(username, saPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(username, saPrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
