package bootstrap

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// End-to-end cobra execution tests for the `claude plugin` tree.  The global
// pluginDepsBuilder is swapped to pin home/project dirs; the bootstrap
// package runs tests sequentially (no t.Parallel anywhere), so the swap is
// safe with a t.Cleanup restore.

// swapPluginDeps replaces pluginDepsBuilder for the test and returns the
// captured streams plus a store over fresh temp home/project dirs.
func swapPluginDeps(t *testing.T) (*bytes.Buffer, *bytes.Buffer, *config.PluginStore) {
	t.Helper()
	var out, errb bytes.Buffer
	store := config.NewPluginStore(t.TempDir(), t.TempDir())
	saved := pluginDepsBuilder
	pluginDepsBuilder = func(stdout, stderr io.Writer) (*pluginDeps, error) {
		return &pluginDeps{Stdout: stdout, Stderr: stderr, Store: store}, nil
	}
	t.Cleanup(func() { pluginDepsBuilder = saved })
	return &out, &errb, store
}

// runPlugin executes argv against the plugin tree and returns the resulting
// error.  Mounted under a fake root for the same reason as the mcp tests:
// cobra's legacyArgs would reject positionals on a bare root.
func runPlugin(t *testing.T, out, errb *bytes.Buffer, argv ...string) error {
	t.Helper()
	root := &cobra.Command{Use: "claude", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newPluginCmd())
	root.SetOut(out)
	root.SetErr(errb)
	root.SetArgs(argv)
	return root.Execute()
}

// pinPluginTime freezes the timeNow hook so JSON timestamps are stable.
func pinPluginTime(t *testing.T) {
	t.Helper()
	saved := timeNow
	timeNow = func() time.Time {
		return time.Date(2026, 9, 5, 3, 6, 53, 301000000, time.UTC)
	}
	t.Cleanup(func() { timeNow = saved })
}

// writeMarketplaceFixture builds the probe marketplace shape on disk: mp
// with plugins hello (full plugin.json) and world.  Returns the marketplace
// root dir.
func writeMarketplaceFixture(t *testing.T, base string) string {
	t.Helper()
	root := filepath.Join(base, "mp")
	for _, d := range []string{
		filepath.Join(root, ".claude-plugin"),
		filepath.Join(root, "plugins", "hello", ".claude-plugin"),
		filepath.Join(root, "plugins", "world", ".claude-plugin"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(root, ".claude-plugin", "marketplace.json"): `{
  "name": "mp",
  "owner": {"name": "probe"},
  "description": "Probe marketplace",
  "plugins": [
    {"name": "hello", "source": "./plugins/hello", "version": "1.0.0", "description": "Hello plugin"},
    {"name": "world", "source": "./plugins/world", "version": "2.0.0", "description": "World plugin"}
  ]
}`,
		filepath.Join(root, "plugins", "hello", ".claude-plugin", "plugin.json"): `{"name":"hello","version":"1.0.0","description":"Hello plugin","author":{"name":"probe"}}`,
		filepath.Join(root, "plugins", "world", ".claude-plugin", "plugin.json"): `{"name":"world","version":"2.0.0","description":"World plugin","author":{"name":"probe"}}`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// setupInstalledFixture wires the marketplace, adds it, and installs hello.
func setupInstalledFixture(t *testing.T) (*bytes.Buffer, *bytes.Buffer, *config.PluginStore, string) {
	t.Helper()
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
	out.Reset()
	errb.Reset()
	return out, errb, store, root
}

func TestPluginCmdParentSemantics(t *testing.T) {
	t.Run("bare-parent-help-to-stderr", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.Len() != 0 || errb.String() != pluginParentHelp {
			t.Fatalf("stdout = %q, stderr len = %d", out.String(), errb.Len())
		}
	})
	t.Run("parent-h-help-to-stdout", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "-h"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginParentHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("unknown-command-no-suggestion", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "error: unknown command 'bogus'\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
	})
	t.Run("unknown-command-with-suggestion", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "lst")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "error: unknown command 'lst'\n(Did you mean list?)\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
	})
	t.Run("unknown-option", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "-x")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "error: unknown option '-x'\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("double-dash-dispatches", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "--", "list"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "No plugins installed. Use `claude plugin install` to install a plugin.\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("double-dash-install-missing-arg", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "--", "install")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "error: missing required argument 'plugin'\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("double-dash-h-unknown-command", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "--", "-h")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "error: unknown command '-h'\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("plugins-alias", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugins", "list"); err != nil {
			t.Fatal(err)
		}
		if out.Len() == 0 {
			t.Fatal("alias produced no output")
		}
	})
}

func TestPluginCmdHelpTopics(t *testing.T) {
	t.Run("bare-help", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "help"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginParentHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("topic", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "help", "install"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginInstallHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("marketplace-topic", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "help", "marketplace"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginMarketplaceHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("bogus-topic-parent-help-to-stderr", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "help", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != pluginParentHelp || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("help-dash-h-is-a-topic", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "help", "-h")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != pluginParentHelp {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("help-help-is-unknown-topic", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "help", "help")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
	})
	t.Run("extra-topic-args-ignored", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "help", "list", "extra"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginListHelp {
			t.Fatalf("stdout = %q", out.String())
		}
	})
}

func TestPluginCmdSubcommandHelp(t *testing.T) {
	helpText := map[string][]string{
		"list":        {"plugin", "list", "-h"},
		"install":     {"plugin", "install", "-h"},
		"uninstall":   {"plugin", "uninstall", "-h"},
		"enable":      {"plugin", "enable", "-h"},
		"disable":     {"plugin", "disable", "-h"},
		"update":      {"plugin", "update", "-h"},
		"validate":    {"plugin", "validate", "-h"},
		"marketplace": {"plugin", "marketplace", "-h"},
		"mp-add":      {"plugin", "marketplace", "add", "-h"},
		"mp-list":     {"plugin", "marketplace", "list", "-h"},
		"mp-remove":   {"plugin", "marketplace", "remove", "-h"},
		"mp-update":   {"plugin", "marketplace", "update", "-h"},
	}
	wantText := map[string]string{
		"list":        pluginListHelp,
		"install":     pluginInstallHelp,
		"uninstall":   pluginUninstallHelp,
		"enable":      pluginEnableHelp,
		"disable":     pluginDisableHelp,
		"update":      pluginUpdateHelp,
		"validate":    pluginValidateHelp,
		"marketplace": pluginMarketplaceHelp,
		"mp-add":      pluginMPAddHelp,
		"mp-list":     pluginMPListHelp,
		"mp-remove":   pluginMPRemoveHelp,
		"mp-update":   pluginMPUpdateHelp,
	}
	for name, argv := range helpText {
		t.Run(name, func(t *testing.T) {
			out, errb, _ := swapPluginDeps(t)
			if err := runPlugin(t, out, errb, argv...); err != nil {
				t.Fatal(err)
			}
			if out.String() != wantText[name] || errb.Len() != 0 {
				t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
			}
		})
	}
}

func TestPluginCmdUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"install-noargs", []string{"plugin", "install"},
			"error: missing required argument 'plugin'\n"},
		{"uninstall-noargs", []string{"plugin", "uninstall"},
			"error: missing required argument 'plugin'\n"},
		{"enable-noargs", []string{"plugin", "enable"},
			"error: missing required argument 'plugin'\n"},
		{"update-noargs", []string{"plugin", "update"},
			"error: missing required argument 'plugin'\n"},
		{"validate-noargs", []string{"plugin", "validate"},
			"error: missing required argument 'path'\n"},
		{"mp-add-noargs", []string{"plugin", "marketplace", "add"},
			"error: missing required argument 'source'\n"},
		{"mp-remove-noargs", []string{"plugin", "marketplace", "remove"},
			"error: missing required argument 'name'\n"},
		{"mp-add-short-s-unknown", []string{"plugin", "marketplace", "add", "x", "-s", "local"},
			"error: unknown option '-s'\n"},
		{"install-unknown-option", []string{"plugin", "install", "x", "--nope"},
			"error: unknown option '--nope'\n(Did you mean --scope?)\n"},
		{"install-missing-scope-arg", []string{"plugin", "install", "x", "-s"},
			"error: option '-s, --scope <scope>' argument missing\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, _ := swapPluginDeps(t)
			err := runPlugin(t, out, errb, tc.argv...)
			if !errors.Is(err, ErrSilent) {
				t.Fatalf("err = %v (%T), want ErrSilent", err, err)
			}
			if out.Len() != 0 || errb.String() != tc.want {
				t.Fatalf("stdout = %q, stderr = %q, want stderr %q", out.String(), errb.String(), tc.want)
			}
		})
	}
}

func TestPluginMPCmdSemantics(t *testing.T) {
	t.Run("bare-group-help-to-stderr", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.Len() != 0 || errb.String() != pluginMarketplaceHelp {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("group-h-help-to-stdout", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "-h"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginMarketplaceHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("unknown-command-with-suggestion", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "lst")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "error: unknown command 'lst'\n(Did you mean list?)\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
	})
	t.Run("bare-help", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "help"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginMarketplaceHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("help-topic", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "help", "add"); err != nil {
			t.Fatal(err)
		}
		if out.String() != pluginMPAddHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("help-bogus-topic", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "help", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != pluginMarketplaceHelp || out.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("double-dash-dispatches", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		if err := runPlugin(t, out, errb, "plugin", "marketplace", "--", "list"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "No marketplaces configured\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("rm-alias", func(t *testing.T) {
		out, errb, _ := swapPluginDeps(t)
		err := runPlugin(t, out, errb, "plugin", "marketplace", "rm", "ghost")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "✘ Failed to remove marketplace: Marketplace 'ghost' not found\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
}

func TestPluginCmdTreeShape(t *testing.T) {
	cmd := newPluginCmd()
	if cmd.Name() != "plugin" {
		t.Fatalf("name = %q", cmd.Name())
	}
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
		if !c.DisableFlagParsing {
			t.Fatalf("subcommand %q must disable flag parsing", c.Name())
		}
	}
	for _, want := range []string{"disable", "enable", "help", "install", "list", "marketplace", "uninstall", "update", "validate"} {
		if !names[want] {
			t.Fatalf("missing subcommand %q (have %v)", want, names)
		}
	}
	mp := cmd.Commands()[5]
	for _, want := range []string{"add", "help", "list", "remove", "update"} {
		found := false
		for _, c := range mp.Commands() {
			if c.Name() == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("marketplace group missing %q", want)
		}
	}
}
