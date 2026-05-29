package v1alpha1

import (
	"errors"
	"fmt"
	"regexp"
)

// ResourceRefScheme is the URI scheme component of a [ResourceRef].
type ResourceRefScheme string

// Allowed schemes for [ResourceRef]. Mirrors the regex in [ResourceRef].
const (
	SchemeSkill     ResourceRefScheme = "skill"
	SchemeAgent     ResourceRefScheme = "agent"
	SchemeTool      ResourceRefScheme = "tool"
	SchemeModel     ResourceRefScheme = "model"
	SchemeData      ResourceRefScheme = "data"
	SchemePrompt    ResourceRefScheme = "prompt"
	SchemeChannel   ResourceRefScheme = "channel"
	SchemeConnector ResourceRefScheme = "connector"
	SchemeMemory    ResourceRefScheme = "memory"
	SchemeQuota     ResourceRefScheme = "quota"
	SchemeRef       ResourceRefScheme = "ref"
	SchemeSIEM      ResourceRefScheme = "siem"
	SchemePolicy    ResourceRefScheme = "policy"
)

// resourceRefSchemeRe matches the scheme alternation that opens a
// [ResourceRef]. Kept in sync with the kubebuilder pattern marker on
// [ResourceRef].
var resourceRefSchemeRe = regexp.MustCompile(`^(skill|agent|tool|model|data|prompt|channel|connector|memory|quota|ref|siem|policy)://`)

// ResourceRefRegex is the full regex that admission applies to a
// [ResourceRef] field. Exported so tests and tooling can use the exact
// same pattern that the API server enforces.
var ResourceRefRegex = regexp.MustCompile(`^(skill|agent|tool|model|data|prompt|channel|connector|memory|quota|ref|siem|policy)://[A-Za-z0-9._/\-]+(@[A-Za-z0-9._\-+]+)?$`)

// resourceRefPathRe matches the allowed character class in the path
// segment.
var resourceRefPathRe = regexp.MustCompile(`^[A-Za-z0-9._/\-]+$`)

// resourceRefVersionRe matches the optional version suffix.
var resourceRefVersionRe = regexp.MustCompile(`^[A-Za-z0-9._\-+]+$`)

// ErrInvalidResourceRef is returned when [ResourceRef.Parse] fails.
var ErrInvalidResourceRef = errors.New("invalid ResourceRef")

// Parse decodes the ResourceRef into its scheme, path, and optional
// version components. The reverse operation is [FormatResourceRef].
//
// Validates: Requirements F25 (round-trip).
func (r ResourceRef) Parse() (ResourceRefScheme, string, string, error) {
	loc := resourceRefSchemeRe.FindStringIndex(string(r))
	if loc == nil {
		return "", "", "", fmt.Errorf("%w: missing scheme in %q", ErrInvalidResourceRef, string(r))
	}
	scheme := ResourceRefScheme(r[:loc[1]-3]) // drop "://"
	rest := string(r[loc[1]:])
	if rest == "" {
		return "", "", "", fmt.Errorf("%w: empty path in %q", ErrInvalidResourceRef, string(r))
	}

	path := rest
	version := ""
	hasVersion := false
	// The version (if any) follows the rightmost '@'. Only one '@' is
	// permitted by the regex, so this is unambiguous.
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == '@' {
			path = rest[:i]
			version = rest[i+1:]
			hasVersion = true
			break
		}
	}
	if path == "" {
		return "", "", "", fmt.Errorf("%w: empty path before '@' in %q", ErrInvalidResourceRef, string(r))
	}
	if hasVersion && version == "" {
		return "", "", "", fmt.Errorf("%w: empty version after '@' in %q", ErrInvalidResourceRef, string(r))
	}
	if !resourceRefPathRe.MatchString(path) {
		return "", "", "", fmt.Errorf("%w: illegal characters in path %q", ErrInvalidResourceRef, path)
	}
	if version != "" && !resourceRefVersionRe.MatchString(version) {
		return "", "", "", fmt.Errorf("%w: illegal characters in version %q", ErrInvalidResourceRef, version)
	}
	return scheme, path, version, nil
}

// FormatResourceRef builds a [ResourceRef] from its components. The
// resulting value is guaranteed to satisfy [ResourceRefRegex] when each
// component is itself well-formed; callers receive a non-nil error
// otherwise.
//
// Validates: Requirements F25 (round-trip).
func FormatResourceRef(scheme ResourceRefScheme, path, version string) (ResourceRef, error) {
	if !resourceRefSchemeRe.MatchString(string(scheme) + "://") {
		return "", fmt.Errorf("%w: unknown scheme %q", ErrInvalidResourceRef, scheme)
	}
	if !resourceRefPathRe.MatchString(path) {
		return "", fmt.Errorf("%w: illegal characters in path %q", ErrInvalidResourceRef, path)
	}
	if version != "" && !resourceRefVersionRe.MatchString(version) {
		return "", fmt.Errorf("%w: illegal characters in version %q", ErrInvalidResourceRef, version)
	}
	out := string(scheme) + "://" + path
	if version != "" {
		out += "@" + version
	}
	return ResourceRef(out), nil
}

// IsValid reports whether the value matches [ResourceRefRegex]. Useful
// for quick checks outside of admission.
func (r ResourceRef) IsValid() bool {
	return ResourceRefRegex.MatchString(string(r))
}
