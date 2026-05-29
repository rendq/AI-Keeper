package v1alpha1

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SemVerRegex is the strict semver regex from
// https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
// (group names dropped for compatibility with K8s OpenAPI).
var SemVerRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// ErrInvalidSemVer is returned when [SemVer.Compare] cannot parse one of
// its inputs.
var ErrInvalidSemVer = errors.New("invalid SemVer")

// IsValid reports whether the value matches [SemVerRegex].
func (s SemVer) IsValid() bool {
	return SemVerRegex.MatchString(string(s))
}

// parsedSemVer holds the destructured form of a [SemVer].
type parsedSemVer struct {
	major, minor, patch int64
	preRelease          []string // empty when no pre-release segment
}

func parseSemVer(s SemVer) (parsedSemVer, error) {
	m := SemVerRegex.FindStringSubmatch(string(s))
	if m == nil {
		return parsedSemVer{}, fmt.Errorf("%w: %q", ErrInvalidSemVer, string(s))
	}
	major, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return parsedSemVer{}, fmt.Errorf("%w: major %q: %v", ErrInvalidSemVer, m[1], err)
	}
	minor, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return parsedSemVer{}, fmt.Errorf("%w: minor %q: %v", ErrInvalidSemVer, m[2], err)
	}
	patch, err := strconv.ParseInt(m[3], 10, 64)
	if err != nil {
		return parsedSemVer{}, fmt.Errorf("%w: patch %q: %v", ErrInvalidSemVer, m[3], err)
	}
	out := parsedSemVer{major: major, minor: minor, patch: patch}
	if m[4] != "" {
		out.preRelease = strings.Split(m[4], ".")
	}
	// Build metadata (m[5]) is ignored for ordering per semver §10.
	return out, nil
}

// Compare orders two [SemVer] values per https://semver.org §11. The
// return value is the conventional -1 / 0 / +1 triplet used by sort.
// Build metadata is ignored. If either input is malformed Compare returns
// 0 — production callers should test [SemVer.IsValid] first when a
// distinguishable error is required.
func (s SemVer) Compare(other SemVer) int {
	a, errA := parseSemVer(s)
	b, errB := parseSemVer(other)
	if errA != nil || errB != nil {
		return 0
	}
	if c := compareInt64(a.major, b.major); c != 0 {
		return c
	}
	if c := compareInt64(a.minor, b.minor); c != 0 {
		return c
	}
	if c := compareInt64(a.patch, b.patch); c != 0 {
		return c
	}
	return comparePreRelease(a.preRelease, b.preRelease)
}

// CompareSemVerStrict is like [SemVer.Compare] but propagates parse
// errors instead of swallowing them. Useful when correctness is required
// (e.g. resolver code paths that already validated input).
func CompareSemVerStrict(a, b SemVer) (int, error) {
	pa, err := parseSemVer(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseSemVer(b)
	if err != nil {
		return 0, err
	}
	if c := compareInt64(pa.major, pb.major); c != 0 {
		return c, nil
	}
	if c := compareInt64(pa.minor, pb.minor); c != 0 {
		return c, nil
	}
	if c := compareInt64(pa.patch, pb.patch); c != 0 {
		return c, nil
	}
	return comparePreRelease(pa.preRelease, pb.preRelease), nil
}

func compareInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// comparePreRelease implements the §11.4 precedence rules:
// no pre-release > any pre-release; identifiers are compared id-by-id;
// numeric < non-numeric; longer chain wins when prefixes are equal.
func comparePreRelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := comparePreReleaseIdent(a[i], b[i]); c != 0 {
			return c
		}
	}
	return compareInt64(int64(len(a)), int64(len(b)))
}

func comparePreReleaseIdent(a, b string) int {
	an, aErr := strconv.ParseInt(a, 10, 64)
	bn, bErr := strconv.ParseInt(b, 10, 64)
	aIsNum := aErr == nil && !strings.HasPrefix(a, "-")
	bIsNum := bErr == nil && !strings.HasPrefix(b, "-")
	switch {
	case aIsNum && bIsNum:
		return compareInt64(an, bn)
	case aIsNum:
		return -1
	case bIsNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
