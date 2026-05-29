// Package cmd implements the cobra command tree for aikctl.
package cmd

import (
	"fmt"
	"os"

	"github.com/ai-keeper/ai-keeper/internal/cli"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "aikctl",
	Short: "AIP command-line tool",
	Long:  `aikctl is the CLI for managing AI Platform resources. It reuses kubeconfig and RBAC.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(describeCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(promoteCmd)
	rootCmd.AddCommand(rollbackCmd)
}

// applyCmd is a thin wrapper (P0 stub delegates to kubectl).
var applyCmd = &cobra.Command{
	Use:   "apply -f <file|dir>",
	Short: "Apply AIP resources from YAML files",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(os.Stderr, "aikctl apply: not yet implemented (use kubectl apply for now)")
		return nil
	},
}

// getCmd is a thin wrapper (P0 stub).
var getCmd = &cobra.Command{
	Use:   "get <resource> [name]",
	Short: "Get AIP resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(os.Stderr, "aikctl get: not yet implemented (use kubectl get for now)")
		return nil
	},
}

// describeCmd is a thin wrapper (P0 stub).
var describeCmd = &cobra.Command{
	Use:   "describe <resource> <name>",
	Short: "Describe an AIP resource",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(os.Stderr, "aikctl describe: not yet implemented (use kubectl describe for now)")
		return nil
	},
}

// deleteCmd is a thin wrapper (P0 stub).
var deleteCmd = &cobra.Command{
	Use:   "delete <resource> <name>",
	Short: "Delete an AIP resource",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(os.Stderr, "aikctl delete: not yet implemented (use kubectl delete for now)")
		return nil
	},
}

// promoteCmd promotes a Skill from beta to stable after eval gate validation.
// Requirements: A9.3
var promoteCmd = &cobra.Command{
	Use:   "promote <skill>",
	Short: "Promote a Skill from beta to stable",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ns, _ := cmd.Flags().GetString("namespace")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		opts := cli.PromoteOptions{
			SkillName: args[0],
			Namespace: ns,
			DryRun:    dryRun,
		}
		// TODO: inject real K8s SkillClient once cluster access is wired
		fmt.Fprintf(os.Stderr, "aikctl promote: K8s client not yet wired (opts: %+v)\n", opts)
		return nil
	},
}

// rollbackCmd rolls back an Agent to its last successful revision.
// Requirements: A9.4
var rollbackCmd = &cobra.Command{
	Use:   "rollback <agent>",
	Short: "Rollback an Agent to previous successful revision",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ns, _ := cmd.Flags().GetString("namespace")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		opts := cli.RollbackOptions{
			AgentName: args[0],
			Namespace: ns,
			DryRun:    dryRun,
		}
		// TODO: inject real K8s AgentClient once cluster access is wired
		fmt.Fprintf(os.Stderr, "aikctl rollback: K8s client not yet wired (opts: %+v)\n", opts)
		return nil
	},
}

func init() {
	promoteCmd.Flags().StringP("namespace", "n", "default", "Namespace of the Skill")
	promoteCmd.Flags().Bool("dry-run", false, "Only print what would happen")
	rollbackCmd.Flags().StringP("namespace", "n", "default", "Namespace of the Agent")
	rollbackCmd.Flags().Bool("dry-run", false, "Only print what would happen")
}
