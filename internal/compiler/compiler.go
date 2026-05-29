// Package compiler implements the Policy → OPA Rego bundle compilation
// pipeline. It takes a set of Policy CRs plus contextual caches (subjects,
// resources) and produces a self-contained OPA bundle (.tar.gz) along with
// its sha256 hash and a monotonically increasing version number.
//
// Decision algorithm (design §9.3): higher priority wins; at equal
// priority, deny wins over allow.
package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync/atomic"

	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
)

// Bundle is the compiled output of the policy compiler.
type Bundle struct {
	// Data is the raw tar.gz bytes of the OPA bundle.
	Data []byte

	// Hash is the sha256 hex digest of Data (prefixed with "sha256:").
	Hash string

	// Version is a monotonically increasing bundle version counter.
	Version int64
}

// SubjectCacheEntry represents a resolved subject entity for policy evaluation.
type SubjectCacheEntry struct {
	Kind      string
	Name      string
	Namespace string
	Labels    map[string]string
}

// ResourceIndexEntry represents a resolved resource entity for policy evaluation.
type ResourceIndexEntry struct {
	Kind           string
	Name           string
	Namespace      string
	Labels         map[string]string
	Classification string
}

// CompileInput holds all inputs needed for bundle compilation.
type CompileInput struct {
	Policies  []policyv1alpha1.Policy
	Subjects  []SubjectCacheEntry
	Resources []ResourceIndexEntry
}

// Compiler compiles Policy CRs into OPA bundles.
type Compiler struct {
	version atomic.Int64
}

// New creates a new Compiler instance.
func New() *Compiler {
	return &Compiler{}
}

// NewWithVersion creates a Compiler with a specific starting version (for testing or recovery).
func NewWithVersion(startVersion int64) *Compiler {
	c := &Compiler{}
	c.version.Store(startVersion)
	return c
}

// Compile takes a set of policies and contextual data and produces an OPA bundle.
// The decision algorithm is: higher priority wins; at same priority, deny wins.
func (c *Compiler) Compile(ctx context.Context, input CompileInput) (*Bundle, error) {
	if len(input.Policies) == 0 {
		return nil, fmt.Errorf("no policies to compile")
	}

	// Filter to only enabled policies.
	policies := filterEnabled(input.Policies)
	if len(policies) == 0 {
		return nil, fmt.Errorf("no enabled policies to compile")
	}

	// Sort policies by priority (desc) then effect (deny first at same priority).
	sortPolicies(policies)

	// Generate Rego modules for each policy.
	regoFiles, err := generateRegoModules(policies)
	if err != nil {
		return nil, fmt.Errorf("generating rego modules: %w", err)
	}

	// Generate data.json with subject/resource context and policy metadata.
	dataJSON, err := generateDataJSON(policies, input.Subjects, input.Resources)
	if err != nil {
		return nil, fmt.Errorf("generating data.json: %w", err)
	}

	// Increment version monotonically.
	version := c.version.Add(1)

	// Generate manifest.
	manifest := generateManifest(version)

	// Pack the bundle as tar.gz.
	bundleData, err := packBundle(regoFiles, dataJSON, manifest)
	if err != nil {
		return nil, fmt.Errorf("packing bundle: %w", err)
	}

	// Compute hash.
	hash := computeHash(bundleData)

	return &Bundle{
		Data:    bundleData,
		Hash:    hash,
		Version: version,
	}, nil
}

// filterEnabled returns only policies where Enabled is nil (defaults true) or explicitly true.
func filterEnabled(policies []policyv1alpha1.Policy) []policyv1alpha1.Policy {
	var result []policyv1alpha1.Policy
	for i := range policies {
		p := &policies[i]
		if p.Spec.Enabled == nil || *p.Spec.Enabled {
			result = append(result, *p)
		}
	}
	return result
}

// sortPolicies sorts by priority descending, then deny before allow at same priority.
func sortPolicies(policies []policyv1alpha1.Policy) {
	sort.SliceStable(policies, func(i, j int) bool {
		pi := getPriority(policies[i])
		pj := getPriority(policies[j])
		if pi != pj {
			return pi > pj // higher priority first
		}
		// Same priority: deny before allow
		return policies[i].Spec.Effect == "deny" && policies[j].Spec.Effect != "deny"
	})
}

// getPriority returns the priority value, defaulting to 500 if not set.
func getPriority(p policyv1alpha1.Policy) int32 {
	if p.Spec.Priority != nil {
		return *p.Spec.Priority
	}
	return 500
}

// computeHash returns the sha256 hex digest of data, prefixed with "sha256:".
func computeHash(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}
