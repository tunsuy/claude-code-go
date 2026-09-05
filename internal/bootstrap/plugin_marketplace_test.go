package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// Marketplace subcommand tests, byte-pinned against the oracle captures
// (/tmp/ccg/pv/mp-*.*, claude v2.1.261).

func TestPluginMPAdd(t *testing.T) {
	t.Run("dir-success-and-state", func(t *testing.T) {
		pinPluginTime(t)
		out, errb, store := swapPluginDeps(t)
		root := writeMarketplaceFixture(t, t.TempDir())
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "add", root); err != nil {
			t.Fatal(err)
		}
		want := "Adding marketplace…✔ Successfully added marketplace: mp (declared in user settings)\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
		km, err := store.LoadKnownMarketplaces()
		if err != nil {
			t.Fatal(err)
		}
		rec := km.Get("mp")
		if rec == nil || rec.SourceType != "directory" || rec.SourcePath != root ||
			rec.InstallLocation != root || rec.LastUpdated != "2026-09-05T03:06:53.301Z" {
			t.Fatalf("registry record = %+v", rec)
		}
		extras, err := store.ExtraMarketplaces(config.PluginScopeUser)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := extras.Get("mp"); !ok {
			t.Fatal("user extras missing mp declaration")
		}
	})
	t.Run("duplicate-already-on-disk", func(t *testing.T) {
		out, errb, store, root := setupInstalledFixture(t)
		out.Reset()
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "add", root, "--scope", "local"); err != nil {
			t.Fatal(err)
		}
		want := "Adding marketplace…✔ Marketplace 'mp' already on disk — declared in local settings\n"
		if out.String() != want {
			t.Fatalf("stdout = %q", out.String())
		}
		// Registry survives; the new scope declares it too.
		km, err := store.LoadKnownMarketplaces()
		if err != nil {
			t.Fatal(err)
		}
		if km.Get("mp") == nil || km.Len() != 1 {
			t.Fatalf("registry after duplicate = %v", km.Names())
		}
		local, err := store.ExtraMarketplaces(config.PluginScopeLocal)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := local.Get("mp"); !ok {
			t.Fatal("local declaration missing after duplicate add")
		}
	})
	t.Run("file-source", func(t *testing.T) {
		out, errb, store := swapPluginDeps(t)
		root := writeMarketplaceFixture(t, t.TempDir())
		manifestPath := filepath.Join(root, ".claude-plugin", "marketplace.json")
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "add", manifestPath); err != nil {
			t.Fatal(err)
		}
		want := "Adding marketplace…✔ Successfully added marketplace: mp (declared in user settings)\n"
		if out.String() != want {
			t.Fatalf("stdout = %q", out.String())
		}
		km, err := store.LoadKnownMarketplaces()
		if err != nil {
			t.Fatal(err)
		}
		rec := km.Get("mp")
		if rec == nil || rec.SourceType != "file" || rec.SourcePath != manifestPath || rec.InstallLocation != root {
			t.Fatalf("registry record = %+v", rec)
		}
	})
	t.Run("missing-path", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		base := t.TempDir()
		missing := filepath.Join(base, "no-such-mp-dir")
		err := runPlugin(t, out, errb, "plugin", "marketplace", "add", missing)
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Path does not exist: " + missing + "\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("invalid-format", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "add", ".")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Invalid marketplace source format. Try: owner/repo, https://..., or ./path\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("remote-owner-repo", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "add", "anthropics/claude-code")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Remote marketplace sources are not supported by this build. Use a local path (./dir or an absolute path).\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("remote-https", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "add", "https://example.com/mp")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "✘ Remote marketplace sources are not supported by this build. Use a local path (./dir or an absolute path).\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("no-manifest", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		empty := t.TempDir()
		err := runPlugin(t, out, errb, "plugin", "marketplace", "add", empty)
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.String() != "Adding marketplace…" {
			t.Fatalf("stdout = %q", out.String())
		}
		want := "✘ Failed to add marketplace: Marketplace file not found at " + empty + "/.claude-plugin/marketplace.json\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
	})
	t.Run("claudeai-unsupported", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "add", "--claudeai", "foo/bar")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ --claudeai marketplaces (hosted on claude.ai) are not supported by this build. Add a local marketplace path instead.\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("invalid-scope", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		root := writeMarketplaceFixture(t, t.TempDir())
		err := runPlugin(t, out, errb, "plugin", "marketplace", "add", root, "--scope", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Invalid scope 'bogus'. Use: user, project, or local\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
}

func TestPluginMPList(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		out, errb, _, root := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "list"); err != nil {
			t.Fatal(err)
		}
		want := "Configured marketplaces:\n\n  ❯ mp\n    Source: Directory (" + root + ")\n\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		out, errb, _, root := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "list", "--json"); err != nil {
			t.Fatal(err)
		}
		want := "[\n  {\n    \"name\": \"mp\",\n    \"source\": \"directory\",\n    \"path\": \"" + root +
			"\",\n    \"installLocation\": \"" + root + "\"\n  }\n]\n"
		if out.String() != want {
			t.Fatalf("stdout = %q, want %q", out.String(), want)
		}
	})
	t.Run("empty", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "list"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "No marketplaces configured\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
}

func TestPluginMPRemove(t *testing.T) {
	t.Run("success-uninstalls-plugins", func(t *testing.T) {
		out, errb, store, _ := setupInstalledFixture(t)
		cachePath := filepath.Join(store.CacheDir(), "mp", "hello", "1.0.0")
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "remove", "mp"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ Successfully removed marketplace: mp\n" || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
		km, err := store.LoadKnownMarketplaces()
		if err != nil {
			t.Fatal(err)
		}
		if km.Len() != 0 {
			t.Fatalf("registry after remove = %v", km.Names())
		}
		ip, err := store.LoadInstalledPlugins()
		if err != nil {
			t.Fatal(err)
		}
		if len(ip.IDs()) != 0 {
			t.Fatalf("installed after remove = %v", ip.IDs())
		}
		if _, err := os.Stat(filepath.Join(cachePath, ".orphaned_at")); err != nil {
			t.Fatalf("orphan marker missing: %v", err)
		}
	})
	t.Run("not-found", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "remove", "foo")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "✘ Failed to remove marketplace: Marketplace 'foo' not found\n"
		if errb.String() != want || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("explicit-scope-keeps-registry", func(t *testing.T) {
		// Unverified edge (no oracle capture): --scope removes only the
		// declaration; the registry record and installed plugins survive.
		out, errb, store, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "remove", "mp", "--scope", "user"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "✔ Successfully removed marketplace: mp\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		km, err := store.LoadKnownMarketplaces()
		if err != nil {
			t.Fatal(err)
		}
		if km.Get("mp") == nil {
			t.Fatal("registry record dropped by scoped remove")
		}
		extras, err := store.ExtraMarketplaces(config.PluginScopeUser)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := extras.Get("mp"); ok {
			t.Fatal("user declaration survived scoped remove")
		}
	})
}

func TestPluginMPUpdate(t *testing.T) {
	t.Run("named-missing", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "update", "foo")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.String() != "Updating marketplace: foo...\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		want := "✘ Failed to update marketplace(s): Marketplace 'foo' not found. Available marketplaces: mp\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("named-success", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "update", "mp"); err != nil {
			t.Fatal(err)
		}
		want := "Updating marketplace: mp...\nValidating local marketplace\n✔ Successfully updated marketplace: mp\n"
		if out.String() != want || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("update-all", func(t *testing.T) {
		out, errb, _, _ := setupInstalledFixture(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "update"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "Updating marketplaces...✔ Successfully updated 1 marketplace\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("update-all-zero", func(t *testing.T) {
		// Unverified edge (no oracle capture): zero marketplaces fails after
		// the progress line.
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "update")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.String() != "Updating marketplaces...\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		if errb.String() != "✘ Failed to update marketplace(s): No marketplaces configured\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
}
