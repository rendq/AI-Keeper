package modelrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	sharedv1alpha1 "github.com/ai-keeper/ai-keeper/api/shared/v1alpha1"
)

// CompiledRule is the runtime representation of a single
// [modelv1alpha1.ModelRouterRule]. The compiled form keeps the
// predicate as opaque payload — task 11.1 will plug in a real CEL
// evaluator. For P0 we just persist the textual expression and the
// structural matchers so the router can dispatch deterministically.
type CompiledRule struct {
	// When carries the structural predicates copied verbatim from the
	// CR. Nil iff `spec.rules[].when` is omitted.
	When *modelv1alpha1.ModelRouterRuleWhen `json:"when,omitempty"`

	// Endpoint is the canonical ResourceRef of the target endpoint.
	Endpoint sharedv1alpha1.ResourceRef `json:"endpoint"`

	// Weight is the routing weight (1-100). 0 means "use default".
	Weight int32 `json:"weight,omitempty"`
}

// Cache mirrors the `spec.cache` block so the runtime can apply the
// same TTL / similarity threshold as the operator declared.
type Cache struct {
	Enabled             bool                        `json:"enabled,omitempty"`
	Mode                string                      `json:"mode,omitempty"`
	TTL                 string                      `json:"ttl,omitempty"`
	SimilarityThreshold float64                     `json:"similarityThreshold,omitempty"`
	Ref                 *sharedv1alpha1.ResourceRef `json:"ref,omitempty"`
}

// RoutingTable is the compiled artefact pushed to every Model_Router
// instance. The Hash is computed over the canonical JSON encoding so
// the runtime can short-circuit unchanged tables.
type RoutingTable struct {
	// Alias mirrors `spec.alias`.
	Alias string `json:"alias"`

	// DefaultEndpoint mirrors `spec.defaultEndpoint`.
	DefaultEndpoint *sharedv1alpha1.ResourceRef `json:"defaultEndpoint,omitempty"`

	// Rules is the ordered set of compiled rules.
	Rules []CompiledRule `json:"rules"`

	// LoadBalancing mirrors `spec.loadBalancing`.
	LoadBalancing string `json:"loadBalancing,omitempty"`

	// Cache mirrors `spec.cache`.
	Cache *Cache `json:"cache,omitempty"`

	// Hash is the lower-case hex-encoded sha256 of the canonical
	// JSON encoding of the rest of the struct (Hash itself excluded).
	Hash string `json:"hash"`
}

// CompileRoutingTable turns a ModelRouter CR into a [RoutingTable].
// The function is pure: same input → same Hash, same JSON. It does
// NOT validate whether the referenced endpoints exist; that is the
// caller's responsibility.
func CompileRoutingTable(mr *modelv1alpha1.ModelRouter) (*RoutingTable, error) {
	if mr == nil {
		return nil, fmt.Errorf("modelrouter: nil CR")
	}
	rules := make([]CompiledRule, 0, len(mr.Spec.Rules))
	for _, raw := range mr.Spec.Rules {
		cr := CompiledRule{Endpoint: raw.Endpoint}
		if raw.When != nil {
			cr.When = raw.When.DeepCopy()
		}
		if raw.Weight != nil {
			cr.Weight = *raw.Weight
		}
		rules = append(rules, cr)
	}

	table := &RoutingTable{
		Alias:           mr.Spec.Alias,
		DefaultEndpoint: mr.Spec.DefaultEndpoint,
		Rules:           rules,
		LoadBalancing:   mr.Spec.LoadBalancing,
	}
	if mr.Spec.Cache != nil {
		c := &Cache{
			Mode: mr.Spec.Cache.Mode,
			Ref:  mr.Spec.Cache.Ref,
		}
		if mr.Spec.Cache.Enabled != nil {
			c.Enabled = *mr.Spec.Cache.Enabled
		}
		if mr.Spec.Cache.TTL != nil {
			c.TTL = string(*mr.Spec.Cache.TTL)
		}
		if mr.Spec.Cache.SimilarityThreshold != nil {
			c.SimilarityThreshold = *mr.Spec.Cache.SimilarityThreshold
		}
		table.Cache = c
	}

	hash, err := canonicalHash(table)
	if err != nil {
		return nil, fmt.Errorf("modelrouter: hash: %w", err)
	}
	table.Hash = hash
	return table, nil
}

// canonicalHash returns the lower-case hex-encoded sha256 of the
// canonical JSON encoding of `t` with the Hash field cleared. JSON
// is canonical-ish for our purposes: Go's encoding/json sorts map
// keys and we never use non-deterministic types in this struct.
func canonicalHash(t *RoutingTable) (string, error) {
	if t == nil {
		return "", fmt.Errorf("nil routing table")
	}
	clone := *t
	clone.Hash = ""
	buf, err := json.Marshal(&clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}
