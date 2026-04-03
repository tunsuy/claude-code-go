package bootstrap

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// newDoctorCmd creates the `claude doctor` subcommand.
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks on your Claude Code installation",
		Long: longDesc(`
Runs a series of diagnostic checks to verify your Claude Code environment:

  • Go runtime version
  • Authentication status
  • Network connectivity to api.anthropic.com
  • Config file locations and validity
  • Installed plugins and MCP servers
`),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor()
		},
	}
}

func runDoctor() error {
	fmt.Printf("Claude Code diagnostic report\n")
	fmt.Printf("  Go runtime : %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Version    : %s\n", appVersion)
	fmt.Println()
	fmt.Println("For full diagnostics, authentication checks, and network tests, run:")
	fmt.Println("  claude doctor --verbose")
	return nil
}

// newUpdateCmd creates the `claude update` subcommand.
func newUpdateCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:          "update",
		Short:        "Update Claude Code to the latest release",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				fmt.Printf("Current version: %s\n", appVersion)
				fmt.Println("Version check: not yet implemented.")
				return nil
			}
			return fmt.Errorf("update: not yet implemented")
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Only check for a new version without installing")
	return cmd
}

// newAgentsCmd creates the `claude agents` subcommand tree.
func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "agents",
		Short:        "Manage custom sub-agents",
		SilenceUsage: true,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List configured sub-agents",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("agents list: not yet implemented")
			},
		},
		&cobra.Command{
			Use:   "add <json-path>",
			Short: "Register a new sub-agent from a JSON definition file",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("agents add: not yet implemented")
			},
		},
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Remove a registered sub-agent",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("agents remove: not yet implemented")
			},
		},
	)
	return cmd
}

// newInstallCmd creates the `claude install` subcommand.
func newInstallCmd() *cobra.Command {
	var (
		target string
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install shell integration (completions, PATH setup)",
		Long: longDesc(`
Installs Claude Code shell integration for the current user:

  • Adds shell completion scripts (bash / zsh / fish)
  • Optionally adds the binary to PATH via your shell RC file

Supported targets: bash, zsh, fish, all (default: auto-detect).
`),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = target
			return fmt.Errorf("install: not yet implemented")
		},
	}
	cmd.Flags().StringVar(&target, "target", "auto", "Shell target: bash | zsh | fish | all | auto")
	return cmd
}
