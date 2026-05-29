package cedar

import (
	"crypto/sha256"
	"fmt"
	"sync"
)

// CedarBundle represents a compiled collection of Cedar policies ready for distribution.
type CedarBundle struct {
	Hash        string // SHA-256 of compiled Cedar text
	Version     int64  // Monotonically increasing version number
	Policies    string // Compiled Cedar policy text
	PolicyCount int    // Number of policies in the bundle
}

// BundleBuilder accumulates policies and builds bundles.
type BundleBuilder struct {
	mu       sync.Mutex
	policies []PolicyInput
	compiler *CedarPolicyCompiler
	version  int64
}

// NewBundleBuilder creates a new BundleBuilder instance.
func NewBundleBuilder() *BundleBuilder {
	return &BundleBuilder{
		compiler: NewCompiler(),
	}
}

// AddPolicy adds a policy to the builder.
func (b *BundleBuilder) AddPolicy(input PolicyInput) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.policies = append(b.policies, input)
}

// Build compiles all accumulated policies into a CedarBundle.
func (b *BundleBuilder) Build() (*CedarBundle, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.policies) == 0 {
		return nil, fmt.Errorf("cannot build bundle: no policies added")
	}

	compiled, err := b.compiler.Compile(b.policies)
	if err != nil {
		return nil, fmt.Errorf("bundle compile failed: %w", err)
	}

	b.version++
	return &CedarBundle{
		Hash:        computeHash(compiled),
		Version:     b.version,
		Policies:    compiled,
		PolicyCount: len(b.policies),
	}, nil
}

// BuildIncremental produces a new bundle by applying added/removed diffs to a previous bundle.
func (b *BundleBuilder) BuildIncremental(previous *CedarBundle, added, removed []PolicyInput) (*CedarBundle, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if previous == nil && len(added) == 0 {
		return nil, fmt.Errorf("cannot build incremental bundle: no previous bundle and no additions")
	}

	// Remove policies that match removed list
	filtered := make([]PolicyInput, 0, len(b.policies))
	for _, p := range b.policies {
		if !containsPolicy(removed, p) {
			filtered = append(filtered, p)
		}
	}

	// Add new policies
	filtered = append(filtered, added...)
	b.policies = filtered

	if len(b.policies) == 0 {
		return nil, fmt.Errorf("cannot build bundle: no policies remaining after incremental update")
	}

	compiled, err := b.compiler.Compile(b.policies)
	if err != nil {
		return nil, fmt.Errorf("incremental bundle compile failed: %w", err)
	}

	b.version++
	return &CedarBundle{
		Hash:        computeHash(compiled),
		Version:     b.version,
		Policies:    compiled,
		PolicyCount: len(b.policies),
	}, nil
}

// computeHash returns the SHA-256 hex digest of the given text.
func computeHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

// containsPolicy checks if a policy matches any entry in the list.
func containsPolicy(list []PolicyInput, p PolicyInput) bool {
	for _, item := range list {
		if item.Subject == p.Subject && item.Action == p.Action &&
			item.Resource == p.Resource && item.Effect == p.Effect {
			return true
		}
	}
	return false
}
