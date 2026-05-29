package guardrail

import (
	"fmt"
	"math"
	"regexp"
)

// --- Prompt Injection Detection Rules ---
//
// This file implements pattern-based PromptInjection and Jailbreak detection
// for the AIPBuiltinProvider. Detection uses two complementary techniques:
//   1. Regex pattern matching against known injection/jailbreak templates
//   2. Perplexity heuristic (character entropy + special-char ratio) for obfuscated attacks

// InjectionPatterns detects common prompt injection attempts.
// These patterns cover:
//   - Instruction override ("ignore previous instructions")
//   - Role switching ("you are now an unrestricted...")
//   - Delimiter injection ([SYSTEM], [INST], <|system|>)
//   - Context manipulation ("new instructions:", "system prompt override")
var InjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|above|prior)\s+(instructions?|prompts?|rules?)`),
	regexp.MustCompile(`(?i)disregard\s+(all\s+)?(previous|above|prior)\s+(instructions?|prompts?|rules?)`),
	regexp.MustCompile(`(?i)forget\s+(all\s+)?(previous|above|prior|your)\s+(previous\s+)?(instructions?|prompts?|rules?|context)`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(a|an|the)\s+`),
	regexp.MustCompile(`(?i)new\s+instructions?\s*:`),
	regexp.MustCompile(`(?i)system\s*prompt\s*override`),
	regexp.MustCompile(`(?i)\boverride\s+(system|safety|security)\b`),
	regexp.MustCompile(`(?i)act\s+as\s+(if\s+)?(you\s+)?(are|were)\s+(a|an|the)?\s*(unrestricted|unfiltered|uncensored)`),
	regexp.MustCompile(`(?i)pretend\s+(that\s+)?(you\s+)?(are|have)\s+(no|zero)\s+(restrictions?|rules?|limitations?)`),
	regexp.MustCompile(`(?i)\[SYSTEM\]|\[INST\]|\<\|system\|\>`),
	regexp.MustCompile(`(?i)translate\s+(the\s+)?(following|this)\s+(to|into)\s+.{0,20}(ignore|disregard|forget)`),
	regexp.MustCompile(`(?i)repeat\s+(after|back)\s+.{0,30}(system|prompt|instruction)`),
}

// JailbreakPatterns detects jailbreak attempts (DAN, roleplay bypass, etc.).
// These cover:
//   - DAN (Do Anything Now) variants
//   - Developer/god/admin mode activation
//   - Safety/ethical bypass requests
//   - Hypothetical scenario exploitation
//   - Refusal suppression
var JailbreakPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bDAN\b.*\b(mode|prompt|jailbreak)\b`),
	regexp.MustCompile(`(?i)do\s+anything\s+now`),
	regexp.MustCompile(`(?i)jailbreak(ed)?`),
	regexp.MustCompile(`(?i)(enable|activate|enter)\s+(developer|god|admin|root|sudo)\s+mode`),
	regexp.MustCompile(`(?i)bypass\s+(safety|content|ethical|security)\s+(filters?|guidelines?|restrictions?|policies?)`),
	regexp.MustCompile(`(?i)remove\s+(all\s+)?(safety|content|ethical)\s+(filters?|guidelines?|restrictions?)`),
	regexp.MustCompile(`(?i)without\s+(any\s+)?(moral|ethical|safety)\s+(constraints?|guidelines?|restrictions?)`),
	regexp.MustCompile(`(?i)no\s+(ethical|safety|content)\s+(guidelines?|restrictions?|boundaries|limits)`),
	regexp.MustCompile(`(?i)hypothetical(ly)?\s+(scenario|situation)\s+(where|in\s+which)\s+(there\s+are\s+)?no\s+(rules?|restrictions?)`),
	regexp.MustCompile(`(?i)from\s+now\s+on,?\s+you\s+(will|must|should|can)\s+(not\s+)?(refuse|decline|reject|say\s+no)`),
	regexp.MustCompile(`(?i)opposite\s+day|opposite\s+mode`),
	regexp.MustCompile(`(?i)in\s+(this|the)\s+(story|fiction|narrative|game),?\s+(there\s+are\s+)?no\s+(rules?|limits?|restrictions?)`),
}

// PerplexityConfig holds thresholds for the perplexity-based obfuscation detection.
type PerplexityConfig struct {
	// EntropyNormalLow is the expected lower bound of Shannon entropy for normal text.
	EntropyNormalLow float64
	// EntropyAnomalyHigh is the threshold above which text is considered anomalous.
	EntropyAnomalyHigh float64
	// PerplexityThreshold is the minimum perplexity score to consider text suspicious.
	PerplexityThreshold float64
	// SpecialCharThreshold is the minimum special-char ratio to combine with perplexity.
	SpecialCharThreshold float64
}

// DefaultPerplexityConfig returns the default perplexity detection thresholds.
func DefaultPerplexityConfig() PerplexityConfig {
	return PerplexityConfig{
		EntropyNormalLow:     4.0,
		EntropyAnomalyHigh:   6.0,
		PerplexityThreshold:  0.7,
		SpecialCharThreshold: 0.3,
	}
}

// ComputeCharEntropy computes Shannon entropy of character distribution in text.
// Normal English text typically has entropy ~4.0-4.5 bits/char.
// Encoded/obfuscated text (base64, hex, unicode tricks) tends to have higher entropy (>5.5).
func ComputeCharEntropy(text string) float64 {
	if len(text) == 0 {
		return 0.0
	}

	freq := make(map[rune]int)
	total := 0
	for _, r := range text {
		freq[r]++
		total++
	}

	entropy := 0.0
	for _, count := range freq {
		p := float64(count) / float64(total)
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// ComputePerplexityScore normalizes entropy to a [0, 1] anomaly score.
func ComputePerplexityScore(text string, cfg PerplexityConfig) float64 {
	entropy := ComputeCharEntropy(text)
	score := (entropy - cfg.EntropyNormalLow) / (cfg.EntropyAnomalyHigh - cfg.EntropyNormalLow)
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

// ComputeSpecialCharRatio returns the ratio of non-alphanumeric, non-space characters.
func ComputeSpecialCharRatio(text string) float64 {
	if len(text) == 0 {
		return 0.0
	}
	special := 0
	total := 0
	for _, r := range text {
		total++
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ') {
			special++
		}
	}
	return float64(special) / float64(total)
}

// DetectPromptInjection uses pattern matching + perplexity heuristic to detect
// prompt injection in the given text.
// Returns (score, triggered, reason).
func DetectPromptInjection(text string) (float64, bool, string) {
	return DetectPromptInjectionWithConfig(text, DefaultPerplexityConfig())
}

// DetectPromptInjectionWithConfig allows custom perplexity thresholds.
func DetectPromptInjectionWithConfig(text string, cfg PerplexityConfig) (float64, bool, string) {
	// Phase 1: Regex pattern matching
	for _, pat := range InjectionPatterns {
		if pat.MatchString(text) {
			return 0.95, true, fmt.Sprintf("prompt injection pattern detected: %s", pat.String())
		}
	}

	// Phase 2: Perplexity heuristic for obfuscated attacks
	perplexity := ComputePerplexityScore(text, cfg)
	specialRatio := ComputeSpecialCharRatio(text)

	if perplexity > cfg.PerplexityThreshold && specialRatio > cfg.SpecialCharThreshold {
		combined := (perplexity + specialRatio) / 2
		return combined, true, "high perplexity and special character ratio indicate obfuscated injection"
	}

	return 0.0, false, ""
}

// DetectJailbreak uses pattern matching to detect jailbreak attempts.
// Returns (score, triggered, reason).
func DetectJailbreak(text string) (float64, bool, string) {
	for _, pat := range JailbreakPatterns {
		if pat.MatchString(text) {
			return 0.92, true, fmt.Sprintf("jailbreak pattern detected: %s", pat.String())
		}
	}
	return 0.0, false, ""
}
