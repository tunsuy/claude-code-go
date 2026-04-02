// Package bootstrap initializes and wires up all application components.
package bootstrap

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "claude",
	Short: "Claude Code - AI coding assistant",
	Long:  `Claude Code is an AI-powered coding assistant that runs in your terminal.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
