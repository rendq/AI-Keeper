package dlp

import (
	"errors"
	"fmt"
)

// Mode defines the DLP engine operating mode.
type Mode string

const (
	// ModeDetectAndMask replaces detected PII with <PII:KIND> placeholders.
	ModeDetectAndMask Mode = "detect_and_mask"
	// ModeDetectAndBlock rejects the request if any PII is detected.
	ModeDetectAndBlock Mode = "detect_and_block"
)

// ErrBlockedByDLP is returned when PII is detected in detect_and_block mode.
var ErrBlockedByDLP = errors.New("BlockedByDLP")

// Classification represents the data classification level.
type Classification string

const (
	ClassificationPublic       Classification = "public"
	ClassificationInternal     Classification = "internal"
	ClassificationConfidential Classification = "confidential"
	ClassificationRestricted   Classification = "restricted"
	ClassificationSecret       Classification = "secret"
)

// InspectRequest contains the input for DLP inspection.
type InspectRequest struct {
	Text           string
	Mode           Mode
	Classification Classification // request-level classification
	PatternsRef    string         // optional: "ref://patterns/<name>" for custom patterns
}

// InspectResult contains the output of DLP inspection.
type InspectResult struct {
	// Text is the (potentially masked) output text.
	Text string
	// Blocked indicates the request was blocked by DLP.
	Blocked bool
	// Redactions lists the PII kinds detected (types only, no values).
	Redactions []PIIKind
	// Classification propagated from request.
	Classification Classification
}

// Engine is the DLP engine that performs PII detection, masking, and blocking.
type Engine struct {
	patterns []Pattern
	registry *PatternRegistry
}

// NewEngine creates a DLP engine with builtin patterns and an optional pattern registry.
func NewEngine(registry *PatternRegistry) *Engine {
	return &Engine{
		patterns: BuiltinPatterns(),
		registry: registry,
	}
}

// Inspect performs PII detection on the input text according to the specified mode.
// It returns the inspection result with masked text (or blocked status) and redaction metadata.
func (e *Engine) Inspect(req InspectRequest) (*InspectResult, error) {
	patterns := e.patterns

	// Load custom patterns if patternsRef is specified
	if req.PatternsRef != "" {
		if e.registry == nil {
			return nil, fmt.Errorf("pattern registry not configured but patternsRef specified: %s", req.PatternsRef)
		}
		customPatterns, err := e.registry.Resolve(req.PatternsRef)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve patternsRef: %w", err)
		}
		patterns = append(patterns, customPatterns...)
	}

	// Detect PII
	detections := e.detect(req.Text, patterns)

	result := &InspectResult{
		Classification: req.Classification,
	}

	if len(detections) == 0 {
		result.Text = req.Text
		return result, nil
	}

	// Collect unique redaction kinds
	result.Redactions = uniqueKinds(detections)

	switch req.Mode {
	case ModeDetectAndBlock:
		result.Blocked = true
		result.Text = ""
		return result, ErrBlockedByDLP

	case ModeDetectAndMask:
		result.Text = e.mask(req.Text, patterns)
		return result, nil

	default:
		return nil, fmt.Errorf("unsupported DLP mode: %s", req.Mode)
	}
}

// InspectOutput performs a second-pass (outbound) inspection on response text.
// Used for B4.3: output PII detection before returning to user.
func (e *Engine) InspectOutput(req InspectRequest) (*InspectResult, error) {
	return e.Inspect(req)
}

// detection represents a single PII match in the text.
type detection struct {
	Kind  PIIKind
	Start int
	End   int
}

// detect finds all PII matches in the text, resolving overlaps by pattern priority.
func (e *Engine) detect(text string, patterns []Pattern) []detection {
	var results []detection
	for _, p := range patterns {
		locs := p.Regex.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			results = append(results, detection{
				Kind:  p.Kind,
				Start: loc[0],
				End:   loc[1],
			})
		}
	}
	// Sort ascending by start, then longer match first
	for i := 1; i < len(results); i++ {
		for j := i; j > 0; j-- {
			lenJ := results[j].End - results[j].Start
			lenPrev := results[j-1].End - results[j-1].Start
			if results[j].Start < results[j-1].Start || (results[j].Start == results[j-1].Start && lenJ > lenPrev) {
				results[j], results[j-1] = results[j-1], results[j]
			} else {
				break
			}
		}
	}
	// Remove overlaps
	if len(results) == 0 {
		return results
	}
	filtered := []detection{results[0]}
	for i := 1; i < len(results); i++ {
		last := filtered[len(filtered)-1]
		if results[i].Start >= last.End {
			filtered = append(filtered, results[i])
		}
	}
	return filtered
}

// replacement represents a text range to be replaced with a placeholder.
type replacement struct {
	start       int
	end         int
	placeholder string
}

// mask replaces all PII matches with their corresponding placeholders.
// Processes patterns in order (more specific first), skips overlapping matches.
func (e *Engine) mask(text string, patterns []Pattern) string {
	var replacements []replacement
	for _, p := range patterns {
		locs := p.Regex.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			replacements = append(replacements, replacement{
				start:       loc[0],
				end:         loc[1],
				placeholder: p.Placeholder,
			})
		}
	}

	// Sort by start position ascending, then by length descending (longer match first)
	sortReplacementsAsc(replacements)

	// Remove overlapping replacements (keep first/longest match per range)
	replacements = removeOverlaps(replacements)

	// Sort descending by start to replace from right to left
	sortReplacementsDesc(replacements)

	result := text
	for _, r := range replacements {
		result = result[:r.start] + r.placeholder + result[r.end:]
	}
	return result
}

// sortReplacementsAsc sorts by start ascending, then by length descending.
func sortReplacementsAsc(reps []replacement) {
	for i := 1; i < len(reps); i++ {
		for j := i; j > 0; j-- {
			lenJ := reps[j].end - reps[j].start
			lenPrev := reps[j-1].end - reps[j-1].start
			if reps[j].start < reps[j-1].start || (reps[j].start == reps[j-1].start && lenJ > lenPrev) {
				reps[j], reps[j-1] = reps[j-1], reps[j]
			} else {
				break
			}
		}
	}
}

// sortReplacementsDesc sorts by start position descending.
func sortReplacementsDesc(reps []replacement) {
	for i := 1; i < len(reps); i++ {
		for j := i; j > 0 && reps[j].start > reps[j-1].start; j-- {
			reps[j], reps[j-1] = reps[j-1], reps[j]
		}
	}
}

// removeOverlaps removes replacements that overlap with earlier (higher priority) ones.
func removeOverlaps(reps []replacement) []replacement {
	if len(reps) == 0 {
		return reps
	}
	var result []replacement
	result = append(result, reps[0])
	for i := 1; i < len(reps); i++ {
		last := result[len(result)-1]
		if reps[i].start >= last.end {
			result = append(result, reps[i])
		}
		// else: overlapping, skip (earlier pattern wins)
	}
	return result
}

// uniqueKinds returns deduplicated PIIKinds from detections.
func uniqueKinds(dets []detection) []PIIKind {
	seen := make(map[PIIKind]struct{})
	var kinds []PIIKind
	for _, d := range dets {
		if _, ok := seen[d.Kind]; !ok {
			seen[d.Kind] = struct{}{}
			kinds = append(kinds, d.Kind)
		}
	}
	return kinds
}
