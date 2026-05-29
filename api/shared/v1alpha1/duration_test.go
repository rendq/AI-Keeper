package v1alpha1

import (
	"errors"
	"testing"
	"time"
)

// TestDuration_ToTimeDuration verifies Requirements A2.2 — the AIP
// duration string maps to the expected time.Duration, and malformed
// inputs surface a typed error.
func TestDuration_ToTimeDuration(t *testing.T) {
	cases := []struct {
		in   Duration
		want time.Duration
		ok   bool
		name string
	}{
		{"1ns", time.Nanosecond, true, "ns"},
		{"500us", 500 * time.Microsecond, true, "us"},
		{"250ms", 250 * time.Millisecond, true, "ms"},
		{"1s", time.Second, true, "1s"},
		{"5m", 5 * time.Minute, true, "5m"},
		{"24h", 24 * time.Hour, true, "24h"},
		{"1d", 24 * time.Hour, true, "1d"},
		{"7d", 7 * 24 * time.Hour, true, "7d"},
		{"1w", 7 * 24 * time.Hour, true, "1w"},
		{"2w", 14 * 24 * time.Hour, true, "2w"},
		{"0s", 0, true, "zero"},
		// malformed
		{"", 0, false, "empty"},
		{"1.5s", 0, false, "fractional"},
		{"5min", 0, false, "long unit"},
		{"-1s", 0, false, "negative"},
		{"1y", 0, false, "year"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, err := c.in.ToTimeDuration()
			if c.ok {
				if err != nil {
					t.Fatalf("ToTimeDuration(%q): unexpected error %v", c.in, err)
				}
				if got != c.want {
					t.Fatalf("ToTimeDuration(%q): got %v, want %v", c.in, got, c.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ToTimeDuration(%q): expected error, got %v", c.in, got)
			}
			if !errors.Is(err, ErrInvalidDuration) {
				t.Fatalf("ToTimeDuration(%q): expected ErrInvalidDuration, got %v", c.in, err)
			}
		})
	}
}
