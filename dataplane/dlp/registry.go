package dlp

import "fmt"

// PatternRegistry manages custom pattern sets loaded via patternsRef.
// P0: only the registration interface; no actual custom sets are provided.
type PatternRegistry struct {
	sets map[string][]Pattern
}

// NewPatternRegistry creates an empty PatternRegistry.
func NewPatternRegistry() *PatternRegistry {
	return &PatternRegistry{
		sets: make(map[string][]Pattern),
	}
}

// Register adds a named pattern set to the registry.
// The name corresponds to the `ref://patterns/<name>` reference.
func (r *PatternRegistry) Register(name string, patterns []Pattern) {
	r.sets[name] = patterns
}

// Resolve loads a pattern set by its patternsRef URI.
// Expected format: "ref://patterns/<name>"
func (r *PatternRegistry) Resolve(ref string) ([]Pattern, error) {
	// Parse ref://patterns/<name>
	const prefix = "ref://patterns/"
	if len(ref) <= len(prefix) || ref[:len(prefix)] != prefix {
		return nil, fmt.Errorf("invalid patternsRef format: %s", ref)
	}
	name := ref[len(prefix):]
	patterns, ok := r.sets[name]
	if !ok {
		return nil, fmt.Errorf("pattern set not found: %s", name)
	}
	return patterns, nil
}
