package bootstrap

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// End-to-end cobra execution tests for the `claude mcp` tree.  The global
// mcpDepsBuilder is swapped to pin home/project dirs; the bootstrap package
// runs tests sequentially (no t.Parallel anywhere), so the swap is safe with
// a t.Cleanup restore.

// swapMCPDeps replaces mcpDepsBuilder for the test and returns the captured
// streams plus the shared store.  The store is created once so a sequence of
// runMCP calls (add → get → remove) all hit the same config.
func swapMCPDeps(t *testing.T) (*bytes.Buffer, *bytes.Buffer, *config.MCPStore) {
	t.Helper()
	var out, errb bytes.Buffer
	store := config.NewMCPStore(t.TempDir(), t.TempDir())
	saved := mcpDepsBuilder
	mcpDepsBuilder = func(stdout, stderr io.Writer) (*mcpDeps, error) {
		return &mcpDeps{
			Stdout: stdout,
			Stderr: stderr,
			Store:  store,
			Health: &fakeMCPHealth{def: mcpHealthResult{Connected: true}},
		}, nil
	}
	t.Cleanup(func() { mcpDepsBuilder = saved })
	return &out, &errb, store
}

// runMCP executes argv against the mcp tree and returns the resulting error.
// The mcp command must run as a child of a root: cobra's legacyArgs rejects
// positional args on a root command that has subcommands, while the real tree
// (bootstrap.buildRootCmd) always mounts mcp under the claude root.  The
// wrapper mirrors production's SilenceErrors/SilenceUsage so ErrSilent and
// render-then-exit paths print nothing extra.
func runMCP(t *testing.T, out, errb *bytes.Buffer, argv ...string) error {
	t.Helper()
	root := &cobra.Command{Use: "claude", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newMCPCmd())
	root.SetOut(out)
	root.SetErr(errb)
	root.SetArgs(argv)
	return root.Execute()
}

func TestMCPCmdAddGetRemove(t *testing.T) {
	out, errb, _ := swapMCPDeps(t)

	if err := runMCP(t, out, errb, "mcp", "add", "alpha", "echo", "hello", "world"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "Added stdio MCP server alpha with command: echo hello world to local config\n") {
		t.Fatalf("stdout = %q", out.String())
	}

	out.Reset()
	errb.Reset()
	if err := runMCP(t, out, errb, "mcp", "get", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "  Status: ✔ Connected\n") {
		t.Fatalf("get stdout = %q", out.String())
	}

	out.Reset()
	errb.Reset()
	if err := runMCP(t, out, errb, "mcp", "remove", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "Removed MCP server \"alpha\" from local config\n") {
		t.Fatalf("remove stdout = %q", out.String())
	}
}

func TestMCPCmdList(t *testing.T) {
	out, errb, _ := swapMCPDeps(t)
	if err := runMCP(t, out, errb, "mcp", "add", "alpha", "echo"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runMCP(t, out, errb, "mcp", "list"); err != nil {
		t.Fatal(err)
	}
	want := "Checking MCP server health…\n\nalpha: echo  - ✔ Connected\n"
	if out.String() != want {
		t.Fatalf("stdout = %q, want %q", out.String(), want)
	}
}

func TestMCPCmdParentSemantics(t *testing.T) {
	t.Run("bare-parent-help-to-stderr", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if out.Len() != 0 || errb.String() != mcpParentHelp {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("parent-h-help-to-stdout", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		if err := runMCP(t, out, errb, "mcp", "-h"); err != nil {
			t.Fatal(err)
		}
		if out.String() != mcpParentHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("unknown-command", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp", "lst")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		want := "error: unknown command 'lst'\n(Did you mean list?)\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
	})
	t.Run("unknown-option", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp", "-x")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "error: unknown option '-x'\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("double-dash-dispatches", func(t *testing.T) {
		// `mcp -- list` still runs list (cobra's matcher stops at --).
		out, errb, _ := swapMCPDeps(t)
		if err := runMCP(t, out, errb, "mcp", "--", "list"); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(out.String(), "No MCP servers configured.") {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("double-dash-add-missing-name", func(t *testing.T) {
		// `mcp -- add` errors with add's missing-name message.
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp", "--", "add")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "error: missing required argument 'name'\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("double-dash-h-unknown-command", func(t *testing.T) {
		// `mcp -- -h`: -h is a positional after --, i.e. an unknown command.
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp", "--", "-h")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != "error: unknown command '-h'\n" {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
}

func TestMCPCmdHelpTopics(t *testing.T) {
	t.Run("bare-help", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		if err := runMCP(t, out, errb, "mcp", "help"); err != nil {
			t.Fatal(err)
		}
		if out.String() != mcpParentHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("topic", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		if err := runMCP(t, out, errb, "mcp", "help", "add"); err != nil {
			t.Fatal(err)
		}
		if out.String() != mcpAddHelp || errb.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
		}
	})
	t.Run("bogus-topic-parent-help-to-stderr", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp", "help", "bogus")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != mcpParentHelp {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("help-dash-h-is-a-topic", func(t *testing.T) {
		// "-h" is just another (unknown) topic here: parent help, stderr.
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp", "help", "-h")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
		if errb.String() != mcpParentHelp {
			t.Fatalf("stderr = %q", errb.String())
		}
	})
	t.Run("help-help-is-unknown-topic", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		err := runMCP(t, out, errb, "mcp", "help", "help")
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
	})
	t.Run("extra-topic-args-ignored", func(t *testing.T) {
		out, errb, _ := swapMCPDeps(t)
		if err := runMCP(t, out, errb, "mcp", "help", "add", "extra"); err != nil {
			t.Fatal(err)
		}
		if out.String() != mcpAddHelp {
			t.Fatalf("stdout = %q", out.String())
		}
	})
}

func TestMCPCmdSubcommandHelp(t *testing.T) {
	// -h on each subcommand shows its own help (stdout, exit 0).
	helpText := map[string]string{
		"add":                     mcpAddHelp,
		"remove":                  mcpRemoveHelp,
		"list":                    mcpListHelp,
		"get":                     mcpGetHelp,
		"add-json":                mcpAddJSONHelp,
		"add-from-claude-desktop": mcpAfcdHelp,
		"reset-project-choices":   mcpResetHelp,
	}
	for name, want := range helpText {
		t.Run(name, func(t *testing.T) {
			out, errb, _ := swapMCPDeps(t)
			if err := runMCP(t, out, errb, "mcp", name, "-h"); err != nil {
				t.Fatal(err)
			}
			if out.String() != want || errb.Len() != 0 {
				t.Fatalf("stdout = %q, stderr = %q", out.String(), errb.String())
			}
		})
	}
}

func TestMCPCmdUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"add-unknown-option", []string{"mcp", "add", "--nope", "x", "y"},
			"error: unknown option '--nope'\n(Did you mean --scope?)\n"},
		{"add-missing-scope-arg", []string{"mcp", "add", "x", "-s"},
			"error: option '-s, --scope <scope>' argument missing\n"},
		{"add-missing-name", []string{"mcp", "add"},
			"error: missing required argument 'name'\n"},
		{"add-one-arg", []string{"mcp", "add", "onlyname"},
			"error: missing required argument 'commandOrUrl'\n"},
		{"addjson-one-arg", []string{"mcp", "add-json", "onlyname"},
			"error: missing required argument 'json'\n"},
		{"remove-noargs", []string{"mcp", "remove"},
			"error: missing required argument 'name'\n"},
		{"get-noargs", []string{"mcp", "get"},
			"error: missing required argument 'name'\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, _ := swapMCPDeps(t)
			err := runMCP(t, out, errb, tc.argv...)
			if !errors.Is(err, ErrSilent) {
				t.Fatalf("err = %v (%T), want ErrSilent", err, err)
			}
			if out.Len() != 0 || errb.String() != tc.want {
				t.Fatalf("stdout = %q, stderr = %q, want stderr %q", out.String(), errb.String(), tc.want)
			}
		})
	}
}

func TestMCPCmdActionErrors(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"bad-scope", []string{"mcp", "add", "x", "y", "-s", "bogus"},
			"Invalid scope: bogus. Must be one of: local, user, project, dynamic, enterprise, claudeai, managed, agent\n"},
		{"dup", []string{"mcp", "add", "x", "echo"},
			"MCP server x already exists in local config\n"},
		{"scoped-miss", []string{"mcp", "remove", "ghost", "-s", "local"},
			"No MCP server named \"ghost\" in local scope\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, _ := swapMCPDeps(t)
			if tc.name == "dup" {
				if err := runMCP(t, out, errb, "mcp", "add", "x", "echo"); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				errb.Reset()
			}
			err := runMCP(t, out, errb, tc.argv...)
			if !errors.Is(err, ErrSilent) {
				t.Fatalf("err = %v (%T), want ErrSilent", err, err)
			}
			if errb.String() != tc.want {
				t.Fatalf("stderr = %q, want %q", errb.String(), tc.want)
			}
		})
	}
}

func TestMCPCmdMissSilent(t *testing.T) {
	out, errb, _ := swapMCPDeps(t)
	if err := runMCP(t, out, errb, "mcp", "add", "alpha", "echo"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	err := runMCP(t, out, errb, "mcp", "get", "alpga")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("err = %v, want ErrSilent", err)
	}
	want := "No MCP server named \"alpga\". Did you mean \"alpha\"? Run `claude mcp list` to see all.\n"
	if errb.String() != want {
		t.Fatalf("stderr = %q, want %q", errb.String(), want)
	}
}

// The tree is wired with DisableFlagParsing everywhere; the root command's
// subcommand matcher must still route raw names.
func TestMCPCmdTreeShape(t *testing.T) {
	cmd := newMCPCmd()
	if cmd.Name() != "mcp" {
		t.Fatalf("name = %q", cmd.Name())
	}
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
		if !c.DisableFlagParsing {
			t.Fatalf("subcommand %q must disable flag parsing", c.Name())
		}
	}
	for _, want := range []string{"serve", "add", "add-from-claude-desktop", "add-json", "get", "list", "remove", "reset-project-choices", "help"} {
		if !names[want] {
			t.Fatalf("missing subcommand %q (have %v)", want, names)
		}
	}
}
