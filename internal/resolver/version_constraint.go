package resolver

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// ErrInvalidConstraint is returned by [ParseConstraint] when the input
// does not match any of the supported npm-style range syntaxes.
var ErrInvalidConstraint = errors.New("resolver: invalid version constraint")

// Constraint matches a [shared.SemVer] against an npm-style version
// range. Supported syntaxes (mirrors node-semver's reduced grammar):
//
//   - `*`              wildcard, matches every release version
//   - `1.x`            major-pinned wildcard
//   - `1.2.x`          minor-pinned wildcard
//   - `1.0.0`          exact version
//   - `=1.0.0`         exact version (alias)
//   - `^1.0.0`         compatible-with-1, allows >=1.0.0 <2.0.0
//   - `^0.2.3`         compatible-with-0.2, allows >=0.2.3 <0.3.0
//   - `^0.0.3`         compatible-with-0.0.3, allows >=0.0.3 <0.0.4
//   - `~1.2.3`         tilde, allows >=1.2.3 <1.3.0
//   - `~1.2`           tilde, allows >=1.2.0 <1.3.0
//   - `>=1.0.0 <2.0.0` whitespace-separated AND clauses
//   - `1.0.0 - 2.0.0`  hyphenated inclusive range
//   - comparators: `>`, `>=`, `<`, `<=`, `=`
//
// Pre-release rules per npm semver:
//
//   - A pre-release version (e.g. `1.0.0-rc.1`) only satisfies a
//     constraint when at least one comparator in the same clause set
//     references the same `MAJOR.MINOR.PATCH` tuple as a pre-release.
//   - The wildcard / `*` constraint never matches pre-release
//     candidates; they would otherwise leak into stable resolutions
//     with surprising precedence.
//
// Constraints are immutable once parsed.
type Constraint struct {
	raw string
	// alts holds disjunctive alternatives. A version satisfies the
	// constraint when **at least one** alternative matches it.
	// Each alternative is itself a conjunction of comparators.
	alts [][]comparator
	// includePre records whether at least one comparator in any
	// alternative references a pre-release version. When false, the
	// constraint never matches pre-release candidates (npm semantics).
	includePre bool
}

// Match reports whether `v` satisfies the constraint. Malformed
// versions never satisfy any constraint.
func (c *Constraint) Match(v shared.SemVer) bool {
	if c == nil || !v.IsValid() {
		return false
	}
	hasPre := semverHasPrerelease(v)
	for _, conj := range c.alts {
		if conjunctionMatches(conj, v, hasPre, c.includePre) {
			return true
		}
	}
	return false
}

// Pretty returns a normalized string representation of the constraint
// (currently the parser input verbatim, after surrounding whitespace
// is trimmed). Useful for logging and Condition messages.
func (c *Constraint) Pretty() string {
	if c == nil {
		return ""
	}
	return c.raw
}

// comparator is a single semver comparator such as `>=1.0.0`.
type comparator struct {
	op      compareOp
	version shared.SemVer
	// hasPre records whether this comparator was specified with a
	// pre-release identifier; required for the npm pre-release gating.
	hasPre bool
}

type compareOp int

const (
	opEQ compareOp = iota
	opLT
	opLTE
	opGT
	opGTE
)

// xRangeRe matches `1`, `1.x`, `1.X`, `1.*`, `1.2.x`, `1.2.X`, `1.2.*`,
// `*`, etc. The numeric components must follow strict-semver rules
// (no leading zeros) so we hand-parse rather than reuse SemVerRegex.
var xRangeRe = regexp.MustCompile(`^(\*|x|X)$|^([0-9]+)(?:\.(\*|x|X|[0-9]+))?(?:\.(\*|x|X|[0-9]+))?$`)

// hyphenSepRe captures the ` - ` separator used in `1.0.0 - 2.0.0`.
var hyphenSepRe = regexp.MustCompile(`\s+-\s+`)

// comparatorPrefixRe extracts the leading operator from a comparator
// token like `>=1.0.0`.
var comparatorPrefixRe = regexp.MustCompile(`^(>=|<=|=|>|<)`)

// alternativeRe splits the raw constraint on `||` (logical OR).
var alternativeRe = regexp.MustCompile(`\s*\|\|\s*`)

// ParseConstraint compiles the supplied range expression. Returns an
// error wrapping [ErrInvalidConstraint] when the input is empty or
// unparseable.
func ParseConstraint(s string) (*Constraint, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidConstraint)
	}
	pieces := alternativeRe.Split(raw, -1)
	out := &Constraint{raw: raw}
	for _, piece := range pieces {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			return nil, fmt.Errorf("%w: empty alternative in %q", ErrInvalidConstraint, raw)
		}
		conj, err := parseConjunction(piece)
		if err != nil {
			return nil, err
		}
		out.alts = append(out.alts, conj)
		for _, c := range conj {
			if c.hasPre {
				out.includePre = true
			}
		}
	}
	return out, nil
}

// parseConjunction parses a whitespace / hyphen-separated AND chain.
func parseConjunction(s string) ([]comparator, error) {
	// Hyphen range: `A - B` ↔ `>=A <=B`.
	if loc := hyphenSepRe.FindStringIndex(s); loc != nil {
		left := strings.TrimSpace(s[:loc[0]])
		right := strings.TrimSpace(s[loc[1]:])
		lo, err := expandPartialAsLower(left)
		if err != nil {
			return nil, fmt.Errorf("%w: hyphen range lower %q: %v", ErrInvalidConstraint, left, err)
		}
		hi, err := expandPartialAsUpper(right)
		if err != nil {
			return nil, fmt.Errorf("%w: hyphen range upper %q: %v", ErrInvalidConstraint, right, err)
		}
		return []comparator{
			{op: opGTE, version: lo, hasPre: semverHasPrerelease(lo)},
			{op: hi.op, version: hi.version, hasPre: semverHasPrerelease(hi.version)},
		}, nil
	}

	// Otherwise split on whitespace and parse each token.
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("%w: empty conjunction", ErrInvalidConstraint)
	}
	out := make([]comparator, 0, len(tokens))
	for _, t := range tokens {
		cmps, err := parseToken(t)
		if err != nil {
			return nil, err
		}
		out = append(out, cmps...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no comparators in %q", ErrInvalidConstraint, s)
	}
	return out, nil
}

// parseToken parses a single token like `^1.0.0`, `>=1.0.0`, `1.x`, `*`.
func parseToken(t string) ([]comparator, error) {
	if t == "" {
		return nil, fmt.Errorf("%w: empty token", ErrInvalidConstraint)
	}
	switch {
	case t == "*" || t == "x" || t == "X":
		// Wildcard — match anything (>=0.0.0). includePre stays false
		// so pre-release versions are excluded as per npm semantics.
		return []comparator{{op: opGTE, version: "0.0.0"}}, nil
	case strings.HasPrefix(t, "^"):
		return caretRange(strings.TrimPrefix(t, "^"))
	case strings.HasPrefix(t, "~"):
		return tildeRange(strings.TrimPrefix(t, "~"))
	}
	if loc := comparatorPrefixRe.FindStringIndex(t); loc != nil {
		op := t[:loc[1]]
		rest := strings.TrimSpace(t[loc[1]:])
		v, err := parseStrictSemVer(rest)
		if err != nil {
			// Allow partial versions per npm semver: `>=1` ↔ `>=1.0.0`,
			// `<2.0` ↔ `<2.0.0`. We expand by zero-filling missing
			// components.
			major, minor, patch, pre, perr := splitPartial(rest)
			if perr != nil {
				return nil, fmt.Errorf("%w: comparator %q: %v", ErrInvalidConstraint, t, err)
			}
			v = composeSemVer(major, minor, patch, pre)
		}
		return []comparator{{op: parseOp(op), version: v, hasPre: semverHasPrerelease(v)}}, nil
	}
	// X-range or exact version.
	if isXRange(t) {
		return xRangeToComparators(t)
	}
	v, err := parseStrictSemVer(t)
	if err != nil {
		return nil, fmt.Errorf("%w: token %q: %v", ErrInvalidConstraint, t, err)
	}
	return []comparator{{op: opEQ, version: v, hasPre: semverHasPrerelease(v)}}, nil
}

// caretRange expands `^A.B.C[-pre]` per npm semver.
func caretRange(s string) ([]comparator, error) {
	major, minor, patch, pre, err := splitPartial(s)
	if err != nil {
		return nil, fmt.Errorf("%w: caret %q: %v", ErrInvalidConstraint, s, err)
	}
	lo := composeSemVer(major, minor, patch, pre)
	var hi shared.SemVer
	switch {
	case major > 0:
		hi = composeSemVer(major+1, 0, 0, "")
	case minor > 0:
		hi = composeSemVer(0, minor+1, 0, "")
	default:
		hi = composeSemVer(0, 0, patch+1, "")
	}
	return []comparator{
		{op: opGTE, version: lo, hasPre: pre != ""},
		{op: opLT, version: hi},
	}, nil
}

// tildeRange expands `~A.B[.C[-pre]]` per npm semver.
func tildeRange(s string) ([]comparator, error) {
	major, minor, patch, pre, err := splitPartial(s)
	if err != nil {
		return nil, fmt.Errorf("%w: tilde %q: %v", ErrInvalidConstraint, s, err)
	}
	lo := composeSemVer(major, minor, patch, pre)
	hi := composeSemVer(major, minor+1, 0, "")
	return []comparator{
		{op: opGTE, version: lo, hasPre: pre != ""},
		{op: opLT, version: hi},
	}, nil
}

// xRangeToComparators expands `1.x`, `1.2.x`, `*`, `1` into a comparator pair.
func xRangeToComparators(s string) ([]comparator, error) {
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return nil, fmt.Errorf("%w: x-range %q", ErrInvalidConstraint, s)
	}
	majorStr := parts[0]
	if isWildcard(majorStr) {
		return []comparator{{op: opGTE, version: "0.0.0"}}, nil
	}
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return nil, fmt.Errorf("%w: x-range major %q: %v", ErrInvalidConstraint, s, err)
	}
	if err := validateNumeric(majorStr); err != nil {
		return nil, fmt.Errorf("%w: x-range %q: %v", ErrInvalidConstraint, s, err)
	}
	if len(parts) == 1 {
		// `1` ↔ `>=1.0.0 <2.0.0`.
		return []comparator{
			{op: opGTE, version: composeSemVer(major, 0, 0, "")},
			{op: opLT, version: composeSemVer(major+1, 0, 0, "")},
		}, nil
	}
	minorStr := parts[1]
	if isWildcard(minorStr) {
		return []comparator{
			{op: opGTE, version: composeSemVer(major, 0, 0, "")},
			{op: opLT, version: composeSemVer(major+1, 0, 0, "")},
		}, nil
	}
	if err := validateNumeric(minorStr); err != nil {
		return nil, fmt.Errorf("%w: x-range %q: %v", ErrInvalidConstraint, s, err)
	}
	minor, _ := strconv.Atoi(minorStr)
	if len(parts) == 2 {
		return []comparator{
			{op: opGTE, version: composeSemVer(major, minor, 0, "")},
			{op: opLT, version: composeSemVer(major, minor+1, 0, "")},
		}, nil
	}
	patchStr := parts[2]
	if !isWildcard(patchStr) {
		// Bare `1.2.3` is handled by parseToken as exact match before
		// reaching x-range. If we hit this path with a numeric patch
		// the token is malformed.
		return nil, fmt.Errorf("%w: x-range %q must end in *.x.X to be a wildcard", ErrInvalidConstraint, s)
	}
	return []comparator{
		{op: opGTE, version: composeSemVer(major, minor, 0, "")},
		{op: opLT, version: composeSemVer(major, minor+1, 0, "")},
	}, nil
}

// expandedHyphen is the upper bound of a hyphen range.
type expandedHyphen struct {
	op      compareOp
	version shared.SemVer
}

// expandPartialAsLower converts a partial version into its lower-bound
// equivalent (`1.2` → `1.2.0`).
func expandPartialAsLower(s string) (shared.SemVer, error) {
	major, minor, patch, pre, err := splitPartial(s)
	if err != nil {
		return "", err
	}
	return composeSemVer(major, minor, patch, pre), nil
}

// expandPartialAsUpper converts a partial version into the inclusive
// upper bound semantics used by hyphen ranges:
//
//	`1.0.0 - 2.0.0`   → `>=1.0.0 <=2.0.0`
//	`1.0.0 - 2.x`     → `>=1.0.0 <3.0.0`  (npm semantics)
//	`1.0.0 - 2`       → `>=1.0.0 <3.0.0`
func expandPartialAsUpper(s string) (expandedHyphen, error) {
	parts := strings.Split(s, ".")
	switch len(parts) {
	case 1:
		// `2` → `<3.0.0`.
		major, err := strconv.Atoi(parts[0])
		if err != nil {
			return expandedHyphen{}, err
		}
		return expandedHyphen{op: opLT, version: composeSemVer(major+1, 0, 0, "")}, nil
	case 2:
		// `2.0` → `<2.1.0` per npm.
		major, err := strconv.Atoi(parts[0])
		if err != nil {
			return expandedHyphen{}, err
		}
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			return expandedHyphen{}, err
		}
		return expandedHyphen{op: opLT, version: composeSemVer(major, minor+1, 0, "")}, nil
	case 3:
		v, err := parseStrictSemVer(s)
		if err != nil {
			return expandedHyphen{}, err
		}
		return expandedHyphen{op: opLTE, version: v}, nil
	default:
		return expandedHyphen{}, fmt.Errorf("invalid partial %q", s)
	}
}

// splitPartial decomposes `A`, `A.B`, `A.B.C`, `A.B.C-pre` into ints +
// pre-release. Missing components default to zero.
func splitPartial(s string) (major, minor, patch int, pre string, err error) {
	main := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		main = s[:i]
		pre = s[i+1:]
		if pre == "" {
			err = fmt.Errorf("empty pre-release in %q", s)
			return
		}
	}
	parts := strings.Split(main, ".")
	if len(parts) == 0 || len(parts) > 3 {
		err = fmt.Errorf("invalid partial %q", s)
		return
	}
	if err = validateNumeric(parts[0]); err != nil {
		return
	}
	major, _ = strconv.Atoi(parts[0])
	if len(parts) >= 2 {
		if err = validateNumeric(parts[1]); err != nil {
			return
		}
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		if err = validateNumeric(parts[2]); err != nil {
			return
		}
		patch, _ = strconv.Atoi(parts[2])
	}
	return
}

// validateNumeric rejects empty strings and leading-zero numbers (e.g.
// `01`) so the parser stays aligned with strict semver rules.
func validateNumeric(s string) error {
	if s == "" {
		return fmt.Errorf("empty numeric component")
	}
	if len(s) > 1 && s[0] == '0' {
		return fmt.Errorf("leading zero in %q", s)
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return fmt.Errorf("non-numeric in %q", s)
		}
	}
	return nil
}

// composeSemVer assembles a strict-semver string from its components.
func composeSemVer(major, minor, patch int, pre string) shared.SemVer {
	out := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	if pre != "" {
		out += "-" + pre
	}
	return shared.SemVer(out)
}

// parseStrictSemVer enforces strict semver and returns a [shared.SemVer]
// that is guaranteed to satisfy [shared.SemVerRegex].
func parseStrictSemVer(s string) (shared.SemVer, error) {
	v := shared.SemVer(s)
	if !v.IsValid() {
		return "", fmt.Errorf("not a strict semver: %q", s)
	}
	return v, nil
}

func parseOp(s string) compareOp {
	switch s {
	case "<":
		return opLT
	case "<=":
		return opLTE
	case ">":
		return opGT
	case ">=":
		return opGTE
	default:
		return opEQ
	}
}

func isWildcard(s string) bool {
	return s == "*" || s == "x" || s == "X"
}

func isXRange(s string) bool {
	if s == "" {
		return false
	}
	if s == "*" || s == "x" || s == "X" {
		return true
	}
	parts := strings.Split(s, ".")
	for _, p := range parts {
		if isWildcard(p) {
			return true
		}
	}
	// Bare `1` or `1.2` are also x-ranges.
	if len(parts) < 3 && xRangeRe.MatchString(s) {
		return true
	}
	return false
}

// semverHasPrerelease reports whether the version carries a
// pre-release segment.
func semverHasPrerelease(v shared.SemVer) bool {
	return strings.Contains(string(v), "-")
}

// conjunctionMatches reports whether `v` satisfies every comparator in
// `conj`. Pre-release gating per npm semver: a pre-release candidate is
// only allowed to satisfy the conjunction when at least one comparator
// shares the candidate's `MAJOR.MINOR.PATCH` tuple AND that comparator
// carries a pre-release segment of its own. The `includePre` argument
// is supplied for symmetry with the conjunction-builder code path; the
// comparator scan below subsumes it.
func conjunctionMatches(conj []comparator, v shared.SemVer, hasPre, includePre bool) bool {
	_ = includePre
	if hasPre && !preReleaseAllowed(conj, v) {
		return false
	}
	for _, c := range conj {
		if !comparatorMatches(c, v) {
			return false
		}
	}
	return true
}

// preReleaseAllowed implements npm's rule: a pre-release version
// satisfies a constraint only when at least one comparator in the
// conjunction was specified with a pre-release segment that shares the
// same `MAJOR.MINOR.PATCH` tuple.
func preReleaseAllowed(conj []comparator, v shared.SemVer) bool {
	vBase := majorMinorPatch(v)
	for _, c := range conj {
		if !c.hasPre {
			continue
		}
		if majorMinorPatch(c.version) == vBase {
			return true
		}
	}
	return false
}

// majorMinorPatch returns the first three numeric segments as a
// canonical "M.m.p" string, dropping any pre-release / build metadata.
func majorMinorPatch(v shared.SemVer) string {
	s := string(v)
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		return s[:i]
	}
	return s
}

// comparatorMatches applies one comparator against a candidate.
func comparatorMatches(c comparator, v shared.SemVer) bool {
	cmp, err := shared.CompareSemVerStrict(v, c.version)
	if err != nil {
		return false
	}
	switch c.op {
	case opEQ:
		return cmp == 0
	case opLT:
		return cmp < 0
	case opLTE:
		return cmp <= 0
	case opGT:
		return cmp > 0
	case opGTE:
		return cmp >= 0
	}
	return false
}

// MaxSatisfying returns the highest version in `candidates` that
// satisfies the constraint, or the empty string when none match. Ties
// fall to the strict-semver ordering provided by [shared.SemVer.Compare].
//
// Validates: Requirement F12 (deterministic version selection).
func (c *Constraint) MaxSatisfying(candidates []shared.SemVer) shared.SemVer {
	if c == nil || len(candidates) == 0 {
		return ""
	}
	var best shared.SemVer
	for _, v := range candidates {
		if !c.Match(v) {
			continue
		}
		if best == "" || v.Compare(best) > 0 {
			best = v
		}
	}
	return best
}
