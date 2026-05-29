package guardrail

import (
	"fmt"
	"strings"

	"github.com/ai-keeper/ai-keeper/dataplane/dlp"
)

// --- PII / PIILeak Detection Rules ---
//
// This file implements PII and PIILeak detection for AIPBuiltinProvider by
// reusing the DLP Engine patterns (身份证, 手机号, 银行卡号, 邮箱, etc.).
//
// PII rule: detects PII in user input (input stage) — action can be warn or block.
// PIILeak rule: detects PII in model output (output stage) — blocks to prevent data leakage.

// DetectPII scans text for PII using the builtin DLP patterns.
// Returns (score, triggered, reason) where score is 1.0 if any PII is found.
func DetectPII(text string) (float64, bool, string) {
	if text == "" {
		return 0.0, false, ""
	}

	patterns := dlp.BuiltinPatterns()
	var found []string

	for _, p := range patterns {
		if p.Regex.MatchString(text) {
			found = append(found, string(p.Kind))
		}
	}

	if len(found) == 0 {
		return 0.0, false, ""
	}

	reason := fmt.Sprintf("PII detected: %s", strings.Join(found, ", "))
	// Score scales with number of distinct PII types found
	score := float64(len(found)) / float64(len(patterns))
	if score > 1.0 {
		score = 1.0
	}
	// Minimum score of 0.8 when any PII is detected
	if score < 0.8 {
		score = 0.8
	}
	return score, true, reason
}

// DetectPIILeak scans model output for PII that should not be leaked.
// This is stricter than DetectPII — any PII in output is a potential data leak.
// Returns (score, triggered, reason).
func DetectPIILeak(text string) (float64, bool, string) {
	if text == "" {
		return 0.0, false, ""
	}

	patterns := dlp.BuiltinPatterns()
	var found []string

	for _, p := range patterns {
		if p.Regex.MatchString(text) {
			found = append(found, string(p.Kind))
		}
	}

	if len(found) == 0 {
		return 0.0, false, ""
	}

	reason := fmt.Sprintf("PII leak detected in output: %s", strings.Join(found, ", "))
	// PIILeak is always high severity — any PII in output is a leak
	return 0.95, true, reason
}
