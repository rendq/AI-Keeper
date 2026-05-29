// Package dlp implements the DLP (Data Loss Prevention) engine for the AIP data plane.
// It provides PII detection, masking, blocking, and classification metadata propagation.
package dlp

import "regexp"

// PIIKind represents a type of personally identifiable information.
type PIIKind string

const (
	PIIKindIDCard     PIIKind = "ID_CARD"
	PIIKindPhone      PIIKind = "PHONE"
	PIIKindBankCard   PIIKind = "BANK_CARD"
	PIIKindEmail      PIIKind = "EMAIL"
	PIIKindPostalCode PIIKind = "POSTAL_CODE"
)

// Pattern defines a compiled regex pattern for PII detection.
type Pattern struct {
	Kind        PIIKind
	Regex       *regexp.Regexp
	Placeholder string // e.g. "<PII:PHONE>"
}

// BuiltinPatterns returns the P0 builtin PII patterns for Chinese PII.
// Patterns are ordered by specificity: more specific patterns first.
func BuiltinPatterns() []Pattern {
	return []Pattern{
		{
			Kind:        PIIKindIDCard,
			Regex:       regexp.MustCompile(`\b\d{17}[\dXx]\b`),
			Placeholder: "<PII:ID_CARD>",
		},
		{
			Kind:        PIIKindPhone,
			Regex:       regexp.MustCompile(`\b1[3-9]\d{9}\b`),
			Placeholder: "<PII:PHONE>",
		},
		{
			Kind:        PIIKindEmail,
			Regex:       regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
			Placeholder: "<PII:EMAIL>",
		},
		{
			Kind:        PIIKindBankCard,
			Regex:       regexp.MustCompile(`\b\d{16,19}\b`),
			Placeholder: "<PII:BANK_CARD>",
		},
		{
			Kind:        PIIKindPostalCode,
			Regex:       regexp.MustCompile(`\b\d{6}\b`),
			Placeholder: "<PII:POSTAL_CODE>",
		},
	}
}
