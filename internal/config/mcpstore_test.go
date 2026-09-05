package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestStore builds a store over fresh temp home/project dirs.
func newTestStore(t *testing.T) *MCPStore {
	t.Helper()
	home := t.TempDir()
	proj := t.TempDir()
	return NewMCPStore(home, proj)
}

// writeTestJSON writes raw JSON to path.
func writeTestJSON(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stdioEntry builds a minimal stdio entry.
func stdioEntry(command string, args ...string) *OrderedMap {
	e := NewOrderedMap()
	e.Set("type", "stdio")
	e.Set("command", command)
	arr := make([]any, len(args))
	for i, a := range args {
		arr[i] = a
	}
	e.Set("args", arr)
	e.Set("env", NewOrderedMap())
	return e
}

func TestOrderedMapRoundTripPreservesOrder(t *testing.T) {
	t.Parallel()
	raw := `{"z":1,"a":{"y":"1","b":"2"},"m":[3,1,2],"z2":true}`
	m := NewOrderedMap()
	if err := json.Unmarshal([]byte(raw), m); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Keys(), ","); got != "z,a,m,z2" {
		t.Fatalf("top-level key order = %q", got)
	}
	nested, _ := m.GetMap("a")
	if nested == nil || strings.Join(nested.Keys(), ",") != "y,b" {
		t.Fatalf("nested key order lost: %v", nested)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var again OrderedMap
	if err := json.Unmarshal(out, &again); err != nil {
		t.Fatal(err)
	}
	if strings.Join(again.Keys(), ",") != "z,a,m,z2" {
		t.Fatalf("marshal sorted or dropped keys: %s", out)
	}
}

func TestOrderedMapSetReplaceKeepsPosition(t *testing.T) {
	t.Parallel()
	m := NewOrderedMap()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 3)
	if got := strings.Join(m.Keys(), ","); got != "a,b" {
		t.Fatalf("keys = %q, want a,b", got)
	}
	m.Delete("a")
	m.Set("a", 4)
	if got := strings.Join(m.Keys(), ","); got != "b,a" {
		t.Fatalf("re-added key moved to end: %q, want b,a", got)
	}
}

func TestAddMCPServerLocalCreatesProjectEntryWithSiblings(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.AddMCPServer(MCPScopeLocal, "srv", stdioEntry("echo", "hi")); err != nil {
		t.Fatal(err)
	}
	doc, err := s.claudeJsonDoc()
	if err != nil {
		t.Fatal(err)
	}
	projects, _ := doc.GetMap("projects")
	if projects == nil {
		t.Fatal("projects map not created")
	}
	entry, ok := projects.GetMap(s.ProjectKey())
	if !ok || entry == nil {
		t.Fatal("project entry for resolved cwd not created")
	}
	// Oracle v2.1.261 seeds exactly these 8 keys on a fresh HOME (the
	// enabled/disabledMcpjsonServers arrays are seeded empty; a 6-key capture
	// ran reset-project-choices first, which deletes them).
	wantKeys := []string{
		"allowedTools", "mcpContextUris", "mcpServers", "enabledMcpjsonServers",
		"disabledMcpjsonServers", "hasTrustDialogAccepted",
		"hasClaudeMdExternalIncludesApproved", "hasClaudeMdExternalIncludesWarningShown",
	}
	if got := strings.Join(entry.Keys(), ","); got != strings.Join(wantKeys, ",") {
		t.Fatalf("project entry keys = %q, want %q (incl. order)", got, strings.Join(wantKeys, ","))
	}
	m, _ := entry.GetMap("mcpServers")
	if m == nil || m.Len() != 1 {
		t.Fatalf("mcpServers = %v", m)
	}
}

func TestAddMCPServerDupDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		scope string
		want  string
	}{
		{MCPScopeLocal, "MCP server srv already exists in local config"},
		{MCPScopeUser, "MCP server srv already exists in user config"},
		{MCPScopeProject, "MCP server srv already exists in .mcp.json"},
	}
	for _, tc := range cases {
		s := newTestStore(t)
		if err := s.AddMCPServer(tc.scope, "srv", stdioEntry("echo")); err != nil {
			t.Fatal(err)
		}
		err := s.AddMCPServer(tc.scope, "srv", stdioEntry("cat"))
		if err == nil || err.Error() != tc.want {
			t.Errorf("%s dup error = %v, want %q", tc.scope, err, tc.want)
		}
	}
}

func TestAddMCPServerUnwritableScope(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.AddMCPServer("enterprise", "srv", stdioEntry("echo"))
	if err == nil || err.Error() != "Cannot add MCP server to scope: enterprise" {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(ClaudeJsonPath(s.homeDir)); !os.IsNotExist(err) {
		t.Error("unwritable-scope add must not create ~/.claude.json")
	}
}

func TestAddMCPServerPreservesForeignKeysAndOrder(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	writeTestJSON(t, ClaudeJsonPath(s.homeDir), `{"alpha":1,"mcpServers":{"old":{"type":"stdio","command":"cat","args":[],"env":{}}},"omega":true}`)
	if err := s.AddMCPServer(MCPScopeUser, "new", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ClaudeJsonPath(s.homeDir))
	if err != nil {
		t.Fatal(err)
	}
	var doc OrderedMap
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(doc.Keys(), ","); got != "alpha,mcpServers,omega" {
		t.Errorf("top-level order changed: %q", got)
	}
	m, _ := doc.GetMap("mcpServers")
	if m == nil || strings.Join(m.Keys(), ",") != "old,new" {
		t.Errorf("server order = %v, want old,new", m)
	}
}

func TestValidateMCPScope(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"local", "user", "project", "dynamic", "enterprise", "claudeai", "managed", "agent"} {
		if err := ValidateMCPScope(ok); err != nil {
			t.Errorf("ValidateMCPScope(%q) = %v", ok, err)
		}
	}
	err := ValidateMCPScope("bogus")
	want := "Invalid scope: bogus. Must be one of: local, user, project, dynamic, enterprise, claudeai, managed, agent"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestRemoveMCPServer(t *testing.T) {
	t.Parallel()
	t.Run("with-scope", func(t *testing.T) {
		s := newTestStore(t)
		for _, scope := range []string{MCPScopeLocal, MCPScopeUser, MCPScopeProject} {
			if err := s.AddMCPServer(scope, "srv", stdioEntry("echo")); err != nil {
				t.Fatal(err)
			}
			removed, err := s.RemoveMCPServer(scope, "srv")
			if err != nil || !removed {
				t.Fatalf("%s remove = %v, %v", scope, removed, err)
			}
			if _, _, ok := s.GetMCPServer("srv"); ok {
				t.Errorf("%s: server still present", scope)
			}
		}
	})
	t.Run("miss", func(t *testing.T) {
		s := newTestStore(t)
		removed, err := s.RemoveMCPServer(MCPScopeUser, "nope")
		if err != nil || removed {
			t.Fatalf("miss remove = %v, %v", removed, err)
		}
	})
	t.Run("unwritable", func(t *testing.T) {
		s := newTestStore(t)
		_, err := s.RemoveMCPServer("dynamic", "x")
		if err == nil || err.Error() != "Cannot remove MCP server from scope: dynamic" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestGetMCPServerPrecedence(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for _, scope := range []string{MCPScopeProject, MCPScopeUser, MCPScopeLocal} {
		if err := s.AddMCPServer(scope, "dup", stdioEntry("cmd-"+scope)); err != nil {
			t.Fatal(err)
		}
	}
	e, scope, ok := s.GetMCPServer("dup")
	if !ok || scope != MCPScopeLocal || e.Command() != "cmd-local" {
		t.Fatalf("get = %v, %q, %v", e.Command(), scope, ok)
	}
	if got := s.ScopesWithServer("dup"); strings.Join(got, ",") != "local,project,user" {
		t.Fatalf("ScopesWithServer = %v", got)
	}
}

func TestListMCPServersOrderAndDedup(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// User scope insertion order deliberately non-alphabetical.
	if err := s.AddMCPServer(MCPScopeUser, "zeta", stdioEntry("cat")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeUser, "useronly", stdioEntry("cat")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeUser, "alpha", stdioEntry("cat")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeLocal, "localonly", stdioEntry("cat")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeProject, "projonly", stdioEntry("cat")); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range s.ListMCPServers() {
		got = append(got, e.Name)
	}
	want := "zeta,useronly,alpha,projonly,localonly"
	if strings.Join(got, ",") != want {
		t.Fatalf("list order = %q, want %q", strings.Join(got, ","), want)
	}
}

func TestListMCPServersOmitsRejectedAndUnknown(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.AddMCPServer(MCPScopeProject, "psrv", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeProject, "keep", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	// Rejection lives in the project's .claude/settings.local.json (oracle
	// v2.1.261 behavior — not the ~/.claude.json project entry).
	writeTestJSON(t, SettingsLocalPath(s.projectDir),
		`{"other":true,"disabledMcpjsonServers":["psrv"]}`)
	if entries := s.ListMCPServers(); len(entries) != 1 || entries[0].Name != "keep" {
		t.Fatalf("list = %v, want only keep", entries)
	}
}

// writeRejection records a rejected .mcp.json server in the project's
// settings.local.json, the way the oracle's approval flow does.
func writeRejection(t *testing.T, s *MCPStore, names ...string) {
	t.Helper()
	raw := `{"disabledMcpjsonServers":[`
	for i, n := range names {
		if i > 0 {
			raw += ","
		}
		raw += `"` + n + `"`
	}
	raw += `]}`
	writeTestJSON(t, SettingsLocalPath(s.projectDir), raw)
}

func TestProjectApprovalState(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.AddMCPServer(MCPScopeProject, "srv", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	if got := s.ProjectApprovalState("srv"); got != "pending" {
		t.Fatalf("fresh project server state = %q, want pending", got)
	}
	// Enabled-listed ≠ approved: the oracle still renders such servers as
	// ⏸ Pending until its interactive flow records something else.
	writeTestJSON(t, SettingsLocalPath(s.projectDir), `{"enabledMcpjsonServers":["srv"]}`)
	if got := s.ProjectApprovalState("srv"); got != "pending" {
		t.Fatalf("enabled-listed server state = %q, want pending (enabled ≠ approved)", got)
	}
	writeRejection(t, s, "srv")
	if got := s.ProjectApprovalState("srv"); got != "rejected" {
		t.Fatalf("rejected server state = %q, want rejected", got)
	}
	// Rejection in any settings layer counts (union across files).
	s2 := newTestStore(t)
	if err := s2.AddMCPServer(MCPScopeProject, "srv", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(s2.projectDir, ".claude", "settings.json"),
		`{"disabledMcpjsonServers":["srv"]}`)
	if got := s2.ProjectApprovalState("srv"); got != "rejected" {
		t.Fatalf("shared-settings rejection state = %q, want rejected", got)
	}
}

func TestVisibleServerNamesExcludesAllProjectServers(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.AddMCPServer(MCPScopeUser, "u", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"pending", "enabled", "rejected"} {
		if err := s.AddMCPServer(MCPScopeProject, p, stdioEntry("echo")); err != nil {
			t.Fatal(err)
		}
	}
	writeTestJSON(t, SettingsLocalPath(s.projectDir),
		`{"enabledMcpjsonServers":["enabled"],"disabledMcpjsonServers":["rejected"]}`)
	if got := strings.Join(s.VisibleServerNames(), ","); got != "u" {
		t.Fatalf("visible = %q, want u (all project servers excluded)", got)
	}
	if got := strings.Join(s.AllServerNames(), ","); got != "enabled,pending,rejected,u" {
		t.Fatalf("all = %q", got)
	}
	if !s.HasPendingProjectServers() {
		t.Fatal("HasPendingProjectServers should be true (pending+enabled unshadowed)")
	}
}

func TestHasPendingProjectServersShadowing(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// Project server whose name also exists in user scope: shadowed, no
	// parenthetical.  Verified against the oracle by controlled experiment.
	if err := s.AddMCPServer(MCPScopeUser, "multi", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeProject, "multi", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	if s.HasPendingProjectServers() {
		t.Fatal("shadowed project server must not trigger pending parenthetical")
	}
	// Same-name in local scope shadows too.
	if err := s.AddMCPServer(MCPScopeLocal, "loc", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeProject, "loc", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	if s.HasPendingProjectServers() {
		t.Fatal("local-shadowed project server must not trigger parenthetical")
	}
	// An unshadowed pending server flips it back on.
	if err := s.AddMCPServer(MCPScopeProject, "solo", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	if !s.HasPendingProjectServers() {
		t.Fatal("unshadowed pending project server should trigger parenthetical")
	}
	// Rejected-only project servers never trigger it.
	s2 := newTestStore(t)
	if err := s2.AddMCPServer(MCPScopeProject, "pp", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	writeRejection(t, s2, "pp")
	if s2.HasPendingProjectServers() {
		t.Fatal("rejected-only project scope must not trigger parenthetical")
	}
}

// wsEntry builds a ws entry the way `mcp add-json` stores it.
func wsEntry(url string) *OrderedMap {
	e := NewOrderedMap()
	e.Set("type", "ws")
	e.Set("url", url)
	return e
}

func TestWSTypeGetVsListVisibility(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.AddMCPServer(MCPScopeLocal, "w", wsEntry("ws://127.0.0.1:1")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMCPServer(MCPScopeLocal, "plain", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	// get finds ws entries (capture: `mcp get w1` rendered a status + Issue).
	if _, scope, ok := s.GetMCPServer("w"); !ok || scope != MCPScopeLocal {
		t.Fatalf("GetMCPServer(ws) = %v, want found in local", ok)
	}
	// list omits them entirely (capture: no status line, no diagnostics).
	for _, e := range s.ListMCPServers() {
		if e.Name == "w" {
			t.Fatal("ws entry must not appear in ListMCPServers")
		}
	}
	// Truly unknown types still surface for diagnostics.
	unknown := NewOrderedMap()
	unknown.Set("type", "bogus")
	unknown.Set("command", "x")
	if err := s.AddMCPServer(MCPScopeUser, "bad", unknown); err != nil {
		t.Fatal(err)
	}
	if entries := s.ListMCPServers(); len(entries) != 1 || entries[0].Name != "plain" {
		t.Fatalf("list = %v, want only plain", entries)
	}
}

func TestResetProjectChoicesAlsoCleansClaudeJSON(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.AddMCPServer(MCPScopeLocal, "srv", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	// Reset with the seeded (empty) approval arrays present in the project
	// entry: capture shows reset deletes them from ~/.claude.json too.
	if err := s.ResetProjectChoices(); err != nil {
		t.Fatal(err)
	}
	doc, err := s.claudeJsonDoc()
	if err != nil {
		t.Fatal(err)
	}
	projects, _ := doc.GetMap("projects")
	entry, _ := projects.GetMap(s.ProjectKey())
	if entry == nil {
		t.Fatal("project entry vanished")
	}
	if _, has := entry.Get("enabledMcpjsonServers"); has {
		t.Error("enabledMcpjsonServers not removed from ~/.claude.json project entry")
	}
	if _, has := entry.Get("disabledMcpjsonServers"); has {
		t.Error("disabledMcpjsonServers not removed from ~/.claude.json project entry")
	}
	// The server itself survives.
	if _, _, ok := s.GetMCPServer("srv"); !ok {
		t.Fatal("reset must not remove configured servers")
	}
}

func TestResetProjectChoices(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if err := s.AddMCPServer(MCPScopeProject, "srv", stdioEntry("echo")); err != nil {
		t.Fatal(err)
	}
	local := SettingsLocalPath(s.projectDir)
	writeTestJSON(t, local, `{"keep":true,"enabledMcpjsonServers":["srv"],"disabledMcpjsonServers":["other"]}`)
	if err := s.ResetProjectChoices(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"keep\": true\n}"
	if string(data) != want {
		t.Fatalf("settings.local.json = %q, want %q", data, want)
	}
	if got := s.ProjectApprovalState("srv"); got != "pending" {
		t.Fatalf("state after reset = %q, want pending", got)
	}
	// No-op when the file is absent — nothing is created.
	s2 := newTestStore(t)
	if err := s2.ResetProjectChoices(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(SettingsLocalPath(s2.projectDir)); !os.IsNotExist(err) {
		t.Error("reset must not create settings.local.json when absent")
	}
	// File present but no approval keys: left byte-identical.
	s3 := newTestStore(t)
	writeTestJSON(t, SettingsLocalPath(s3.projectDir), `{"model":"opus"}`)
	if err := s3.ResetProjectChoices(); err != nil {
		t.Fatal(err)
	}
	data3, err := os.ReadFile(SettingsLocalPath(s3.projectDir))
	if err != nil {
		t.Fatal(err)
	}
	if string(data3) != `{"model":"opus"}` {
		t.Errorf("unrelated settings.local.json rewritten: %q", data3)
	}
}

func TestMcpJsonHasServers(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if s.McpJsonHasServers() {
		t.Fatal("no .mcp.json → false")
	}
	writeTestJSON(t, McpJsonPath(s.projectDir), `{"mcpServers":{}}`)
	if s.McpJsonHasServers() {
		t.Fatal("empty mcpServers → false")
	}
	writeTestJSON(t, McpJsonPath(s.projectDir), `{"mcpServers":{"x":{"type":"stdio"}}}`)
	if !s.McpJsonHasServers() {
		t.Fatal("one server → true")
	}
}

func TestWriteOrderedJSONAtomicFormatting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	m := NewOrderedMap()
	m.Set("b", 1)
	m.Set("a", []any{"x", json.Number("2")})
	if err := WriteOrderedJSONAtomic(path, m); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"b\": 1,\n  \"a\": [\n    \"x\",\n    2\n  ]\n}"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", data, want)
	}
}
