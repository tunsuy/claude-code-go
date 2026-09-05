package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run-body tests for `plugin list` / `enable` / `disable` / `update`, byte-
// pinned against the oracle captures in /tmp/ccg/pv (claude v2.1.261).

func TestPluginListEmpty(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "list"); err != nil {
			t.Fatal(err)
		}
		want := "No plugins installed. Use `claude plugin install` to install a plugin.\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "list", "--json"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "[]\n" || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("available-text-ignored-without-json", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "list", "--available"); err != nil {
			t.Fatal(err)
		}
		want := "No plugins installed. Use `claude plugin install` to install a plugin.\n"
		if out.String() != want {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("available-json-no-marketplaces", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "list", "--available", "--json"); err != nil {
			t.Fatal(err)
		}
		want := "{\n  \"installed\": [],\n  \"available\": []\n}\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
}

func TestPluginListOne(t *testing.T) {
	out, errb, store, _ := setupInstalledFixture(t)

	installPath := filepath.Join(store.CacheDir(), "mp", "hello", "1.0.0")
	out.Reset()
	if err := runPlugin(t, out, errb, "plugin", "list"); err != nil {
		t.Fatal(err)
	}
	wantText := "Installed plugins:\n\n  ❯ hello@mp\n    Version: 1.0.0\n    Scope: user\n    Status: ✔ enabled\n\n"
	if out.String() != wantText {
		t.Fatalf("text:\n got %q\nwant %q", out.String(), wantText)
	}

	out.Reset()
	if err := runPlugin(t, out, errb, "plugin", "list", "--json"); err != nil {
		t.Fatal(err)
	}
	wantJSON := `[
  {
    "id": "hello@mp",
    "version": "1.0.0",
    "scope": "user",
    "enabled": true,
    "installPath": "` + installPath + `",
    "installedAt": "2026-09-05T03:06:53.301Z",
    "lastUpdated": "2026-09-05T03:06:53.301Z"
  }
]
`
	if out.String() != wantJSON {
		t.Fatalf("json:\n got %q\nwant %q", out.String(), wantJSON)
	}

	// --available folds the not-yet-installed marketplace entries in.
	out.Reset()
	if err := runPlugin(t, out, errb, "plugin", "list", "--available", "--json"); err != nil {
		t.Fatal(err)
	}
	wantAvail := `{
  "installed": [
    {
      "id": "hello@mp",
      "version": "1.0.0",
      "scope": "user",
      "enabled": true,
      "installPath": "` + installPath + `",
      "installedAt": "2026-09-05T03:06:53.301Z",
      "lastUpdated": "2026-09-05T03:06:53.301Z"
    }
  ],
  "available": [
    {
      "pluginId": "world@mp",
      "name": "world",
      "description": "World plugin",
      "marketplaceName": "mp",
      "version": "2.0.0",
      "source": "./plugins/world"
    }
  ]
}
`
	if out.String() != wantAvail {
		t.Fatalf("avail json:\n got %q\nwant %q", out.String(), wantAvail)
	}
}

func TestPluginEnableDisableLifecycle(t *testing.T) {
	t.Run("disable-then-list-then-enable", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)

		if err := runPlugin(t, out, errb, "plugin", "disable", "hello"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ Successfully disabled plugin: hello (scope: user)\n" {
			t.Fatalf("disable stdout = %q", out.String())
		}

		out.Reset()
		if err := runPlugin(t, out, errb, "plugin", "list"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Status: ✘ disabled") {
			t.Fatalf("list after disable = %q", out.String())
		}

		out.Reset()
		if err := runPlugin(t, out, errb, "plugin", "enable", "hello@mp"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ Successfully enabled plugin: hello (scope: user)\n" {
			t.Fatalf("enable stdout = %q", out.String())
		}

		out.Reset()
		if err := runPlugin(t, out, errb, "plugin", "list"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Status: ✔ enabled") {
			t.Fatalf("list after enable = %q", out.String())
		}
	})
	t.Run("already-enabled-without-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "enable", "hello")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to enable plugin \"hello\": Plugin \"hello\" is already enabled\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("already-enabled-with-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "enable", "hello", "-s", "user")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to enable plugin \"hello\": Plugin \"hello\" is already enabled at user scope\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("already-disabled", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "disable", "hello"); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		errb.Reset()
		err := runPlugin(t, out, errb, "plugin", "disable", "hello")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to disable plugin \"hello\": Plugin \"hello\" is already disabled\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("enable-missing", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "enable", "nosuchplugin")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to enable plugin \"nosuchplugin\": Plugin \"nosuchplugin\" not found in any editable settings scope. Use plugin@marketplace format.\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("disable-missing", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "disable", "nosuchplugin")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to disable plugin \"nosuchplugin\": Plugin \"nosuchplugin\" not found in any editable settings scope. Use plugin@marketplace format.\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("enable-invalid-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "enable", "hello", "-s", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "Invalid scope \"bogus\". Valid scopes: user, project, local\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
}

func TestPluginDisableAllAndBare(t *testing.T) {
	t.Run("disable-all-one", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "disable", "--all"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ Disabled 1 plugin\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		out.Reset()
		if err := runPlugin(t, out, errb, "plugin", "disable", "--all"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ No enabled plugins to disable\n" {
			t.Fatalf("stdout again = %q", out.String())
		}
	})
	t.Run("disable-all-nothing-installed", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "disable", "--all"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ No enabled plugins to disable\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("bare-disable", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "disable")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "Please specify a plugin name or use --all to disable all plugins\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
}

func TestPluginUpdate(t *testing.T) {
	t.Run("missing-user-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "update", "nosuchplugin")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.String() != "Checking for updates for plugin \"nosuchplugin\" at user scope…\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		if errb.String() != "✘ Failed to update plugin \"nosuchplugin\": Plugin \"nosuchplugin\" is not installed\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("missing-managed-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "update", "hello", "-s", "managed")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to update plugin \"hello\": Plugin \"hello\" is not installed at scope managed\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("missing-project-scope", func(t *testing.T) {
		out, errb, store, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "update", "hello", "-s", "project")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		cwd, cerr := filepath.EvalSymlinks(store.ProjectDir())
		if cerr != nil {
			cwd = store.ProjectDir()
		}
		want := "✘ Failed to update plugin \"hello\": Plugin \"hello\" is not installed at scope project (" + cwd + ")\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
	})
	t.Run("invalid-scope", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "update", "hello", "-s", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "Invalid scope \"bogus\". Valid scopes: user, project, local, managed\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("already-latest", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "update", "hello"); err != nil {
			t.Fatal(err)
		}
		want := "Checking for updates for plugin \"hello\" at user scope…\n✔ hello is already at the latest version (1.0.0).\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("update-to-newer-plugin-json", func(t *testing.T) {
		out, errb, store, root := setupInstalledFixture(t)
		// plugin.json wins over the marketplace entry version, so bumping it
		// makes 1.1.0 the latest.
		pj := filepath.Join(root, "plugins", "hello", ".claude-plugin", "plugin.json")
		if err := os.WriteFile(pj, []byte(`{"name":"hello","version":"1.1.0","description":"Hello plugin","author":{"name":"probe"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := runPlugin(t, out, errb, "plugin", "update", "hello"); err != nil {
			t.Fatal(err)
		}
		want := "Checking for updates for plugin \"hello\" at user scope…\n" +
			"✔ Plugin \"hello\" updated from 1.0.0 to 1.1.0 for scope user. Restart to apply changes.\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
		// The record now carries 1.1.0 and the new cache path.
		ip, err := store.LoadInstalledPlugins()
		if err != nil {
			t.Fatal(err)
		}
		if recs := ip.Records("hello@mp"); len(recs) != 1 || recs[0].Version != "1.1.0" {
			t.Fatalf("record after update = %+v", recs)
		}
		if _, err := os.Stat(filepath.Join(store.CacheDir(), "mp", "hello", "1.1.0")); err != nil {
			t.Fatalf("new cache dir missing: %v", err)
		}
	})
}
