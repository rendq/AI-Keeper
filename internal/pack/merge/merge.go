package merge

import (
	"fmt"
	"sort"
	"strings"
)

// Result holds the outcome of a three-way merge of a resource.
type Result struct {
	// Merged contains the final merged document (excludes conflicting paths).
	Merged map[string]interface{}

	// Report contains any conflicts detected during the merge.
	Report ConflictReport
}

// IsClean returns true if the merge completed without conflicts.
func (r *Result) IsClean() bool {
	return !r.Report.HasConflicts()
}

// ResourceSetResult holds the result of merging an entire set of resources.
type ResourceSetResult struct {
	// Merged maps resource keys to their merged documents.
	Merged map[string]map[string]interface{}

	// Conflicts maps resource keys to their conflict reports.
	Conflicts map[string]*ConflictReport

	// Added lists resource keys that exist only in theirs (new upstream resources).
	Added []string

	// Removed lists resource keys that were removed in theirs (upstream deletions).
	Removed []string
}

// HasConflicts returns true if any resource has conflicts.
func (r *ResourceSetResult) HasConflicts() bool {
	for _, report := range r.Conflicts {
		if report.HasConflicts() {
			return true
		}
	}
	return false
}

// ThreeWayMerge performs a recursive three-way merge of YAML documents.
//
// Rules:
//   - If base==ours, take theirs (upstream change accepted).
//   - If base==theirs, take ours (user change preserved).
//   - If ours==theirs, take either (both agree).
//   - If all three differ at a leaf, record a conflict.
//   - For nested maps, recurse into sub-keys.
//   - New keys in theirs (absent from base and ours) are added.
//   - Keys deleted in ours (present in base, absent in ours) with unchanged theirs: deletion wins.
//   - Keys deleted with changed theirs: conflict.
//
// The resourceKey parameter is used for conflict reporting.
func ThreeWayMerge(base, ours, theirs map[string]interface{}, resourceKey string) *Result {
	result := &Result{
		Merged: make(map[string]interface{}),
		Report: ConflictReport{ResourceKey: resourceKey},
	}

	mergeRecursive(base, ours, theirs, "", result)
	return result
}

// ThreeWayMergeResourceSet merges entire resource sets (multiple resources keyed by identifier).
//
// It handles:
//   - Resources present in all three → three-way merge per resource
//   - Resources only in theirs → added
//   - Resources absent from theirs but present in base → removed (upstream deletion)
//   - Resources only in ours (user-added) → preserved
func ThreeWayMergeResourceSet(
	base map[string]map[string]interface{},
	ours map[string]map[string]interface{},
	theirs map[string]map[string]interface{},
) *ResourceSetResult {
	result := &ResourceSetResult{
		Merged:    make(map[string]map[string]interface{}),
		Conflicts: make(map[string]*ConflictReport),
	}

	// Collect all resource keys.
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
		baseDoc, inBase := base[key]
		oursDoc, inOurs := ours[key]
		theirsDoc, inTheirs := theirs[key]

		switch {
		// New resource in theirs only — added by upstream.
		case !inBase && !inOurs && inTheirs:
			result.Added = append(result.Added, key)
			result.Merged[key] = theirsDoc

		// Resource only in ours — user-added, preserve.
		case !inBase && inOurs && !inTheirs:
			result.Merged[key] = oursDoc

		// Both added independently.
		case !inBase && inOurs && inTheirs:
			mr := ThreeWayMerge(nil, oursDoc, theirsDoc, key)
			result.Merged[key] = mr.Merged
			if mr.Report.HasConflicts() {
				report := mr.Report
				result.Conflicts[key] = &report
			}

		// Resource removed by both.
		case inBase && !inOurs && !inTheirs:
			// Omit from result.

		// User deleted, theirs still has it.
		case inBase && !inOurs && inTheirs:
			if deepEqual(baseDoc, theirsDoc) {
				// Theirs unchanged, user deletion wins.
			} else {
				// Theirs changed, conflict on entire resource.
				result.Removed = append(result.Removed, key)
				report := &ConflictReport{
					ResourceKey: key,
					Conflicts: []Conflict{
						classifyConflict("(resource)", mapToInterface(baseDoc), nil, mapToInterface(theirsDoc)),
					},
				}
				result.Conflicts[key] = report
			}

		// Theirs deleted, ours still has it.
		case inBase && inOurs && !inTheirs:
			if deepEqual(baseDoc, oursDoc) {
				// Ours unchanged, upstream deletion wins.
				result.Removed = append(result.Removed, key)
			} else {
				// User modified but upstream deleted — conflict.
				report := &ConflictReport{
					ResourceKey: key,
					Conflicts: []Conflict{
						classifyConflict("(resource)", mapToInterface(baseDoc), mapToInterface(oursDoc), nil),
					},
				}
				result.Conflicts[key] = report
				result.Merged[key] = oursDoc
			}

		// All three have it — standard three-way merge.
		case inBase && inOurs && inTheirs:
			mr := ThreeWayMerge(baseDoc, oursDoc, theirsDoc, key)
			result.Merged[key] = mr.Merged
			if mr.Report.HasConflicts() {
				report := mr.Report
				result.Conflicts[key] = &report
			}
		}
	}

	sort.Strings(result.Added)
	sort.Strings(result.Removed)
	return result
}

// mergeRecursive performs the recursive three-way merge traversal.
func mergeRecursive(base, ours, theirs map[string]interface{}, prefix string, result *Result) {
	// Use empty maps for nil inputs.
	if base == nil {
		base = make(map[string]interface{})
	}
	if ours == nil {
		ours = make(map[string]interface{})
	}
	if theirs == nil {
		theirs = make(map[string]interface{})
	}

	// Collect all keys.
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
		path := joinPath(prefix, key)
		baseVal, inBase := base[key]
		oursVal, inOurs := ours[key]
		theirsVal, inTheirs := theirs[key]

		switch {
		// Only in theirs — new upstream field, add.
		case !inBase && !inOurs && inTheirs:
			result.Merged[key] = theirsVal

		// Only in ours — user-added field, keep.
		case !inBase && inOurs && !inTheirs:
			result.Merged[key] = oursVal

		// Only in base — both deleted, omit.
		case inBase && !inOurs && !inTheirs:
			// Skip.

		// In base and theirs, not ours (user deleted).
		case inBase && !inOurs && inTheirs:
			if valueEqual(baseVal, theirsVal) {
				// Theirs unchanged, user deletion wins.
			} else {
				// Theirs changed but user deleted — conflict.
				result.Report.Conflicts = append(result.Report.Conflicts,
					classifyConflict(path, baseVal, nil, theirsVal))
			}

		// In base and ours, not theirs (upstream deleted).
		case inBase && inOurs && !inTheirs:
			if valueEqual(baseVal, oursVal) {
				// Ours unchanged, upstream deletion wins.
			} else {
				// Ours changed but upstream deleted — conflict.
				result.Report.Conflicts = append(result.Report.Conflicts,
					classifyConflict(path, baseVal, oursVal, nil))
			}

		// Both added independently (not in base).
		case !inBase && inOurs && inTheirs:
			mergeValues(path, key, nil, oursVal, theirsVal, prefix, result)

		// All three present — standard merge.
		case inBase && inOurs && inTheirs:
			mergeValues(path, key, baseVal, oursVal, theirsVal, prefix, result)
		}
	}
}

// mergeValues handles merging of individual values, recursing into nested maps.
func mergeValues(path, key string, baseVal, oursVal, theirsVal interface{}, prefix string, result *Result) {
	// If all three are maps, recurse.
	baseMap, baseIsMap := toMap(baseVal)
	oursMap, oursIsMap := toMap(oursVal)
	theirsMap, theirsIsMap := toMap(theirsVal)

	if oursIsMap && theirsIsMap {
		subResult := &Result{
			Merged: make(map[string]interface{}),
			Report: result.Report,
		}
		var bm map[string]interface{}
		if baseIsMap {
			bm = baseMap
		}
		mergeRecursive(bm, oursMap, theirsMap, path, subResult)
		result.Merged[key] = subResult.Merged
		result.Report = subResult.Report
		return
	}

	// Leaf value comparison.
	switch {
	case valueEqual(baseVal, oursVal) && valueEqual(baseVal, theirsVal):
		// No change.
		result.Merged[key] = baseVal
	case valueEqual(baseVal, oursVal):
		// Only theirs changed.
		result.Merged[key] = theirsVal
	case valueEqual(baseVal, theirsVal):
		// Only ours changed.
		result.Merged[key] = oursVal
	case valueEqual(oursVal, theirsVal):
		// Both changed to same value.
		result.Merged[key] = oursVal
	default:
		// Conflict.
		result.Report.Conflicts = append(result.Report.Conflicts,
			classifyConflict(path, baseVal, oursVal, theirsVal))
	}
}

// toMap attempts to cast a value to map[string]interface{}.
func toMap(v interface{}) (map[string]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	m, ok := v.(map[string]interface{})
	return m, ok
}

// valueEqual compares two values for equality.
func valueEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Both maps — deep compare.
	aMap, aIsMap := toMap(a)
	bMap, bIsMap := toMap(b)
	if aIsMap && bIsMap {
		return deepEqual(aMap, bMap)
	}

	// Both slices — compare element by element.
	aSlice, aIsSlice := toSlice(a)
	bSlice, bIsSlice := toSlice(b)
	if aIsSlice && bIsSlice {
		return sliceEqual(aSlice, bSlice)
	}

	// Scalar comparison via string representation.
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// deepEqual compares two maps recursively.
func deepEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !valueEqual(av, bv) {
			return false
		}
	}
	return true
}

// toSlice attempts to cast a value to []interface{}.
func toSlice(v interface{}) ([]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	s, ok := v.([]interface{})
	return s, ok
}

// sliceEqual compares two slices for equality.
func sliceEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !valueEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// joinPath creates a dot-separated path.
func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// mapToInterface converts map[string]interface{} to interface{} for conflict reporting.
func mapToInterface(m map[string]interface{}) interface{} {
	if m == nil {
		return nil
	}
	return m
}

// ResourceKey generates a standard resource identifier from kind and name.
func ResourceKey(kind, name string) string {
	return strings.TrimSpace(kind) + "/" + strings.TrimSpace(name)
}
