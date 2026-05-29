package cedar

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// DecisionEngine evaluates authorization requests against loaded policies.
type DecisionEngine interface {
	Evaluate(ctx context.Context, request DecisionRequest) (*DecisionResponse, error)
}

// DecisionRequest represents an authorization query.
type DecisionRequest struct {
	Principal string
	Action    string
	Resource  string
	Context   map[string]interface{}
}

// DecisionResponse represents the result of a policy evaluation.
type DecisionResponse struct {
	Decision        string   // "allow" or "deny"
	MatchedPolicies []string // IDs/descriptions of matched policies
	Reason          string
}

// policyRule is an internal parsed representation of a Cedar policy statement.
type policyRule struct {
	Effect    string // "permit" or "forbid"
	Principal string
	Action    string
	Resource  string
}

// CedarEngine implements DecisionEngine using a simple rule-matching engine
// that parses compiled Cedar policy text (permit/forbid statements).
type CedarEngine struct {
	mu       sync.RWMutex
	policies []policyRule
}

// NewCedarEngine creates a new CedarEngine instance.
func NewCedarEngine() *CedarEngine {
	return &CedarEngine{}
}

// LoadPolicies parses Cedar policy text and loads the rules into the engine.
func (e *CedarEngine) LoadPolicies(cedarText string) error {
	rules, err := parsePolicies(cedarText)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.policies = rules
	e.mu.Unlock()
	return nil
}

// Evaluate checks the request against loaded policies.
// Deny (forbid) policies take precedence over allow (permit) policies.
// Default decision is deny when no matching policy is found.
func (e *CedarEngine) Evaluate(_ context.Context, req DecisionRequest) (*DecisionResponse, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matched []policyRule
	for _, p := range e.policies {
		if matchesRule(p, req) {
			matched = append(matched, p)
		}
	}

	if len(matched) == 0 {
		return &DecisionResponse{
			Decision: "deny",
			Reason:   "no matching policy found (default deny)",
		}, nil
	}

	// Forbid takes precedence over permit
	for _, m := range matched {
		if m.Effect == "forbid" {
			return &DecisionResponse{
				Decision:        "deny",
				MatchedPolicies: ruleDescriptions(matched),
				Reason:          "explicit forbid policy matched",
			}, nil
		}
	}

	return &DecisionResponse{
		Decision:        "allow",
		MatchedPolicies: ruleDescriptions(matched),
		Reason:          "permit policy matched",
	}, nil
}

// matchesRule checks whether a request matches a policy rule.
func matchesRule(rule policyRule, req DecisionRequest) bool {
	if !entityMatches(rule.Principal, req.Principal) {
		return false
	}
	if !entityMatches(rule.Action, req.Action) {
		return false
	}
	if !entityMatches(rule.Resource, req.Resource) {
		return false
	}
	return true
}

// entityMatches checks if a policy entity constraint matches the request value.
func entityMatches(pattern, value string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return pattern == value
}

// ruleDescriptions returns a description slice of matched rules.
func ruleDescriptions(rules []policyRule) []string {
	descs := make([]string, len(rules))
	for i, r := range rules {
		descs[i] = fmt.Sprintf("%s(principal=%s, action=%s, resource=%s)", r.Effect, r.Principal, r.Action, r.Resource)
	}
	return descs
}

// parsePolicies parses Cedar policy text into policyRule slices.
// Supports permit(...) and forbid(...) statements.
func parsePolicies(text string) ([]policyRule, error) {
	var rules []policyRule
	// Split by semicolons to separate statements
	statements := strings.Split(text, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		rule, err := parseStatement(stmt)
		if err != nil {
			continue // skip unparseable statements (e.g. when clauses without body)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// parseStatement parses a single Cedar permit/forbid statement.
func parseStatement(stmt string) (policyRule, error) {
	var rule policyRule

	if strings.HasPrefix(stmt, "permit") {
		rule.Effect = "permit"
	} else if strings.HasPrefix(stmt, "forbid") {
		rule.Effect = "forbid"
	} else {
		return rule, fmt.Errorf("unknown effect in statement: %s", stmt)
	}

	// Extract content between parentheses
	start := strings.Index(stmt, "(")
	end := strings.LastIndex(stmt, ")")
	if start < 0 || end < 0 || end <= start {
		return rule, fmt.Errorf("malformed statement: missing parentheses")
	}
	body := stmt[start+1 : end]

	// Parse principal, action, resource from comma-separated clauses
	clauses := strings.Split(body, ",")
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if strings.HasPrefix(clause, "principal") {
			rule.Principal = extractEntityValue(clause)
		} else if strings.HasPrefix(clause, "action") {
			rule.Action = extractEntityValue(clause)
		} else if strings.HasPrefix(clause, "resource") {
			rule.Resource = extractEntityValue(clause)
		}
	}

	return rule, nil
}

// extractEntityValue extracts the entity value from a clause like "principal == AIK::User::alice".
// It normalizes quoted action names: AIK::Action::"invoke" → AIK::Action::invoke
func extractEntityValue(clause string) string {
	parts := strings.SplitN(clause, "==", 2)
	if len(parts) != 2 {
		return "*"
	}
	val := strings.TrimSpace(parts[1])
	// Normalize: remove embedded quotes around action names
	val = strings.ReplaceAll(val, "\"", "")
	return val
}
