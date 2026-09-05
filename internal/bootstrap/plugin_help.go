package bootstrap

// Oracle-verified help texts for the `claude plugin` command tree (claude
// v2.1.261, non-TTY 80-column rendering).  These are byte-exact literals —
// including the wrapped lines and the two-space continuation indent inside the
// Commands columns — and must not be reflowed.
//
// Documented divergence: `plugin details`, `eval`, `init`, `prune` and `tag`
// are not implemented in this CLI, so their rows are dropped from the plugin
// parent help (the oracle lists them).  Unknown-command suggestions likewise
// run over the implemented names only.

const pluginParentHelp = `Usage: claude plugin|plugins [options] [command]

Manage Claude Code plugins

Options:
  -h, --help                           Display help for command

Commands:
  disable [options] [plugin]           Disable an enabled plugin
  enable [options] <plugin>            Enable a disabled plugin
  help [command]                       display help for command
  install|i [options] <plugin>         Install a plugin from available
                                       marketplaces (use plugin@marketplace for
                                       specific marketplace)
  list [options]                       List installed plugins
  marketplace                          Manage Claude Code marketplaces
  uninstall|remove [options] <plugin>  Uninstall an installed plugin
  update [options] <plugin>            Update a plugin to the latest version
                                       (restart required to apply)
  validate [options] <path>            Validate a plugin or marketplace
                                       manifest, or the skills, agents, and
                                       commands in a directory
`

const pluginListHelp = `Usage: claude plugin list [options]

List installed plugins

Options:
  --available  Include available plugins from marketplaces (requires --json)
  -h, --help   Display help for command
  --json       Output as JSON
`

const pluginInstallHelp = `Usage: claude plugin install|i [options] <plugin>

Install a plugin from available marketplaces (use plugin@marketplace for
specific marketplace)

Options:
  --config <key=value>  Set a userConfig option declared in the plugin's
                        manifest (repeatable). Values are validated against the
                        schema and stored via the same path as the interactive
                        /plugin configure flow.
  -h, --help            Display help for command
  -s, --scope <scope>   Installation scope: user, project, or local (default:
                        "user")
  -y, --yes             Accept the displayed marketplace-declared command
                        without the confirmation prompt — a plugin installed by
                        running a command, or one whose archive is fetched
                        through a headersHelper command (required when stdin or
                        stdout is not a TTY)
`

const pluginUninstallHelp = `Usage: claude plugin uninstall|remove [options] <plugin>

Uninstall an installed plugin

Options:
  -h, --help           Display help for command
  --keep-data          Preserve the plugin's persistent data directory
                       (~/.claude/plugins/data/{id}/)
  --prune              Also remove auto-installed dependencies that are no
                       longer needed (requires -y in non-interactive contexts)
  -s, --scope <scope>  Uninstall from scope: user, project, or local (default:
                       "user")
  -y, --yes            Skip the --prune confirmation prompt (required when stdin
                       or stdout is not a TTY)
`

const pluginEnableHelp = `Usage: claude plugin enable [options] <plugin>

Enable a disabled plugin

Options:
  -h, --help           Display help for command
  -s, --scope <scope>  Installation scope: user, project, local (default:
                       auto-detect)
`

const pluginDisableHelp = `Usage: claude plugin disable [options] [plugin]

Disable an enabled plugin

Options:
  -a, --all            Disable all enabled plugins
  -h, --help           Display help for command
  -s, --scope <scope>  Installation scope: user, project, local (default:
                       auto-detect)
`

const pluginUpdateHelp = `Usage: claude plugin update [options] <plugin>

Update a plugin to the latest version (restart required to apply)

Options:
  -h, --help           Display help for command
  -s, --scope <scope>  Installation scope: user, project, local, managed
                       (default: user)
  -y, --yes            Accept the displayed marketplace-declared command without
                       the confirmation prompt — a changed install command, or
                       the headersHelper command that fetches its archive
                       (required when stdin or stdout is not a TTY)
`

const pluginValidateHelp = `Usage: claude plugin validate [options] <path>

Validate a plugin or marketplace manifest, or the skills, agents, and commands
in a directory

Options:
  -h, --help  Display help for command
  --json      Output the validation report as JSON (same exit codes)
  --strict    Treat warnings as errors (exit 1). Use in CI to fail on
              unrecognized fields, missing metadata, and other issues that the
              runtime tolerates.
`

const pluginMarketplaceHelp = `Usage: claude plugin marketplace [options] [command]

Manage Claude Code marketplaces

Options:
  -h, --help                  Display help for command

Commands:
  add [options] <source>      Add a marketplace from a URL, path, or GitHub repo
  help [command]              display help for command
  list [options]              List all configured marketplaces
  remove|rm [options] <name>  Remove a configured marketplace
  update [options] [name]     Update marketplace(s) from their source - updates
                              all if no name specified
`

const pluginMPAddHelp = `Usage: claude plugin marketplace add [options] <source>

Add a marketplace from a URL, path, or GitHub repo

Options:
  --claudeai           Add the marketplace of this name that claude.ai hosts for
                       you, by its listed name or its local name (see: claude
                       plugin marketplace list)
  -h, --help           Display help for command
  --scope <scope>      Where to declare the marketplace: user (default),
                       project, or local
  --sparse <paths...>  Limit checkout to specific directories via git
                       sparse-checkout (for monorepos). Example: --sparse
                       .claude-plugin plugins
`

const pluginMPListHelp = `Usage: claude plugin marketplace list [options]

List all configured marketplaces

Options:
  -h, --help  Display help for command
  --json      Output as JSON
`

const pluginMPRemoveHelp = `Usage: claude plugin marketplace remove|rm [options] <name>

Remove a configured marketplace

Options:
  -h, --help       Display help for command
  --scope <scope>  Remove the marketplace declaration from a specific settings
                   scope: user, project, or local. Omit to remove it from every
                   scope.
`

const pluginMPUpdateHelp = `Usage: claude plugin marketplace update [options] [name]

Update marketplace(s) from their source - updates all if no name specified

Options:
  -h, --help  Display help for command
`

// pluginHelpTopics maps `plugin help <topic>` to its help text.  "help" itself
// is deliberately absent: `plugin help help` shows the parent help on stderr
// (oracle-verified, same as the mcp tree).  "marketplace" shows the marketplace
// group help.
var pluginHelpTopics = map[string]string{
	"disable":     pluginDisableHelp,
	"enable":      pluginEnableHelp,
	"install":     pluginInstallHelp,
	"list":        pluginListHelp,
	"marketplace": pluginMarketplaceHelp,
	"uninstall":   pluginUninstallHelp,
	"update":      pluginUpdateHelp,
	"validate":    pluginValidateHelp,
}

// pluginMPHelpTopics maps `plugin marketplace help <topic>`.
var pluginMPHelpTopics = map[string]string{
	"add":    pluginMPAddHelp,
	"list":   pluginMPListHelp,
	"remove": pluginMPRemoveHelp,
	"update": pluginMPUpdateHelp,
}
