package v1alpha1

import "testing"

// TestRegex covers every regex referenced by Requirements A2.1 — A2.6.
// Validates: Requirements A2.1, A2.2, A2.3, A2.5, A2.6.
func TestRegex(t *testing.T) {
	t.Run("ResourceRef", func(t *testing.T) {
		cases := []struct {
			in   string
			ok   bool
			name string
		}{
			// positive
			{"skill://contract-review@1.2.0", true, "skill with version"},
			{"agent://legal-bot", true, "agent without version"},
			{"tool://docusign/create-envelope@v3", true, "tool with path"},
			{"model://gpt-4o-eu@2024-05-13", true, "model with date version"},
			{"data://legal-kb", true, "data ref"},
			{"prompt://contract-summary@1.0.0-rc.1", true, "prompt with prerelease"},
			{"channel://feishu", true, "channel"},
			{"connector://confluence", true, "connector"},
			{"memory://session-cache", true, "memory"},
			{"quota://default-tenant", true, "quota"},
			{"ref://patterns/pii-cn", true, "ref with subpath"},
			{"siem://splunk-prod", true, "siem"},
			{"policy://classified-data-v2@1.0.0", true, "policy"},
			{"skill://my.skill_v1@1.0.0+build.7", true, "build metadata"},
			// negative
			{"http://example.com", false, "wrong scheme"},
			{"skill:/contract-review", false, "single slash"},
			{"skill://", false, "empty path"},
			{"skill://x with space", false, "space in path"},
			{"skill://name@", false, "empty version"},
			{"SKILL://contract-review", false, "uppercase scheme"},
			{"", false, "empty"},
			{"skill://name@1.0.0@2.0.0", false, "double version"},
		}
		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				got := ResourceRefRegex.MatchString(c.in)
				if got != c.ok {
					t.Fatalf("ResourceRef(%q): got match=%v, want %v", c.in, got, c.ok)
				}
			})
		}
	})

	t.Run("Duration", func(t *testing.T) {
		cases := []struct {
			in   string
			ok   bool
			name string
		}{
			{"1ns", true, "ns"},
			{"500us", true, "us"},
			{"250ms", true, "ms"},
			{"30s", true, "s"},
			{"5m", true, "m"},
			{"24h", true, "h"},
			{"7d", true, "d"},
			{"2w", true, "w"},
			{"0s", true, "zero"},
			// negative
			{"1.5s", false, "decimal"},
			{"1S", false, "uppercase unit"},
			{"5min", false, "long unit"},
			{"-1s", false, "negative"},
			{"", false, "empty"},
			{"1y", false, "year not supported"},
			{"1", false, "missing unit"},
			{"s", false, "missing number"},
		}
		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				got := DurationRegex.MatchString(c.in)
				if got != c.ok {
					t.Fatalf("Duration(%q): got match=%v, want %v", c.in, got, c.ok)
				}
			})
		}
	})

	t.Run("SemVer", func(t *testing.T) {
		cases := []struct {
			in   string
			ok   bool
			name string
		}{
			// positive
			{"1.0.0", true, "basic"},
			{"0.0.0", true, "all-zero"},
			{"10.20.30", true, "two-digit"},
			{"1.0.0-alpha", true, "prerelease"},
			{"1.0.0-alpha.1", true, "prerelease compound"},
			{"1.0.0-0.3.7", true, "prerelease numeric"},
			{"1.0.0-x.7.z.92", true, "prerelease alphanumeric"},
			{"1.0.0+20130313144700", true, "build metadata"},
			{"1.0.0-beta+exp.sha.5114f85", true, "prerelease and build"},
			{"1.0.0-rc.1+build.123", true, "rc with build"},
			// negative
			{"1", false, "single digit"},
			{"1.2", false, "two parts"},
			{"01.0.0", false, "leading zero"},
			{"1.0.0-", false, "trailing dash"},
			{"1.0.0+", false, "trailing plus"},
			{"v1.0.0", false, "v prefix"},
			{"1.0.0-α", false, "non-ascii"},
			{"", false, "empty"},
		}
		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				got := SemVerRegex.MatchString(c.in)
				if got != c.ok {
					t.Fatalf("SemVer(%q): got match=%v, want %v", c.in, got, c.ok)
				}
			})
		}
	})

	t.Run("Classification", func(t *testing.T) {
		valid := map[Classification]bool{
			ClassificationPublic: true, ClassificationInternal: true,
			ClassificationConfidential: true, ClassificationRestricted: true,
			ClassificationSecret: true,
		}
		for _, v := range []Classification{"public", "internal", "confidential", "restricted", "secret"} {
			if !valid[v] {
				t.Fatalf("classification %q missing constant", v)
			}
		}
		// Make sure none of the off-by-one variants accidentally match a constant.
		for _, v := range []Classification{"PUBLIC", "secrets", "open", ""} {
			if valid[v] {
				t.Fatalf("classification %q should not be valid", v)
			}
		}
	})

	t.Run("Stage", func(t *testing.T) {
		valid := map[Stage]bool{
			StageExperimental: true, StageBeta: true,
			StageStable: true, StageDeprecated: true,
		}
		for _, v := range []Stage{"experimental", "beta", "stable", "deprecated"} {
			if !valid[v] {
				t.Fatalf("stage %q missing constant", v)
			}
		}
		for _, v := range []Stage{"EXPERIMENTAL", "ga", ""} {
			if valid[v] {
				t.Fatalf("stage %q should not be valid", v)
			}
		}
	})
}

// TestResourceRef_RoundTrip verifies Requirements F25 / P26: every
// fixed-form ResourceRef survives Parse∘Format identity.
func TestResourceRef_RoundTrip(t *testing.T) {
	cases := []ResourceRef{
		"skill://contract-review",
		"skill://contract-review@1.2.0",
		"agent://legal-bot",
		"tool://docusign/create-envelope@v3",
		"model://gpt-4o-eu@2024-05-13",
		"prompt://contract-summary@1.0.0-rc.1",
		"data://legal-kb",
		"ref://patterns/pii-cn",
		"policy://classified-data-v2@1.0.0",
		"skill://my.skill_v1@1.0.0+build.7",
	}
	for _, in := range cases {
		in := in
		t.Run(string(in), func(t *testing.T) {
			scheme, path, version, err := in.Parse()
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", in, err)
			}
			out, err := FormatResourceRef(scheme, path, version)
			if err != nil {
				t.Fatalf("Format(%q,%q,%q) failed: %v", scheme, path, version, err)
			}
			if out != in {
				t.Fatalf("round-trip mismatch: in=%q out=%q", in, out)
			}
		})
	}
}

// TestResourceRef_ParseErrors covers malformed inputs that the regex
// would already reject at admission time, but Parse may still see when
// callers skip validation.
func TestResourceRef_ParseErrors(t *testing.T) {
	bad := []ResourceRef{
		"http://example.com",
		"skill://",
		"skill://name with space",
		"skill://name@",
		"",
	}
	for _, in := range bad {
		in := in
		t.Run(string(in), func(t *testing.T) {
			if _, _, _, err := in.Parse(); err == nil {
				t.Fatalf("Parse(%q) expected error, got nil", in)
			}
		})
	}
}
