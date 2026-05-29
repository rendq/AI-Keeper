package v1alpha1

import "testing"

// TestSemVer_Compare exercises §11 of semver.org including the
// pre-release precedence rules that downstream resolvers depend on.
//
// Validates: Requirements A2.3.
func TestSemVer_Compare(t *testing.T) {
	cases := []struct {
		a, b SemVer
		want int
		name string
	}{
		// equality
		{"1.0.0", "1.0.0", 0, "equal-basic"},
		{"1.0.0+build.7", "1.0.0+build.99", 0, "build-metadata-ignored"},
		// major / minor / patch
		{"1.0.0", "2.0.0", -1, "major"},
		{"2.0.0", "1.0.0", 1, "major-rev"},
		{"1.0.0", "1.1.0", -1, "minor"},
		{"1.1.0", "1.0.0", 1, "minor-rev"},
		{"1.0.0", "1.0.1", -1, "patch"},
		{"1.0.1", "1.0.0", 1, "patch-rev"},
		// pre-release vs no pre-release
		{"1.0.0-alpha", "1.0.0", -1, "prerelease<release"},
		{"1.0.0", "1.0.0-alpha", 1, "release>prerelease"},
		// pre-release ordering (semver.org §11 examples)
		{"1.0.0-alpha", "1.0.0-alpha.1", -1, "alpha<alpha.1"},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1, "alpha.1<alpha.beta"},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1, "alpha.beta<beta"},
		{"1.0.0-beta", "1.0.0-beta.2", -1, "beta<beta.2"},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1, "beta.2<beta.11 (numeric)"},
		{"1.0.0-beta.11", "1.0.0-rc.1", -1, "beta.11<rc.1"},
		{"1.0.0-rc.1", "1.0.0", -1, "rc.1<release"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Compare(c.b); got != c.want {
				t.Fatalf("Compare(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestSemVer_IsValid(t *testing.T) {
	good := []SemVer{"0.0.0", "1.0.0", "1.0.0-rc.1", "1.0.0+build.1"}
	bad := []SemVer{"", "1", "01.0.0", "1.0", "v1.0.0"}
	for _, v := range good {
		if !v.IsValid() {
			t.Fatalf("expected %q to be valid", v)
		}
	}
	for _, v := range bad {
		if v.IsValid() {
			t.Fatalf("expected %q to be invalid", v)
		}
	}
}

func TestCompareSemVerStrict_PropagatesError(t *testing.T) {
	if _, err := CompareSemVerStrict("not-a-version", "1.0.0"); err == nil {
		t.Fatalf("expected error for malformed input")
	}
	if _, err := CompareSemVerStrict("1.0.0", "01.0.0"); err == nil {
		t.Fatalf("expected error for malformed second arg")
	}
}
