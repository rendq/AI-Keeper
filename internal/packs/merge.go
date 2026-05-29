package packs

import "fmt"

// MergeResult holds the outcome of a three-way merge operation.
type MergeResult struct {
	// Merged contains the final merged key-value pairs (excludes conflicting keys).
	Merged map[string]interface{}

	// Conflicts lists keys where both ours and theirs diverged from base.
	Conflicts []Conflict

	// HasConflicts is true when at least one conflict was detected.
	HasConflicts bool
}

// Conflict describes a single merge conflict at a given key path.
type Conflict struct {
	// Path is the key that has conflicting changes.
	Path string

	// Base is the original value (nil if key did not exist in base).
	Base interface{}

	// Ours is the user-modified value (nil if key was deleted by user).
	Ours interface{}

	// Theirs is the new upstream value (nil if key was deleted upstream).
	Theirs interface{}
}

// ThreeWayMerge performs a flat three-way merge of configuration maps.
//
// Rules:
//   - If base==ours, take theirs (upstream change accepted).
//   - If base==theirs, take ours (user change preserved).
//   - If ours==theirs, take either (both agree).
//   - If all three differ, record a conflict.
//   - New keys in theirs (absent from base and ours) are added.
//   - Keys deleted in ours (present in base, absent in ours) but kept in theirs are a conflict.
//   - Keys deleted in theirs (present in base, absent in theirs) but kept in ours are a conflict.
//
// Covers requirement C5.
func ThreeWayMerge(base, ours, theirs map[string]interface{}) *MergeResult {
	result := &MergeResult{
		Merged:    make(map[string]interface{}),
		Conflicts: nil,
	}

	// Collect all keys from all three maps.
	allKeys := make(map[string]struct{})
	for k := range base {
		allKeys[k] = struct{}{}
	}
	for k := range ours {
		allKeys[k] = struct{}{}
	}
	for k := range theirs {
		allKeys[k] = struct{}{}
	}

	for key := range allKeys {
		baseVal, inBase := base[key]
		oursVal, inOurs := ours[key]
		theirsVal, inTheirs := theirs[key]

		switch {
		// Key only in theirs (new upstream field) — add it.
		case !inBase && !inOurs && inTheirs:
			result.Merged[key] = theirsVal

		// Key only in ours (user added field, not in base or theirs) — keep it.
		case !inBase && inOurs && !inTheirs:
			result.Merged[key] = oursVal

		// Key only in base (both deleted) — omit.
		case inBase && !inOurs && !inTheirs:
			// Both removed, nothing to add.

		// Key in base and theirs but not ours (user deleted, theirs kept).
		case inBase && !inOurs && inTheirs:
			if equal(baseVal, theirsVal) {
				// Theirs unchanged, user deletion wins.
			} else {
				// Theirs changed the value but user deleted — conflict.
				result.Conflicts = append(result.Conflicts, Conflict{
					Path:   key,
					Base:   baseVal,
					Ours:   nil,
					Theirs: theirsVal,
				})
			}

		// Key in base and ours but not theirs (theirs deleted, ours kept).
		case inBase && inOurs && !inTheirs:
			if equal(baseVal, oursVal) {
				// Ours unchanged, theirs deletion wins.
			} else {
				// Ours changed the value but theirs deleted — conflict.
				result.Conflicts = append(result.Conflicts, Conflict{
					Path:   key,
					Base:   baseVal,
					Ours:   oursVal,
					Theirs: nil,
				})
			}

		// Key in all three maps — standard three-way logic.
		case inBase && inOurs && inTheirs:
			switch {
			case equal(baseVal, oursVal) && equal(baseVal, theirsVal):
				// No change anywhere.
				result.Merged[key] = baseVal
			case equal(baseVal, oursVal):
				// Only theirs changed — accept upstream.
				result.Merged[key] = theirsVal
			case equal(baseVal, theirsVal):
				// Only ours changed — preserve user change.
				result.Merged[key] = oursVal
			case equal(oursVal, theirsVal):
				// Both changed to same value — no conflict.
				result.Merged[key] = oursVal
			default:
				// All three differ — conflict.
				result.Conflicts = append(result.Conflicts, Conflict{
					Path:   key,
					Base:   baseVal,
					Ours:   oursVal,
					Theirs: theirsVal,
				})
			}

		// Key in ours and theirs but not base (both added independently).
		case !inBase && inOurs && inTheirs:
			if equal(oursVal, theirsVal) {
				result.Merged[key] = oursVal
			} else {
				result.Conflicts = append(result.Conflicts, Conflict{
					Path:   key,
					Base:   nil,
					Ours:   oursVal,
					Theirs: theirsVal,
				})
			}
		}
	}

	result.HasConflicts = len(result.Conflicts) > 0
	return result
}

// equal does a simple equality check for flat values.
func equal(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
