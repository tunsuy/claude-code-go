package bootstrap

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newMCPCmd creates the `claude mcp` subcommand tree.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mcp",
		Short:        "Manage Model Context Protocol (MCP) server integrations",
		SilenceUsage: true,
	}

	cmd.AddCommand(
		newMCPServeCmd(),
		newMCPAddCmd(),
		newMCPRemoveCmd(),
		newMCPListCmd(),
		newMCPGetCmd(),
		newMCPAddJSONCmd(),
		newMCPAddFromClaudeDesktopCmd(),
		newMCPResetProjectChoicesCmd(),
	)
	return cmd
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start Claude as an MCP server on stdin/stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp serve: not yet implemented")
		},
	}
}

func newMCPAddCmd() *cobra.Command {
	var (
		scope     string
		transport string
		envVars   []string
	)
	cmd := &cobra.Command{
		Use:   "add <name> <command> [args...]",
		Short: "Add an MCP stdio server",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = scope
			_ = transport
			_ = envVars
			return fmt.Errorf("mcp add: not yet implemented")
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "local", "Config scope: local | project | user")
	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport: stdio | sse | http")
	cmd.Flags().StringArrayVar(&envVars, "env", nil, "Environment variables KEY=VALUE")
	return cmd
}

func newMCPRemoveCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an MCP server configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = scope
			return fmt.Errorf("mcp remove: not yet implemented")
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "local", "Config scope: local | project | user")
	return cmd
}

func newMCPListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured MCP servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp list: not yet implemented")
		},
	}
}

func newMCPGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show details for a single MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp get: not yet implemented")
		},
	}
}

func newMCPAddJSONCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "add-json <name> <json>",
		Short: "Add an MCP server from a raw JSON definition",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = scope
			return fmt.Errorf("mcp add-json: not yet implemented")
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "local", "Config scope: local | project | user")
	return cmd
}

func newMCPAddFromClaudeDesktopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-from-claude-desktop",
		Short: "Import MCP servers from the Claude desktop app configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp add-from-claude-desktop: not yet implemented")
		},
	}
}

func newMCPResetProjectChoicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-project-choices",
		Short: "Reset project-level MCP server approval choices",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp reset-project-choices: not yet implemented")
		},
	}
}
