package v1alpha1

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ErrInvalidDNS1123Subdomain is returned when [IsValidDNS1123Subdomain]
// rejects a metadata.name value.
var ErrInvalidDNS1123Subdomain = errors.New("invalid DNS-1123 subdomain")

// IsValidDNS1123Subdomain returns nil iff `name` is a legal K8s
// metadata.name (≤253 chars, RFC-1123 subdomain). Mirrors the rule
// applied by Requirements A2.4.
//
// We delegate to apimachinery's stdlib validator and wrap its detailed
// error list into a single error so callers can `errors.Is(err,
// ErrInvalidDNS1123Subdomain)`.
func IsValidDNS1123Subdomain(name string) error {
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("%w: %q: %s", ErrInvalidDNS1123Subdomain, name, strings.Join(errs, "; "))
	}
	return nil
}
