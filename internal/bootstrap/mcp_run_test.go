package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// Shared fixtures for the mcp_*_test.go files: a canned health checker, a
// deps bundle over fresh temp dirs, and entry builders.  The bootstrap
// package runs tests sequentially (TestMain pins HOME), so no t.Parallel.

// fakeMCPHealth returns canned health results by server name (def when the
// name was not seeded) and records every check.
type fakeMCPHealth struct {
	byName map[string]mcpHealthResult
	def    mcpHealthResult
	calls  []string
}

func (f *fakeMCPHealth) Check(_ context.Context, name string, _ config.MCPServerEntry) mcpHealthResult {
	f.calls = append(f.calls, name)
	if r, ok := f.byName[name]; ok {
		return r
	}
	return f.def
}

// newTestMCPDeps builds deps over fresh temp home/project dirs with captured
// streams and an always-connected fake checker.
func newTestMCPDeps(t *testing.T) (*mcpDeps, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	d := &mcpDeps{
		Stdout: &out,
		Stderr: &errb,
		Store:  config.NewMCPStore(t.TempDir(), t.TempDir()),
		Health: &fakeMCPHealth{def: mcpHealthResult{Connected: true}},
	}
	return d, &out, &errb
}

// resetMCPStreams clears both captured streams — call after a seeding add so
// assertions see only the command under test.
func resetMCPStreams(d *mcpDeps) {
	d.Stdout.(*bytes.Buffer).Reset()
	d.Stderr.(*bytes.Buffer).Reset()
}

// errActionMsg returns the message of an *errMCPAction, failing the test for
// any other error type.
func errActionMsg(t *testing.T, err error) string {
	t.Helper()
	var e *errMCPAction
	if !errors.As(err, &e) {
		t.Fatalf("error %v (%T) is not *errMCPAction", err, err)
	}
	return e.msg
}

// readStoredEntry re-reads a stored entry through the store so assertions see
// what a later command would see (not the in-memory OrderedMap we passed in).
func readStoredEntry(t *testing.T, d *mcpDeps, scope, name string) config.MCPServerEntry {
	t.Helper()
	entries, err := d.Store.ScopeEntries(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("server %q not found in %s scope", name, scope)
	return config.MCPServerEntry{}
}

func TestRunMCPAddStdio(t *testing.T) {
	d, out, _ := newTestMCPDeps(t)
	if err := runMCPAdd(d, []string{"alpha", "echo", "hello", "world"}); err != nil {
		t.Fatal(err)
	}
	want := "Added stdio MCP server alpha with command: echo hello world to local config\n" +
		"File modified: " + config.ClaudeJsonPath(d.Store.HomeDir()) + " [project: " + d.Store.ProjectKey() + "]\n"
	if out.String() != want {
		t.Fatalf("stdout =\n%q\nwant\n%q", out.String(), want)
	}
	e := readStoredEntry(t, d, config.MCPScopeLocal, "alpha")
	if e.Type() != "stdio" || e.Command() != "echo" {
		t.Fatalf("stored entry = %+v", e.Raw)
	}
	if got := strings.Join(e.Args(), ","); got != "hello,world" {
		t.Fatalf("args = %q", got)
	}
	if _, has := e.Raw.Get("env"); !has {
		t.Fatal("stdio entry must always store env (even empty)")
	}
}

func TestRunMCPAddStdioEnvAndCommandless(t *testing.T) {
	d, out, _ := newTestMCPDeps(t)
	if err := runMCPAdd(d, []string{"beta", "-e", "K1=v1", "-e", "K2=v2", "--", "cat", "-A"}); err != nil {
		t.Fatal(err)
	}
	// Greedy -e stops at "--"; cat -A land in positionals (command + args).
	if got := out.String(); !strings.Contains(got, "Added stdio MCP server beta with command: cat -A to local config\n") {
		t.Fatalf("stdout = %q", got)
	}
	e := readStoredEntry(t, d, config.MCPScopeLocal, "beta")
	pairs := e.EnvPairs()
	if len(pairs) != 2 || pairs[0].Key != "K1" || pairs[0].Value != "v1" || pairs[1].Key != "K2" {
		t.Fatalf("env pairs = %+v", pairs)
	}
}

func TestRunMCPAddTransports(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"http", []string{"hh", "--transport", "http", "http://hh.test/mcp"},
			"Added HTTP MCP server hh with URL: http://hh.test/mcp to local config\n"},
		{"sse", []string{"ss", "-t", "sse", "http://ss.test/sse"},
			"Added SSE MCP server ss with URL: http://ss.test/sse to local config\n"},
		// streamable-http is an alias for http (display type "HTTP").
		{"streamable", []string{"st1", "http://st.test/", "-t", "streamable-http"},
			"Added HTTP MCP server st1 with URL: http://st.test to local config\n"},
		// The URL display trims exactly one trailing slash.
		{"http-slash", []string{"st2", "-t", "http", "http://st2.test/a/"},
			"Added HTTP MCP server st2 with URL: http://st2.test/a to local config\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, out, _ := newTestMCPDeps(t)
			if err := runMCPAdd(d, tc.args); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(out.String(), tc.want) {
				t.Fatalf("stdout = %q, want prefix %q", out.String(), tc.want)
			}
		})
	}
}

func TestRunMCPAddScopes(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"uu", "--scope", "user", "--", "echo"}); err != nil {
			t.Fatal(err)
		}
		want := "Added stdio MCP server uu with command: echo  to user config\n" +
			"File modified: " + config.ClaudeJsonPath(d.Store.HomeDir()) + "\n"
		if out.String() != want {
			t.Fatalf("stdout =\n%q\nwant\n%q", out.String(), want)
		}
	})
	t.Run("project", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"pp", "--scope", "project", "--", "echo"}); err != nil {
			t.Fatal(err)
		}
		want := "Added stdio MCP server pp with command: echo  to project config\n" +
			"File modified: " + d.Store.ProjectKey() + "/.mcp.json\n"
		if out.String() != want {
			t.Fatalf("stdout =\n%q\nwant\n%q", out.String(), want)
		}
	})
}

func TestRunMCPAddValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"bad-scope", []string{"x", "y", "-s", "bogus"},
			"Invalid scope: bogus. Must be one of: local, user, project, dynamic, enterprise, claudeai, managed, agent"},
		{"unwritable-scope", []string{"x", "y", "-s", "enterprise"},
			"Cannot add MCP server to scope: enterprise"},
		{"bad-transport", []string{"x", "y", "-t", "bogus"},
			"Invalid transport type: bogus. Must be one of: stdio, sse, http (or streamable-http)"},
		{"env-badfmt", []string{"eb", "-e", "BADFORMAT", "--", "echo"},
			"Invalid environment variable format: BADFORMAT, environment variables should be added as: -e KEY1=value1 -e KEY2=value2"},
		{"env-empty-key", []string{"eb", "-e", "=v", "--", "echo"},
			"Invalid environment variable format: =v, environment variables should be added as: -e KEY1=value1 -e KEY2=value2"},
		{"header-badfmt", []string{"o7", "http://z.test/", "-t", "http", "--callback-port", "abc", "-H", "nope"},
			"Invalid header format: \"nope\". Expected format: \"Header-Name: value\""},
		{"callback-port-bad", []string{"cb1", "--callback-port", "abc", "--client-id", "cid", "-t", "http", "http://cb.test/m"},
			"Error: --callback-port must be an integer in [1, 65535]"},
		{"callback-port-range", []string{"cb1", "--callback-port", "0", "--client-id", "cid", "-t", "http", "http://cb.test/m"},
			"Error: --callback-port must be an integer in [1, 65535]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := newTestMCPDeps(t)
			err := runMCPAdd(d, tc.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := errActionMsg(t, err); got != tc.want {
				t.Fatalf("error = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunMCPAddClientSecretGate(t *testing.T) {
	args := []string{"cs1", "--client-secret", "--client-id", "cid", "-t", "http", "http://cs1.test/m"}

	t.Run("no-env", func(t *testing.T) {
		t.Setenv("MCP_CLIENT_SECRET", "")
		d, _, _ := newTestMCPDeps(t)
		err := runMCPAdd(d, args)
		if err == nil {
			t.Fatal("expected error")
		}
		want := "No TTY available to prompt for client secret. Set MCP_CLIENT_SECRET env var instead."
		if got := errActionMsg(t, err); got != want {
			t.Fatalf("error = %q, want %q", got, want)
		}
	})
	t.Run("with-env", func(t *testing.T) {
		t.Setenv("MCP_CLIENT_SECRET", "x")
		d, out, errb := newTestMCPDeps(t)
		if err := runMCPAdd(d, args); err != nil {
			t.Fatal(err)
		}
		want := "Server added, but the client secret could not be stored. " +
			"Re-run with --client-secret once secure storage is available.\n"
		if errb.String() != want {
			t.Fatalf("stderr = %q, want %q", errb.String(), want)
		}
		if !strings.Contains(out.String(), "Added HTTP MCP server cs1") {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("flag-alone-ignored", func(t *testing.T) {
		// --client-secret without any OAuth config is silently ignored
		// (no prompt, no warning, stdio command).
		d, out, errb := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"x", "http://y.test/", "--client-secret"}); err != nil {
			t.Fatal(err)
		}
		if errb.String() == "" {
			t.Fatal("URL warning expected (stderr)")
		}
		if strings.Contains(errb.String(), "client secret") {
			t.Fatalf("unexpected client-secret warning: %q", errb.String())
		}
		if !strings.Contains(out.String(), "Added stdio MCP server x") {
			t.Fatalf("stdout = %q", out.String())
		}
	})
}

func TestRunMCPAddDup(t *testing.T) {
	d, _, _ := newTestMCPDeps(t)
	if err := runMCPAdd(d, []string{"alpha", "echo"}); err != nil {
		t.Fatal(err)
	}
	err := runMCPAdd(d, []string{"alpha", "echo", "dup"})
	if err == nil {
		t.Fatal("expected dup error")
	}
	if got := errActionMsg(t, err); got != "MCP server alpha already exists in local config" {
		t.Fatalf("error = %q", got)
	}
}

func TestRunMCPAddHeadersStoredAndRendered(t *testing.T) {
	d, out, _ := newTestMCPDeps(t)
	// URL before the -H flags: a bare token after a greedy -H would be
	// swallowed as another header value (see the trap test below).
	args := []string{"hh2", "-t", "http", "http://hh2.test/m", "-H", "Authorization: Bearer x", "-H", "X-Custom: val"}
	if err := runMCPAdd(d, args); err != nil {
		t.Fatal(err)
	}
	want := "Added HTTP MCP server hh2 with URL: http://hh2.test/m to local config\n" +
		"Headers: {\n" +
		"  \"Authorization\": \"[REDACTED]\",\n" +
		"  \"X-Custom\": \"val\"\n" +
		"}\n" +
		"File modified: " + config.ClaudeJsonPath(d.Store.HomeDir()) + " [project: " + d.Store.ProjectKey() + "]\n"
	if out.String() != want {
		t.Fatalf("stdout =\n%q\nwant\n%q", out.String(), want)
	}
	e := readStoredEntry(t, d, config.MCPScopeLocal, "hh2")
	if got := e.URL(); got != "http://hh2.test/m" {
		t.Fatalf("url = %q (raw, untrimmed)", got)
	}
}

func TestRunMCPAddGreedyHeaderSwallowsURL(t *testing.T) {
	// Oracle capture add-greedy-h: the second -H greedily consumes the URL,
	// so commandOrUrl is missing.
	d, _, _ := newTestMCPDeps(t)
	args := []string{"hh2", "-t", "http", "-H", "X-Api:abc", "-H", "X-Custom:val", "http://hh2.test/mcp"}
	err := runMCPAdd(d, args)
	var missing *mcpMissingArgError
	if !errors.As(err, &missing) || missing.Name != "commandOrUrl" {
		t.Fatalf("err = %v, want missing commandOrUrl", err)
	}
}

func TestRunMCPAddURLWarning(t *testing.T) {
	d, out, errb := newTestMCPDeps(t)
	if err := runMCPAdd(d, []string{"warn", "http://example.com/api"}); err != nil {
		t.Fatal(err)
	}
	// stdout keeps the raw command (no trim — not a display of a URL
	// transport) and the double-space empty-args join.
	if !strings.Contains(out.String(), "Added stdio MCP server warn with command: http://example.com/api  to local config\n") {
		t.Fatalf("stdout = %q", out.String())
	}
	want := "\nWarning: The command \"http://example.com/api\" looks like a URL, " +
		"but is being interpreted as a stdio server as --transport was not specified.\n" +
		"If this is an HTTP server, use: claude mcp add --transport http warn http://example.com/api\n" +
		"If this is an SSE server, use: claude mcp add --transport sse warn http://example.com/api\n"
	if errb.String() != want {
		t.Fatalf("stderr =\n%q\nwant\n%q", errb.String(), want)
	}
}

func TestRunMCPAddStdioOAuthFlagsWarning(t *testing.T) {
	// Oracle capture o4: the ignored-OAuth-flags warning precedes the
	// blank-line-led URL warning.
	d, _, errb := newTestMCPDeps(t)
	if err := runMCPAdd(d, []string{"o4", "http://x.test/", "--client-id", "cid"}); err != nil {
		t.Fatal(err)
	}
	want := "Warning: --client-id, --client-secret, --callback-port, and --xaa are only supported for HTTP/SSE transports and will be ignored for stdio.\n" +
		"\nWarning: The command \"http://x.test\" looks like a URL, " +
		"but is being interpreted as a stdio server as --transport was not specified.\n" +
		"If this is an HTTP server, use: claude mcp add --transport http o4 http://x.test\n" +
		"If this is an SSE server, use: claude mcp add --transport sse o4 http://x.test\n"
	if errb.String() != want {
		t.Fatalf("stderr =\n%q\nwant\n%q", errb.String(), want)
	}
}

func TestRunMCPAddOAuthStored(t *testing.T) {
	d, _, _ := newTestMCPDeps(t)
	args := []string{"oa", "--client-id", "cid", "--callback-port", "8080", "-t", "http", "http://oa.test/mcp"}
	if err := runMCPAdd(d, args); err != nil {
		t.Fatal(err)
	}
	e := readStoredEntry(t, d, config.MCPScopeLocal, "oa")
	oauth, ok := e.Raw.GetMap("oauth")
	if !ok {
		t.Fatal("oauth block not stored")
	}
	if got, _ := oauth.Get("clientId"); got != "cid" {
		t.Fatalf("clientId = %v", got)
	}
	if got, _ := oauth.Get("callbackPort"); got == nil {
		t.Fatal("callbackPort not stored")
	}
}

func TestRunMCPAddJSON(t *testing.T) {
	t.Run("typeless-with-bogus-field", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPAddJSON(d, []string{"j3", `{"command":"echo","bogusField":1}`}); err != nil {
			t.Fatal(err)
		}
		if out.String() != "Added stdio MCP server j3 to local config\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		e := readStoredEntry(t, d, config.MCPScopeLocal, "j3")
		if _, has := e.Raw.Get("type"); has {
			t.Fatal("typeless input must not store a type key")
		}
		if _, has := e.Raw.Get("env"); has {
			t.Fatal("env must not be stored when absent from input")
		}
		if got, _ := e.Raw.Get("args"); got == nil {
			t.Fatal("args must be stored (defaulted to [])")
		}
	})
	t.Run("stdio-typed", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		raw := `{"type":"stdio","command":"echo","args":["a"],"env":{"K":"v"}}`
		if err := runMCPAddJSON(d, []string{"s1", raw}); err != nil {
			t.Fatal(err)
		}
		e := readStoredEntry(t, d, config.MCPScopeLocal, "s1")
		if e.Type() != "stdio" || len(e.EnvPairs()) != 1 {
			t.Fatalf("entry = %+v", e.Raw)
		}
	})
	t.Run("http-headers-only", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		raw := `{"type":"http","url":"http://h1.test/m","headers":{"A":"b"},"env":{"X":"y"}}`
		if err := runMCPAddJSON(d, []string{"h1", raw}); err != nil {
			t.Fatal(err)
		}
		e := readStoredEntry(t, d, config.MCPScopeLocal, "h1")
		if e.Type() != "http" || e.URL() != "http://h1.test/m" {
			t.Fatalf("entry = %+v", e.Raw)
		}
		if len(e.Headers()) != 1 {
			t.Fatalf("headers = %+v", e.Headers())
		}
		if _, has := e.Raw.Get("env"); has {
			t.Fatal("http entries never store env")
		}
	})
	t.Run("ws-verbatim", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPAddJSON(d, []string{"w1", `{"type":"ws","url":"ws://127.0.0.1:1"}`}); err != nil {
			t.Fatal(err)
		}
		if out.String() != "Added ws MCP server w1 to local config\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
	t.Run("invalid-shapes", func(t *testing.T) {
		for name, raw := range map[string]string{
			"not-json":   "notjson",
			"empty":      "{}",
			"wss":        `{"type":"wss","url":"ws://127.0.0.1:1"}`,
			"websocket":  `{"type":"websocket","url":"ws://127.0.0.1:1"}`,
			"no-command": `{"type":"stdio"}`,
			"no-url":     `{"type":"http"}`,
		} {
			d, _, _ := newTestMCPDeps(t)
			err := runMCPAddJSON(d, []string{"x", raw})
			if got := errActionMsg(t, err); got != "Invalid configuration: : Invalid input" {
				t.Fatalf("%s: error = %q", name, got)
			}
		}
	})
	t.Run("missing-json", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		err := runMCPAddJSON(d, []string{"onlyname"})
		var missing *mcpMissingArgError
		if !errors.As(err, &missing) || missing.Name != "json" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRunMCPGet(t *testing.T) {
	seed := func(t *testing.T) *mcpDeps {
		d, _, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"beta", "-e", "K1=v1", "-e", "K2=v2", "--", "cat", "-A"}); err != nil {
			t.Fatal(err)
		}
		resetMCPStreams(d)
		return d
	}
	t.Run("stdio-full-detail", func(t *testing.T) {
		d := seed(t)
		d.Health = &fakeMCPHealth{def: mcpHealthResult{Issue: "-32000: MCP error -32000: Connection closed"}}
		if err := runMCPGet(d, []string{"beta"}); err != nil {
			t.Fatal(err)
		}
		out := d.Stdout.(*bytes.Buffer).String()
		want := "beta:\n" +
			"  Scope: Local config (private to you in this project)\n" +
			"  Status: ✘ Failed to connect\n" +
			"  Issue: -32000: MCP error -32000: Connection closed\n" +
			"  Type: stdio\n" +
			"  Command: cat\n" +
			"  Args: -A\n" +
			"  Environment:\n" +
			"    K1=v1\n" +
			"    K2=v2\n" +
			"\nTo remove this server, run: claude mcp remove beta -s local\n"
		if out != want {
			t.Fatalf("stdout =\n%q\nwant\n%q", out, want)
		}
	})
	t.Run("connected-no-issue-line", func(t *testing.T) {
		d := seed(t)
		if err := runMCPGet(d, []string{"beta"}); err != nil {
			t.Fatal(err)
		}
		out := d.Stdout.(*bytes.Buffer).String()
		if !strings.Contains(out, "  Status: ✔ Connected\n") || strings.Contains(out, "Issue:") {
			t.Fatalf("stdout = %q", out)
		}
	})
	t.Run("extra-positionals-ignored", func(t *testing.T) {
		d := seed(t)
		if err := runMCPGet(d, []string{"beta", "extra1", "extra2"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(d.Stdout.(*bytes.Buffer).String(), "beta:\n") {
			t.Fatal("expected beta report")
		}
	})
	t.Run("pending-project-no-health", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"pp", "--scope", "project", "--", "echo"}); err != nil {
			t.Fatal(err)
		}
		resetMCPStreams(d)
		h := &fakeMCPHealth{}
		d.Health = h
		if err := runMCPGet(d, []string{"pp"}); err != nil {
			t.Fatal(err)
		}
		out := d.Stdout.(*bytes.Buffer).String()
		want := "pp:\n" +
			"  Scope: Project config (shared via .mcp.json)\n" +
			"  Status: ⏸ Pending approval (run `claude` to approve)\n" +
			"  Type: stdio\n" +
			"  Command: echo\n" +
			"  Args:\n" +
			"  Environment:\n" +
			"\nTo remove this server, run: claude mcp remove pp -s project\n"
		if out != want {
			t.Fatalf("stdout =\n%q\nwant\n%q", out, want)
		}
		if len(h.calls) != 0 {
			t.Fatalf("health checked anyway: %v", h.calls)
		}
	})
	t.Run("noargs", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		err := runMCPGet(d, nil)
		var missing *mcpMissingArgError
		if !errors.As(err, &missing) || missing.Name != "name" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRunMCPGetMiss(t *testing.T) {
	t.Run("suggest-and-pending-parenthetical", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"alpha", "echo", "hello", "world"}); err != nil {
			t.Fatal(err)
		}
		if err := runMCPAdd(d, []string{"pp", "--scope", "project", "--", "echo"}); err != nil {
			t.Fatal(err)
		}
		err := runMCPGet(d, []string{"alpga"})
		var miser *mcpMiser
		if !errors.As(err, &miser) {
			t.Fatalf("err = %v (%T)", err, err)
		}
		want := "No MCP server named \"alpga\". Did you mean \"alpha\"? Run `claude mcp list` to see all." +
			" (.mcp.json servers are awaiting approval — run `claude` in this directory to review them.)\n"
		if miser.msg != want {
			t.Fatalf("msg =\n%q\nwant\n%q", miser.msg, want)
		}
	})
	t.Run("configured-list-and-more", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		for i := 1; i <= 10; i++ {
			name := fmt.Sprintf("a%d", i)
			if err := runMCPAdd(d, []string{name, "true"}); err != nil {
				t.Fatal(err)
			}
		}
		err := runMCPGet(d, []string{"ghost"})
		var miser *mcpMiser
		if !errors.As(err, &miser) {
			t.Fatalf("err = %v", err)
		}
		want := "No MCP server named \"ghost\". Configured servers: a1, a10, a2, a3, a4, a5, a6, a7" +
			" (and 2 more — run `claude mcp list` to see all)\n"
		if miser.msg != want {
			t.Fatalf("msg =\n%q\nwant\n%q", miser.msg, want)
		}
	})
	t.Run("empty-store", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		err := runMCPGet(d, []string{"ghost"})
		var miser *mcpMiser
		if !errors.As(err, &miser) {
			t.Fatalf("err = %v", err)
		}
		if miser.msg != "No MCP server named \"ghost\". Run `claude mcp add` to add one.\n" {
			t.Fatalf("msg = %q", miser.msg)
		}
	})
	t.Run("only-pending-project", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"pp", "--scope", "project", "--", "echo"}); err != nil {
			t.Fatal(err)
		}
		err := runMCPGet(d, []string{"ghost"})
		var miser *mcpMiser
		if !errors.As(err, &miser) {
			t.Fatalf("err = %v", err)
		}
		want := "No MCP server named \"ghost\". .mcp.json servers are awaiting approval — " +
			"run `claude` in this directory to review them.\n"
		if miser.msg != want {
			t.Fatalf("msg = %q, want %q", miser.msg, want)
		}
	})
}

func TestRunMCPRemove(t *testing.T) {
	seed := func(t *testing.T) *mcpDeps {
		d, _, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"alpha", "echo"}); err != nil {
			t.Fatal(err)
		}
		resetMCPStreams(d)
		return d
	}
	t.Run("bad-scope-word", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		err := runMCPRemove(d, []string{"ghost", "-s", "bogus"})
		if got := errActionMsg(t, err); !strings.HasPrefix(got, "Invalid scope: bogus.") {
			t.Fatalf("error = %q", got)
		}
	})
	t.Run("unwritable-scope", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		err := runMCPRemove(d, []string{"ghost", "-s", "enterprise"})
		if got := errActionMsg(t, err); got != "Cannot remove MCP server from scope: enterprise" {
			t.Fatalf("error = %q", got)
		}
	})
	t.Run("scoped-miss", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		err := runMCPRemove(d, []string{"ghost", "-s", "local"})
		if got := errActionMsg(t, err); got != "No MCP server named \"ghost\" in local scope" {
			t.Fatalf("error = %q", got)
		}
	})
	t.Run("scoped-success-unquoted", func(t *testing.T) {
		d := seed(t)
		if err := runMCPRemove(d, []string{"alpha", "-s", "local"}); err != nil {
			t.Fatal(err)
		}
		want := "Removed MCP server alpha from local config\n" +
			"File modified: " + config.ClaudeJsonPath(d.Store.HomeDir()) + " [project: " + d.Store.ProjectKey() + "]\n"
		if got := d.Stdout.(*bytes.Buffer).String(); got != want {
			t.Fatalf("stdout =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("bare-single-quoted", func(t *testing.T) {
		d := seed(t)
		if err := runMCPRemove(d, []string{"alpha"}); err != nil {
			t.Fatal(err)
		}
		if got := d.Stdout.(*bytes.Buffer).String(); !strings.HasPrefix(got, "Removed MCP server \"alpha\" from local config\n") {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("bare-multi", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"multi", "-s", "project", "--", "echo2"}); err != nil {
			t.Fatal(err)
		}
		if err := runMCPAdd(d, []string{"multi", "-s", "user", "--", "echo3"}); err != nil {
			t.Fatal(err)
		}
		err := runMCPRemove(d, []string{"multi"})
		var exit *errMCPExit
		if !errors.As(err, &exit) || exit.code != 1 {
			t.Fatalf("err = %v, want errMCPExit(1)", err)
		}
		errb := d.Stderr.(*bytes.Buffer).String()
		want := "MCP server \"multi\" exists in multiple scopes:\n" +
			"  - Project config (shared via .mcp.json) (" + d.Store.ProjectKey() + "/.mcp.json)\n" +
			"  - User config (available in all your projects) (" + config.ClaudeJsonPath(d.Store.HomeDir()) + ")\n" +
			"\nTo remove from a specific scope, use:\n" +
			"  claude mcp remove multi -s project\n" +
			"  claude mcp remove multi -s user\n"
		if errb != want {
			t.Fatalf("stderr =\n%q\nwant\n%q", errb, want)
		}
	})
	t.Run("bare-miss-no-pending-parenthetical", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"alpha", "echo", "hello", "world"}); err != nil {
			t.Fatal(err)
		}
		if err := runMCPAdd(d, []string{"pp", "--scope", "project", "--", "echo"}); err != nil {
			t.Fatal(err)
		}
		err := runMCPRemove(d, []string{"alpga"})
		var miser *mcpMiser
		if !errors.As(err, &miser) {
			t.Fatalf("err = %v", err)
		}
		want := "No MCP server named \"alpga\". Did you mean \"alpha\"? Run `claude mcp list` to see all.\n"
		if miser.msg != want {
			t.Fatalf("msg = %q, want %q", miser.msg, want)
		}
	})
	t.Run("noargs", func(t *testing.T) {
		d, _, _ := newTestMCPDeps(t)
		err := runMCPRemove(d, nil)
		var missing *mcpMissingArgError
		if !errors.As(err, &missing) || missing.Name != "name" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRunMCPResetProjectChoices(t *testing.T) {
	t.Run("with-mcpjson-servers", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPAdd(d, []string{"pp", "--scope", "project", "--", "echo"}); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		if err := runMCPResetProjectChoices(d, nil); err != nil {
			t.Fatal(err)
		}
		want := "Project-scoped (.mcp.json) server approvals and rejections stored for this project have been reset.\n" +
			"You will be prompted for approval next time you start Claude Code.\n"
		if out.String() != want {
			t.Fatalf("stdout =\n%q\nwant\n%q", out.String(), want)
		}
	})
	t.Run("without-mcpjson-servers", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPResetProjectChoices(d, nil); err != nil {
			t.Fatal(err)
		}
		want := "Project-scoped (.mcp.json) server approvals and rejections stored for this project have been reset.\n"
		if out.String() != want {
			t.Fatalf("stdout = %q, want %q", out.String(), want)
		}
	})
}

func TestRunMCPAddFromClaudeDesktop(t *testing.T) {
	t.Run("no-desktop-config", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPAddFromClaudeDesktop(d, nil); err != nil {
			t.Fatal(err)
		}
		want := "No MCP servers found in Claude Desktop configuration or configuration file does not exist.\n"
		if out.String() != want {
			t.Fatalf("stdout = %q, want %q", out.String(), want)
		}
	})
	t.Run("with-desktop-config", func(t *testing.T) {
		// Documented divergence: the oracle opens an interactive import
		// prompt here; this CLI prints "No servers were imported."
		d, out, _ := newTestMCPDeps(t)
		cfgDir := filepath.Join(d.Store.HomeDir(), "Library", "Application Support", "Claude")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		raw := `{"mcpServers":{"desk":{"command":"echo"}}}`
		if err := os.WriteFile(filepath.Join(cfgDir, "claude_desktop_config.json"), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := runMCPAddFromClaudeDesktop(d, nil); err != nil {
			t.Fatal(err)
		}
		if out.String() != "No servers were imported.\n" {
			t.Fatalf("stdout = %q", out.String())
		}
	})
}

func TestMCPParseEnvAndHeaderLists(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		pairs, err := mcpParseEnvList([]string{"A=1", "B="})
		if err != nil || len(pairs) != 2 || pairs[1].Value != "" {
			t.Fatalf("pairs = %+v, err = %v", pairs, err)
		}
		if _, err := mcpParseEnvList([]string{"noequals"}); err == nil {
			t.Fatal("expected error for value without =")
		}
	})
	t.Run("header", func(t *testing.T) {
		pairs, err := mcpParseHeaderList([]string{"X-A: 1 ", "X-B:2"})
		if err != nil || len(pairs) != 2 || pairs[0].Key != "X-A" || pairs[0].Value != "1" || pairs[1].Value != "2" {
			t.Fatalf("pairs = %+v, err = %v", pairs, err)
		}
		if _, err := mcpParseHeaderList([]string{"nocolon"}); err == nil {
			t.Fatal("expected error for header without colon")
		}
		if pairs, err := mcpParseHeaderList(nil); err != nil || pairs != nil {
			t.Fatalf("empty list must succeed: %v %v", pairs, err)
		}
	})
}
