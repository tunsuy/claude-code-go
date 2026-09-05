package bootstrap

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// The `claude plugin` command tree, built the same way as the `claude mcp`
// tree (mcp.go): every command disables cobra's flag parsing and receives the
// raw argv tail, which parseMCPFlags (mcp_parse.go) walks with commander.js
// semantics.  The dispatch layer prints failures itself and returns ErrSilent
// so main exits 1 without a prefix.  The mcp-prefixed error types are shared
// commander-emulation plumbing, not mcp-specific — plugin reuses them.
//
// Documented divergence: `plugin details`, `eval`, `init`, `prune` and `tag`
// are not implemented; their help rows are dropped and unknown-command
// suggestions run over the implemented names only (plugin_help.go).

// newPluginCmd creates the `claude plugin` subcommand tree.
func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "plugin",
		Aliases:            []string{"plugins"},
		Short:              "Manage Claude Code plugins",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginParent(cmd, args)
		},
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
		newPluginHelpCmd(),
	)
	return cmd
}

// pluginDepsBuilder builds the deps bundle from a command's streams.  Swapped
// in tests to pin the home/project directories.
var pluginDepsBuilder = newPluginDepsWith

// pluginDispatchRun wires one subcommand: build deps from the command's
// streams, run fn, then translate a typed failure into the oracle's reporting
// convention (stderr, no "Error:" prefix, silent exit 1).
func pluginDispatchRun(cmd *cobra.Command, args []string, fn func(*pluginDeps, []string) error) error {
	d, err := pluginDepsBuilder(cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if err := fn(d, args); err != nil {
		return pluginReportError(d, err)
	}
	return nil
}

// pluginReportError prints a run failure the way the oracle does and returns
// ErrSilent for a silent exit 1.  Internal (untyped) errors pass through for
// the generic handler.
func pluginReportError(d *pluginDeps, err error) error {
	switch e := err.(type) {
	case *mcpUsageError:
		fmt.Fprintln(d.Stderr, e.Line1)
		if e.Line2 != "" {
			fmt.Fprintln(d.Stderr, e.Line2)
		}
	case *mcpMissingArgError:
		fmt.Fprintln(d.Stderr, e.Error())
	case *errMCPAction:
		fmt.Fprintln(d.Stderr, e.msg)
	case *errMCPExit:
		// Diagnostics were already printed; only the exit code matters.
	default:
		return err
	}
	return ErrSilent
}

// pluginCommandNames lists the subcommand names for unknown-command
// suggestions — implemented commands only (see the divergence note above).
var pluginCommandNames = []string{
	"disable", "enable", "help", "install", "list", "marketplace",
	"uninstall", "update", "validate",
}

// pluginUnknownCommandError builds the commander unknown-command error with
// its "(Did you mean …?)" suggestion line.
func pluginUnknownCommandError(name string) error {
	e := &mcpUsageError{Line1: fmt.Sprintf("error: unknown command '%s'", name)}
	if s := suggestSimilar(name, pluginCommandNames); len(s) > 0 {
		e.Line2 = "(Did you mean " + strings.Join(s, " or ") + "?)"
	}
	return e
}

// pluginSubcommands maps subcommand names to run functions for the parent's
// post-`--` dispatch (cobra matches pre-`--` names itself).  Oracle-verified:
// `plugin -- list` runs list.
var pluginSubcommands = map[string]func(*pluginDeps, []string) error{
	"disable":     runPluginDisable,
	"enable":      runPluginEnable,
	"help":        runPluginHelpArgs,
	"install":     runPluginInstall,
	"list":        runPluginList,
	"marketplace": runPluginMPParentBody,
	"uninstall":   runPluginUninstall,
	"update":      runPluginUpdate,
	"validate":    runPluginValidate,
}

// runPluginParent handles the `plugin` command itself: bare `plugin` prints
// the parent help to stderr and exits 1; `-h` prints it to stdout; an unknown
// first positional is a commander unknown-command error; tokens after `--`
// still dispatch to the named subcommand.
func runPluginParent(cmd *cobra.Command, args []string) error {
	d, err := pluginDepsBuilder(cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	p, perr := parseMCPFlags(args, pluginParentFlagSpecs)
	if perr != nil {
		return pluginReportError(d, perr)
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginParentHelp)
		return nil
	}
	if len(p.Positionals) > 0 {
		name := p.Positionals[0]
		if fn, ok := pluginSubcommands[name]; ok {
			if err := fn(d, p.Positionals[1:]); err != nil {
				return pluginReportError(d, err)
			}
			return nil
		}
		return pluginReportError(d, pluginUnknownCommandError(name))
	}
	fmt.Fprint(d.Stderr, pluginParentHelp)
	return ErrSilent
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "list [options]",
		Short:              "List installed plugins",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginList)
		},
	}
}

func newPluginInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "install [options] <plugin>",
		Aliases:            []string{"i"},
		Short:              "Install a plugin from available marketplaces",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginInstall)
		},
	}
}

func newPluginUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "uninstall [options] <plugin>",
		Aliases:            []string{"remove"},
		Short:              "Uninstall an installed plugin",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginUninstall)
		},
	}
}

func newPluginEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "enable [options] <plugin>",
		Short:              "Enable a disabled plugin",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginEnable)
		},
	}
}

func newPluginDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "disable [options] [plugin]",
		Short:              "Disable an enabled plugin",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginDisable)
		},
	}
}

func newPluginUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "update [options] <plugin>",
		Short:              "Update a plugin to the latest version (restart required to apply)",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginUpdate)
		},
	}
}

func newPluginValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "validate [options] <path>",
		Short:              "Validate a plugin or marketplace manifest, or the skills, agents, and commands in a directory",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginValidate)
		},
	}
}

func newPluginHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "help [command]",
		Short:              "display help for command",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginHelpArgs)
		},
	}
}

// runPluginHelpArgs implements `plugin help [command]`.  Every argument is a
// topic name — "-h" included.  Oracle-verified: no args → parent help on
// stdout exit 0; known topic (incl. "marketplace") → topic help on stdout;
// unknown topic or "help" → parent help on stderr exit 1.
func runPluginHelpArgs(d *pluginDeps, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(d.Stdout, pluginParentHelp)
		return nil
	}
	if help, ok := pluginHelpTopics[args[0]]; ok {
		fmt.Fprint(d.Stdout, help)
		return nil
	}
	fmt.Fprint(d.Stderr, pluginParentHelp)
	return ErrSilent
}

// --- plugin marketplace subtree ----------------------------------------------

// newPluginMarketplaceCmd creates the `plugin marketplace` group.
func newPluginMarketplaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "marketplace [options] [command]",
		Short:              "Manage Claude Code marketplaces",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginMPParentBody)
		},
	}
	cmd.AddCommand(
		newPluginMPAddCmd(),
		newPluginMPListCmd(),
		newPluginMPRemoveCmd(),
		newPluginMPUpdateCmd(),
		newPluginMPHelpCmd(),
	)
	return cmd
}

// pluginMPCommandNames lists the marketplace subcommands for suggestions.
var pluginMPCommandNames = []string{"add", "help", "list", "remove", "update"}

func pluginMPUnknownCommandError(name string) error {
	e := &mcpUsageError{Line1: fmt.Sprintf("error: unknown command '%s'", name)}
	if s := suggestSimilar(name, pluginMPCommandNames); len(s) > 0 {
		e.Line2 = "(Did you mean " + strings.Join(s, " or ") + "?)"
	}
	return e
}

// pluginMPSubcommands maps marketplace subcommand names for the group's
// post-`--` dispatch.
var pluginMPSubcommands = map[string]func(*pluginDeps, []string) error{
	"add":    runPluginMPAdd,
	"help":   runPluginMPHelpArgs,
	"list":   runPluginMPList,
	"remove": runPluginMPRemove,
	"update": runPluginMPUpdate,
}

// runPluginMPParentBody handles the `plugin marketplace` group itself, with
// the same dispatch shape as runPluginParent but over the marketplace
// subcommands (oracle-verified: `plugin marketplace lst` suggests list).
func runPluginMPParentBody(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginMPParentFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginMarketplaceHelp)
		return nil
	}
	if len(p.Positionals) > 0 {
		name := p.Positionals[0]
		if fn, ok := pluginMPSubcommands[name]; ok {
			if err := fn(d, p.Positionals[1:]); err != nil {
				return err
			}
			return nil
		}
		return pluginMPUnknownCommandError(name)
	}
	fmt.Fprint(d.Stderr, pluginMarketplaceHelp)
	return ErrSilent
}

func newPluginMPAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "add [options] <source>",
		Short:              "Add a marketplace from a URL, path, or GitHub repo",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginMPAdd)
		},
	}
}

func newPluginMPListCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "list [options]",
		Short:              "List all configured marketplaces",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginMPList)
		},
	}
}

func newPluginMPRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "remove [options] <name>",
		Aliases:            []string{"rm"},
		Short:              "Remove a configured marketplace",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginMPRemove)
		},
	}
}

func newPluginMPUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "update [options] [name]",
		Short:              "Update marketplace(s) from their source - updates all if no name specified",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginMPUpdate)
		},
	}
}

func newPluginMPHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "help [command]",
		Short:              "display help for command",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pluginDispatchRun(cmd, args, runPluginMPHelpArgs)
		},
	}
}

// runPluginMPHelpArgs implements `plugin marketplace help [command]`.
// Oracle-verified: no args → marketplace help on stdout exit 0; known topic →
// topic help on stdout; unknown topic → marketplace help on stderr exit 1.
func runPluginMPHelpArgs(d *pluginDeps, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(d.Stdout, pluginMarketplaceHelp)
		return nil
	}
	if help, ok := pluginMPHelpTopics[args[0]]; ok {
		fmt.Fprint(d.Stdout, help)
		return nil
	}
	fmt.Fprint(d.Stderr, pluginMarketplaceHelp)
	return ErrSilent
}
