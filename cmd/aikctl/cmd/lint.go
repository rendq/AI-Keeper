package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ai-keeper/ai-keeper/cmd/aikctl/lint"
)

var lintStrict bool

var lintCmd = &cobra.Command{
	Use:   "lint <dir>",
	Short: "Lint AIP YAML manifests for common errors",
	Long: `Lint applies static analysis rules to AIP YAML resources.
In --strict mode, any error-level violation causes a non-zero exit code.

P0 rules (error level):
  skill/version-bumped          spec changed but version not bumped
  agent/skills-resolved         skills[].ref references unresolvable skill
  agent/sandbox-required        pattern=react with code tools needs sandbox
  policy/no-conflict            hard conflicts between policies
  tool/destructive-needs-approval  sideEffects=destructive needs requiresApproval`,
	Args: cobra.ExactArgs(1),
	RunE: runLint,
}

func init() {
	lintCmd.Flags().BoolVar(&lintStrict, "strict", false, "exit non-zero on any error-level violation")
}

func runLint(cmd *cobra.Command, args []string) error {
	dir := args[0]

	// Collect YAML files from the directory.
	files, err := collectYAMLFiles(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No YAML files found in %s\n", dir)
		return nil
	}

	// Load all resources.
	resources, err := lint.LoadResources(files)
	if err != nil {
		return fmt.Errorf("loading resources: %w", err)
	}

	// Run lint rules.
	results := lint.RunAllRules(resources)

	// Print results.
	hasErrors := false
	for _, r := range results {
		level := "error"
		if r.Level == lint.LevelWarn {
			level = "warn"
		}
		fmt.Fprintf(os.Stdout, "[%s] %s: %s (%s)\n", level, r.Rule, r.Message, r.File)
		if r.Level == lint.LevelError {
			hasErrors = true
		}
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stdout, "All checks passed.")
	}

	if lintStrict && hasErrors {
		os.Exit(1)
	}

	return nil
}

func collectYAMLFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
