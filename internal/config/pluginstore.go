package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The plugin CLI state store.  The on-disk layout mirrors the oracle TS CLI
// (claude v2.1.261) exactly, because the two CLIs share a home directory:
//
//	~/.claude/plugins/installed_plugins.json   {version: 2, plugins: {id: [records]}}
//	~/.claude/plugins/known_marketplaces.json  {name: {source, installLocation, lastUpdated}}
//	~/.claude/plugins/cache/<mp>/<plugin>/<version>/   installed plugin copies
//
// Marketplace and plugin enablement live in the three-level settings files,
// under enabledPlugins (id → bool) and extraKnownMarketplaces (name → source):
//
//	user    ~/.claude/settings.json
//	project <cwd>/.claude/settings.json
//	local   <cwd>/.claude/settings.local.json
//
// All JSON documents round-trip through OrderedMap so insertion order (which
// `plugin list` renders in) survives load/edit/save cycles.

// Plugin installation scopes, as written into installed_plugins.json and the
// settings files.
const (
	PluginScopeUser    = "user"
	PluginScopeProject = "project"
	PluginScopeLocal   = "local"
)

// PluginScopeOrder is the precedence order used when resolving a plugin's
// enabled state: local overrides project overrides user (oracle-verified via
// `plugin enable` reporting "scope: local" once a local entry exists).
var PluginScopeOrder = []string{PluginScopeLocal, PluginScopeProject, PluginScopeUser}

// PluginStore owns the plugin state files under a home and project directory.
type PluginStore struct {
	homeDir    string
	projectDir string
}

// NewPluginStore creates a store over the given home and project (cwd) dirs.
func NewPluginStore(homeDir, projectDir string) *PluginStore {
	return &PluginStore{homeDir: homeDir, projectDir: projectDir}
}

// HomeDir returns the store's home directory.
func (s *PluginStore) HomeDir() string { return s.homeDir }

// ProjectDir returns the store's project directory (the cwd as given).
func (s *PluginStore) ProjectDir() string { return s.projectDir }

// ResolvedProjectDir returns the project directory with symlinks resolved,
// the form the oracle stores in installed_plugins.json's projectPath field
// (capture: /tmp/... recorded as /private/tmp/... on macOS).
func (s *PluginStore) ResolvedProjectDir() (string, error) {
	resolved, err := filepath.EvalSymlinks(s.projectDir)
	if err != nil {
		return "", fmt.Errorf("plugin store: resolve project dir: %w", err)
	}
	return resolved, nil
}

// PluginsDir returns ~/.claude/plugins.
func (s *PluginStore) PluginsDir() string {
	return filepath.Join(s.homeDir, ClaudeDir, "plugins")
}

// CacheDir returns ~/.claude/plugins/cache.
func (s *PluginStore) CacheDir() string {
	return filepath.Join(s.PluginsDir(), "cache")
}

// InstalledPluginsPath returns the installed_plugins.json path.
func (s *PluginStore) InstalledPluginsPath() string {
	return filepath.Join(s.PluginsDir(), "installed_plugins.json")
}

// KnownMarketplacesPath returns the known_marketplaces.json path.
func (s *PluginStore) KnownMarketplacesPath() string {
	return filepath.Join(s.PluginsDir(), "known_marketplaces.json")
}

// SettingsPath maps a scope to its settings file.  Note the local scope is
// <cwd>/.claude/settings.local.json — the oracle's plugin settings file —
// which differs from the loader's legacy .claude.local path.
func (s *PluginStore) SettingsPath(scope string) string {
	switch scope {
	case PluginScopeProject:
		return filepath.Join(s.projectDir, ClaudeDir, SettingsFile)
	case PluginScopeLocal:
		return filepath.Join(s.projectDir, ClaudeDir, "settings.local.json")
	default:
		return filepath.Join(s.homeDir, ClaudeDir, SettingsFile)
	}
}

// PluginTimestamp renders the oracle's JSON timestamp format: UTC with
// millisecond precision and a literal Z (2026-09-05T03:16:38.020Z).
func PluginTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// --- installed_plugins.json -------------------------------------------------

// InstalledPluginRecord is one scope's installation of a plugin id.
type InstalledPluginRecord struct {
	Scope       string
	InstallPath string
	Version     string
	InstalledAt string
	LastUpdated string
	ProjectPath string // project scope only; empty otherwise
}

// InstalledPlugins is the ordered installed_plugins.json document.
type InstalledPlugins struct {
	doc     *OrderedMap // full document (version, plugins)
	plugins *OrderedMap // id → []*InstalledPluginRecord, insertion-ordered
}

// LoadInstalledPlugins reads installed_plugins.json; a missing file yields an
// empty registry, not an error.
func (s *PluginStore) LoadInstalledPlugins() (*InstalledPlugins, error) {
	doc, err := LoadOrderedJSON(s.InstalledPluginsPath())
	if err != nil {
		return nil, fmt.Errorf("plugin store: load installed: %w", err)
	}
	plugins, _ := doc.GetMap("plugins")
	if plugins == nil {
		plugins = NewOrderedMap()
	}
	return &InstalledPlugins{doc: doc, plugins: plugins}, nil
}

// Save writes the registry back to installed_plugins.json, ensuring the
// document keeps the oracle's {version, plugins} shape and key order.
func (ip *InstalledPlugins) Save(s *PluginStore) error {
	ip.doc.Set("version", json.Number("2"))
	ip.doc.Set("plugins", ip.plugins)
	if err := WriteOrderedJSONAtomic(s.InstalledPluginsPath(), ip.doc); err != nil {
		return fmt.Errorf("plugin store: save installed: %w", err)
	}
	return nil
}

// IDs returns the installed plugin ids in insertion order.
func (ip *InstalledPlugins) IDs() []string { return ip.plugins.Keys() }

// Records returns the scope records for id, in insertion order.
func (ip *InstalledPlugins) Records(id string) []*InstalledPluginRecord {
	v, _ := ip.plugins.Get(id)
	arr, _ := v.([]any)
	recs := make([]*InstalledPluginRecord, 0, len(arr))
	for _, item := range arr {
		om, ok := item.(*OrderedMap)
		if !ok {
			continue
		}
		recs = append(recs, recordFromMap(om))
	}
	return recs
}

// SetRecords replaces the records for id (appending the id at the end when
// new).
func (ip *InstalledPlugins) SetRecords(id string, recs []*InstalledPluginRecord) {
	arr := make([]any, 0, len(recs))
	for _, r := range recs {
		arr = append(arr, recordToMap(r))
	}
	ip.plugins.Set(id, arr)
}

// Delete removes id from the registry.
func (ip *InstalledPlugins) Delete(id string) { ip.plugins.Delete(id) }

// Find resolves a plugin reference the way the uninstall/enable commands do:
// an exact id match wins; otherwise the first id (in insertion order) whose
// name part (before "@") equals the reference.  Returns ("", nil) on a miss.
func (ip *InstalledPlugins) Find(ref string) (string, []*InstalledPluginRecord) {
	if recs := ip.Records(ref); len(recs) > 0 {
		return ref, recs
	}
	name := PluginNamePart(ref)
	for _, id := range ip.plugins.Keys() {
		if PluginNamePart(id) == name {
			return id, ip.Records(id)
		}
	}
	return "", nil
}

// RecordAt returns the record for id at the given scope, or nil.
func (ip *InstalledPlugins) RecordAt(id, scope string) *InstalledPluginRecord {
	for _, r := range ip.Records(id) {
		if r.Scope == scope {
			return r
		}
	}
	return nil
}

// PluginNamePart returns the part of a plugin id before "@" (a bare name is
// returned unchanged).
func PluginNamePart(idOrName string) string {
	if i := strings.Index(idOrName, "@"); i >= 0 {
		return idOrName[:i]
	}
	return idOrName
}

// recordFromMap converts a parsed record object into the struct form.
func recordFromMap(om *OrderedMap) *InstalledPluginRecord {
	return &InstalledPluginRecord{
		Scope:       om.GetString("scope"),
		InstallPath: om.GetString("installPath"),
		Version:     om.GetString("version"),
		InstalledAt: om.GetString("installedAt"),
		LastUpdated: om.GetString("lastUpdated"),
		ProjectPath: om.GetString("projectPath"),
	}
}

// recordToMap renders a record with the oracle's key order; projectPath sits
// right after scope and is omitted for user/local records.
func recordToMap(r *InstalledPluginRecord) *OrderedMap {
	om := NewOrderedMap()
	om.Set("scope", r.Scope)
	if r.ProjectPath != "" {
		om.Set("projectPath", r.ProjectPath)
	}
	om.Set("installPath", r.InstallPath)
	om.Set("version", r.Version)
	om.Set("installedAt", r.InstalledAt)
	om.Set("lastUpdated", r.LastUpdated)
	return om
}

// --- known_marketplaces.json ------------------------------------------------

// KnownMarketplace is one entry of the marketplace registry.
type KnownMarketplace struct {
	SourceType      string // "directory" | "file"
	SourcePath      string
	InstallLocation string // marketplace root (the dir containing .claude-plugin/)
	LastUpdated     string
}

// KnownMarketplaces is the ordered known_marketplaces.json document.
type KnownMarketplaces struct {
	doc *OrderedMap // name → record map
}

// LoadKnownMarketplaces reads known_marketplaces.json; missing → empty.
func (s *PluginStore) LoadKnownMarketplaces() (*KnownMarketplaces, error) {
	doc, err := LoadOrderedJSON(s.KnownMarketplacesPath())
	if err != nil {
		return nil, fmt.Errorf("plugin store: load marketplaces: %w", err)
	}
	return &KnownMarketplaces{doc: doc}, nil
}

// Save writes the registry back to disk.
func (km *KnownMarketplaces) Save(s *PluginStore) error {
	if err := WriteOrderedJSONAtomic(s.KnownMarketplacesPath(), km.doc); err != nil {
		return fmt.Errorf("plugin store: save marketplaces: %w", err)
	}
	return nil
}

// Names returns the marketplace names in insertion order.
func (km *KnownMarketplaces) Names() []string { return km.doc.Keys() }

// Get returns the record for name, or nil.
func (km *KnownMarketplaces) Get(name string) *KnownMarketplace {
	om, _ := km.doc.GetMap(name)
	if om == nil {
		return nil
	}
	rec := &KnownMarketplace{
		SourceType:      "",
		SourcePath:      "",
		InstallLocation: om.GetString("installLocation"),
		LastUpdated:     om.GetString("lastUpdated"),
	}
	if src, ok := om.GetMap("source"); ok && src != nil {
		rec.SourceType = src.GetString("source")
		rec.SourcePath = src.GetString("path")
	}
	return rec
}

// Set stores a marketplace record, preserving insertion order for known names.
func (km *KnownMarketplaces) Set(name string, rec *KnownMarketplace) {
	src := NewOrderedMap()
	src.Set("source", rec.SourceType)
	src.Set("path", rec.SourcePath)
	om := NewOrderedMap()
	om.Set("source", src)
	om.Set("installLocation", rec.InstallLocation)
	om.Set("lastUpdated", rec.LastUpdated)
	km.doc.Set(name, om)
}

// Delete removes a marketplace record.
func (km *KnownMarketplaces) Delete(name string) { km.doc.Delete(name) }

// Len returns the number of registered marketplaces.
func (km *KnownMarketplaces) Len() int { return km.doc.Len() }

// --- settings.json: enabledPlugins / extraKnownMarketplaces ------------------

// settingsDoc loads a scope's settings file, preserving unknown keys and
// order.  A missing file yields an empty document.
func (s *PluginStore) settingsDoc(scope string) (*OrderedMap, string, error) {
	path := s.SettingsPath(scope)
	doc, err := LoadOrderedJSON(path)
	if err != nil {
		return nil, path, fmt.Errorf("plugin store: load settings: %w", err)
	}
	return doc, path, nil
}

// EnabledPlugins returns the scope's enabledPlugins map (empty when absent).
func (s *PluginStore) EnabledPlugins(scope string) (*OrderedMap, error) {
	doc, _, err := s.settingsDoc(scope)
	if err != nil {
		return nil, err
	}
	enabled, _ := doc.GetMap("enabledPlugins")
	if enabled == nil {
		enabled = NewOrderedMap()
	}
	return enabled, nil
}

// SetPluginEnabled writes enabledPlugins[id] = enabled in the scope's
// settings file, creating the file and key when needed.
func (s *PluginStore) SetPluginEnabled(scope, id string, enabled bool) error {
	doc, path, err := s.settingsDoc(scope)
	if err != nil {
		return err
	}
	m, _ := doc.GetMap("enabledPlugins")
	if m == nil {
		m = NewOrderedMap()
		doc.Set("enabledPlugins", m)
	}
	m.Set(id, enabled)
	return WriteOrderedJSONAtomic(path, doc)
}

// RemovePluginEnabled deletes enabledPlugins[id] in the scope's settings file.
// The (possibly now-empty) enabledPlugins key is retained once it exists —
// the oracle leaves `"enabledPlugins": {}` behind (capture-verified).
func (s *PluginStore) RemovePluginEnabled(scope, id string) error {
	doc, path, err := s.settingsDoc(scope)
	if err != nil {
		return err
	}
	m, ok := doc.GetMap("enabledPlugins")
	if !ok || m == nil {
		return nil // nothing recorded in this scope
	}
	if _, has := m.Get(id); !has {
		return nil
	}
	m.Delete(id)
	return WriteOrderedJSONAtomic(path, doc)
}

// ExtraMarketplaces returns the scope's extraKnownMarketplaces map
// (name → {source: {source, path}}), empty when absent.
func (s *PluginStore) ExtraMarketplaces(scope string) (*OrderedMap, error) {
	doc, _, err := s.settingsDoc(scope)
	if err != nil {
		return nil, err
	}
	m, _ := doc.GetMap("extraKnownMarketplaces")
	if m == nil {
		m = NewOrderedMap()
	}
	return m, nil
}

// SetExtraMarketplace declares a marketplace in the scope's settings file.
func (s *PluginStore) SetExtraMarketplace(scope, name, srcType, srcPath string) error {
	doc, path, err := s.settingsDoc(scope)
	if err != nil {
		return err
	}
	m, _ := doc.GetMap("extraKnownMarketplaces")
	if m == nil {
		m = NewOrderedMap()
		doc.Set("extraKnownMarketplaces", m)
	}
	src := NewOrderedMap()
	src.Set("source", srcType)
	src.Set("path", srcPath)
	entry := NewOrderedMap()
	entry.Set("source", src)
	m.Set(name, entry)
	return WriteOrderedJSONAtomic(path, doc)
}

// RemoveExtraMarketplace deletes a marketplace declaration from the scope.
func (s *PluginStore) RemoveExtraMarketplace(scope, name string) error {
	doc, path, err := s.settingsDoc(scope)
	if err != nil {
		return err
	}
	m, ok := doc.GetMap("extraKnownMarketplaces")
	if !ok || m == nil {
		return nil
	}
	if _, has := m.Get(name); !has {
		return nil
	}
	m.Delete(name)
	return WriteOrderedJSONAtomic(path, doc)
}

// --- manifests ---------------------------------------------------------------

// PluginManifest is the parsed .claude-plugin/plugin.json of a plugin.
type PluginManifest struct {
	Name        string
	Version     string
	Description string
	HasAuthor   bool
}

// LoadPluginManifest reads dir/.claude-plugin/plugin.json.  A missing file
// returns (nil, nil) — callers decide whether that is an error.
func LoadPluginManifest(dir string) (*PluginManifest, error) {
	path := filepath.Join(dir, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin store: read %s: %w", path, err)
	}
	om := NewOrderedMap()
	if err := unmarshalOrdered(data, om); err != nil {
		return nil, fmt.Errorf("plugin store: parse %s: %w", path, err)
	}
	return &PluginManifest{
		Name:        om.GetString("name"),
		Version:     om.GetString("version"),
		Description: om.GetString("description"),
		HasAuthor:   hasNonEmpty(om, "author"),
	}, nil
}

// MarketplaceEntry is one plugins[] entry of a marketplace manifest.
type MarketplaceEntry struct {
	Name        string
	Source      string
	Version     string
	Description string
}

// MarketplaceManifest is the parsed .claude-plugin/marketplace.json.
type MarketplaceManifest struct {
	Name         string
	Description  string
	HasOwner     bool
	Plugins      []*MarketplaceEntry // file order
	RootDir      string              // dir containing .claude-plugin/
	ManifestPath string
}

// LoadMarketplaceManifest reads rootDir/.claude-plugin/marketplace.json.
// A missing file returns (nil, nil).
func LoadMarketplaceManifest(rootDir string) (*MarketplaceManifest, error) {
	return loadMarketplace(filepath.Join(rootDir, ".claude-plugin", "marketplace.json"), rootDir)
}

// LoadMarketplaceManifestFile reads a marketplace.json by direct file path.
// The marketplace root — what entry sources resolve against — is the parent
// of the .claude-plugin dir when the file lives in one, else the file's own
// directory (oracle-verified: a bare x.json resolves ./plugins/x against the
// dir holding x.json).
func LoadMarketplaceManifestFile(path string) (*MarketplaceManifest, error) {
	root := filepath.Dir(path)
	if filepath.Base(root) == ".claude-plugin" {
		root = filepath.Dir(root)
	}
	return loadMarketplace(path, root)
}

func loadMarketplace(path, rootDir string) (*MarketplaceManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin store: read %s: %w", path, err)
	}
	om := NewOrderedMap()
	if err := unmarshalOrdered(data, om); err != nil {
		return nil, fmt.Errorf("plugin store: parse %s: %w", path, err)
	}
	m := &MarketplaceManifest{
		Name:         om.GetString("name"),
		Description:  om.GetString("description"),
		HasOwner:     isObject(om, "owner"),
		RootDir:      rootDir,
		ManifestPath: path,
	}
	if arr, ok := om.Get("plugins"); ok {
		if list, ok := arr.([]any); ok {
			for _, item := range list {
				e, ok := item.(*OrderedMap)
				if !ok {
					continue
				}
				m.Plugins = append(m.Plugins, &MarketplaceEntry{
					Name:        e.GetString("name"),
					Source:      e.GetString("source"),
					Version:     e.GetString("version"),
					Description: e.GetString("description"),
				})
			}
		}
	}
	return m, nil
}

// ManifestPathFor returns the marketplace.json path a known marketplace's
// install resolves through: "file" sources point at the json itself,
// "directory" sources at <root>/.claude-plugin/marketplace.json.
func (rec *KnownMarketplace) ManifestPathFor() string {
	if rec.SourceType == "file" {
		return rec.SourcePath
	}
	return filepath.Join(rec.InstallLocation, ".claude-plugin", "marketplace.json")
}

// PluginDir resolves an entry's source path against the marketplace root.
func (m *MarketplaceManifest) PluginDir(e *MarketplaceEntry) string {
	return filepath.Join(m.RootDir, filepath.FromSlash(e.Source))
}

// hasNonEmpty reports whether key exists with a non-empty scalar or
// non-empty object value (author: {} or "" count as absent).
func hasNonEmpty(om *OrderedMap, key string) bool {
	v, ok := om.Get(key)
	if !ok {
		return false
	}
	switch t := v.(type) {
	case string:
		return t != ""
	case *OrderedMap:
		return t.Len() > 0
	default:
		return true
	}
}

// isObject reports whether key exists and is a JSON object.
func isObject(om *OrderedMap, key string) bool {
	v, ok := om.Get(key)
	if !ok {
		return false
	}
	_, isObj := v.(*OrderedMap)
	return isObj
}

// unmarshalOrdered parses JSON into an OrderedMap (exported helper is
// UnmarshalJSON on the type; this keeps call sites terse).
func unmarshalOrdered(data []byte, om *OrderedMap) error {
	return om.UnmarshalJSON(data)
}

// --- cache -------------------------------------------------------------------

// InstallToCache copies srcDir (a plugin's source directory inside a
// marketplace) into cache/<marketplace>/<plugin>/<version>/, replacing any
// previous copy (a re-install also clears the .orphaned_at marker).  Returns
// the destination path.
func (s *PluginStore) InstallToCache(marketplace, plugin, version, srcDir string) (string, error) {
	dst := filepath.Join(s.CacheDir(), marketplace, plugin, version)
	if err := os.RemoveAll(dst); err != nil {
		return "", fmt.Errorf("plugin store: clear %s: %w", dst, err)
	}
	if err := copyDir(srcDir, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// OrphanCache marks an uninstalled plugin's cache directory with
// .orphaned_at (epoch milliseconds, the oracle's marker content).  The
// directory itself is left in place — a later prune removes it.
func OrphanCache(installPath string) error {
	if installPath == "" {
		return nil
	}
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		return fmt.Errorf("plugin store: create %s: %w", installPath, err)
	}
	marker := filepath.Join(installPath, ".orphaned_at")
	content := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if err := os.WriteFile(marker, []byte(content), 0o644); err != nil {
		return fmt.Errorf("plugin store: write %s: %w", marker, err)
	}
	return nil
}

// copyDir recursively copies src to dst (files and directories, preserving
// file modes).  An empty/missing src is an error.
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("plugin store: stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("plugin store: %s is not a directory", src)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("plugin store: create %s: %w", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("plugin store: read %s: %w", src, err)
	}
	for _, e := range entries {
		sPath := filepath.Join(src, e.Name())
		dPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(sPath, dPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(sPath)
		if err != nil {
			return fmt.Errorf("plugin store: read %s: %w", sPath, err)
		}
		info, err := e.Info()
		if err != nil {
			return fmt.Errorf("plugin store: stat %s: %w", sPath, err)
		}
		if err := os.WriteFile(dPath, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("plugin store: write %s: %w", dPath, err)
		}
	}
	return nil
}
