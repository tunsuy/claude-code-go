package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// Install / uninstall tests, byte-pinned against the oracle captures
// (/tmp/ccg/pv/install1.*, uninstall-ok.*, uninstall-missing.*).

func TestPluginInstall(t *testing.T) {
	t.Run("success-bytes-and-state", func(t *testing.T) {
		pinPluginTime(t)
		out, errb, store := swapPluginDeps(t)
		root := writeMarketplaceFixture(t, t.TempDir())
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "add", root); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		errb.Reset()

		if err := runPlugin(t, out, errb, "plugin", "install", "hello"); err != nil {
			t.Fatal(err)
		}
		want := "Installing plugin \"hello\"...✔ Successfully installed plugin: hello@mp (scope: user)\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}

		// State: registry record, cache copy, enabled settings key.
		ip, err := store.LoadInstalledPlugins()
		if err != nil {
			t.Fatal(err)
		}
		recs := ip.Records("hello@mp")
		if len(recs) != 1 {
			t.Fatalf("records = %+v", recs)
		}
		wantPath := filepath.Join(store.CacheDir(), "mp", "hello", "1.0.0")
		if recs[0].Scope != config.PluginScopeUser || recs[0].InstallPath != wantPath ||
			recs[0].Version != "1.0.0" || recs[0].InstalledAt != "2026-09-05T03:06:53.301Z" ||
			recs[0].LastUpdated != "2026-09-05T03:06:53.301Z" {
			t.Fatalf("record = %+v", recs[0])
		}
		if _, err := os.Stat(filepath.Join(wantPath, ".claude-plugin", "plugin.json")); err != nil {
			t.Fatalf("cache copy missing: %v", err)
		}
		enabled, err := store.EnabledPlugins(config.PluginScopeUser)
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := enabled.Get("hello@mp"); !ok || v != true {
			t.Fatalf("enabledPlugins[user][hello@mp] = %v, %v", v, ok)
		}
	})
	t.Run("at-form", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		out.Reset()
		if err := runPlugin(t, out, errb, "plugin", "uninstall", "hello"); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		if err := runPlugin(t, out, errb, "plugin", "install", "hello@mp"); err != nil {
			t.Fatal(err)
		}
		want := "Installing plugin \"hello@mp\"...✔ Successfully installed plugin: hello@mp (scope: user)\n"
		if out.String() != want {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("project-scope-record", func(t *testing.T) {
		out, errb, store, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "install", "hello", "-s", "project"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "Installing plugin \"hello\"...✔ Successfully installed plugin: hello@mp (scope: project)\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		ip, err := store.LoadInstalledPlugins()
		if err != nil {
			t.Fatal(err)
		}
		if rec := pluginRecordAtScope(ip.Records("hello@mp"), config.PluginScopeProject); rec == nil || rec.ProjectPath == "" {
			t.Fatalf("project record = %+v", ip.Records("hello@mp"))
		}
	})
	t.Run("already-installed", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "install", "hello"); err != nil {
			t.Fatal(err)
		}
		want := "Installing plugin \"hello\"...✔ Plugin \"hello@mp\" is already installed (scope: user)\n"
		if out.String() != want {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("not-found-any-marketplace", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "install", "nosuch")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.String() != "Installing plugin \"nosuch\"...\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		want := "✘ Failed to install plugin \"nosuch\": Plugin \"nosuch\" not found in any configured marketplace\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("not-found-in-named-marketplace", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "install", "nosuch@mp")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.String() != "Installing plugin \"nosuch@mp\"...\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		want := "✘ Failed to install plugin \"nosuch@mp\": Plugin \"nosuch\" not found in marketplace \"mp\". Your local copy may be out of date — try `claude plugin marketplace update mp`.\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
	})
	t.Run("invalid-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "install", "hello", "-s", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "Invalid scope: bogus. Must be one of: user, project, local.\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
}

func TestPluginUninstall(t *testing.T) {
	t.Run("success-and-state", func(t *testing.T) {
		out, errb, store, _ := setupInstalledFixture(t)
		cachePath := filepath.Join(store.CacheDir(), "mp", "hello", "1.0.0")
		if err := runPlugin(t, out, errb, "plugin", "uninstall", "hello"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ Successfully uninstalled plugin: hello (scope: user)\n" || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
		ip, err := store.LoadInstalledPlugins()
		if err != nil {
			t.Fatal(err)
		}
		if recs := ip.Records("hello@mp"); len(recs) != 0 {
			t.Fatalf("records after uninstall = %+v", recs)
		}
		if _, err := os.Stat(filepath.Join(cachePath, ".orphaned_at")); err != nil {
			t.Fatalf("orphan marker missing: %v", err)
		}
		enabled, err := store.EnabledPlugins(config.PluginScopeUser)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := enabled.Get("hello@mp"); ok {
			t.Fatal("enabledPlugins key survived uninstall")
		}
	})
	t.Run("remove-alias", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "remove", "hello"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ Successfully uninstalled plugin: hello (scope: user)\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("missing", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "uninstall", "nosuchplugin")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to uninstall plugin \"nosuchplugin\": Plugin \"nosuchplugin\" not found in installed plugins\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("wrong-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "uninstall", "hello", "-s", "project")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to uninstall plugin \"hello\": Plugin \"hello\" is not installed in project scope. Use --scope to specify the correct scope.\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("prune-note", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "uninstall", "hello", "--prune"); err != nil {
			t.Fatal(err)
		}
		want := "✔ Successfully uninstalled plugin: hello (scope: user)\n" +
			"Nothing to prune (no auto-installed plugins at user scope).\n"
		if out.String() != want {
			t.Fatalf("stdout = %q", out.String())
		}
	})
}
