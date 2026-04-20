// Package main is the entry point for the Claude Code CLI.
// It delegates immediately to internal/bootstrap after handling any
// zero-dependency fast-path flags (--version, etc.).
package main

import (
	"fmt"
	"os"

	"github.com/tunsuy/claude-code-go/internal/bootstrap"
)

func main() {
	// Phase 0: handle fast-path flags without initialising cobra or any
	// other dependency.  Returns true if the process should exit cleanly.
	if bootstrap.HandleFastPath(os.Args) {
		return
	}

	// Phase 1–6: full initialisation + cobra root command execution.
	if err := bootstrap.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
