// Package cli implements the core logic for aikctl subcommands.
//
// Requirements: A9.4
package cli

import (
	"context"
	"fmt"
)

// AgentClient abstracts K8s Agent resource operations for rollback.
type AgentClient interface {
	// GetAgent retrieves the Agent resource by name in the given namespace.
	GetAgent(ctx context.Context, namespace, name string) (*AgentInfo, error)
	// RollbackAgent rolls back the Agent spec to the given revision.
	RollbackAgent(ctx context.Context, namespace, name string, revision int64) error
}

// AgentInfo holds the subset of Agent data needed for rollback decisions.
type AgentInfo struct {
	Name            string
	Namespace       string
	CurrentRevision int64
	// LastSuccessfulRevision is the revision of the last successful rollout.
	// Zero means no previous successful revision exists.
	LastSuccessfulRevision int64
}

// RollbackOptions configures the rollback command.
type RollbackOptions struct {
	AgentName string
	Namespace string
	DryRun    bool
}

// RunRollback rolls back an Agent spec to the last successful rollout revision.
//
// Returns an error if no previous successful revision exists.
func RunRollback(ctx context.Context, client AgentClient, opts RollbackOptions) error {
	if opts.AgentName == "" {
		return fmt.Errorf("agent name is required")
	}
	if opts.Namespace == "" {
		opts.Namespace = "default"
	}

	agent, err := client.GetAgent(ctx, opts.Namespace, opts.AgentName)
	if err != nil {
		return fmt.Errorf("failed to get agent %q: %w", opts.AgentName, err)
	}

	if agent.LastSuccessfulRevision == 0 {
		return fmt.Errorf("agent %q has no previous successful revision to rollback to", opts.AgentName)
	}

	if agent.LastSuccessfulRevision == agent.CurrentRevision {
		return fmt.Errorf("agent %q is already at the last successful revision %d", opts.AgentName, agent.CurrentRevision)
	}

	if opts.DryRun {
		fmt.Printf("dry-run: would rollback agent %q from revision %d to %d\n",
			opts.AgentName, agent.CurrentRevision, agent.LastSuccessfulRevision)
		return nil
	}

	if err := client.RollbackAgent(ctx, opts.Namespace, opts.AgentName, agent.LastSuccessfulRevision); err != nil {
		return fmt.Errorf("failed to rollback agent %q: %w", opts.AgentName, err)
	}

	fmt.Printf("agent %q rolled back from revision %d to %d\n",
		opts.AgentName, agent.CurrentRevision, agent.LastSuccessfulRevision)
	return nil
}
