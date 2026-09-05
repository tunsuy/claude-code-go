package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// The `claude plugin marketplace` subcommands (add/list/remove/update),
// byte-pinned against the oracle (claude v2.1.261).
//
// Documented divergence: only local marketplace sources are supported — a
// directory path, a path to a marketplace.json file, or ./relative paths.
// Remote sources (GitHub owner/repo, https URLs, npm, --claudeai hosted
// marketplaces) are not implemented and report an honest error instead of
// fetching.  --sparse (sparse-checkout) is accepted and ignored for local
// sources.  The mp-add error streams: source-format, remote and
// path-existence failures print no progress line; the manifest-missing
// failure prints the progress (no newline) then the ✘ on stderr.

// runPluginMPAdd implements `plugin marketplace add <source>`.
func runPluginMPAdd(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginMPAddFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginMPAddHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, pluginSourceArg); err != nil {
		return err
	}
	source := p.Positionals[0]
	scope := p.Values["scope"]
	if scope == "" {
		scope = config.PluginScopeUser
	}
	if err := pluginValidateScope(scope, pluginScopeMarketplace); err != nil {
		return err
	}
	if p.Bools["claudeai"] {
		return mcpActionErrorf("✘ --claudeai marketplaces (hosted on claude.ai) are not supported by this build. Add a local marketplace path instead.")
	}

	// Classify the source (oracle-verified: a slash-less word like "." is a
	// format error, not a missing path).
	switch pluginMPSourceKind(source) {
	case pluginMPSourceRemote:
		return mcpActionErrorf("✘ Remote marketplace sources are not supported by this build. Use a local path (./dir or an absolute path).")
	case pluginMPSourceInvalid:
		return mcpActionErrorf("✘ Invalid marketplace source format. Try: owner/repo, https://..., or ./path")
	}

	resolved := source
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		if abs, aerr := filepath.Abs(source); aerr == nil {
			resolved = abs
		}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return mcpActionErrorf("✘ Path does not exist: %s", source)
	}

	// Progress line — no newline; the result follows on the same line, or the
	// failure goes to stderr with no stdout newline (oracle-verified).
	fmt.Fprint(d.Stdout, "Adding marketplace…")

	var srcType, srcPath, rootDir string
	var manifest *config.MarketplaceManifest
	if info.IsDir() {
		srcType, srcPath, rootDir = "directory", resolved, resolved
		manifest, err = config.LoadMarketplaceManifest(rootDir)
	} else {
		srcType, srcPath = "file", resolved
		manifest, err = config.LoadMarketplaceManifestFile(resolved)
		if manifest != nil {
			rootDir = manifest.RootDir
		}
	}
	if err != nil {
		return mcpActionErrorf("✘ Failed to add marketplace: %s", err.Error())
	}
	if manifest == nil {
		return mcpActionErrorf("✘ Failed to add marketplace: Marketplace file not found at %s/.claude-plugin/marketplace.json", resolved)
	}
	name := manifest.Name

	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return mcpActionErrorf("✘ Failed to add marketplace: %s", err.Error())
	}
	existing := km.Get(name)
	if existing != nil {
		// Already on disk: keep the registry record, only (re)declare it in
		// the requested settings scope.
		if err := d.Store.SetExtraMarketplace(scope, name, existing.SourceType, existing.SourcePath); err != nil {
			return mcpActionErrorf("✘ Failed to add marketplace: %s", err.Error())
		}
		fmt.Fprintf(d.Stdout, "✔ Marketplace '%s' already on disk — declared in %s settings\n", name, scope)
		return nil
	}

	km.Set(name, &config.KnownMarketplace{
		SourceType:      srcType,
		SourcePath:      srcPath,
		InstallLocation: rootDir,
		LastUpdated:     config.PluginTimestamp(timeNow()),
	})
	if err := km.Save(d.Store); err != nil {
		return mcpActionErrorf("✘ Failed to add marketplace: %s", err.Error())
	}
	if err := d.Store.SetExtraMarketplace(scope, name, srcType, srcPath); err != nil {
		return mcpActionErrorf("✘ Failed to add marketplace: %s", err.Error())
	}
	fmt.Fprintf(d.Stdout, "✔ Successfully added marketplace: %s (declared in %s settings)\n", name, scope)
	return nil
}

// pluginMPSourceKind classifies an add source.
type pluginMPSource int

const (
	pluginMPSourceLocal   pluginMPSource = iota // /abs, ./rel, ../rel, *.json file
	pluginMPSourceRemote                        // https://, npm:, owner/repo
	pluginMPSourceInvalid                       // slash-less word (e.g. ".")
)

func pluginMPSourceKind(source string) pluginMPSource {
	if strings.Contains(source, "://") || strings.HasPrefix(source, "npm:") {
		return pluginMPSourceRemote
	}
	if !strings.Contains(source, "/") {
		return pluginMPSourceInvalid
	}
	if !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") &&
		strings.Count(source, "/") == 1 && filepath.Ext(source) == "" {
		return pluginMPSourceRemote // GitHub owner/repo
	}
	return pluginMPSourceLocal
}

// runPluginMPList implements `plugin marketplace list`.
func runPluginMPList(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginMPListFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginMPListHelp)
		return nil
	}
	if p.Bools["json"] {
		return pluginRenderMPListJSON(d)
	}
	return pluginRenderMPListText(d)
}

// runPluginMPRemove implements `plugin marketplace remove <name>`.  Default:
// remove the declaration from every settings scope, drop the registry record
// and uninstall the marketplace's plugins.  With --scope: only that scope's
// declaration stays removed; the registry and plugins survive (unverified
// edge — only the default form is captured).
func runPluginMPRemove(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginMPRemoveFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginMPRemoveHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, pluginNameArg); err != nil {
		return err
	}
	name := p.Positionals[0]
	explicit := p.Values["scope"]
	if explicit != "" {
		if err := pluginValidateScope(explicit, pluginScopeMarketplace); err != nil {
			return err
		}
	}

	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return err
	}
	registered := km.Get(name) != nil

	if explicit != "" {
		if !registered && !pluginMPDeclaredIn(d, name) {
			return mcpActionErrorf("✘ Failed to remove marketplace: Marketplace '%s' not found", name)
		}
		if err := d.Store.RemoveExtraMarketplace(explicit, name); err != nil {
			return err
		}
		fmt.Fprintf(d.Stdout, "✔ Successfully removed marketplace: %s\n", name)
		return nil
	}

	if !registered && !pluginMPDeclaredIn(d, name) {
		return mcpActionErrorf("✘ Failed to remove marketplace: Marketplace '%s' not found", name)
	}

	// Uninstall every plugin that came from this marketplace, then drop the
	// declarations and the registry record.
	ip, err := d.Store.LoadInstalledPlugins()
	if err != nil {
		return err
	}
	for _, id := range append([]string(nil), ip.IDs()...) {
		if marketplacePart(id) != name {
			continue
		}
		for _, r := range ip.Records(id) {
			_ = config.OrphanCache(r.InstallPath)
		}
		ip.Delete(id)
		for _, scope := range config.PluginScopeOrder {
			if err := d.Store.RemovePluginEnabled(scope, id); err != nil {
				return err
			}
		}
	}
	if err := ip.Save(d.Store); err != nil {
		return err
	}
	for _, scope := range config.PluginScopeOrder {
		if err := d.Store.RemoveExtraMarketplace(scope, name); err != nil {
			return err
		}
	}
	km.Delete(name)
	if err := km.Save(d.Store); err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "✔ Successfully removed marketplace: %s\n", name)
	return nil
}

// pluginMPDeclaredIn reports whether name is declared in any settings scope.
func pluginMPDeclaredIn(d *pluginDeps, name string) bool {
	for _, scope := range config.PluginScopeOrder {
		extras, err := d.Store.ExtraMarketplaces(scope)
		if err != nil {
			continue
		}
		if _, ok := extras.Get(name); ok {
			return true
		}
	}
	return false
}

// runPluginMPUpdate implements `plugin marketplace update [name]`.  Local
// marketplaces have nothing to fetch, so a named update re-validates the
// manifest (the oracle prints "Validating local marketplace") and refreshes
// lastUpdated; an update-all counts them.  Oracle byte shapes: named progress
// ends WITH a newline; update-all progress does not.
func runPluginMPUpdate(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginMPUpdateFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginMPUpdateHelp)
		return nil
	}

	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return err
	}
	names := km.Names()

	if len(p.Positionals) > 0 {
		name := p.Positionals[0]
		fmt.Fprintf(d.Stdout, "Updating marketplace: %s...\n", name)
		rec := km.Get(name)
		if rec == nil {
			return mcpActionErrorf("✘ Failed to update marketplace(s): Marketplace '%s' not found. Available marketplaces: %s", name, strings.Join(names, ", "))
		}
		fmt.Fprint(d.Stdout, "Validating local marketplace\n")
		manifest, err := config.LoadMarketplaceManifestFile(rec.ManifestPathFor())
		if err != nil || manifest == nil {
			return mcpActionErrorf("✘ Failed to update marketplace(s): Marketplace '%s' could not be read from %s", name, rec.ManifestPathFor())
		}
		rec.LastUpdated = config.PluginTimestamp(timeNow())
		km.Set(name, rec)
		if err := km.Save(d.Store); err != nil {
			return err
		}
		fmt.Fprintf(d.Stdout, "✔ Successfully updated marketplace: %s\n", name)
		return nil
	}

	// Update all (unverified edge: the zero-marketplaces case).
	fmt.Fprint(d.Stdout, "Updating marketplaces...")
	if len(names) == 0 {
		fmt.Fprint(d.Stdout, "\n")
		return mcpActionErrorf("✘ Failed to update marketplace(s): No marketplaces configured")
	}
	for _, name := range names {
		if rec := km.Get(name); rec != nil {
			rec.LastUpdated = config.PluginTimestamp(timeNow())
			km.Set(name, rec)
		}
	}
	if err := km.Save(d.Store); err != nil {
		return err
	}
	noun := "marketplaces"
	if len(names) == 1 {
		noun = "marketplace"
	}
	fmt.Fprintf(d.Stdout, "✔ Successfully updated %d %s\n", len(names), noun)
	return nil
}
