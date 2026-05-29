// Package main is the entry point for the aikctl CLI.
//
// aikctl provides apply / get / describe / delete / lint subcommands for
// managing AIP resources. It reuses kubeconfig + RBAC for cluster access.
//
// Requirements: A9.1, A9.2
package main

import (
	"fmt"
	"os"

	"github.com/ai-keeper/ai-keeper/cmd/aikctl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
