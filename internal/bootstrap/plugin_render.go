package bootstrap

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// Rendering for `plugin list` and `plugin marketplace list` — text and JSON,
// byte-pinned against the oracle (claude v2.1.261).  JSON output is 2-space
// indented with a trailing newline; the text list separates every plugin
// block with a blank line and ends with one.

// pluginListEntry is one installed record in the JSON forms.
type pluginListEntry struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	InstallPath string `json:"installPath"`
	InstalledAt string `json:"installedAt"`
	LastUpdated string `json:"lastUpdated"`
}

// pluginCollectEntries walks the installed registry in insertion order,
// emitting one entry per (id, record) pair — the order `plugin list` renders
// in (oracle-verified: record array order within an id).
func pluginCollectEntries(d *pluginDeps) ([]pluginListEntry, error) {
	ip, err := d.Store.LoadInstalledPlugins()
	if err != nil {
		return nil, err
	}
	entries := make([]pluginListEntry, 0) // [] not null when empty
	for _, id := range ip.IDs() {
		for _, rec := range ip.Records(id) {
			enabled, err := pluginEffectiveEnabled(d, ip, rec.Scope, id)
			if err != nil {
				return nil, err
			}
			entries = append(entries, pluginListEntry{
				ID:          id,
				Version:     rec.Version,
				Scope:       rec.Scope,
				Enabled:     enabled,
				InstallPath: rec.InstallPath,
				InstalledAt: rec.InstalledAt,
				LastUpdated: rec.LastUpdated,
			})
		}
	}
	return entries, nil
}

// pluginRenderListText prints the text installed list.  Empty registry:
// single guidance line.
func pluginRenderListText(d *pluginDeps) error {
	entries, err := pluginCollectEntries(d)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprint(d.Stdout, "No plugins installed. Use `claude plugin install` to install a plugin.\n")
		return nil
	}
	var b strings.Builder
	b.WriteString("Installed plugins:\n")
	for _, e := range entries {
		status := "✘ disabled"
		if e.Enabled {
			status = "✔ enabled"
		}
		fmt.Fprintf(&b, "\n  ❯ %s\n    Version: %s\n    Scope: %s\n    Status: %s\n", e.ID, e.Version, e.Scope, status)
	}
	b.WriteString("\n")
	fmt.Fprint(d.Stdout, b.String())
	return nil
}

// pluginRenderListJSON prints the JSON installed list ([] when empty), or —
// with available — the {installed, available} report.  --available without
// --json never reaches here (runPluginList routes it to the text form).
func pluginRenderListJSON(d *pluginDeps, available bool) error {
	entries, err := pluginCollectEntries(d)
	if err != nil {
		return err
	}
	if !available {
		return pluginWriteJSON(d, entries)
	}

	avail, err := pluginCollectAvailable(d, entries)
	if err != nil {
		return err
	}
	return pluginWriteJSON(d, pluginAvailableReport{Installed: entries, Available: avail})
}

// pluginAvailableReport is the `plugin list --available --json` top level.
type pluginAvailableReport struct {
	Installed []pluginListEntry      `json:"installed"`
	Available []pluginAvailableEntry `json:"available"`
}

// pluginAvailableEntry is one not-yet-installed plugin in the --available
// JSON report.
type pluginAvailableEntry struct {
	PluginID        string `json:"pluginId"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	MarketplaceName string `json:"marketplaceName"`
	Version         string `json:"version"`
	Source          string `json:"source"`
}

// pluginCollectAvailable walks every configured marketplace (registry order)
// and lists its entries that are not already installed.
func pluginCollectAvailable(d *pluginDeps, installed []pluginListEntry) ([]pluginAvailableEntry, error) {
	installedIDs := map[string]bool{}
	for _, e := range installed {
		installedIDs[e.ID] = true
	}
	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return nil, err
	}
	var out = make([]pluginAvailableEntry, 0) // [] not null when empty
	for _, mpName := range km.Names() {
		rec := km.Get(mpName)
		if rec == nil {
			continue
		}
		manifest, err := config.LoadMarketplaceManifestFile(rec.ManifestPathFor())
		if err != nil || manifest == nil {
			continue
		}
		for _, e := range manifest.Plugins {
			id := e.Name + "@" + mpName
			if installedIDs[id] {
				continue
			}
			out = append(out, pluginAvailableEntry{
				PluginID:        id,
				Name:            e.Name,
				Description:     e.Description,
				MarketplaceName: mpName,
				Version:         e.Version,
				Source:          e.Source,
			})
		}
	}
	return out, nil
}

// pluginWriteJSON marshals v with the oracle's JSON conventions (2-space
// indent, trailing newline).  A nil slice renders as null, so callers pass
// explicit empty slices for [].
func pluginWriteJSON(d *pluginDeps, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "%s\n", data)
	return nil
}

// pluginRenderMPListText prints the text marketplace list.
func pluginRenderMPListText(d *pluginDeps) error {
	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return err
	}
	names := km.Names()
	if len(names) == 0 {
		fmt.Fprint(d.Stdout, "No marketplaces configured\n")
		return nil
	}
	var b strings.Builder
	b.WriteString("Configured marketplaces:\n")
	for _, name := range names {
		rec := km.Get(name)
		kind := "File"
		path := rec.SourcePath
		if rec.SourceType == "directory" {
			kind = "Directory"
		}
		fmt.Fprintf(&b, "\n  ❯ %s\n    Source: %s (%s)\n", name, kind, path)
	}
	b.WriteString("\n")
	fmt.Fprint(d.Stdout, b.String())
	return nil
}

// pluginRenderMPListJSON prints the JSON marketplace list.
func pluginRenderMPListJSON(d *pluginDeps) error {
	km, err := d.Store.LoadKnownMarketplaces()
	if err != nil {
		return err
	}
	type mpEntry struct {
		Name            string `json:"name"`
		Source          string `json:"source"`
		Path            string `json:"path"`
		InstallLocation string `json:"installLocation"`
	}
	entries := make([]mpEntry, 0) // [] not null when empty
	for _, name := range km.Names() {
		rec := km.Get(name)
		if rec == nil {
			continue
		}
		entries = append(entries, mpEntry{
			Name:            name,
			Source:          rec.SourceType,
			Path:            rec.SourcePath,
			InstallLocation: rec.InstallLocation,
		})
	}
	return pluginWriteJSON(d, entries)
}
