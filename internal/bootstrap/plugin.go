package bootstrap

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newPluginCmd creates the `claude plugin` subcommand tree.
func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "plugin",
		Short:        "Manage Claude Code plugins",
		SilenceUsage: true,
	}

	cmd.AddCommand(
		newPluginListCmd(),
		newPluginInstallCmd(),
		newPluginUninstallCmd(),
		newPluginEnableCmd(),
		newPluginDisableCmd(),
		newPluginUpdateCmd(),
		newPluginValidateCmd(),
		newPluginMarketplaceCmd(),
	)
	return cmd
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin list: not yet implemented")
		},
	}
}

func newPluginInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <plugin>",
		Short: "Install a plugin by name or path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin install: not yet implemented")
		},
	}
}

func newPluginUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <plugin>",
		Short: "Uninstall a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin uninstall: not yet implemented")
		},
	}
}

func newPluginEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <plugin>",
		Short: "Enable a disabled plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin enable: not yet implemented")
		},
	}
}

func newPluginDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <plugin>",
		Short: "Disable a plugin without uninstalling it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin disable: not yet implemented")
		},
	}
}

func newPluginUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [plugin]",
		Short: "Update one or all plugins to their latest versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin update: not yet implemented")
		},
	}
}

func newPluginValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <path>",
		Short: "Validate a plugin manifest without installing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin validate: not yet implemented")
		},
	}
}

func newPluginMarketplaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "marketplace",
		Short: "Browse available plugins in the marketplace",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("plugin marketplace: not yet implemented")
		},
	}
}
