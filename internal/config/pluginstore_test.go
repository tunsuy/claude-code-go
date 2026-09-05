package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestPluginStore builds a plugin store over fresh temp home/project dirs.
func newTestPluginStore(t *testing.T) *PluginStore {
	t.Helper()
	return NewPluginStore(t.TempDir(), t.TempDir())
}

// installRec builds a record with all fields set (project scope adds
// projectPath).
func installRec(scope string) *InstalledPluginRecord {
	r := &InstalledPluginRecord{
		Scope:       scope,
		InstallPath: filepath.Join("/cache", "mp", "p", "1.0.0"),
		Version:     "1.0.0",
		InstalledAt: "2026-09-05T03:16:38.020Z",
		LastUpdated: "2026-09-05T03:16:38.020Z",
	}
	if scope == PluginScopeProject {
		r.ProjectPath = "/resolved/cwd"
	}
	return r
}

func TestPluginStorePaths(t *testing.T) {
	t.Parallel()
	s := NewPluginStore("/home/u", "/proj/wd")
	if got := s.PluginsDir(); got != filepath.Join("/home/u", ".claude", "plugins") {
		t.Fatalf("PluginsDir = %q", got)
	}
	if got := s.CacheDir(); got != filepath.Join("/home/u", ".claude", "plugins", "cache") {
		t.Fatalf("CacheDir = %q", got)
	}
	if got := s.InstalledPluginsPath(); got != filepath.Join("/home/u", ".claude", "plugins", "installed_plugins.json") {
		t.Fatalf("InstalledPluginsPath = %q", got)
	}
	if got := s.KnownMarketplacesPath(); got != filepath.Join("/home/u", ".claude", "plugins", "known_marketplaces.json") {
		t.Fatalf("KnownMarketplacesPath = %q", got)
	}
	// The plugin settings trio: user home, project shared, project local.
	// Note the local path is .claude/settings.local.json (the oracle's plugin
	// settings file), not the loader's legacy .claude.local path.
	cases := map[string]string{
		PluginScopeUser:    filepath.Join("/home/u", ".claude", "settings.json"),
		PluginScopeProject: filepath.Join("/proj/wd", ".claude", "settings.json"),
		PluginScopeLocal:   filepath.Join("/proj/wd", ".claude", "settings.local.json"),
	}
	for scope, want := range cases {
		if got := s.SettingsPath(scope); got != want {
			t.Fatalf("SettingsPath(%q) = %q, want %q", scope, got, want)
		}
	}
}

func TestPluginTimestampFormat(t *testing.T) {
	t.Parallel()
	// 2026-09-05 03:16:38.0205 UTC → the oracle's millisecond-Z format.
	got := PluginTimestamp(time.Date(2026, 9, 5, 3, 16, 38, 20500000, time.UTC))
	if got != "2026-09-05T03:16:38.020Z" {
		t.Fatalf("PluginTimestamp = %q", got)
	}
	// Non-UTC input converts.
	loc := time.FixedZone("CST", 8*3600)
	got = PluginTimestamp(time.Date(2026, 9, 5, 11, 16, 38, 0, loc))
	if got != "2026-09-05T03:16:38.000Z" {
		t.Fatalf("PluginTimestamp(non-UTC) = %q", got)
	}
}

func TestInstalledPluginsRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestPluginStore(t)

	// Missing file → empty registry, no error.
	ip, err := s.LoadInstalledPlugins()
	if err != nil {
		t.Fatal(err)
	}
	if ids := ip.IDs(); len(ids) != 0 {
		t.Fatalf("fresh registry ids = %v", ids)
	}

	ip.SetRecords("myplugin@mp1", []*InstalledPluginRecord{installRec(PluginScopeUser)})
	ip.SetRecords("rich@mp1", []*InstalledPluginRecord{installRec(PluginScopeUser)})
	if err := ip.Save(s); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(s.InstalledPluginsPath())
	if err != nil {
		t.Fatal(err)
	}
	// Byte shape: 2-space indent, no trailing newline, {version, plugins}
	// order, record key order with projectPath omitted for user scope.
	want := `{
  "version": 2,
  "plugins": {
    "myplugin@mp1": [
      {
        "scope": "user",
        "installPath": "/cache/mp/p/1.0.0",
        "version": "1.0.0",
        "installedAt": "2026-09-05T03:16:38.020Z",
        "lastUpdated": "2026-09-05T03:16:38.020Z"
      }
    ],
    "rich@mp1": [
      {
        "scope": "user",
        "installPath": "/cache/mp/p/1.0.0",
        "version": "1.0.0",
        "installedAt": "2026-09-05T03:16:38.020Z",
        "lastUpdated": "2026-09-05T03:16:38.020Z"
      }
    ]
  }
}`
	if string(raw) != want {
		t.Fatalf("installed_plugins.json =\n%s\nwant\n%s", raw, want)
	}

	// Reload preserves insertion order and appends multi-scope records.
	ip2, err := s.LoadInstalledPlugins()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ip2.IDs(), ","); got != "myplugin@mp1,rich@mp1" {
		t.Fatalf("ids order = %q", got)
	}
	ip2.SetRecords("myplugin@mp1", append(ip2.Records("myplugin@mp1"), installRec(PluginScopeProject)))
	if err := ip2.Save(s); err != nil {
		t.Fatal(err)
	}
	ip3, err := s.LoadInstalledPlugins()
	if err != nil {
		t.Fatal(err)
	}
	recs := ip3.Records("myplugin@mp1")
	if len(recs) != 2 || recs[0].Scope != PluginScopeUser || recs[1].Scope != PluginScopeProject {
		t.Fatalf("records = %+v", recs)
	}
	if recs[1].ProjectPath != "/resolved/cwd" {
		t.Fatalf("project record projectPath = %q", recs[1].ProjectPath)
	}
	if r := ip3.RecordAt("myplugin@mp1", PluginScopeProject); r == nil || r.ProjectPath != "/resolved/cwd" {
		t.Fatalf("RecordAt(project) = %+v", r)
	}
	if r := ip3.RecordAt("myplugin@mp1", PluginScopeLocal); r != nil {
		t.Fatalf("RecordAt(local) = %+v, want nil", r)
	}

	// Delete drops the id.
	ip3.Delete("rich@mp1")
	if err := ip3.Save(s); err != nil {
		t.Fatal(err)
	}
	ip4, err := s.LoadInstalledPlugins()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ip4.IDs(), ","); got != "myplugin@mp1" {
		t.Fatalf("after delete ids = %q", got)
	}
}

func TestInstalledPluginsFind(t *testing.T) {
	t.Parallel()
	s := newTestPluginStore(t)
	ip, err := s.LoadInstalledPlugins()
	if err != nil {
		t.Fatal(err)
	}
	ip.SetRecords("myplugin@mp1", []*InstalledPluginRecord{installRec(PluginScopeUser)})
	ip.SetRecords("other@mp2", []*InstalledPluginRecord{installRec(PluginScopeUser)})
	ip.SetRecords("myplugin@mp9", []*InstalledPluginRecord{installRec(PluginScopeLocal)})

	cases := []struct {
		ref  string
		want string // resolved id, "" for miss
	}{
		{"myplugin@mp1", "myplugin@mp1"}, // exact id
		{"myplugin", "myplugin@mp1"},     // name part → first insertion match
		{"other", "other@mp2"},
		{"ghost", ""},
		{"ghost@mp1", ""}, // no exact id and no name-part match
	}
	for _, tc := range cases {
		id, recs := ip.Find(tc.ref)
		if id != tc.want {
			t.Fatalf("Find(%q) id = %q, want %q", tc.ref, id, tc.want)
		}
		if (id == "") != (len(recs) == 0) {
			t.Fatalf("Find(%q) recs/id mismatch: %q %+v", tc.ref, id, recs)
		}
	}
}

func TestKnownMarketplacesRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestPluginStore(t)
	km, err := s.LoadKnownMarketplaces()
	if err != nil {
		t.Fatal(err)
	}
	if km.Len() != 0 {
		t.Fatalf("fresh registry len = %d", km.Len())
	}
	km.Set("mp1", &KnownMarketplace{
		SourceType:      "directory",
		SourcePath:      "/tmp/ccg/mp1",
		InstallLocation: "/tmp/ccg/mp1",
		LastUpdated:     "2026-09-05T03:16:38.020Z",
	})
	km.Set("mpfile", &KnownMarketplace{
		SourceType:      "file",
		SourcePath:      "/tmp/ccg/mp1/.claude-plugin/marketplace.json",
		InstallLocation: "/tmp/ccg/mp1",
		LastUpdated:     "2026-09-05T03:16:38.020Z",
	})
	if err := km.Save(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.KnownMarketplacesPath())
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "mp1": {
    "source": {
      "source": "directory",
      "path": "/tmp/ccg/mp1"
    },
    "installLocation": "/tmp/ccg/mp1",
    "lastUpdated": "2026-09-05T03:16:38.020Z"
  },
  "mpfile": {
    "source": {
      "source": "file",
      "path": "/tmp/ccg/mp1/.claude-plugin/marketplace.json"
    },
    "installLocation": "/tmp/ccg/mp1",
    "lastUpdated": "2026-09-05T03:16:38.020Z"
  }
}`
	if string(raw) != want {
		t.Fatalf("known_marketplaces.json =\n%s\nwant\n%s", raw, want)
	}

	km2, err := s.LoadKnownMarketplaces()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(km2.Names(), ","); got != "mp1,mpfile" {
		t.Fatalf("names = %q", got)
	}
	rec := km2.Get("mp1")
	if rec == nil || rec.SourceType != "directory" || rec.InstallLocation != "/tmp/ccg/mp1" {
		t.Fatalf("Get(mp1) = %+v", rec)
	}
	if got := rec.ManifestPathFor(); got != filepath.Join("/tmp/ccg/mp1", ".claude-plugin", "marketplace.json") {
		t.Fatalf("directory ManifestPathFor = %q", got)
	}
	recF := km2.Get("mpfile")
	if got := recF.ManifestPathFor(); got != "/tmp/ccg/mp1/.claude-plugin/marketplace.json" {
		t.Fatalf("file ManifestPathFor = %q", got)
	}
	if km2.Get("ghost") != nil {
		t.Fatal("Get(ghost) must be nil")
	}
	km2.Delete("mp1")
	if err := km2.Save(s); err != nil {
		t.Fatal(err)
	}
	km3, err := s.LoadKnownMarketplaces()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(km3.Names(), ","); got != "mpfile" {
		t.Fatalf("after delete names = %q", got)
	}
}

func TestPluginSettingsOps(t *testing.T) {
	t.Parallel()
	s := newTestPluginStore(t)

	// SetPluginEnabled creates the file and preserves foreign keys/order in
	// an existing one.
	writeTestJSON(t, s.SettingsPath(PluginScopeUser),
		`{"model":"sonnet","verbose":true}`)
	if err := s.SetPluginEnabled(PluginScopeUser, "myplugin@mp1", true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.SettingsPath(PluginScopeUser))
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "model": "sonnet",
  "verbose": true,
  "enabledPlugins": {
    "myplugin@mp1": true
  }
}`
	if string(raw) != want {
		t.Fatalf("settings.json =\n%s\nwant\n%s", raw, want)
	}

	// Toggling replaces in place.
	if err := s.SetPluginEnabled(PluginScopeUser, "myplugin@mp1", false); err != nil {
		t.Fatal(err)
	}
	enabled, err := s.EnabledPlugins(PluginScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := enabled.Get("myplugin@mp1")
	if v != false {
		t.Fatalf("enabled value = %v, want false", v)
	}

	// RemovePluginEnabled keeps the (now empty) enabledPlugins key.
	if err := s.RemovePluginEnabled(PluginScopeUser, "myplugin@mp1"); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(s.SettingsPath(PluginScopeUser))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"enabledPlugins": {}`) {
		t.Fatalf("empty enabledPlugins key must be retained: %s", raw)
	}

	// Removing an absent key in a missing file creates nothing.
	if err := s.RemovePluginEnabled(PluginScopeLocal, "ghost"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.SettingsPath(PluginScopeLocal)); !os.IsNotExist(err) {
		t.Fatalf("local settings must not be created by a delete-only op")
	}
}

func TestExtraMarketplaceOps(t *testing.T) {
	t.Parallel()
	s := newTestPluginStore(t)
	if err := s.SetExtraMarketplace(PluginScopeUser, "mp1", "directory", "/tmp/ccg/mp1"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.SettingsPath(PluginScopeUser))
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "extraKnownMarketplaces": {
    "mp1": {
      "source": {
        "source": "directory",
        "path": "/tmp/ccg/mp1"
      }
    }
  }
}`
	if string(raw) != want {
		t.Fatalf("settings.json =\n%s\nwant\n%s", raw, want)
	}
	m, err := s.ExtraMarketplaces(PluginScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1 {
		t.Fatalf("extra marketplaces len = %d", m.Len())
	}
	if err := s.RemoveExtraMarketplace(PluginScopeUser, "mp1"); err != nil {
		t.Fatal(err)
	}
	m, err = s.ExtraMarketplaces(PluginScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if m.Len() != 0 {
		t.Fatalf("after remove len = %d", m.Len())
	}
	// The empty extraKnownMarketplaces key is retained, like enabledPlugins.
	raw, err = os.ReadFile(s.SettingsPath(PluginScopeUser))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"extraKnownMarketplaces": {}`) {
		t.Fatalf("empty key must be retained: %s", raw)
	}
}

func TestLoadPluginManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Missing manifest → nil, nil.
	m, err := LoadPluginManifest(dir)
	if err != nil || m != nil {
		t.Fatalf("missing manifest = (%+v, %v)", m, err)
	}
	writeTestJSON(t, filepath.Join(dir, ".claude-plugin", "plugin.json"),
		`{"name":"myplugin","version":"9.9.9","description":"A probe plugin"}`)
	m, err = LoadPluginManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "myplugin" || m.Version != "9.9.9" || m.Description != "A probe plugin" || m.HasAuthor {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestLoadMarketplaceManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Missing manifest → nil, nil.
	m, err := LoadMarketplaceManifest(root)
	if err != nil || m != nil {
		t.Fatalf("missing manifest = (%+v, %v)", m, err)
	}
	writeTestJSON(t, filepath.Join(root, ".claude-plugin", "marketplace.json"),
		`{"name":"mp1","owner":{"name":"probe"},"description":"Probe marketplace",
		  "plugins":[{"name":"myplugin","source":"./plugins/myplugin","version":"2.0.0","description":"A probe plugin"}]}`)
	m, err = LoadMarketplaceManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "mp1" || !m.HasOwner || m.RootDir != root || len(m.Plugins) != 1 {
		t.Fatalf("manifest = %+v", m)
	}
	e := m.Plugins[0]
	if e.Name != "myplugin" || e.Source != "./plugins/myplugin" || e.Version != "2.0.0" {
		t.Fatalf("entry = %+v", e)
	}
	// PluginDir resolves against the marketplace root.
	if got := m.PluginDir(e); got != filepath.Join(root, "plugins", "myplugin") {
		t.Fatalf("PluginDir = %q", got)
	}

	// File-path load: root is the dir containing the .claude-plugin dir.
	fm, err := LoadMarketplaceManifestFile(filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fm.RootDir != root || fm.Name != "mp1" {
		t.Fatalf("file-loaded manifest = %+v", fm)
	}
}

func TestInstallToCacheAndOrphan(t *testing.T) {
	t.Parallel()
	s := newTestPluginStore(t)
	src := t.TempDir()
	writeTestJSON(t, filepath.Join(src, ".claude-plugin", "plugin.json"), `{"name":"p"}`)
	if err := os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	dst, err := s.InstallToCache("mp1", "p", "1.0.0", src)
	if err != nil {
		t.Fatal(err)
	}
	if dst != filepath.Join(s.CacheDir(), "mp1", "p", "1.0.0") {
		t.Fatalf("dst = %q", dst)
	}
	data, err := os.ReadFile(filepath.Join(dst, ".claude-plugin", "plugin.json"))
	if err != nil || string(data) != `{"name":"p"}` {
		t.Fatalf("cached manifest = (%q, %v)", data, err)
	}
	// Executable mode preserved.
	info, err := os.Stat(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("run.sh mode = %v, want executable", info.Mode())
	}

	// Orphan writes an epoch-ms .orphaned_at inside the cache dir.
	if err := OrphanCache(dst); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(dst, ".orphaned_at"))
	if err != nil {
		t.Fatal(err)
	}
	if len(marker) == 0 || strings.ContainsAny(string(marker), ".") {
		t.Fatalf(".orphaned_at = %q, want epoch milliseconds", marker)
	}

	// Reinstall replaces the directory, clearing the orphan marker.
	if _, err := s.InstallToCache("mp1", "p", "1.0.0", src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".orphaned_at")); !os.IsNotExist(err) {
		t.Fatalf(".orphaned_at must be gone after reinstall: %v", err)
	}
}

func TestPluginNamePart(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"myplugin@mp1": "myplugin",
		"myplugin":     "myplugin",
		"@mp1":         "",
	}
	for in, want := range cases {
		if got := PluginNamePart(in); got != want {
			t.Fatalf("PluginNamePart(%q) = %q, want %q", in, got, want)
		}
	}
}
