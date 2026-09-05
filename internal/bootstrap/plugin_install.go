package bootstrap

import (
	"fmt"
	"strings"
	"time"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// `claude plugin install` and `claude plugin uninstall`.  Both resolve the
// plugin through the configured marketplaces (install) or the installed
// registry (uninstall), manage the cache copy, and keep the settings trio in
// sync — all byte-pinned against the oracle (claude v2.1.261).

// pluginProgress prints the install progress line WITHOUT a newline; the
// success or failure text follows on the same line (success) or after a
// newline (failure).  Oracle captures:
//
//	success: `Installing plugin "x"...✔ Successfully installed plugin: …\n`
//	failure: `Installing plugin "x"...\n` (stdout) + `✘ …\n` (stderr, exit 1)
func pluginProgress(d *pluginDeps, input string) {
	fmt.Fprintf(d.Stdout, "Installing plugin %q...", input)
}

// pluginFindMarketplaceEntry resolves a plugin id (name@marketplace) to its
// marketplace manifest and entry.  Returns (nil, nil) when the marketplace is
// not registered or has no such entry.
func pluginFindMarketplaceEntry(d *pluginDeps, id string) (*config.MarketplaceManifest, *config.MarketplaceEntry) {
	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return nil, nil
	}
	mpName := marketplacePart(id)
	rec := km.Get(mpName)
	if rec == nil {
		return nil, nil
	}
	manifest, err := config.LoadMarketplaceManifestFile(rec.ManifestPathFor())
	if err != nil || manifest == nil {
		return nil, nil
	}
	name := config.PluginNamePart(id)
	for _, e := range manifest.Plugins {
		if e.Name == name {
			return manifest, e
		}
	}
	return manifest, nil
}

// marketplacePart returns the part of a plugin id after "@" ("" when bare).
func marketplacePart(id string) string {
	if i := strings.Index(id, "@"); i >= 0 {
		return id[i+1:]
	}
	return ""
}

// runPluginInstall implements `plugin install <plugin>`.  The input may be
// `name`, `name@marketplace`, or a path-like reference (only the two id forms
// are implemented; remote/path sources are a documented divergence).  Install
// order (oracle-verified): progress line → resolve → copy to cache → record +
// enable → success line.
func runPluginInstall(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginInstallFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginInstallHelp)
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
	if err := pluginValidateScope(scope, pluginScopeColonList); err != nil {
		return err
	}

	pluginProgress(d, input)

	ip, err := d.Store.LoadInstalledPlugins()
	if err != nil {
		return pluginInstallFail(d, input, "plugin state unavailable")
	}

	// Resolve: with @marketplace, that marketplace only; bare name searches
	// all configured marketplaces in registry order.
	var manifest *config.MarketplaceManifest
	var entry *config.MarketplaceEntry
	if mp := marketplacePart(input); mp != "" {
		manifest, entry = pluginFindMarketplaceEntry(d, input)
		if manifest == nil || entry == nil {
			name := config.PluginNamePart(input)
			return pluginInstallFail(d, input, fmt.Sprintf("Plugin %q not found in marketplace %q. Your local copy may be out of date — try `claude plugin marketplace update %s`.", name, mp, mp))
		}
	} else {
		manifest, entry, err = pluginSearchMarketplaces(d, input)
		if err != nil {
			return pluginInstallFail(d, input, err.Error())
		}
		if manifest == nil || entry == nil {
			return pluginInstallFail(d, input, fmt.Sprintf("Plugin %q not found in any configured marketplace", input))
		}
	}

	id := entry.Name + "@" + manifest.Name
	// Version precedence: the plugin's own plugin.json wins over the
	// marketplace entry version (oracle-verified: 9.9.9 installs when
	// plugin.json says 9.9.9 and the entry says 2.0.0).
	version := entry.Version
	srcDir := manifest.PluginDir(entry)
	if m, _ := config.LoadPluginManifest(srcDir); m != nil && m.Version != "" {
		version = m.Version
	}
	installPath, err := d.Store.InstallToCache(manifest.Name, entry.Name, version, srcDir)
	if err != nil {
		return pluginInstallFail(d, input, err.Error())
	}

	now := config.PluginTimestamp(timeNow())
	rec := &config.InstalledPluginRecord{
		Scope:       scope,
		InstallPath: installPath,
		Version:     version,
		InstalledAt: now,
		LastUpdated: now,
	}
	if scope == config.PluginScopeProject {
		cwd, cerr := d.Store.ResolvedProjectDir()
		if cerr != nil {
			return pluginInstallFail(d, input, cerr.Error())
		}
		rec.ProjectPath = cwd
	}

	if existing := pluginRecordAtScope(ip.Records(id), scope); existing != nil {
		// Already installed at this scope: the oracle keeps the existing
		// record (installedAt preserved) but still ensures the plugin is
		// enabled at this scope.
		if err := d.Store.SetPluginEnabled(scope, id, true); err != nil {
			return pluginInstallFail(d, input, err.Error())
		}
		fmt.Fprintf(d.Stdout, "✔ Plugin %q is already installed (scope: %s)\n", id, scope)
		return nil
	}

	recs := append(ip.Records(id), rec)
	ip.SetRecords(id, recs)
	if err := ip.Save(d.Store); err != nil {
		return pluginInstallFail(d, input, err.Error())
	}
	if err := d.Store.SetPluginEnabled(scope, id, true); err != nil {
		return pluginInstallFail(d, input, err.Error())
	}
	fmt.Fprintf(d.Stdout, "✔ Successfully installed plugin: %s (scope: %s)\n", id, scope)
	return nil
}

// pluginSearchMarketplaces looks for name across every configured marketplace,
// in registry (insertion) order.
func pluginSearchMarketplaces(d *pluginDeps, name string) (*config.MarketplaceManifest, *config.MarketplaceEntry, error) {
	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return nil, nil, err
	}
	for _, mpName := range km.Names() {
		rec := km.Get(mpName)
		if rec == nil {
			continue
		}
		manifest, err := config.LoadMarketplaceManifestFile(rec.ManifestPathFor())
		if err != nil {
			return nil, nil, err
		}
		if manifest == nil {
			continue
		}
		for _, e := range manifest.Plugins {
			if e.Name == name {
				return manifest, e, nil
			}
		}
	}
	return nil, nil, nil
}

// pluginInstallFail terminates a failed install: newline on stdout (closing
// the progress line), the ✘ failure on stderr, exit 1.
func pluginInstallFail(d *pluginDeps, input, reason string) error {
	fmt.Fprintf(d.Stdout, "\n")
	return mcpActionErrorf("✘ Failed to install plugin %q: %s", input, reason)
}

// runPluginUninstall implements `plugin uninstall <plugin>`.  Default target
// is user scope (regardless of where records live); with -s, that scope only.
// Oracle-verified: the not-found-in-registry miss reports the raw input; the
// wrong-scope miss reports the resolved name part; success reports the name
// part with the acted-on scope.
func runPluginUninstall(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginUninstallFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginUninstallHelp)
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
	if err := pluginValidateScope(scope, pluginScopeColonList); err != nil {
		return err
	}

	ip, err := d.Store.LoadInstalledPlugins()
	if err != nil {
		return err
	}
	id, recs := ip.Find(input)
	if len(recs) == 0 {
		return mcpActionErrorf("✘ Failed to uninstall plugin %q: Plugin %q not found in installed plugins", input, input)
	}
	name := config.PluginNamePart(id)
	if pluginRecordAtScope(recs, scope) == nil {
		return mcpActionErrorf("✘ Failed to uninstall plugin %q: Plugin %q is not installed in %s scope. Use --scope to specify the correct scope.", input, name, scope)
	}

	remaining := make([]*config.InstalledPluginRecord, 0, len(recs))
	for _, r := range recs {
		if r.Scope != scope {
			remaining = append(remaining, r)
		}
	}
	if len(remaining) == 0 {
		ip.Delete(id)
	} else {
		ip.SetRecords(id, remaining)
	}
	if err := ip.Save(d.Store); err != nil {
		return err
	}
	// The cache copy is only orphaned when no scope's record survives.
	if len(remaining) == 0 {
		for _, r := range recs {
			if r.Scope == scope {
				_ = config.OrphanCache(r.InstallPath)
			}
		}
	}
	if err := d.Store.RemovePluginEnabled(scope, id); err != nil {
		return err
	}

	fmt.Fprintf(d.Stdout, "✔ Successfully uninstalled plugin: %s (scope: %s)\n", name, scope)
	if p.Bools["prune"] {
		fmt.Fprint(d.Stdout, "Nothing to prune (no auto-installed plugins at user scope).\n")
	}
	return nil
}

// timeNow is the clock hook for timestamps written into installed_plugins.json
// (swapped in tests to pin the value).
var timeNow = time.Now
