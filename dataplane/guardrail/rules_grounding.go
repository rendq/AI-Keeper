package guardrail

import (
	"encoding/json"
	"fmt"
	"strings"
)

// --- Hallucination / Grounding / ClassificationLeak Detection Rules ---
//
// This file implements output-stage guardrail rules for AIPBuiltinProvider:
// - Hallucination: checks if output is grounded in provided source texts (word overlap heuristic)
// - Grounding: semantic consistency check between output and KB retrieval results
// - ClassificationLeak: checks if output classification level exceeds the allowed max

// classificationLevels defines the hierarchy of classification levels (higher index = more restricted).
var classificationLevels = map[string]int{
	"public":       0,
	"internal":     1,
	"confidential": 2,
	"restricted":   3,
	"secret":       4,
}

// DetectHallucination checks if output content is grounded in provided sources using
// a word overlap ratio heuristic. If the overlap score is below the threshold, the
// output is considered potentially hallucinated.
// Returns (score, triggered, reason).
func DetectHallucination(output string, sources []string, threshold float64) (float64, bool, string) {
	if output == "" || len(sources) == 0 {
		return 0.0, false, ""
	}

	score := computeWordOverlap(output, sources)

	if score < threshold {
		return score, true, fmt.Sprintf("hallucination detected: grounding score %.2f below threshold %.2f", score, threshold)
	}

	return score, false, ""
}

// DetectGrounding performs a semantic consistency check between the output and
// KB (knowledge base) retrieval results. Uses word overlap as a simple heuristic
// for the aip-builtin provider.
// Returns (score, triggered, reason).
func DetectGrounding(output string, kbResults []string) (float64, bool, string) {
	if output == "" || len(kbResults) == 0 {
		return 0.0, false, ""
	}

	score := computeWordOverlap(output, kbResults)

	// Default grounding threshold: 0.3 (output should share at least 30% words with KB)
	const groundingThreshold = 0.3
	if score < groundingThreshold {
		return score, true, fmt.Sprintf("grounding violation: output consistency score %.2f below threshold %.2f", score, groundingThreshold)
	}

	return score, false, ""
}

// DetectClassificationLeak checks if the output classification level exceeds the
// maximum allowed classification level. Classification hierarchy:
// public < internal < confidential < restricted < secret.
// Returns (score, triggered, reason).
func DetectClassificationLeak(outputClassification, maxClassification string) (float64, bool, string) {
	if outputClassification == "" || maxClassification == "" {
		return 0.0, false, ""
	}

	outLevel, outOk := classificationLevels[strings.ToLower(outputClassification)]
	maxLevel, maxOk := classificationLevels[strings.ToLower(maxClassification)]

	if !outOk || !maxOk {
		// Unknown classification level - cannot determine, don't trigger
		return 0.0, false, ""
	}

	if outLevel > maxLevel {
		return 1.0, true, fmt.Sprintf("classification leak: output level %q exceeds max allowed %q", outputClassification, maxClassification)
	}

	return 0.0, false, ""
}

// computeWordOverlap computes the ratio of words in the output that appear in any of
// the source texts. Returns a score in [0, 1].
func computeWordOverlap(output string, sources []string) float64 {
	outputWords := tokenize(output)
	if len(outputWords) == 0 {
		return 0.0
	}

	// Build a set of all words from sources
	sourceWordSet := make(map[string]struct{})
	for _, src := range sources {
		for _, w := range tokenize(src) {
			sourceWordSet[w] = struct{}{}
		}
	}

	if len(sourceWordSet) == 0 {
		return 0.0
	}

	// Count how many output words appear in sources
	matchCount := 0
	for _, w := range outputWords {
		if _, found := sourceWordSet[w]; found {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(outputWords))
}

// tokenize splits text into lowercase words, filtering out short stop words.
func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	result := make([]string, 0, len(words))
	for _, w := range words {
		// Strip common punctuation
		w = strings.Trim(w, ".,;:!?\"'()[]{}"+"\u00b7\u3001\u3002\uff0c\uff1b\uff1a\uff01\uff1f\u201c\u201d\u2018\u2019\uff08\uff09\u3010\u3011")
		if len(w) >= 2 { // skip single-char tokens as stop words
			result = append(result, w)
		}
	}
	return result
}

// ParseKBSources parses the "kb.sources" metadata field (JSON array of strings).
func ParseKBSources(metadata map[string]string) ([]string, error) {
	raw, ok := metadata["kb.sources"]
	if !ok || raw == "" {
		return nil, nil
	}
	var sources []string
	if err := json.Unmarshal([]byte(raw), &sources); err != nil {
		return nil, fmt.Errorf("failed to parse kb.sources: %w", err)
	}
	return sources, nil
}
