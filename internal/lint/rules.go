// Package lint implements the 14 aikctl lint rules defined in Requirement A9.2.
// Each rule checks a parsed YAML resource and returns violations.
package lint

import (
	"fmt"
	"strings"
	"time"
)

// Level is the severity of a lint violation.
type Level string

const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
)

// LintViolation represents a single lint rule failure.
type LintViolation struct {
	Level   Level  `json:"level"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

// LintRule is the interface that all lint rules implement.
type LintRule interface {
	// Name returns the rule identifier (e.g. "skill/has-eval-set").
	Name() string
	// Check examines a resource and returns any violations.
	Check(res *Resource) []LintViolation
}

// Resource is a simplified, YAML-parsed representation of an AIP resource
// used by lint rules. Fields are kept generic (maps/slices) so that lint
// operates on raw YAML without importing the full CRD type system.
type Resource struct {
	Kind string
	Name string
	Spec map[string]interface{}
}

// helper to dig into nested maps
func getMap(m map[string]interface{}, keys ...string) (map[string]interface{}, bool) {
	cur := m
	for _, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil, false
		}
		sub, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur = sub
	}
	return cur, true
}

func getString(m map[string]interface{}, keys ...string) (string, bool) {
	if len(keys) == 0 {
		return "", false
	}
	if len(keys) == 1 {
		v, ok := m[keys[0]]
		if !ok {
			return "", false
		}
		s, ok := v.(string)
		return s, ok
	}
	sub, ok := getMap(m, keys[:len(keys)-1]...)
	if !ok {
		return "", false
	}
	return getString(sub, keys[len(keys)-1])
}

func getBool(m map[string]interface{}, keys ...string) (bool, bool) {
	if len(keys) == 0 {
		return false, false
	}
	if len(keys) == 1 {
		v, ok := m[keys[0]]
		if !ok {
			return false, false
		}
		b, ok := v.(bool)
		return b, ok
	}
	sub, ok := getMap(m, keys[:len(keys)-1]...)
	if !ok {
		return false, false
	}
	return getBool(sub, keys[len(keys)-1])
}

func getInt(m map[string]interface{}, keys ...string) (int, bool) {
	if len(keys) == 0 {
		return 0, false
	}
	if len(keys) == 1 {
		v, ok := m[keys[0]]
		if !ok {
			return 0, false
		}
		switch n := v.(type) {
		case int:
			return n, true
		case int64:
			return int(n), true
		case float64:
			return int(n), true
		}
		return 0, false
	}
	sub, ok := getMap(m, keys[:len(keys)-1]...)
	if !ok {
		return 0, false
	}
	return getInt(sub, keys[len(keys)-1])
}

func getSlice(m map[string]interface{}, key string) ([]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	s, ok := v.([]interface{})
	return s, ok
}

// classificationAtLeast returns true if cls >= threshold in the ordering:
// public < internal < confidential < restricted < secret
func classificationAtLeast(cls, threshold string) bool {
	order := map[string]int{
		"public":       0,
		"internal":     1,
		"confidential": 2,
		"restricted":   3,
		"secret":       4,
	}
	c, ok1 := order[strings.ToLower(cls)]
	t, ok2 := order[strings.ToLower(threshold)]
	if !ok1 || !ok2 {
		return false
	}
	return c >= t
}

// AllRules returns all 14 lint rules.
func AllRules() []LintRule {
	return []LintRule{
		&SkillHasEvalSet{},
		&SkillHasFallback{},
		&SkillBudgetSet{},
		&SkillVersionBumped{},
		&AgentSkillsResolved{},
		&AgentSandboxRequired{},
		&AgentAuditMinLevel{},
		&PolicyNoConflict{},
		&PolicySanePriority{},
		&PolicyEffectiveWindow{},
		&ToolDestructiveNeedsApproval{},
		&ModelDPARequired{},
		&KBACLNotOpen{},
		&KBPostFilterWarn{},
	}
}

// RunAll runs all 14 lint rules against a resource and returns violations.
func RunAll(res *Resource) []LintViolation {
	var violations []LintViolation
	for _, rule := range AllRules() {
		violations = append(violations, rule.Check(res)...)
	}
	return violations
}

// --- Rule 1: skill/has-eval-set ---

type SkillHasEvalSet struct{}

func (r *SkillHasEvalSet) Name() string { return "skill/has-eval-set" }

func (r *SkillHasEvalSet) Check(res *Resource) []LintViolation {
	if res.Kind != "Skill" {
		return nil
	}
	stability, _ := getString(res.Spec, "stability")
	if stability != "stable" {
		return nil
	}
	eval, hasEval := getMap(res.Spec, "evaluation")
	if !hasEval {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "Skill with stability=stable must have evaluation.evalSet configured",
			Path:    "spec.evaluation.evalSet",
		}}
	}
	if _, hasEvalSet := eval["evalSet"]; !hasEvalSet {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "Skill with stability=stable must have evaluation.evalSet configured",
			Path:    "spec.evaluation.evalSet",
		}}
	}
	return nil
}

// --- Rule 2: skill/has-fallback ---

type SkillHasFallback struct{}

func (r *SkillHasFallback) Name() string { return "skill/has-fallback" }

func (r *SkillHasFallback) Check(res *Resource) []LintViolation {
	if res.Kind != "Skill" {
		return nil
	}
	stability, _ := getString(res.Spec, "stability")
	if stability != "stable" && stability != "beta" {
		return nil
	}
	rel, hasRel := getMap(res.Spec, "reliability")
	if !hasRel {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "Production Skill should have reliability.fallback configured",
			Path:    "spec.reliability.fallback",
		}}
	}
	if _, hasFb := rel["fallback"]; !hasFb {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "Production Skill should have reliability.fallback configured",
			Path:    "spec.reliability.fallback",
		}}
	}
	return nil
}

// --- Rule 3: skill/budget-set ---

type SkillBudgetSet struct{}

func (r *SkillBudgetSet) Name() string { return "skill/budget-set" }

func (r *SkillBudgetSet) Check(res *Resource) []LintViolation {
	if res.Kind != "Skill" {
		return nil
	}
	cost, hasCost := getMap(res.Spec, "cost")
	if !hasCost {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "Skill should have cost.budget configured",
			Path:    "spec.cost.budget",
		}}
	}
	if _, hasBudget := cost["budget"]; !hasBudget {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "Skill should have cost.budget configured",
			Path:    "spec.cost.budget",
		}}
	}
	return nil
}

// --- Rule 4: skill/version-bumped ---
// This rule checks if there's a previous version provided in context.
// In a lint scenario, we compare the resource against an optional "previous"
// snapshot. For simplicity, we store the previous spec hash externally.
// Here we implement: if PreviousSpec is set and differs from current spec,
// and version is unchanged, it's a violation.

type SkillVersionBumped struct{}

func (r *SkillVersionBumped) Name() string { return "skill/version-bumped" }

func (r *SkillVersionBumped) Check(res *Resource) []LintViolation {
	if res.Kind != "Skill" {
		return nil
	}
	// The rule needs a "previous" version to compare. If the resource
	// carries a metadata annotation "ai-keeper.io/previous-version" and the
	// current version equals it while "ai-keeper.io/spec-changed" is "true",
	// that's a violation. This models the CI lint workflow.
	_, hasVersion := getString(res.Spec, "version")
	if !hasVersion {
		return nil
	}
	// Check for a special lint context field indicating spec changed
	// without version bump (provided by caller).
	if res.Spec["_lintSpecChanged"] == true {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "spec change must bump version",
			Path:    "spec.version",
		}}
	}
	return nil
}

// --- Rule 5: agent/skills-resolved ---

type AgentSkillsResolved struct{}

func (r *AgentSkillsResolved) Name() string { return "agent/skills-resolved" }

func (r *AgentSkillsResolved) Check(res *Resource) []LintViolation {
	if res.Kind != "Agent" {
		return nil
	}
	skills, ok := getSlice(res.Spec, "skills")
	if !ok || len(skills) == 0 {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "Agent must have at least one skill binding",
			Path:    "spec.skills",
		}}
	}
	var violations []LintViolation
	for i, s := range skills {
		sm, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		ref, hasRef := sm["ref"]
		if !hasRef || ref == "" {
			violations = append(violations, LintViolation{
				Level:   LevelError,
				Rule:    r.Name(),
				Message: fmt.Sprintf("skills[%d].ref must be set and resolvable", i),
				Path:    fmt.Sprintf("spec.skills[%d].ref", i),
			})
		}
	}
	return violations
}

// --- Rule 6: agent/sandbox-required ---

type AgentSandboxRequired struct{}

func (r *AgentSandboxRequired) Name() string { return "agent/sandbox-required" }

func (r *AgentSandboxRequired) Check(res *Resource) []LintViolation {
	if res.Kind != "Agent" {
		return nil
	}
	runtime, ok := getMap(res.Spec, "runtime")
	if !ok {
		return nil
	}
	pattern, _ := getString(runtime, "pattern")
	if pattern != "react" {
		return nil
	}
	// Check if any skill looks like a code tool (heuristic: skill ref
	// contains "code" or there's an explicit _hasCodeTool lint hint).
	hasCodeTool := false
	if res.Spec["_hasCodeTool"] == true {
		hasCodeTool = true
	}
	// Also check skills for "code" in ref
	if skills, ok := getSlice(res.Spec, "skills"); ok {
		for _, s := range skills {
			sm, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			ref, _ := sm["ref"].(string)
			if strings.Contains(strings.ToLower(ref), "code") {
				hasCodeTool = true
				break
			}
		}
	}
	if !hasCodeTool {
		return nil
	}
	// Check sandbox enabled
	sandbox, hasSandbox := getMap(runtime, "sandbox")
	if !hasSandbox {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "Agent with pattern=react and code tool must have sandbox enabled",
			Path:    "spec.runtime.sandbox.enabled",
		}}
	}
	enabled, hasEnabled := getBool(sandbox, "enabled")
	if !hasEnabled || !enabled {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "Agent with pattern=react and code tool must have sandbox enabled",
			Path:    "spec.runtime.sandbox.enabled",
		}}
	}
	return nil
}

// --- Rule 7: agent/audit-min-level ---

type AgentAuditMinLevel struct{}

func (r *AgentAuditMinLevel) Name() string { return "agent/audit-min-level" }

func (r *AgentAuditMinLevel) Check(res *Resource) []LintViolation {
	if res.Kind != "Agent" {
		return nil
	}
	// Check governance.classification (from agent spec top level or nested)
	cls, _ := getString(res.Spec, "governance", "classification")
	if cls == "" {
		// Agent doesn't embed governance at top level; check via _classification hint
		if c, ok := res.Spec["_classification"].(string); ok {
			cls = c
		}
	}
	if !classificationAtLeast(cls, "confidential") {
		return nil
	}
	// Audit level must be at least "high"
	auditLevel, hasAudit := getString(res.Spec, "audit", "level")
	if !hasAudit {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "Agent with classification >= confidential should have audit level at least 'high'",
			Path:    "spec.audit.level",
		}}
	}
	auditOrder := map[string]int{"off": 0, "basic": 1, "high": 2, "forensic": 3}
	if auditOrder[auditLevel] < auditOrder["high"] {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "Agent with classification >= confidential should have audit level at least 'high'",
			Path:    "spec.audit.level",
		}}
	}
	return nil
}

// --- Rule 8: policy/no-conflict ---

// PolicyNoConflict checks if the resource set contains conflicting
// policies. For single-resource lint, we use a special _policies context.
type PolicyNoConflict struct{}

func (r *PolicyNoConflict) Name() string { return "policy/no-conflict" }

func (r *PolicyNoConflict) Check(res *Resource) []LintViolation {
	if res.Kind != "Policy" {
		return nil
	}
	// This rule requires multi-resource context. When run on a single
	// policy, check if _conflictWith is set (populated by the lint engine
	// when it detects same priority allow/deny on same subject+resource).
	if conflictWith, ok := res.Spec["_conflictWith"].(string); ok && conflictWith != "" {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: fmt.Sprintf("Policy conflicts with %s: same priority allow/deny on same subject+resource", conflictWith),
			Path:    "spec",
		}}
	}
	return nil
}

// --- Rule 9: policy/sane-priority ---

type PolicySanePriority struct{}

func (r *PolicySanePriority) Name() string { return "policy/sane-priority" }

func (r *PolicySanePriority) Check(res *Resource) []LintViolation {
	if res.Kind != "Policy" {
		return nil
	}
	priority, ok := getInt(res.Spec, "priority")
	if !ok {
		return nil
	}
	if priority == 0 || priority == 1000 {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: fmt.Sprintf("Extreme priority %d is not recommended; use values between 1 and 999", priority),
			Path:    "spec.priority",
		}}
	}
	return nil
}

// --- Rule 10: policy/effective-window ---

type PolicyEffectiveWindow struct{}

func (r *PolicyEffectiveWindow) Name() string { return "policy/effective-window" }

func (r *PolicyEffectiveWindow) Check(res *Resource) []LintViolation {
	if res.Kind != "Policy" {
		return nil
	}
	ew, ok := getMap(res.Spec, "effectiveWindow")
	if !ok {
		return nil
	}
	notAfterStr, ok := getString(ew, "notAfter")
	if !ok {
		return nil
	}
	notAfter, err := time.Parse(time.RFC3339, notAfterStr)
	if err != nil {
		return nil
	}
	fiveYears := time.Now().AddDate(5, 0, 0)
	if notAfter.After(fiveYears) {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "effectiveWindow.notAfter should not exceed 5 years from now",
			Path:    "spec.effectiveWindow.notAfter",
		}}
	}
	return nil
}

// --- Rule 11: tool/destructive-needs-approval ---

type ToolDestructiveNeedsApproval struct{}

func (r *ToolDestructiveNeedsApproval) Name() string { return "tool/destructive-needs-approval" }

func (r *ToolDestructiveNeedsApproval) Check(res *Resource) []LintViolation {
	if res.Kind != "Tool" {
		return nil
	}
	gov, ok := getMap(res.Spec, "governance")
	if !ok {
		return nil
	}
	sideEffects, _ := getString(gov, "sideEffects")
	if sideEffects != "destructive" {
		return nil
	}
	requiresApproval, hasRA := getBool(gov, "requiresApproval")
	if !hasRA || !requiresApproval {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "Tool with sideEffects=destructive must set requiresApproval=true",
			Path:    "spec.governance.requiresApproval",
		}}
	}
	return nil
}

// --- Rule 12: model/dpa-required ---

type ModelDPARequired struct{}

func (r *ModelDPARequired) Name() string { return "model/dpa-required" }

func (r *ModelDPARequired) Check(res *Resource) []LintViolation {
	if res.Kind != "ModelEndpoint" {
		return nil
	}
	// Check if compliance contains GDPR
	compliance, ok := getSlice(res.Spec, "compliance")
	if !ok {
		return nil
	}
	hasGDPR := false
	for _, c := range compliance {
		if s, ok := c.(string); ok && strings.EqualFold(s, "GDPR") {
			hasGDPR = true
			break
		}
	}
	if !hasGDPR {
		return nil
	}
	// privacy.dpaSigned must be true
	dpaSigned, hasDPA := getBool(res.Spec, "privacy", "dpaSigned")
	if !hasDPA || !dpaSigned {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "ModelEndpoint with GDPR compliance must have privacy.dpaSigned=true",
			Path:    "spec.privacy.dpaSigned",
		}}
	}
	return nil
}

// --- Rule 13: kb/acl-not-open ---

type KBACLNotOpen struct{}

func (r *KBACLNotOpen) Name() string { return "kb/acl-not-open" }

func (r *KBACLNotOpen) Check(res *Resource) []LintViolation {
	if res.Kind != "KnowledgeBase" {
		return nil
	}
	// Check classification >= confidential
	cls, _ := getString(res.Spec, "governance", "classification")
	if !classificationAtLeast(cls, "confidential") {
		return nil
	}
	// acl.mode cannot be "open"
	aclMode, _ := getString(res.Spec, "acl", "mode")
	if strings.EqualFold(aclMode, "open") {
		return []LintViolation{{
			Level:   LevelError,
			Rule:    r.Name(),
			Message: "KnowledgeBase with classification >= confidential cannot use acl.mode=open",
			Path:    "spec.acl.mode",
		}}
	}
	return nil
}

// --- Rule 14: kb/post-filter-warn ---

type KBPostFilterWarn struct{}

func (r *KBPostFilterWarn) Name() string { return "kb/post-filter-warn" }

func (r *KBPostFilterWarn) Check(res *Resource) []LintViolation {
	if res.Kind != "KnowledgeBase" {
		return nil
	}
	enforcement, _ := getString(res.Spec, "acl", "enforcement")
	if enforcement == "post_filter" {
		return []LintViolation{{
			Level:   LevelWarn,
			Rule:    r.Name(),
			Message: "acl.enforcement=post_filter is not recommended due to side-channel risk",
			Path:    "spec.acl.enforcement",
		}}
	}
	return nil
}
