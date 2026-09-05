package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// `mcp list` rendering, `mcp add-from-claude-desktop`, and the add-json field
// helpers, split from mcp_run.go to respect file-size limits.  All user-visible
// strings are byte-pinned against the oracle (claude v2.1.261).

// runMCPList implements `mcp list`.  The empty-store message and the health
// banner are gated on different sets (oracle-verified by controlled probes):
// the banner shows when ANY scope holds a known-type, non-rejected entry —
// even a ws entry, which renders no line — while the message shows otherwise,
// and diagnostics render after either.  Extra positionals are ignored.
func runMCPList(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpListFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpListHelp)
		return nil
	}
	if !mcpHasVisibleServers(d.Store) {
		fmt.Fprint(d.Stdout, "No MCP servers configured. Use `claude mcp add` to add a server.\n")
	} else {
		fmt.Fprint(d.Stdout, "Checking MCP server health…\n\n")
		for _, e := range d.Store.ListMCPServers() {
			status, issue := mcpListStatus(d, e)
			if issue != "" {
				fmt.Fprintf(d.Stdout, "%s: %s - %s — %s\n", e.Name, mcpListEndpoint(e), status, issue)
			} else {
				fmt.Fprintf(d.Stdout, "%s: %s - %s\n", e.Name, mcpListEndpoint(e), status)
			}
		}
	}
	if mcpHasDiagnostics(d.Store) {
		mcpRenderDiagnostics(d.Stdout, d.Store)
	}
	return nil
}

// mcpHasVisibleServers reports whether any scope holds an entry that counts
// toward `mcp list`'s has-servers check: known type (typeless/stdio/sse/http/
// ws) and, in project scope, not rejected.  ws entries count (banner, no
// line); rejected project and unknown-type entries do not (empty message).
func mcpHasVisibleServers(s *config.MCPStore) bool {
	for _, scope := range config.ListScopeOrder {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.KnownType() {
				continue
			}
			if scope == config.MCPScopeProject && s.ProjectApprovalState(e.Name) == "rejected" {
				continue
			}
			return true
		}
	}
	return false
}

// mcpListStatus decides the status marker for one list line.  When the winning
// definition of the name is an unrejected project-scope server, the line shows
// ⏸ Pending without a health check; otherwise the entry is health-checked.
func mcpListStatus(d *mcpDeps, e config.MCPServerEntry) (string, string) {
	if _, scope, ok := d.Store.GetMCPServer(e.Name); ok && scope == config.MCPScopeProject &&
		d.Store.ProjectApprovalState(e.Name) != "rejected" {
		return mcpStatusPending, ""
	}
	res := d.Health.Check(context.Background(), e.Name, e)
	if res.Connected {
		return mcpStatusConnected, ""
	}
	return "✘ Failed to connect", res.Issue
}

// mcpHasDiagnostics reports whether the "MCP config diagnostics" section
// renders after the server lines: any scope holds unknown-type entries, or
// any name conflicts across scopes.
func mcpHasDiagnostics(s *config.MCPStore) bool {
	for _, scope := range config.ListScopeOrder {
		unknown, err := mcpUnknownTypeEntries(s, scope)
		if err == nil && len(unknown) > 0 {
			return true
		}
	}
	return len(mcpConflicts(s)) > 0
}

// mcpUnknownTypeEntries returns a scope's entries whose type is not even
// gettable, in stored order.  (ws is known — get finds it — so it never
// surfaces as a diagnostic.)
func mcpUnknownTypeEntries(s *config.MCPStore, scope string) ([]config.MCPServerEntry, error) {
	entries, err := s.ScopeEntries(scope)
	if err != nil {
		return nil, err
	}
	var out []config.MCPServerEntry
	for _, e := range entries {
		if !e.KnownType() {
			out = append(out, e)
		}
	}
	return out, nil
}

// mcpScopeConflict is one server name defined in multiple scopes with
// different endpoints (the [Conflicting scopes] diagnostic).
type mcpScopeConflict struct {
	Name      string
	Scopes    []string // in list order, parallel to Endpoints
	Endpoints []string
}

// mcpConflicts returns the store's conflicting server names in report order:
// scopes walked in list order (user, project, local), each scope's names in
// stored order, a name claimed at the first scope holding a typed entry.
// Only stdio/sse/http entries participate — typeless, ws and unknown-type
// entries never conflict (probes: t10, w9, unk1); rejected project entries
// do (probe rej1).
func mcpConflicts(s *config.MCPStore) []mcpScopeConflict {
	var out []mcpScopeConflict
	claimed := map[string]bool{}
	for _, scope := range config.ListScopeOrder {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if claimed[e.Name] || !mcpConflictParticipant(e.Type()) {
				continue
			}
			claimed[e.Name] = true
			if c, ok := mcpNameConflict(s, e.Name); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

// mcpNameConflict gathers a name's typed entries across scopes; two or more
// whose endpoints are not all identical is a conflict (probe s9: equal
// endpoints across different transports suppress it).
func mcpNameConflict(s *config.MCPStore, name string) (mcpScopeConflict, bool) {
	var c mcpScopeConflict
	for _, scope := range config.ListScopeOrder {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name != name || !mcpConflictParticipant(e.Type()) {
				continue
			}
			c.Name = name
			c.Scopes = append(c.Scopes, scope)
			c.Endpoints = append(c.Endpoints, e.Endpoint())
		}
	}
	if len(c.Scopes) < 2 {
		return c, false
	}
	for _, ep := range c.Endpoints[1:] {
		if ep != c.Endpoints[0] {
			return c, true
		}
	}
	return c, false
}

// mcpConflictParticipant reports whether a stored type participates in the
// conflicting-scopes diagnostic.
func mcpConflictParticipant(t string) bool {
	switch t {
	case "stdio", "sse", "http":
		return true
	}
	return false
}

// mcpRunAfcd implements `mcp add-from-claude-desktop`.  On a machine without
// the desktop app (or without a TTY) the oracle prints the single
// not-found line; with servers present it hangs in an interactive prompt we
// deliberately do not reproduce — printing "No servers were imported."
// instead (documented divergence).
func mcpRunAfcd(d *mcpDeps, p *mcpParsed) error {
	servers, err := config.DesktopConfigServers(d.Store.HomeDir())
	if err != nil {
		// An unreadable desktop config degrades to the same not-found message.
		servers = nil
	}
	if len(servers) == 0 {
		fmt.Fprint(d.Stdout, "No MCP servers found in Claude Desktop configuration or configuration file does not exist.\n")
		return nil
	}
	fmt.Fprint(d.Stdout, "No servers were imported.\n")
	return nil
}

// mcpJSONUnmarshalObject decodes raw into m, rejecting non-objects.
func mcpJSONUnmarshalObject(raw string, m *config.OrderedMap) error {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return fmt.Errorf("mcp: not a JSON object")
	}
	if err := json.Unmarshal([]byte(raw), m); err != nil {
		return err
	}
	return nil
}

// mcpJSONString fetches a string field (ok=false when absent or not a string).
func mcpJSONString(m *config.OrderedMap, key string) (string, bool) {
	v, _ := m.Get(key)
	s, ok := v.(string)
	return s, ok
}

// mcpJSONStringArray fetches an optional field defaulting to []any{}; every
// element must be a string.
func mcpJSONStringArray(m *config.OrderedMap, key string) ([]any, error) {
	out := []any{}
	v, _ := m.Get(key)
	if v == nil {
		return out, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("mcp: not an array")
	}
	for _, el := range arr {
		s, ok := el.(string)
		if !ok {
			return nil, fmt.Errorf("mcp: non-string element")
		}
		out = append(out, s)
	}
	return out, nil
}

// mcpJSONStringMap fetches an optional string→string map.
func mcpJSONStringMap(m *config.OrderedMap, key string) (*config.OrderedMap, error) {
	out := config.NewOrderedMap()
	v, _ := m.Get(key)
	if v == nil {
		return out, nil
	}
	nested, ok := v.(*config.OrderedMap)
	if !ok {
		return nil, fmt.Errorf("mcp: not a map")
	}
	for _, k := range nested.Keys() {
		raw, _ := nested.Get(k)
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("mcp: non-string value")
		}
		out.Set(k, s)
	}
	return out, nil
}

// mcpHasClientSecretEnv reports whether MCP_CLIENT_SECRET is set (non-empty).
func mcpHasClientSecretEnv() bool { return os.Getenv("MCP_CLIENT_SECRET") != "" }

// mcpDisplayCommand renders the commandOrUrl for `add` success lines and the
// URL warning: an http(s) URL has exactly one trailing slash stripped
// (http://a/ → http://a, http://a/// → http://a//), everything else is
// verbatim (cmd/ stays cmd/).  Storage, `get` URL lines and `list` endpoints
// always keep the raw value (captures: st1–st4, o4, ew, slash1/slash2, gst2).
func mcpDisplayCommand(cmd string) string {
	if strings.HasPrefix(cmd, "http://") || strings.HasPrefix(cmd, "https://") {
		return strings.TrimSuffix(cmd, "/")
	}
	return cmd
}

// mcpWSHref renders the href the oracle's ws failure message carries: the
// stored URL with "/" appended when its path is empty
// (ws://127.0.0.1:1 → ws://127.0.0.1:1/).
func mcpWSHref(url string) string {
	rest := url
	if i := strings.Index(url, "://"); i >= 0 {
		rest = url[i+3:]
	}
	if !strings.Contains(rest, "/") {
		return url + "/"
	}
	return url
}
