// Package cli implements the core logic for aikctl subcommands.
//
// Requirements: A9.3
package cli

import (
	"context"
	"fmt"
)

// SkillClient abstracts K8s Skill resource operations for promote/rollback.
type SkillClient interface {
	// GetSkill retrieves the Skill resource by name in the given namespace.
	GetSkill(ctx context.Context, namespace, name string) (*SkillInfo, error)
	// UpdateSkillStability updates the Skill's stability field.
	UpdateSkillStability(ctx context.Context, namespace, name, stability string) error
}

// SkillInfo holds the subset of Skill data needed for promote decisions.
type SkillInfo struct {
	Name      string
	Namespace string
	Stability string
	// EvalPassing indicates whether the latest evaluation passed the promoteToStable gates.
	EvalPassing bool
}

// PromoteOptions configures the promote command.
type PromoteOptions struct {
	SkillName string
	Namespace string
	DryRun    bool
}

// RunPromote validates that the latest eval results pass gates, then updates
// the Skill stability from beta to stable.
//
// Returns an error if the latest evaluation didn't pass or the skill is not in beta.
func RunPromote(ctx context.Context, client SkillClient, opts PromoteOptions) error {
	if opts.SkillName == "" {
		return fmt.Errorf("skill name is required")
	}
	if opts.Namespace == "" {
		opts.Namespace = "default"
	}

	skill, err := client.GetSkill(ctx, opts.Namespace, opts.SkillName)
	if err != nil {
		return fmt.Errorf("failed to get skill %q: %w", opts.SkillName, err)
	}

	if skill.Stability != "beta" {
		return fmt.Errorf("skill %q is in %q stage; only beta skills can be promoted to stable", opts.SkillName, skill.Stability)
	}

	if !skill.EvalPassing {
		return fmt.Errorf("skill %q latest evaluation did not pass promoteToStable gates; cannot promote", opts.SkillName)
	}

	if opts.DryRun {
		fmt.Printf("dry-run: would promote skill %q from beta to stable\n", opts.SkillName)
		return nil
	}

	if err := client.UpdateSkillStability(ctx, opts.Namespace, opts.SkillName, "stable"); err != nil {
		return fmt.Errorf("failed to update skill %q stability: %w", opts.SkillName, err)
	}

	fmt.Printf("skill %q promoted from beta to stable\n", opts.SkillName)
	return nil
}
