package v1alpha1

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// DurationRegex is the full pattern enforced on [Duration] CRD fields.
var DurationRegex = regexp.MustCompile(`^\d+(ns|us|ms|s|m|h|d|w)$`)

// durationSplitRe captures the numeric and unit components.
var durationSplitRe = regexp.MustCompile(`^(\d+)(ns|us|ms|s|m|h|d|w)$`)

// ErrInvalidDuration is returned when a [Duration] cannot be parsed.
var ErrInvalidDuration = errors.New("invalid Duration")

// unitToTime expands the AIP duration units into Go's [time.Duration].
// Note: AIP adds the 'd' (day) and 'w' (week) units that the stdlib does
// not understand.
var unitToTime = map[string]time.Duration{
	"ns": time.Nanosecond,
	"us": time.Microsecond,
	"ms": time.Millisecond,
	"s":  time.Second,
	"m":  time.Minute,
	"h":  time.Hour,
	"d":  24 * time.Hour,
	"w":  7 * 24 * time.Hour,
}

// ToTimeDuration converts the [Duration] into Go's [time.Duration].
// Returns an error if the value does not match [DurationRegex].
func (d Duration) ToTimeDuration() (time.Duration, error) {
	m := durationSplitRe.FindStringSubmatch(string(d))
	if m == nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDuration, string(d))
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %v", ErrInvalidDuration, string(d), err)
	}
	unit, ok := unitToTime[m[2]]
	if !ok {
		return 0, fmt.Errorf("%w: unknown unit %q", ErrInvalidDuration, m[2])
	}
	// Detect overflow when multiplying.
	if n > 0 && int64(unit) > 0 && n > int64(time.Duration(1<<63-1))/int64(unit) {
		return 0, fmt.Errorf("%w: overflow for %q", ErrInvalidDuration, string(d))
	}
	return time.Duration(n) * unit, nil
}

// IsValid reports whether the value matches [DurationRegex].
func (d Duration) IsValid() bool {
	return DurationRegex.MatchString(string(d))
}
