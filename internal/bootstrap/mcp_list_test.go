package bootstrap

import (
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// Entry builders over raw ordered maps, mirroring what the storage layer
// round-trips (typeless entries have no "type" key at all).

// rawStdioEntry builds a stdio entry (type always stored by `add`).
func rawStdioEntry(command string, args ...string) *config.OrderedMap {
	e := config.NewOrderedMap()
	e.Set("type", "stdio")
	e.Set("command", command)
	arr := make([]any, len(args))
	for i, a := range args {
		arr[i] = a
	}
	e.Set("args", arr)
	e.Set("env", config.NewOrderedMap())
	return e
}

// rawURLEntry builds an sse/http/ws entry.
func rawURLEntry(typ, url string) *config.OrderedMap {
	e := config.NewOrderedMap()
	e.Set("type", typ)
	e.Set("url", url)
	return e
}

// addServer stores entry under name/scope, failing the test on error.
func addServer(t *testing.T, s *config.MCPStore, scope, name string, entry *config.OrderedMap) {
	t.Helper()
	if err := s.AddMCPServer(scope, name, entry); err != nil {
		t.Fatalf("add %s/%s: %v", scope, name, err)
	}
}

func TestMCPHasVisibleServers(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*config.MCPStore)
		want  bool
	}{
		{"empty", func(*config.MCPStore) {}, false},
		{"typeless", func(s *config.MCPStore) {
			e := config.NewOrderedMap()
			e.Set("command", "echo")
			addServer(t, s, config.MCPScopeLocal, "j1", e)
		}, true},
		{"ws", func(s *config.MCPStore) {
			addServer(t, s, config.MCPScopeLocal, "w1", rawURLEntry("ws", "ws://127.0.0.1:1"))
		}, true},
		{"unknown-type-only", func(s *config.MCPStore) {
			addServer(t, s, config.MCPScopeLocal, "bad", rawURLEntry("bogus", "x"))
		}, false},
		{"pending-project", func(s *config.MCPStore) {
			addServer(t, s, config.MCPScopeProject, "pp", rawStdioEntry("echo"))
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := config.NewMCPStore(t.TempDir(), t.TempDir())
			tc.setup(s)
			if got := mcpHasVisibleServers(s); got != tc.want {
				t.Fatalf("mcpHasVisibleServers = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMCPConflicts(t *testing.T) {
	t.Run("user-vs-project", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeProject, "multi", rawStdioEntry("echo2"))
		addServer(t, s, config.MCPScopeUser, "multi", rawStdioEntry("echo3"))
		cs := mcpConflicts(s)
		if len(cs) != 1 {
			t.Fatalf("conflicts = %+v", cs)
		}
		c := cs[0]
		if c.Name != "multi" ||
			strings.Join(c.Scopes, ",") != "user,project" ||
			strings.Join(c.Endpoints, ",") != "echo3,echo2" {
			t.Fatalf("conflict = %+v", c)
		}
	})
	t.Run("equal-endpoints-suppressed", func(t *testing.T) {
		// Same URL across http and sse is NOT a conflict (probe s9); stdio
		// command+args joins are compared the same way.
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeUser, "same", rawURLEntry("http", "http://x.test/m"))
		addServer(t, s, config.MCPScopeLocal, "same", rawURLEntry("sse", "http://x.test/m"))
		if cs := mcpConflicts(s); len(cs) != 0 {
			t.Fatalf("conflicts = %+v", cs)
		}
	})
	t.Run("trailing-slash-not-normalized", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeUser, "u", rawURLEntry("http", "http://x.test/m"))
		addServer(t, s, config.MCPScopeLocal, "u", rawURLEntry("http", "http://x.test/m/"))
		cs := mcpConflicts(s)
		if len(cs) != 1 || cs[0].Endpoints[0] != "http://x.test/m" || cs[0].Endpoints[1] != "http://x.test/m/" {
			t.Fatalf("conflicts = %+v", cs)
		}
	})
	t.Run("three-way-lists-every-scope", func(t *testing.T) {
		// Even when two of the three endpoints are equal, all participating
		// scopes are listed (probe e3).
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeUser, "e", rawStdioEntry("cmd"))
		addServer(t, s, config.MCPScopeProject, "e", rawStdioEntry("cmd"))
		addServer(t, s, config.MCPScopeLocal, "e", rawStdioEntry("other"))
		cs := mcpConflicts(s)
		if len(cs) != 1 {
			t.Fatalf("conflicts = %+v", cs)
		}
		if strings.Join(cs[0].Scopes, ",") != "user,project,local" {
			t.Fatalf("scopes = %v", cs[0].Scopes)
		}
	})
	t.Run("non-participants", func(t *testing.T) {
		// typeless, ws and unknown-type entries never conflict; a rejected
		// project entry does (probe rej1).
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		typeless := config.NewOrderedMap()
		typeless.Set("command", "echo")
		addServer(t, s, config.MCPScopeUser, "t10", typeless)
		addServer(t, s, config.MCPScopeLocal, "t10", rawStdioEntry("other"))
		addServer(t, s, config.MCPScopeUser, "w9", rawURLEntry("ws", "ws://a"))
		addServer(t, s, config.MCPScopeLocal, "w9", rawURLEntry("ws", "ws://b"))
		addServer(t, s, config.MCPScopeUser, "unk1", rawURLEntry("bogus", "x"))
		addServer(t, s, config.MCPScopeLocal, "unk1", rawURLEntry("bogus", "y"))
		if cs := mcpConflicts(s); len(cs) != 0 {
			t.Fatalf("conflicts = %+v", cs)
		}
	})
	t.Run("report-order-scope-major-then-insertion", func(t *testing.T) {
		// Names are claimed in ListScopeOrder (user, project, local); within a
		// scope in stored (insertion) order, not alphabetical.
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeProject, "aa", rawStdioEntry("c1"))
		addServer(t, s, config.MCPScopeLocal, "aa", rawStdioEntry("c2"))
		addServer(t, s, config.MCPScopeUser, "zz", rawStdioEntry("c3"))
		addServer(t, s, config.MCPScopeLocal, "zz", rawStdioEntry("c4"))
		cs := mcpConflicts(s)
		if len(cs) != 2 || cs[0].Name != "zz" || cs[1].Name != "aa" {
			t.Fatalf("conflict order = %+v", cs)
		}
	})
}

func TestMCPHasDiagnostics(t *testing.T) {
	t.Run("unknown-type-only", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeLocal, "bad", rawURLEntry("bogus", "x"))
		if !mcpHasDiagnostics(s) {
			t.Fatal("unknown-type entry must trigger diagnostics")
		}
	})
	t.Run("conflict-only", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeUser, "u", rawStdioEntry("a"))
		addServer(t, s, config.MCPScopeLocal, "u", rawStdioEntry("b"))
		if !mcpHasDiagnostics(s) {
			t.Fatal("conflict must trigger diagnostics")
		}
	})
	t.Run("clean-store", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeUser, "u", rawStdioEntry("a"))
		addServer(t, s, config.MCPScopeLocal, "u", rawStdioEntry("a"))
		if mcpHasDiagnostics(s) {
			t.Fatal("no diagnostics expected")
		}
	})
}

func TestMCPListStatus(t *testing.T) {
	t.Run("pending-project-winner", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeProject, "pp", rawStdioEntry("echo"))
		d := &mcpDeps{Store: s, Health: &fakeMCPHealth{}}
		status, issue := mcpListStatus(d, config.MCPServerEntry{Name: "pp"})
		if status != mcpStatusPending || issue != "" {
			t.Fatalf("status = %q, issue = %q", status, issue)
		}
		if len(d.Health.(*fakeMCPHealth).calls) != 0 {
			t.Fatal("pending project entries are never health-checked")
		}
	})
	t.Run("connected-and-failed", func(t *testing.T) {
		s := config.NewMCPStore(t.TempDir(), t.TempDir())
		addServer(t, s, config.MCPScopeLocal, "ok", rawStdioEntry("echo"))
		addServer(t, s, config.MCPScopeLocal, "bad", rawStdioEntry("nope"))
		h := &fakeMCPHealth{byName: map[string]mcpHealthResult{
			"ok":  {Connected: true},
			"bad": {Issue: "ENOENT: Executable not found in $PATH: \"nope\""},
		}}
		d := &mcpDeps{Store: s, Health: h}
		if st, iss := mcpListStatus(d, config.MCPServerEntry{Name: "ok"}); st != mcpStatusConnected || iss != "" {
			t.Fatalf("ok: %q %q", st, iss)
		}
		st, iss := mcpListStatus(d, config.MCPServerEntry{Name: "bad"})
		if st != "✘ Failed to connect" || iss != "ENOENT: Executable not found in $PATH: \"nope\"" {
			t.Fatalf("bad: %q %q", st, iss)
		}
	})
}

func TestRunMCPList(t *testing.T) {
	t.Run("empty-store", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		if err := runMCPList(d, nil); err != nil {
			t.Fatal(err)
		}
		want := "No MCP servers configured. Use `claude mcp add` to add a server.\n"
		if out.String() != want {
			t.Fatalf("stdout = %q, want %q", out.String(), want)
		}
	})
	t.Run("empty-store-with-diagnostics", func(t *testing.T) {
		// Unknown-type-only stores show the empty message AND diagnostics.
		d, out, _ := newTestMCPDeps(t)
		addServer(t, d.Store, config.MCPScopeLocal, "bad", rawURLEntry("bogus", "x"))
		if err := runMCPList(d, nil); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !strings.HasPrefix(got, "No MCP servers configured. Use `claude mcp add` to add a server.\n") {
			t.Fatalf("stdout = %q", got)
		}
		if !strings.Contains(got, "[Contains warnings] Local config (private to you in this project)\n") {
			t.Fatalf("missing warnings header: %q", got)
		}
		wantLine := " └ [Warning] [bad] mcpServers.bad: Skipped — unknown MCP server type \"bogus\" for server \"bad\"\n"
		if !strings.Contains(got, wantLine) {
			t.Fatalf("missing warning line, want %q in %q", wantLine, got)
		}
		if !strings.Contains(got, "Location: "+config.ClaudeJsonPath(d.Store.HomeDir())+" [project: "+d.Store.ProjectKey()+"]\n") {
			t.Fatalf("missing Location line: %q", got)
		}
	})
	t.Run("health-lines", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		addServer(t, d.Store, config.MCPScopeLocal, "alpha", rawStdioEntry("echo", "hello", "world"))
		addServer(t, d.Store, config.MCPScopeLocal, "uu", rawStdioEntry("echo"))
		addServer(t, d.Store, config.MCPScopeLocal, "hh", rawURLEntry("http", "http://hh.test/mcp"))
		d.Health = &fakeMCPHealth{byName: map[string]mcpHealthResult{
			"alpha": {Connected: true},
			"uu":    {Issue: "-32000: MCP error -32000: Connection closed"},
			"hh":    {Issue: "ENOTFOUND: getaddrinfo ENOTFOUND hh.test"},
		}}
		if err := runMCPList(d, nil); err != nil {
			t.Fatal(err)
		}
		want := "Checking MCP server health…\n\n" +
			"alpha: echo hello world - ✔ Connected\n" +
			"uu: echo  - ✘ Failed to connect — -32000: MCP error -32000: Connection closed\n" +
			"hh: http://hh.test/mcp (HTTP) - ✘ Failed to connect — ENOTFOUND: getaddrinfo ENOTFOUND hh.test\n"
		if out.String() != want {
			t.Fatalf("stdout =\n%q\nwant\n%q", out.String(), want)
		}
	})
	t.Run("conflict-diagnostics-after-lines", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		addServer(t, d.Store, config.MCPScopeUser, "multi", rawStdioEntry("echo3"))
		addServer(t, d.Store, config.MCPScopeProject, "multi", rawStdioEntry("echo2"))
		if err := runMCPList(d, nil); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		want := "\nMCP config diagnostics ⚠\n\n" +
			"For help configuring MCP servers, see: https://code.claude.com/docs/en/mcp\n\n" +
			"[Conflicting scopes]\n" +
			"├ Server \"multi\" is defined in multiple scopes with different endpoints: " +
			"user (echo3), project (echo2). OAuth tokens are stored per endpoint, " +
			"so authenticating in one context will not carry over.\n" +
			"└ Keep the correct endpoint and remove the others: " +
			"`claude mcp remove multi -s user` or `claude mcp remove multi -s project`\n\n"
		if !strings.HasSuffix(got, want) {
			t.Fatalf("stdout tail =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("warnings-before-conflicts", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		addServer(t, d.Store, config.MCPScopeLocal, "bad", rawURLEntry("bogus", "x"))
		addServer(t, d.Store, config.MCPScopeUser, "multi", rawStdioEntry("echo3"))
		addServer(t, d.Store, config.MCPScopeProject, "multi", rawStdioEntry("echo2"))
		if err := runMCPList(d, nil); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if i, j := strings.Index(got, "[Contains warnings]"), strings.Index(got, "[Conflicting scopes]"); i < 0 || j < 0 || i > j {
			t.Fatalf("warnings must precede conflicts: %d vs %d in %q", i, j, got)
		}
	})
	t.Run("extra-positionals-and-help", func(t *testing.T) {
		d, out, _ := newTestMCPDeps(t)
		addServer(t, d.Store, config.MCPScopeLocal, "alpha", rawStdioEntry("echo"))
		if err := runMCPList(d, []string{"extra-positional"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "alpha: echo  - ✔ Connected\n") {
			t.Fatalf("stdout = %q", out.String())
		}
		d2, out2, _ := newTestMCPDeps(t)
		if err := runMCPList(d2, []string{"-h"}); err != nil {
			t.Fatal(err)
		}
		if out2.String() != mcpListHelp {
			t.Fatalf("-h output = %q", out2.String())
		}
	})
}

func TestMCPJSONHelpers(t *testing.T) {
	t.Run("unmarshal-object", func(t *testing.T) {
		m := config.NewOrderedMap()
		if err := mcpJSONUnmarshalObject(`{"a":1}`, m); err != nil {
			t.Fatal(err)
		}
		if err := mcpJSONUnmarshalObject(`  {"a":1}  `, m); err != nil {
			t.Fatalf("trimmed object must parse: %v", err)
		}
		for _, raw := range []string{`[1]`, `"s"`, `notjson`} {
			if err := mcpJSONUnmarshalObject(raw, m); err == nil {
				t.Fatalf("%q must be rejected", raw)
			}
		}
	})
	t.Run("string", func(t *testing.T) {
		m := config.NewOrderedMap()
		if err := mcpJSONUnmarshalObject(`{"s":"v","n":1}`, m); err != nil {
			t.Fatal(err)
		}
		if v, ok := mcpJSONString(m, "s"); !ok || v != "v" {
			t.Fatalf("s = %q %v", v, ok)
		}
		if _, ok := mcpJSONString(m, "n"); ok {
			t.Fatal("non-string must not be ok")
		}
		if _, ok := mcpJSONString(m, "absent"); ok {
			t.Fatal("absent must not be ok")
		}
	})
	t.Run("string-array", func(t *testing.T) {
		m := config.NewOrderedMap()
		if err := mcpJSONUnmarshalObject(`{"a":["x","y"],"b":[1]}`, m); err != nil {
			t.Fatal(err)
		}
		arr, err := mcpJSONStringArray(m, "a")
		if err != nil || len(arr) != 2 {
			t.Fatalf("a = %v %v", arr, err)
		}
		if _, err := mcpJSONStringArray(m, "b"); err == nil {
			t.Fatal("non-string element must fail")
		}
		arr, err = mcpJSONStringArray(m, "absent")
		if err != nil || arr == nil || len(arr) != 0 {
			t.Fatalf("absent must default to empty non-nil: %v %v", arr, err)
		}
	})
	t.Run("string-map", func(t *testing.T) {
		m := config.NewOrderedMap()
		if err := mcpJSONUnmarshalObject(`{"m":{"K":"v","J":"w"},"bad":{"K":1}}`, m); err != nil {
			t.Fatal(err)
		}
		mm, err := mcpJSONStringMap(m, "m")
		if err != nil || mm.Len() != 2 || strings.Join(mm.Keys(), ",") != "K,J" {
			t.Fatalf("m = %v %v", mm, err)
		}
		if _, err := mcpJSONStringMap(m, "bad"); err == nil {
			t.Fatal("non-string value must fail")
		}
		mm, err = mcpJSONStringMap(m, "absent")
		if err != nil || mm.Len() != 0 {
			t.Fatalf("absent must default to empty: %v %v", mm, err)
		}
	})
}

func TestMCPHasClientSecretEnv(t *testing.T) {
	t.Setenv("MCP_CLIENT_SECRET", "")
	if mcpHasClientSecretEnv() {
		t.Fatal("empty env must be false")
	}
	t.Setenv("MCP_CLIENT_SECRET", "x")
	if !mcpHasClientSecretEnv() {
		t.Fatal("set env must be true")
	}
}

func TestMCPDisplayCommand(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://a/", "http://a"},
		{"http://a//", "http://a/"},
		{"http://a///", "http://a//"},
		{"http://a", "http://a"},
		{"https://x/", "https://x"},
		{"cmd/", "cmd/"},
		{"echo", "echo"},
		{"ws://a/", "ws://a/"},
	}
	for _, tc := range cases {
		if got := mcpDisplayCommand(tc.in); got != tc.want {
			t.Fatalf("mcpDisplayCommand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMCPWSHref(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ws://127.0.0.1:1", "ws://127.0.0.1:1/"},
		{"ws://a/b", "ws://a/b"},
		{"ws://a/", "ws://a/"},
		{"wss://a", "wss://a/"},
	}
	for _, tc := range cases {
		if got := mcpWSHref(tc.in); got != tc.want {
			t.Fatalf("mcpWSHref(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
