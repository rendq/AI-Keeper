package webhook

import (
	"context"
	"errors"
	"fmt"
	"strings"

	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AIPSystemAnnotation is the annotation that marks a ServiceAccount as
// a "system writer" allowed to mutate `audit.ai-keeper.io` resources
// (Requirement A1.5).
const AIPSystemAnnotation = "ai-keeper.io/system"

// AIPSystemAnnotationValue is the only annotation value that grants
// system-write rights. Any other value (including the empty string) is
// rejected, so flipping the annotation back to "false" immediately
// revokes access.
const AIPSystemAnnotationValue = "true"

// serviceAccountUserPrefix is the canonical prefix K8s assigns to
// SA-issued tokens in `req.UserInfo.Username`.
const serviceAccountUserPrefix = "system:serviceaccount:"

// systemMastersGroup is K8s' built-in group that bypasses every
// authorization check. We honour it here so cluster admins can still
// rescue an audit pipeline manually if the SA annotation is missing.
const systemMastersGroup = "system:masters"

// errMissingClient is returned when SystemSAChecker.Check is invoked
// without a client. Webhook handlers panic on this because it is a
// programming error.
var errMissingClient = errors.New("webhook: system SA checker is missing a client")

// SystemSAChecker decides whether a UserInfo is "system enough" to
// CREATE/UPDATE/DELETE AuditEvent resources.
//
// Decision rules (highest precedence first):
//
//  1. If UserInfo.Username starts with `system:serviceaccount:`, look up
//     the referenced ServiceAccount and require the
//     `ai-keeper.io/system=true` annotation. This is the canonical happy
//     path for in-cluster components.
//  2. If any of UserInfo.Groups equals `system:masters`, allow. Cluster
//     admins can always perform manual interventions.
//  3. Otherwise, deny.
//
// The fall-back to `system:masters` matches K8s' authorizer behaviour
// (cluster-admin bypass) and avoids accidentally locking everyone out
// of a broken audit pipeline.
type SystemSAChecker struct {
	// Client is used to look up ServiceAccount annotations. May be nil
	// in tests that exclusively exercise the group-based fast path.
	Client client.Client
}

// NewSystemSAChecker builds a SystemSAChecker around `c`.
func NewSystemSAChecker(c client.Client) *SystemSAChecker {
	return &SystemSAChecker{Client: c}
}

// Check returns nil iff `userInfo` is allowed to write AuditEvent
// resources. The error is suitable for returning from a
// CustomValidator, where controller-runtime turns it into an admission
// "Denied" response.
func (s *SystemSAChecker) Check(ctx context.Context, userInfo authnv1.UserInfo) error {
	// Cluster admin bypass.
	for _, g := range userInfo.Groups {
		if g == systemMastersGroup {
			return nil
		}
	}

	ns, name, ok := splitServiceAccountUsername(userInfo.Username)
	if !ok {
		return fmt.Errorf("AuditEvent CREATE/UPDATE/DELETE is restricted to system ServiceAccounts annotated %s=%s; user %q is not a ServiceAccount",
			AIPSystemAnnotation, AIPSystemAnnotationValue, userInfo.Username)
	}

	if s == nil || s.Client == nil {
		return errMissingClient
	}

	sa := &corev1.ServiceAccount{}
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sa); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("AuditEvent CREATE/UPDATE/DELETE is restricted to system ServiceAccounts annotated %s=%s; ServiceAccount %s/%s not found",
				AIPSystemAnnotation, AIPSystemAnnotationValue, ns, name)
		}
		return fmt.Errorf("failed to look up ServiceAccount %s/%s: %w", ns, name, err)
	}

	if sa.Annotations[AIPSystemAnnotation] != AIPSystemAnnotationValue {
		return fmt.Errorf("AuditEvent CREATE/UPDATE/DELETE is restricted to system ServiceAccounts annotated %s=%s; ServiceAccount %s/%s lacks the annotation",
			AIPSystemAnnotation, AIPSystemAnnotationValue, ns, name)
	}
	return nil
}

// splitServiceAccountUsername parses `system:serviceaccount:<ns>:<name>`
// into its components. Returns ok=false for any other shape.
func splitServiceAccountUsername(username string) (namespace, name string, ok bool) {
	if !strings.HasPrefix(username, serviceAccountUserPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(username, serviceAccountUserPrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// userInfoFromContext pulls the admission Request out of `ctx` and
// returns its UserInfo. Returns ok=false when no request is in scope —
// the caller should treat that as a programming error and deny.
func userInfoFromContext(ctx context.Context) (authnv1.UserInfo, bool) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return authnv1.UserInfo{}, false
	}
	return req.UserInfo, true
}
