package resolver

import (
	"errors"
	"testing"

	shared "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

func TestParseConstraint_Errors(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"   ",
		">>1.0.0",
		"^abc",
		"~",
		"1.2.3.4",
		"1.0.0 - ",
		" - 2.0.0",
		"||",
		"^",
	}
	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			_, err := ParseConstraint(in)
			if err == nil {
				t.Fatalf("ParseConstraint(%q) expected error, got nil", in)
			}
			if !errors.Is(err, ErrInvalidConstraint) {
				t.Fatalf("ParseConstraint(%q) error not ErrInvalidConstraint: %v", in, err)
			}
		})
	}
}

func TestConstraint_Match(t *testing.T) {
	t.Parallel()

	type tc struct {
		name       string
		constraint string
		version    shared.SemVer
		want       bool
	}

	cases := []tc{
		// Wildcard.
		{"wildcard star matches stable", "*", "1.2.3", true},
		{"wildcard star rejects pre-release (npm)", "*", "1.0.0-rc.1", false},
		{"wildcard X matches", "X", "0.0.1", true},

		// Exact.
		{"exact match", "1.2.3", "1.2.3", true},
		{"exact mismatch patch", "1.2.3", "1.2.4", false},
		{"exact mismatch minor", "1.2.3", "1.3.3", false},

		// = operator alias.
		{"eq alias match", "=1.0.0", "1.0.0", true},
		{"eq alias miss", "=1.0.0", "1.0.1", false},

		// Caret.
		{"^1.0.0 allows 1.0.5", "^1.0.0", "1.0.5", true},
		{"^1.0.0 allows 1.99.99", "^1.0.0", "1.99.99", true},
		{"^1.0.0 rejects 2.0.0", "^1.0.0", "2.0.0", false},
		{"^1.0.0 rejects 0.9.9", "^1.0.0", "0.9.9", false},
		{"^0.2.3 allows 0.2.4", "^0.2.3", "0.2.4", true},
		{"^0.2.3 rejects 0.3.0", "^0.2.3", "0.3.0", false},
		{"^0.0.3 rejects 0.0.4", "^0.0.3", "0.0.4", false},
		{"^0.0.3 matches 0.0.3 only", "^0.0.3", "0.0.3", true},

		// Tilde.
		{"~1.2.0 allows 1.2.5", "~1.2.0", "1.2.5", true},
		{"~1.2.0 rejects 1.3.0", "~1.2.0", "1.3.0", false},
		{"~1.2 allows 1.2.5", "~1.2", "1.2.5", true},
		{"~1.2 rejects 1.3.0", "~1.2", "1.3.0", false},

		// X-range.
		{"1.x matches 1.4.5", "1.x", "1.4.5", true},
		{"1.x rejects 2.0.0", "1.x", "2.0.0", false},
		{"1.2.x matches 1.2.99", "1.2.x", "1.2.99", true},
		{"1.2.x rejects 1.3.0", "1.2.x", "1.3.0", false},
		{"1 (bare major) matches 1.5.0", "1", "1.5.0", true},
		{"1 (bare major) rejects 2.0.0", "1", "2.0.0", false},
		{"1.2 matches 1.2.5", "1.2", "1.2.5", true},
		{"1.2 rejects 1.3.0", "1.2", "1.3.0", false},

		// Whitespace conjunction.
		{">=1.0.0 <2.0.0 matches 1.5.0", ">=1.0.0 <2.0.0", "1.5.0", true},
		{">=1.0.0 <2.0.0 rejects 2.0.0", ">=1.0.0 <2.0.0", "2.0.0", false},
		{">=1.0.0 <2.0.0 rejects 0.9.0", ">=1.0.0 <2.0.0", "0.9.0", false},
		{">1.0.0 rejects 1.0.0", ">1.0.0", "1.0.0", false},
		{">1.0.0 matches 1.0.1", ">1.0.0", "1.0.1", true},
		{"<=2.0.0 matches 2.0.0", "<=2.0.0", "2.0.0", true},
		{"<=2.0.0 rejects 2.0.1", "<=2.0.0", "2.0.1", false},

		// Hyphen range.
		{"1.0.0 - 2.0.0 matches 1.5.0", "1.0.0 - 2.0.0", "1.5.0", true},
		{"1.0.0 - 2.0.0 matches 2.0.0 inclusive", "1.0.0 - 2.0.0", "2.0.0", true},
		{"1.0.0 - 2.0.0 rejects 2.0.1", "1.0.0 - 2.0.0", "2.0.1", false},
		{"1.0.0 - 2.0 expands to <2.1.0", "1.0.0 - 2.0", "2.0.5", true},
		{"1.0.0 - 2.0 rejects 2.1.0", "1.0.0 - 2.0", "2.1.0", false},
		{"1.0.0 - 2 expands to <3.0.0", "1.0.0 - 2", "2.999.0", true},
		{"1.0.0 - 2 rejects 3.0.0", "1.0.0 - 2", "3.0.0", false},

		// Pre-release gating.
		{"^1.0.0-rc.1 allows 1.0.0-rc.2", "^1.0.0-rc.1", "1.0.0-rc.2", true},
		{"^1.0.0-rc.1 rejects 1.0.0-beta.1", "^1.0.0-rc.1", "1.0.0-beta.1", false},
		{"^1.0.0-rc.1 allows 1.0.0", "^1.0.0-rc.1", "1.0.0", true},
		{"^1.0.0-rc.1 rejects 2.0.0-rc.1 (pre on different base)", "^1.0.0-rc.1", "2.0.0-rc.1", false},
		{"^1.0.0 rejects 1.0.1-alpha (pre on different patch)", "^1.0.0", "1.0.1-alpha", false},
		{"^1.0.0 rejects 1.5.0-alpha", "^1.0.0", "1.5.0-alpha", false},
		{">=1.0.0-rc.1 <2.0.0 allows 1.0.0-rc.2", ">=1.0.0-rc.1 <2.0.0", "1.0.0-rc.2", true},
		{">=1.0.0-rc.1 <2.0.0 rejects 1.5.0-alpha (different base)", ">=1.0.0-rc.1 <2.0.0", "1.5.0-alpha", false},

		// Disjunction.
		{"^1.0.0 || ^2.0.0 matches 1.5.0", "^1.0.0 || ^2.0.0", "1.5.0", true},
		{"^1.0.0 || ^2.0.0 matches 2.5.0", "^1.0.0 || ^2.0.0", "2.5.0", true},
		{"^1.0.0 || ^2.0.0 rejects 3.0.0", "^1.0.0 || ^2.0.0", "3.0.0", false},

		// Invalid candidate never matches.
		{"invalid candidate never matches", "*", "not.semver", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			cc, err := ParseConstraint(c.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q): %v", c.constraint, err)
			}
			got := cc.Match(c.version)
			if got != c.want {
				t.Fatalf("Match(%q)=%v, want %v", c.version, got, c.want)
			}
		})
	}
}

func TestConstraint_MaxSatisfying(t *testing.T) {
	t.Parallel()
	cands := []shared.SemVer{"0.9.0", "1.0.0", "1.2.3", "1.5.0", "2.0.0"}

	cases := []struct {
		constraint string
		want       shared.SemVer
	}{
		{"^1.0.0", "1.5.0"},
		{">=1.0.0 <2.0.0", "1.5.0"},
		{"^2.0.0", "2.0.0"},
		{"^3.0.0", ""},
		{"*", "2.0.0"},
		{"~1.2.0", "1.2.3"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.constraint, func(t *testing.T) {
			cc, err := ParseConstraint(c.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q): %v", c.constraint, err)
			}
			got := cc.MaxSatisfying(cands)
			if got != c.want {
				t.Fatalf("MaxSatisfying(%q)=%q, want %q", c.constraint, got, c.want)
			}
		})
	}
}

func TestConstraint_PrettyRoundTrip(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"^1.0.0", "1.0.0 - 2.0.0", ">=1 <2"} {
		c, err := ParseConstraint(in)
		if err != nil {
			t.Fatalf("ParseConstraint(%q): %v", in, err)
		}
		if got := c.Pretty(); got != in {
			t.Fatalf("Pretty()=%q, want %q", got, in)
		}
	}
}
