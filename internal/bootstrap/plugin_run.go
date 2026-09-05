package bootstrap

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// RunE bodies for the `claude plugin` subcommands (enable/disable/update and
// list; install/uninstall live in plugin_install.go, marketplace in
// plugin_marketplace.go, validate in plugin_validate.go, rendering in
// plugin_render.go).  Each body takes a pluginDeps bundle (writers + store)
// and the raw argv tail, so the full surface is testable without a TTY or the
// real home dir.  Every user-visible string here is byte-pinned against the
// oracle (claude v2.1.261); see the probes in internal/bootstrap/CONTEXT.md.

// pluginDeps carries everything a RunE body needs.
type pluginDeps struct {
	Stdout io.Writer
	Stderr io.Writer
	Store  *config.PluginStore
}

// newPluginDepsWith builds deps over explicit streams (the cobra wiring passes
// cmd.OutOrStdout()/cmd.ErrOrStderr() so tests can capture output).
func newPluginDepsWith(stdout, stderr io.Writer) (*pluginDeps, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("plugin: get working directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("plugin: home directory: %w", err)
	}
	return &pluginDeps{
		Stdout: stdout,
		Stderr: stderr,
		Store:  config.NewPluginStore(home, cwd),
	}, nil
}

// Flag specs per subcommand, shared with the mcp tree's parser (parseMCPFlags).
// -y/--yes, --config, --keep-data and --sparse/--claudeai are accepted and
// ignored: the confirmation prompts they gate only fire on a TTY (none in this
// non-interactive build), userConfig storage is not implemented, and
// --keep-data only matters for plugins with a data directory (documented
// divergences, listed in test/parity/cases.md).

var (
	pluginScopeFlag = mcpScopeFlag // -s, --scope <scope>

	// --scope without the -s short (marketplace add/remove).
	pluginMPNoShortScopeFlag = mcpFlagSpec{Long: "scope", Kind: mcpFlagValue, Flags: "--scope <scope>"}

	pluginParentFlagSpecs   = []mcpFlagSpec{mcpHelpFlag}
	pluginMPParentFlagSpecs = []mcpFlagSpec{mcpHelpFlag}

	pluginListFlagSpecs = []mcpFlagSpec{
		{Long: "available", Kind: mcpFlagBool, Flags: "--available"},
		mcpHelpFlag,
		{Long: "json", Kind: mcpFlagBool, Flags: "--json"},
	}

	pluginInstallFlagSpecs = []mcpFlagSpec{
		{Long: "config", Kind: mcpFlagValue, Flags: "--config <key=value>"},
		mcpHelpFlag,
		pluginScopeFlag,
		{Long: "yes", Short: "y", Kind: mcpFlagBool, Flags: "-y, --yes"},
	}

	pluginUninstallFlagSpecs = []mcpFlagSpec{
		mcpHelpFlag,
		{Long: "keep-data", Kind: mcpFlagBool, Flags: "--keep-data"},
		{Long: "prune", Kind: mcpFlagBool, Flags: "--prune"},
		pluginScopeFlag,
		{Long: "yes", Short: "y", Kind: mcpFlagBool, Flags: "-y, --yes"},
	}

	pluginEnableFlagSpecs = []mcpFlagSpec{mcpHelpFlag, pluginScopeFlag}

	pluginDisableFlagSpecs = []mcpFlagSpec{
		{Long: "all", Short: "a", Kind: mcpFlagBool, Flags: "-a, --all"},
		mcpHelpFlag,
		pluginScopeFlag,
	}

	pluginUpdateFlagSpecs = []mcpFlagSpec{
		mcpHelpFlag,
		pluginScopeFlag,
		{Long: "yes", Short: "y", Kind: mcpFlagBool, Flags: "-y, --yes"},
	}

	pluginValidateFlagSpecs = []mcpFlagSpec{
		mcpHelpFlag,
		{Long: "json", Kind: mcpFlagBool, Flags: "--json"},
		{Long: "strict", Kind: mcpFlagBool, Flags: "--strict"},
	}

	pluginMPAddFlagSpecs = []mcpFlagSpec{
		{Long: "claudeai", Kind: mcpFlagBool, Flags: "--claudeai"},
		mcpHelpFlag,
		pluginMPNoShortScopeFlag,
		{Long: "sparse", Kind: mcpFlagGreedy, Flags: "--sparse <paths...>"},
	}

	pluginMPListFlagSpecs = []mcpFlagSpec{
		mcpHelpFlag,
		{Long: "json", Kind: mcpFlagBool, Flags: "--json"},
	}

	pluginMPRemoveFlagSpecs = []mcpFlagSpec{mcpHelpFlag, pluginMPNoShortScopeFlag}
	pluginMPUpdateFlagSpecs = []mcpFlagSpec{mcpHelpFlag}
)

// Required positional argument names per subcommand (oracle-verified: install,
// uninstall, enable and update all report 'plugin'; validate reports 'path';
// marketplace add reports 'source' and remove reports 'name').
const (
	pluginPositional = "plugin"
	pluginPathArg    = "path"
	pluginSourceArg  = "source"
	pluginNameArg    = "name"
)

// pluginScopeFamily selects an invalid-scope error format — the oracle words
// it differently per subcommand family (all stderr, exit 1; only marketplace's
// carries the ✘ prefix, embedded).
type pluginScopeFamily int

const (
	pluginScopeColonList   pluginScopeFamily = iota // install/uninstall
	pluginScopeQuoted                               // enable/disable
	pluginScopeUpdate                               // update (adds managed)
	pluginScopeMarketplace                          // marketplace add/remove
)

// pluginValidateScope validates a --scope word for the family.  An empty scope
// passes (callers substitute their default first).
func pluginValidateScope(scope string, family pluginScopeFamily) error {
	switch family {
	case pluginScopeColonList:
		if scope != config.PluginScopeUser && scope != config.PluginScopeProject && scope != config.PluginScopeLocal {
			return mcpActionErrorf("Invalid scope: %s. Must be one of: user, project, local.", scope)
		}
	case pluginScopeQuoted:
		if scope != config.PluginScopeUser && scope != config.PluginScopeProject && scope != config.PluginScopeLocal {
			return mcpActionErrorf("Invalid scope %q. Valid scopes: user, project, local", scope)
		}
	case pluginScopeUpdate:
		if scope != config.PluginScopeUser && scope != config.PluginScopeProject && scope != config.PluginScopeLocal && scope != "managed" {
			return mcpActionErrorf("Invalid scope %q. Valid scopes: user, project, local, managed", scope)
		}
	case pluginScopeMarketplace:
		if scope != config.PluginScopeUser && scope != config.PluginScopeProject && scope != config.PluginScopeLocal {
			return mcpActionErrorf("✘ Invalid scope '%s'. Use: user, project, or local", scope)
		}
	}
	return nil
}

// pluginEffectiveEnabled resolves a plugin id's effective enabled state at one
// scope: the settings key when present, otherwise the installed-by-default
// (true when installed anywhere, false when not).
func pluginEffectiveEnabled(d *pluginDeps, ip *config.InstalledPlugins, scope, id string) (bool, error) {
	enabled, err := d.Store.EnabledPlugins(scope)
	if err != nil {
		return false, err
	}
	if v, ok := enabled.Get(id); ok {
		b, _ := v.(bool)
		return b, nil
	}
	_, recs := ip.Find(id)
	return len(recs) > 0, nil
}

// pluginResolveScope picks the scope an enable/disable acts on when -s is
// absent: the first scope in PluginScopeOrder (local, project, user) whose
// enabledPlugins has a key for the id, else user.
func pluginResolveScope(d *pluginDeps, id string) (string, error) {
	for _, scope := range config.PluginScopeOrder {
		enabled, err := d.Store.EnabledPlugins(scope)
		if err != nil {
			return "", err
		}
		if _, ok := enabled.Get(id); ok {
			return scope, nil
		}
	}
	return config.PluginScopeUser, nil
}

// pluginSettingsKey resolves an enable/disable reference to the enabledPlugins
// key it acts on.  A `name@marketplace` form is the settings key itself —
// valid when installed under that exact id or already present as a key; a bare
// name must match an installed plugin (exact id or name part).  Returns "" on
// a miss.
func pluginSettingsKey(d *pluginDeps, ip *config.InstalledPlugins, input string) (string, error) {
	if strings.Contains(input, "@") {
		if _, recs := ip.Find(input); len(recs) > 0 {
			return input, nil
		}
		for _, scope := range config.PluginScopeOrder {
			enabled, err := d.Store.EnabledPlugins(scope)
			if err != nil {
				return "", err
			}
			if _, ok := enabled.Get(input); ok {
				return input, nil
			}
		}
		return "", nil
	}
	id, recs := ip.Find(input)
	if len(recs) == 0 {
		return "", nil
	}
	return id, nil
}

// runPluginEnable implements `plugin enable`.  Oracle-verified shapes:
// success prints the name part and the acted-on scope; the already-enabled
// failure echoes the input verbatim and only carries " at <s> scope" when -s
// was given; an unresolvable reference reports the editable-settings miss.
func runPluginEnable(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginEnableFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginEnableHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, pluginPositional); err != nil {
		return err
	}
	input := p.Positionals[0]
	explicit := p.Values["scope"]
	if explicit != "" {
		if err := pluginValidateScope(explicit, pluginScopeQuoted); err != nil {
			return err
		}
	}

	ip, err := d.Store.LoadInstalledPlugins()
	if err != nil {
		return err
	}
	id, err := pluginSettingsKey(d, ip, input)
	if err != nil {
		return err
	}
	if id == "" {
		return mcpActionErrorf("✘ Failed to enable plugin %q: Plugin %q not found in any editable settings scope. Use plugin@marketplace format.", input, input)
	}

	scope := explicit
	if scope == "" {
		if scope, err = pluginResolveScope(d, id); err != nil {
			return err
		}
	}
	enabledNow, err := pluginEffectiveEnabled(d, ip, scope, id)
	if err != nil {
		return err
	}
	if enabledNow {
		return mcpActionErrorf("✘ Failed to enable plugin %q: Plugin %q is already enabled%s", input, input, pluginAlreadyScopeSuffix(explicit))
	}
	if err := d.Store.SetPluginEnabled(scope, id, true); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "✔ Successfully enabled plugin: %s (scope: %s)\n", config.PluginNamePart(id), scope)
	return nil
}

// pluginAlreadyScopeSuffix renders the " at <s> scope" tail of an
// already-enabled/already-disabled error — present only when -s was given
// (oracle-verified both ways).
func pluginAlreadyScopeSuffix(explicit string) string {
	if explicit == "" {
		return ""
	}
	return " at " + explicit + " scope"
}

// runPluginDisable implements `plugin disable`.  Bare disable (no plugin, no
// --all) is a plain stderr message without the ✘ prefix; --all flips every
// true key across the settings scopes.
func runPluginDisable(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginDisableFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginDisableHelp)
		return nil
	}
	explicit := p.Values["scope"]
	if explicit != "" {
		if err := pluginValidateScope(explicit, pluginScopeQuoted); err != nil {
			return err
		}
	}

	if p.Bools["all"] {
		return pluginDisableAll(d)
	}
	if len(p.Positionals) == 0 {
		fmt.Fprint(d.Stderr, "Please specify a plugin name or use --all to disable all plugins\n")
		return &errMCPExit{code: 1}
	}
	input := p.Positionals[0]

	ip, err := d.Store.LoadInstalledPlugins()
	if err != nil {
		return err
	}
	id, err := pluginSettingsKey(d, ip, input)
	if err != nil {
		return err
	}
	if id == "" {
		return mcpActionErrorf("✘ Failed to disable plugin %q: Plugin %q not found in any editable settings scope. Use plugin@marketplace format.", input, input)
	}

	scope := explicit
	if scope == "" {
		if scope, err = pluginResolveScope(d, id); err != nil {
			return err
		}
	}
	enabledNow, err := pluginEffectiveEnabled(d, ip, scope, id)
	if err != nil {
		return err
	}
	if !enabledNow {
		return mcpActionErrorf("✘ Failed to disable plugin %q: Plugin %q is already disabled%s", input, input, pluginAlreadyScopeSuffix(explicit))
	}
	if err := d.Store.SetPluginEnabled(scope, id, false); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "✔ Successfully disabled plugin: %s (scope: %s)\n", config.PluginNamePart(id), scope)
	return nil
}

// pluginDisableAll flips every enabledPlugins key that is currently true,
// across all settings scopes (local, project, user), counting the flips.
// Unverified edge: the oracle's per-scope iteration order — only the
// one-plugin and zero-plugin cases are captured.
func pluginDisableAll(d *pluginDeps) error {
	count := 0
	for _, scope := range config.PluginScopeOrder {
		enabled, err := d.Store.EnabledPlugins(scope)
		if err != nil {
			return err
		}
		for _, id := range enabled.Keys() {
			if v, _ := enabled.Get(id); v != true {
				continue
			}
			if err := d.Store.SetPluginEnabled(scope, id, false); err != nil {
				return err
			}
			count++
		}
	}
	switch count {
	case 0:
		fmt.Fprint(d.Stdout, "✔ No enabled plugins to disable\n")
	case 1:
		fmt.Fprint(d.Stdout, "✔ Disabled 1 plugin\n")
	default:
		fmt.Fprintf(d.Stdout, "✔ Disabled %d plugins\n", count)
	}
	return nil
}

// runPluginUpdate implements `plugin update`.  Resolution is by name part
// (even for an id-form input); the progress line echoes the input verbatim
// with the scope word and a U+2026 ellipsis.  Only local marketplaces can
// yield a newer version — there is no remote fetch (documented divergence).
func runPluginUpdate(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginUpdateFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginUpdateHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, pluginPositional); err != nil {
		return err
	}
	input := p.Positionals[0]
	scope := p.Values["scope"]
	if scope == "" {
		scope = config.PluginScopeUser
	}
	if err := pluginValidateScope(scope, pluginScopeUpdate); err != nil {
		return err
	}

	fmt.Fprintf(d.Stdout, "Checking for updates for plugin %q at %s scope…\n", input, scope)

	ip, err := d.Store.LoadInstalledPlugins()
	if err != nil {
		return err
	}
	// Resolution by name part: Find matches the exact id first, then any id
	// whose name part equals the reference's name part.
	id, recs := ip.Find(config.PluginNamePart(input))
	if len(recs) == 0 {
		return pluginUpdateMissingError(input, scope, d.Store)
	}
	rec := pluginRecordAtScope(recs, scope)
	if rec == nil {
		return pluginUpdateMissingError(input, scope, d.Store)
	}

	mp, entry := pluginFindMarketplaceEntry(d, id)
	if mp == nil || entry == nil {
		// The marketplace (or its entry) is gone: nothing to update from.
		fmt.Fprintf(d.Stdout, "✔ %s is already at the latest version (%s).\n", config.PluginNamePart(id), rec.Version)
		return nil
	}
	// Version precedence matches install: the plugin's own plugin.json wins
	// over the marketplace entry's version.
	latest := entry.Version
	if m, _ := config.LoadPluginManifest(mp.PluginDir(entry)); m != nil && m.Version != "" {
		latest = m.Version
	}
	if latest == "" || latest == rec.Version {
		fmt.Fprintf(d.Stdout, "✔ %s is already at the latest version (%s).\n", config.PluginNamePart(id), rec.Version)
		return nil
	}

	installPath, err := d.Store.InstallToCache(mp.Name, config.PluginNamePart(id), latest, mp.PluginDir(entry))
	if err != nil {
		return err
	}
	oldVersion := rec.Version
	rec.Version = latest
	rec.InstallPath = installPath
	rec.LastUpdated = config.PluginTimestamp(timeNow())
	ip.SetRecords(id, recs)
	if err := ip.Save(d.Store); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "✔ Plugin %q updated from %s to %s for scope %s. Restart to apply changes.\n",
		config.PluginNamePart(id), oldVersion, latest, scope)
	return nil
}

// pluginUpdateMissingError renders the scope-specific not-installed failure.
// Oracle-verified formats: user carries no scope, no cwd, no period;
// project/local append the resolved cwd; managed has no cwd and no period.
func pluginUpdateMissingError(input, scope string, s *config.PluginStore) error {
	switch scope {
	case config.PluginScopeProject, config.PluginScopeLocal:
		cwd, err := s.ResolvedProjectDir()
		if err != nil {
			cwd = s.ProjectDir()
		}
		return mcpActionErrorf("✘ Failed to update plugin %q: Plugin %q is not installed at scope %s (%s)", input, input, scope, cwd)
	case "managed":
		return mcpActionErrorf("✘ Failed to update plugin %q: Plugin %q is not installed at scope managed", input, input)
	default:
		return mcpActionErrorf("✘ Failed to update plugin %q: Plugin %q is not installed", input, input)
	}
}

// pluginRecordAtScope returns the record at scope from a record list.
func pluginRecordAtScope(recs []*config.InstalledPluginRecord, scope string) *config.InstalledPluginRecord {
	for _, r := range recs {
		if r.Scope == scope {
			return r
		}
	}
	return nil
}

// runPluginList implements `plugin list`.  --available without --json is
// ignored in text mode (oracle-verified: plain installed list).
func runPluginList(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginListFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginListHelp)
		return nil
	}
	if p.Bools["json"] {
		return pluginRenderListJSON(d, p.Bools["available"])
	}
	return pluginRenderListText(d)
}
