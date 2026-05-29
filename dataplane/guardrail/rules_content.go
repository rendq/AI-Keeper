package guardrail

import (
	"fmt"
	"regexp"
	"strings"
)

// --- Toxicity / Bias / Profanity Detection Rules ---
//
// This file implements content moderation rules for AIPBuiltinProvider.
// The aip-builtin provider uses keyword lists and pattern matching as a baseline.
// In production, these rules typically delegate to external models (LlamaGuard, etc.)
// for higher accuracy.

// ToxicityPatterns detects toxic content including threats, harassment, and hate speech.
var ToxicityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(kill|murder|destroy|eliminate)\s+(you|him|her|them|everyone|all)\b`),
	regexp.MustCompile(`(?i)\b(death\s+threat|threaten|threatening)\b`),
	regexp.MustCompile(`(?i)\b(hate|despise)\s+(you|him|her|them|all)\b`),
	regexp.MustCompile(`(?i)\byou\s+(deserve|should)\s+(to\s+)?(die|suffer|be\s+killed)\b`),
	regexp.MustCompile(`(?i)\b(violent|violence)\s+(against|towards)\b`),
	regexp.MustCompile(`(?i)\b(harass|harassment|bully|bullying)\b`),
}

// ToxicKeywords are individual toxic terms that indicate harmful content.
var ToxicKeywords = []string{
	"杀死", "去死", "自杀", "暴力", "恐怖主义",
	"屠杀", "残害", "虐待",
}

// BiasPatterns detects content exhibiting discriminatory bias.
var BiasPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(all|every)\s+(men|women|blacks|whites|asians|jews|muslims|christians)\s+(are|should)\b`),
	regexp.MustCompile(`(?i)\b(inferior|superior)\s+(race|gender|sex)\b`),
	regexp.MustCompile(`(?i)\b(women|men)\s+(can'?t|cannot|shouldn'?t|are\s+not\s+able)\b`),
	regexp.MustCompile(`(?i)\bracial\s+(superiority|inferiority)\b`),
	regexp.MustCompile(`(?i)\bgender\s+(superiority|inferiority)\b`),
}

// BiasKeywords are terms indicating biased content in Chinese context.
var BiasKeywords = []string{
	"种族歧视", "性别歧视", "地域歧视", "民族歧视",
}

// ProfanityPatterns detects profane or vulgar language.
var ProfanityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(fuck|shit|damn|bitch|asshole|bastard|crap)\b`),
	regexp.MustCompile(`(?i)\b(dick|cock|pussy|tits|boobs)\b`),
	regexp.MustCompile(`(?i)\b(wtf|stfu|lmfao)\b`),
}

// ProfanityKeywords are profane terms in Chinese.
var ProfanityKeywords = []string{
	"操你", "他妈的", "傻逼", "草泥马", "去你妈",
	"狗日的", "混蛋", "王八蛋", "废物",
}

// DetectToxicity checks text for toxic content using patterns and keywords.
// Returns (score, triggered, reason).
func DetectToxicity(text string) (float64, bool, string) {
	if text == "" {
		return 0.0, false, ""
	}

	// Check regex patterns
	for _, pat := range ToxicityPatterns {
		if pat.MatchString(text) {
			return 0.90, true, fmt.Sprintf("toxicity pattern detected: %s", pat.String())
		}
	}

	// Check keywords
	lower := strings.ToLower(text)
	for _, kw := range ToxicKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) || strings.Contains(text, kw) {
			return 0.85, true, fmt.Sprintf("toxic keyword detected: %s", kw)
		}
	}

	return 0.0, false, ""
}

// DetectBias checks text for biased or discriminatory content.
// Returns (score, triggered, reason).
func DetectBias(text string) (float64, bool, string) {
	if text == "" {
		return 0.0, false, ""
	}

	// Check regex patterns
	for _, pat := range BiasPatterns {
		if pat.MatchString(text) {
			return 0.88, true, fmt.Sprintf("bias pattern detected: %s", pat.String())
		}
	}

	// Check keywords
	for _, kw := range BiasKeywords {
		if strings.Contains(text, kw) {
			return 0.85, true, fmt.Sprintf("bias keyword detected: %s", kw)
		}
	}

	return 0.0, false, ""
}

// DetectProfanity checks text for profane or vulgar language.
// Returns (score, triggered, reason).
func DetectProfanity(text string) (float64, bool, string) {
	if text == "" {
		return 0.0, false, ""
	}

	// Check regex patterns
	for _, pat := range ProfanityPatterns {
		if pat.MatchString(text) {
			return 0.90, true, fmt.Sprintf("profanity pattern detected: %s", pat.String())
		}
	}

	// Check keywords
	for _, kw := range ProfanityKeywords {
		if strings.Contains(text, kw) {
			return 0.85, true, fmt.Sprintf("profanity keyword detected: %s", kw)
		}
	}

	return 0.0, false, ""
}
