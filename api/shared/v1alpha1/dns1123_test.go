package v1alpha1

import (
	"errors"
	"strings"
	"testing"
)

// TestIsValidDNS1123Subdomain covers Requirements A2.4: metadata.name
// must be a DNS-1123 subdomain ≤253 chars.
func TestIsValidDNS1123Subdomain(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		name string
	}{
		{"my-skill", true, "simple"},
		{"foo.bar.baz", true, "dotted"},
		{"a", true, "single char"},
		{"contract-review-v2", true, "hyphens"},
		// negative
		{"", false, "empty"},
		{"My-Skill", false, "uppercase"},
		{"-foo", false, "leading dash"},
		{"foo-", false, "trailing dash"},
		{".foo", false, "leading dot"},
		{"foo_bar", false, "underscore"},
		{strings.Repeat("a", 254), false, "too-long"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			err := IsValidDNS1123Subdomain(c.in)
			if c.ok && err != nil {
				t.Fatalf("expected %q valid, got %v", c.in, err)
			}
			if !c.ok {
				if err == nil {
					t.Fatalf("expected %q invalid, got nil", c.in)
				}
				if !errors.Is(err, ErrInvalidDNS1123Subdomain) {
					t.Fatalf("expected ErrInvalidDNS1123Subdomain, got %v", err)
				}
			}
		})
	}
}
