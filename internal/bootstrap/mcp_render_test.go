package bootstrap

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// Byte-exact render tests against oracle fixtures (claude v2.1.261), captured
// in /tmp/ccg/cmp/oracle.txt during B2 verification.

func TestMCPRedactHeader(t *testing.T) {
	redact := []string{"Authorization", "X-Api-Key", "x-apikey", "Api-Token", "X-Token", "Client-Secret", "Set-Cookie", "x-password"}
	for _, name := range redact {
		if !mcpRedactHeader(name) {
			t.Fatalf("%q must be redacted", name)
		}
	}
	clear := []string{"X-Custom", "Accept", "Content-Type"}
	for _, name := range clear {
		if mcpRedactHeader(name) {
			t.Fatalf("%q must not be redacted", name)
		}
	}
}

func TestMCPScopeWord(t *testing.T) {
	cases := map[string]string{
		config.MCPScopeLocal:   "local",
		config.MCPScopeUser:    "user",
		config.MCPScopeProject: "project",
		"":                     "local",
	}
	for scope, want := range cases {
		if got := mcpScopeWord(scope); got != want {
			t.Fatalf("mcpScopeWord(%q) = %q, want %q", scope, got, want)
		}
	}
}

func TestMCPRenderAdded(t *testing.T) {
	cases := []struct {
		name      string
		server    string
		transport string
		display   string
		args      []string
		scope     string
		want      string
	}{
		{"stdio-args", "alpha", "stdio", "echo", []string{"hello", "world"}, config.MCPScopeLocal,
			"Added stdio MCP server alpha with command: echo hello world to local config\n"},
		// The oracle joins args with an extra space when there are none:
		// "echo  to" (double space).
		{"stdio-noargs", "uu", "stdio", "echo", nil, config.MCPScopeUser,
			"Added stdio MCP server uu with command: echo  to user config\n"},
		{"http", "hh", "http", "http://hh.test/mcp", nil, config.MCPScopeLocal,
			"Added HTTP MCP server hh with URL: http://hh.test/mcp to local config\n"},
		{"sse", "ss", "sse", "http://ss.test/sse", nil, config.MCPScopeProject,
			"Added SSE MCP server ss with URL: http://ss.test/sse to project config\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			mcpRenderAdded(&b, tc.transport, tc.server, tc.display, tc.args, tc.scope)
			if b.String() != tc.want {
				t.Fatalf("got %q, want %q", b.String(), tc.want)
			}
		})
	}
}

func TestMCPRenderHeaders(t *testing.T) {
	var b bytes.Buffer
	mcpRenderHeaders(&b, []config.EnvPair{
		{Key: "Authorization", Value: "Bearer secret"},
		{Key: "X-Api-Key", Value: "k"},
		{Key: "X-Custom", Value: "val"},
	})
	want := "Headers: {\n" +
		"  \"Authorization\": \"[REDACTED]\",\n" +
		"  \"X-Api-Key\": \"[REDACTED]\",\n" +
		"  \"X-Custom\": \"val\"\n" +
		"}\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
	b.Reset()
	mcpRenderHeaders(&b, nil)
	if b.Len() != 0 {
		t.Fatalf("no headers must render nothing, got %q", b.String())
	}
}

func TestMCPRenderURLWarning(t *testing.T) {
	var b bytes.Buffer
	mcpRenderURLWarning(&b, "warn", "http://example.com/api")
	want := "\nWarning: The command \"http://example.com/api\" looks like a URL, " +
		"but is being interpreted as a stdio server as --transport was not specified.\n" +
		"If this is an HTTP server, use: claude mcp add --transport http warn http://example.com/api\n" +
		"If this is an SSE server, use: claude mcp add --transport sse warn http://example.com/api\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

func TestMCPListEndpoint(t *testing.T) {
	cases := []struct {
		name  string
		entry config.MCPServerEntry
		want  string
	}{
		{"stdio-args", config.MCPServerEntry{Raw: rawStdioEntry("echo", "hello", "world")}, "echo hello world"},
		{"stdio-noargs", config.MCPServerEntry{Raw: rawStdioEntry("echo")}, "echo "},
		{"http", config.MCPServerEntry{Raw: rawURLEntry("http", "http://h/m")}, "http://h/m (HTTP)"},
		{"sse", config.MCPServerEntry{Raw: rawURLEntry("sse", "http://s/sse")}, "http://s/sse (SSE)"},
		{"typeless", config.MCPServerEntry{Raw: func() *config.OrderedMap {
			e := config.NewOrderedMap()
			e.Set("command", "echo")
			return e
		}()}, "echo "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpListEndpoint(tc.entry); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// alphaEntry mirrors `mcp add alpha echo hello world` storage.
func alphaEntry() *config.OrderedMap {
	return rawStdioEntry("echo", "hello", "world")
}

func TestMCPRenderGetStdio(t *testing.T) {
	var b bytes.Buffer
	mcpRenderGet(&b, "alpha", config.MCPScopeLocal, config.MCPServerEntry{Name: "alpha", Raw: alphaEntry()},
		"✘ Failed to connect", "-32000: MCP error -32000: Connection closed")
	want := "alpha:\n" +
		"  Scope: Local config (private to you in this project)\n" +
		"  Status: ✘ Failed to connect\n" +
		"  Issue: -32000: MCP error -32000: Connection closed\n" +
		"  Type: stdio\n" +
		"  Command: echo\n" +
		"  Args: hello world\n" +
		"  Environment:\n" +
		"\nTo remove this server, run: claude mcp remove alpha -s local\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

func TestMCPRenderGetHTTPOAuth(t *testing.T) {
	e := rawURLEntry("http", "http://oa.test/mcp")
	oauth := config.NewOrderedMap()
	oauth.Set("clientId", "cid")
	oauth.Set("callbackPort", 8080)
	e.Set("oauth", oauth)
	hdrs := config.NewOrderedMap()
	hdrs.Set("Authorization", "Bearer x")
	e.Set("headers", hdrs)

	var b bytes.Buffer
	mcpRenderGet(&b, "oa", config.MCPScopeLocal, config.MCPServerEntry{Name: "oa", Raw: e},
		"✘ Failed to connect", "ENOTFOUND: getaddrinfo ENOTFOUND oa.test")
	want := "oa:\n" +
		"  Scope: Local config (private to you in this project)\n" +
		"  Status: ✘ Failed to connect\n" +
		"  Issue: ENOTFOUND: getaddrinfo ENOTFOUND oa.test\n" +
		"  Type: http\n" +
		"  URL: http://oa.test/mcp\n" +
		"  Headers:\n" +
		"    Authorization: Bearer x\n" +
		"  OAuth: client_id configured, callback_port 8080\n" +
		"\nTo remove this server, run: claude mcp remove oa -s local\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

func TestMCPRenderGetNoDetailBlock(t *testing.T) {
	// ws and typeless entries render Scope/Status only (captures get-ws and
	// get-typeless).
	for _, tc := range []struct {
		name  string
		raw   *config.OrderedMap
		issue string
	}{
		{"ws", rawURLEntry("ws", "ws://127.0.0.1:1"),
			"WebSocket connection to 'ws://127.0.0.1:1/' failed: Failed to connect"},
		{"typeless", func() *config.OrderedMap {
			e := config.NewOrderedMap()
			e.Set("command", "echo")
			return e
		}(), "-32000: MCP error -32000: Connection closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			mcpRenderGet(&b, "x", config.MCPScopeLocal, config.MCPServerEntry{Name: "x", Raw: tc.raw},
				"✘ Failed to connect", tc.issue)
			want := "x:\n" +
				"  Scope: Local config (private to you in this project)\n" +
				"  Status: ✘ Failed to connect\n" +
				"  Issue: " + tc.issue + "\n" +
				"\nTo remove this server, run: claude mcp remove x -s local\n"
			if b.String() != want {
				t.Fatalf("got %q, want %q", b.String(), want)
			}
		})
	}
}

func TestMCPRenderMiss(t *testing.T) {
	cases := []struct {
		name       string
		miss       string
		suggestion string
		all        []string
		forGet     bool
		hasPending bool
		want       string
	}{
		{"suggestion", "alpga", "alpha", nil, false, false,
			"No MCP server named \"alpga\". Did you mean \"alpha\"? Run `claude mcp list` to see all.\n"},
		{"empty", "ghost", "", nil, false, false,
			"No MCP server named \"ghost\". Run `claude mcp add` to add one.\n"},
		{"configured", "ghost", "", []string{"alpha", "beta"}, false, false,
			"No MCP server named \"ghost\". Configured servers: alpha, beta\n"},
		// qat: with nothing else to list, the pending hint replaces the whole
		// message.
		{"pending-only", "ghost", "", nil, true, true,
			"No MCP server named \"ghost\". .mcp.json servers are awaiting approval — run `claude` in this directory to review them.\n"},
		// …but a suggestion still wins the Ltn branch, with the pending note
		// appended (capture: get alpga with alpha local + pp pending).
		{"suggestion-plus-pending", "alpga", "alpha", []string{"alpha"}, true, true,
			"No MCP server named \"alpga\". Did you mean \"alpha\"? Run `claude mcp list` to see all." +
				" (.mcp.json servers are awaiting approval — run `claude` in this directory to review them.)\n"},
		// remove never carries pending hints, even with pending servers
		// present (capture: rm-sugg while a .mcp.json server was pending).
		{"remove-ignores-pending", "alpga", "alpha", nil, false, true,
			"No MCP server named \"alpga\". Did you mean \"alpha\"? Run `claude mcp list` to see all.\n"},
		{"remove-pending-only-store", "ghost", "", []string{"pp"}, false, true,
			"No MCP server named \"ghost\". Configured servers: pp\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			mcpRenderMiss(&b, tc.miss, tc.suggestion, tc.all, tc.forGet, tc.hasPending)
			if b.String() != tc.want {
				t.Fatalf("got %q, want %q", b.String(), tc.want)
			}
		})
	}
}

func TestMCPRenderMissListCap(t *testing.T) {
	all := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	var b bytes.Buffer
	mcpRenderMiss(&b, "x", "", all, false, false)
	want := "No MCP server named \"x\". Configured servers: a, b, c, d, e, f, g, h" +
		" (and 3 more — run `claude mcp list` to see all)\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}

	// The get-only pending parenthetical is appended after the capped list.
	b.Reset()
	mcpRenderMiss(&b, "x", "", all, true, true)
	want = "No MCP server named \"x\". Configured servers: a, b, c, d, e, f, g, h" +
		" (and 3 more — run `claude mcp list` to see all)" +
		" (.mcp.json servers are awaiting approval — run `claude` in this directory to review them.)\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

func TestMCPRenderRemoveMulti(t *testing.T) {
	s := config.NewMCPStore(t.TempDir(), t.TempDir())
	var b bytes.Buffer
	mcpRenderRemoveMulti(&b, "multi", []string{config.MCPScopeProject, config.MCPScopeUser}, s)
	want := "MCP server \"multi\" exists in multiple scopes:\n" +
		"  - Project config (shared via .mcp.json) (" + s.ProjectKey() + "/.mcp.json)\n" +
		"  - User config (available in all your projects) (" + config.ClaudeJsonPath(s.HomeDir()) + ")\n" +
		"\nTo remove from a specific scope, use:\n" +
		"  claude mcp remove multi -s project\n" +
		"  claude mcp remove multi -s user\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

func TestMCPRenderDiagnostics(t *testing.T) {
	t.Run("unknown-type-block", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeLocal, "bad", rawURLEntry("bogus", "x"))
		var b bytes.Buffer
		mcpRenderDiagnostics(&b, s)
		want := "\nMCP config diagnostics ⚠\n\n" +
			"For help configuring MCP servers, see: https://code.claude.com/docs/en/mcp\n\n" +
			"[Contains warnings] Local config (private to you in this project)\n" +
			"Location: " + config.ClaudeJsonPath(s.HomeDir()) + " [project: " + s.ProjectKey() + "]\n" +
			" └ [Warning] [bad] mcpServers.bad: Skipped — unknown MCP server type \"bogus\" for server \"bad\"\n\n"
		if b.String() != want {
			t.Fatalf("got %q, want %q", b.String(), want)
		}
	})
	t.Run("conflicts-block-single", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeUser, "multi", rawStdioEntry("echo3"))
		addServer(t, s, config.MCPScopeProject, "multi", rawStdioEntry("echo2"))
		var b bytes.Buffer
		mcpRenderDiagnostics(&b, s)
		want := "\nMCP config diagnostics ⚠\n\n" +
			"For help configuring MCP servers, see: https://code.claude.com/docs/en/mcp\n\n" +
			"[Conflicting scopes]\n" +
			"├ Server \"multi\" is defined in multiple scopes with different endpoints: " +
			"user (echo3), project (echo2). OAuth tokens are stored per endpoint, " +
			"so authenticating in one context will not carry over.\n" +
			"└ Keep the correct endpoint and remove the others: " +
			"`claude mcp remove multi -s user` or `claude mcp remove multi -s project`\n\n"
		if b.String() != want {
			t.Fatalf("got %q, want %q", b.String(), want)
		}
	})
	t.Run("conflicts-block-last-group-uses-corner-branch", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeUser, "m1", rawStdioEntry("a1"))
		addServer(t, s, config.MCPScopeLocal, "m1", rawStdioEntry("b1"))
		addServer(t, s, config.MCPScopeUser, "m2", rawStdioEntry("a2"))
		addServer(t, s, config.MCPScopeLocal, "m2", rawStdioEntry("b2"))
		var b bytes.Buffer
		mcpRenderDiagnostics(&b, s)
		got := b.String()
		if strings.Count(got, "├ Server") != 2 {
			t.Fatalf("expected two ├ Server lines: %q", got)
		}
		if strings.Count(got, "├ Keep") != 1 || strings.Count(got, "└ Keep") != 1 {
			t.Fatalf("last group's Keep must use └: %q", got)
		}
		if !strings.HasSuffix(got, "└ Keep the correct endpoint and remove the others: `claude mcp remove m2 -s user` or `claude mcp remove m2 -s local`\n\n") {
			t.Fatalf("tail = %q", got)
		}
	})
}
