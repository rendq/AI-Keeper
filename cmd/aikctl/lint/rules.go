package lint

// Level represents the severity of a lint finding.
type Level int

const (
	// LevelError is a blocking violation.
	LevelError Level = iota
	// LevelWarn is an advisory violation.
	LevelWarn
)

// Result is a single lint finding.
type Result struct {
	// Rule is the rule identifier (e.g. "skill/version-bumped").
	Rule string

	// Level is error or warn.
	Level Level

	// Message is the human-readable explanation.
	Message string

	// File is the source YAML file.
	File string
}

// Rule is the interface all lint rules implement.
type Rule interface {
	// Run executes the rule against the resource set and returns findings.
	Run(rs *ResourceSet) []Result
}

// allRules is the ordered list of P0 lint rules.
var allRules = []Rule{
	&SkillVersionBumped{},
	&AgentSkillsResolved{},
	&AgentSandboxRequired{},
	&PolicyNoConflict{},
	&ToolDestructiveNeedsApproval{},
}

// RunAllRules executes all registered lint rules.
func RunAllRules(rs *ResourceSet) []Result {
	var results []Result
	for _, rule := range allRules {
		results = append(results, rule.Run(rs)...)
	}
	return results
}
