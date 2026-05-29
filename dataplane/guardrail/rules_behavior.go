package guardrail

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// --- Behavior Stage Rules ---
//
// This file implements behavior-stage guardrail rules for AIPBuiltinProvider:
// - RequiredCitations: verifies output contains source citations
// - BlockedTopics: rejects text matching blocked topic keywords
// - Custom CEL: evaluates user-provided CEL expressions against text

// CitationPatterns defines common citation marker patterns.
var CitationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\[\d+\]`),                  // [1], [2], etc.
	regexp.MustCompile(`\[source\]`),               // [source]
	regexp.MustCompile(`\[Source\]`),               // [Source]
	regexp.MustCompile(`\[citation\]`),             // [citation]
	regexp.MustCompile(`\[Citation\]`),             // [Citation]
	regexp.MustCompile(`\[ref\]`),                  // [ref]
	regexp.MustCompile(`\[Ref\]`),                  // [Ref]
	regexp.MustCompile(`「引用」`),                     // Chinese citation marker
	regexp.MustCompile(`引用来源`),                     // Chinese "citation source"
	regexp.MustCompile(`(?i)\bsource:\s*\S+`),     // source: <url/reference>
	regexp.MustCompile(`(?i)\breference:\s*\S+`),  // reference: <url/reference>
	regexp.MustCompile(`(?i)\bcitation:\s*\S+`),   // citation: <reference>
	regexp.MustCompile(`https?://[^\s]+`),         // URLs as citations
}

// DetectMissingCitations checks if output contains citation markers.
// Returns (score, triggered, reason). Triggered=true means citations are MISSING.
func DetectMissingCitations(output string) (float64, bool, string) {
	if output == "" {
		return 0.0, false, ""
	}

	for _, pat := range CitationPatterns {
		if pat.MatchString(output) {
			// Found a citation — not triggered
			return 0.0, false, ""
		}
	}

	// No citations found — trigger
	return 0.90, true, "output does not contain any source citations"
}

// DetectBlockedTopics checks if text matches any blocked topic using keyword matching.
// blockedTopics is a list of topic keywords (case-insensitive matching).
// Returns (score, triggered, reason).
func DetectBlockedTopics(text string, blockedTopics []string) (float64, bool, string) {
	if text == "" || len(blockedTopics) == 0 {
		return 0.0, false, ""
	}

	lower := strings.ToLower(text)
	for _, topic := range blockedTopics {
		topicLower := strings.ToLower(strings.TrimSpace(topic))
		if topicLower == "" {
			continue
		}
		if strings.Contains(lower, topicLower) {
			return 0.95, true, fmt.Sprintf("blocked topic detected: %s", topic)
		}
	}

	return 0.0, false, ""
}

// EvaluateCustomCEL evaluates a CEL expression against the text.
// The expression has access to a "text" variable containing the input text.
// Returns (score, triggered, reason).
func EvaluateCustomCEL(text string, expression string) (float64, bool, string) {
	if expression == "" {
		return 0.0, false, ""
	}

	env, err := cel.NewEnv(
		cel.Variable("text", cel.StringType),
	)
	if err != nil {
		return 0.0, false, fmt.Sprintf("CEL env error: %v", err)
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return 0.0, false, fmt.Sprintf("CEL compile error: %v", issues.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return 0.0, false, fmt.Sprintf("CEL program error: %v", err)
	}

	out, _, err := prg.Eval(map[string]interface{}{
		"text": text,
	})
	if err != nil {
		return 0.0, false, fmt.Sprintf("CEL eval error: %v", err)
	}

	// The expression should return a bool
	if out.Type() == types.BoolType {
		triggered := out.Value().(bool)
		if triggered {
			return 0.90, true, fmt.Sprintf("custom CEL expression triggered: %s", expression)
		}
		return 0.0, false, ""
	}

	return 0.0, false, fmt.Sprintf("CEL expression returned non-bool type: %s", out.Type())
}

// parseBehaviorBlockedTopics parses a JSON array of topic keywords from config.
func parseBehaviorBlockedTopics(configValue string) ([]string, error) {
	var topics []string
	if err := json.Unmarshal([]byte(configValue), &topics); err != nil {
		return nil, fmt.Errorf("failed to parse blockedTopics config: %w", err)
	}
	return topics, nil
}
