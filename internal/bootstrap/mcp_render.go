package bootstrap

import (
	"fmt"
	"io"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// Renderers for `claude mcp` output.  Every string here is byte-exact against
// the oracle (claude v2.1.261) — including the unicode glyphs (✔ ✘ ⏸ ⚠ — …),
// the double space in empty-arg command joins, and the redaction set.

// mcpHeaderSecrets are header-name substrings (matched case-insensitively)
// whose values are replaced with "[REDACTED]" in `add` output.
var mcpHeaderSecrets = []string{
	"authorization", "api-key", "apikey", "api-token", "token", "secret", "cookie", "password",
}

// mcpRedactHeader reports whether a header name triggers value redaction.
func mcpRedactHeader(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range mcpHeaderSecrets {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// mcpScopeWord maps a scope to the word used in success messages
// ("local config", "user config", "project config" minus "config" — the
// caller appends " config").
func mcpScopeWord(scope string) string {
	switch scope {
	case config.MCPScopeUser:
		return "user"
	case config.MCPScopeProject:
		return "project"
	}
	return "local"
}

// mcpFileModifiedLine renders the "File modified:" line for a scope.
func mcpFileModifiedLine(s *config.MCPStore, scope string) string {
	switch scope {
	case config.MCPScopeUser:
		return fmt.Sprintf("File modified: %s\n", config.ClaudeJsonPath(s.HomeDir()))
	case config.MCPScopeProject:
		return fmt.Sprintf("File modified: %s\n", mcpProjectConfigPath(s))
	default:
		return fmt.Sprintf("File modified: %s [project: %s]\n", config.ClaudeJsonPath(s.HomeDir()), s.ProjectKey())
	}
}

// mcpProjectConfigPath is the symlink-resolved .mcp.json path the oracle
// displays (on macOS, /tmp/cwd resolves to /private/tmp/cwd).
func mcpProjectConfigPath(s *config.MCPStore) string {
	return s.ProjectKey() + "/.mcp.json"
}

// mcpLocationPath renders the "Location:" path used by diagnostics and the
// multi-scope remove hint: user → ~/.claude.json, project → resolved
// .mcp.json, local → ~/.claude.json [project: <key>].
func mcpLocationPath(s *config.MCPStore, scope string) string {
	switch scope {
	case config.MCPScopeProject:
		return mcpProjectConfigPath(s)
	case config.MCPScopeLocal:
		return fmt.Sprintf("%s [project: %s]", config.ClaudeJsonPath(s.HomeDir()), s.ProjectKey())
	default:
		return config.ClaudeJsonPath(s.HomeDir())
	}
}

// mcpRenderAdded writes the `add` success line.  display is the display form
// of commandOrUrl (see mcpDisplayCommand) used for BOTH the stdio command and
// the http/sse URL; args join with the oracle's double space when empty.
func mcpRenderAdded(w io.Writer, transport, name, display string, args []string, scope string) {
	scopeWord := mcpScopeWord(scope)
	switch transport {
	case "http":
		fmt.Fprintf(w, "Added HTTP MCP server %s with URL: %s to %s config\n", name, display, scopeWord)
	case "sse":
		fmt.Fprintf(w, "Added SSE MCP server %s with URL: %s to %s config\n", name, display, scopeWord)
	default:
		fmt.Fprintf(w, "Added stdio MCP server %s with command: %s %s to %s config\n",
			name, display, strings.Join(args, " "), scopeWord)
	}
}

// mcpRenderHeaders writes the redacted headers block appended to `add`
// success output.  Keys keep insertion order.
func mcpRenderHeaders(w io.Writer, pairs []config.EnvPair) {
	if len(pairs) == 0 {
		return
	}
	fmt.Fprint(w, "Headers: {\n")
	for i, p := range pairs {
		val := p.Value
		if mcpRedactHeader(p.Key) {
			val = "[REDACTED]"
		}
		sep := ","
		if i == len(pairs)-1 {
			sep = ""
		}
		fmt.Fprintf(w, "  %q: %q%s\n", p.Key, val, sep)
	}
	fmt.Fprint(w, "}\n")
}

// mcpRenderURLWarning writes the stdio-misinterpretation warning the oracle
// prints when the command looks like a URL and --transport was not given.
func mcpRenderURLWarning(w io.Writer, name, command string) {
	fmt.Fprintf(w, "\nWarning: The command \"%s\" looks like a URL, but is being interpreted as a stdio server as --transport was not specified.\n", command)
	fmt.Fprintf(w, "If this is an HTTP server, use: claude mcp add --transport http %s %s\n", name, command)
	fmt.Fprintf(w, "If this is an SSE server, use: claude mcp add --transport sse %s %s\n", name, command)
}

// mcpStatusConnected / failed / pending / rejected are the status strings used
// by both `list` and `get`.
const (
	mcpStatusConnected = "✔ Connected"
	mcpStatusPending   = "⏸ Pending approval (run `claude` to approve)"
	mcpStatusRejected  = "✘ Rejected (see disabledMcpjsonServers in settings)"
)

// mcpListEndpoint renders the endpoint column of `list`: joined command+args
// for stdio (keeping the trailing-space join), the URL plus transport marker
// otherwise.
func mcpListEndpoint(e config.MCPServerEntry) string {
	switch e.Type() {
	case "http":
		return e.URL() + " (HTTP)"
	case "sse":
		return e.URL() + " (SSE)"
	default:
		return e.Command() + " " + strings.Join(e.Args(), " ")
	}
}

// mcpRenderGet writes the full `get` report.  issue is empty for connected,
// pending and rejected servers (no Issue line is rendered then).  The detail
// block renders only for stdio/sse/http — a typeless entry and a ws entry
// both show Scope/Status only (captures: j1 typeless, w1 ws).
func mcpRenderGet(w io.Writer, name, scope string, e config.MCPServerEntry, status, issue string) {
	fmt.Fprintf(w, "%s:\n", name)
	fmt.Fprintf(w, "  Scope: %s\n", config.ScopeLabel(scope))
	fmt.Fprintf(w, "  Status: %s\n", status)
	if issue != "" {
		fmt.Fprintf(w, "  Issue: %s\n", issue)
	}
	switch e.Type() {
	case "stdio":
		fmt.Fprintf(w, "  Type: %s\n", e.Type())
		fmt.Fprintf(w, "  Command: %s\n", e.Command())
		args := e.Args()
		if len(args) > 0 {
			fmt.Fprintf(w, "  Args: %s\n", strings.Join(args, " "))
		} else {
			fmt.Fprint(w, "  Args:\n")
		}
		if _, has := e.Raw.Get("env"); has {
			fmt.Fprint(w, "  Environment:\n")
			for _, p := range e.EnvPairs() {
				fmt.Fprintf(w, "    %s=%s\n", p.Key, p.Value)
			}
		}
	case "sse", "http":
		fmt.Fprintf(w, "  Type: %s\n", e.Type())
		fmt.Fprintf(w, "  URL: %s\n", e.URL())
		if _, has := e.Raw.Get("headers"); has {
			fmt.Fprint(w, "  Headers:\n")
			for _, p := range e.Headers() {
				fmt.Fprintf(w, "    %s: %s\n", p.Key, p.Value)
			}
		}
		if _, has := e.Raw.Get("oauth"); has {
			parts := mcpOAuthParts(e)
			fmt.Fprintf(w, "  OAuth: %s\n", strings.Join(parts, ", "))
		}
	}
	fmt.Fprintf(w, "\nTo remove this server, run: claude mcp remove %s -s %s\n", name, scope)
}

// mcpOAuthParts renders the OAuth detail parts of `get`: "client_id
// configured" and "callback_port N".
func mcpOAuthParts(e config.MCPServerEntry) []string {
	var parts []string
	if oauth, ok := e.Raw.GetMap("oauth"); ok {
		if cid, _ := oauth.Get("clientId"); cid != nil {
			parts = append(parts, "client_id configured")
		}
		if port, _ := oauth.Get("callbackPort"); port != nil {
			parts = append(parts, fmt.Sprintf("callback_port %v", port))
		}
	}
	return parts
}

// mcpRenderMiss writes the miss message shared by `get` and `remove`.
//
//	suggestion: the suggested server name ("" for none)
//	all:        the candidate names `get`/`remove` may list, sorted — first 8
//	            appear in the "Configured servers:" hint
//	forGet:     `get` carries the pending-.mcp.json hints (remove never does:
//	            the oracle's remove calls the plain list renderer directly)
//	hasPending: a non-rejected, non-shadowed .mcp.json server awaits approval
//
// With nothing else to list, a pending .mcp.json server replaces the whole
// message; otherwise the pending note is appended as a parenthetical — even
// when a suggestion was found.
func mcpRenderMiss(w io.Writer, name, suggestion string, all []string, forGet, hasPending bool) {
	if forGet && hasPending && len(all) == 0 {
		fmt.Fprintf(w, "No MCP server named \"%s\". .mcp.json servers are awaiting approval — run `claude` in this directory to review them.\n", name)
		return
	}
	switch {
	case suggestion != "":
		fmt.Fprintf(w, "No MCP server named \"%s\". Did you mean \"%s\"? Run `claude mcp list` to see all.", name, suggestion)
	case len(all) == 0:
		fmt.Fprintf(w, "No MCP server named \"%s\". Run `claude mcp add` to add one.", name)
	default:
		shown := all
		if len(shown) > 8 {
			shown = shown[:8]
		}
		fmt.Fprintf(w, "No MCP server named \"%s\". Configured servers: %s", name, strings.Join(shown, ", "))
		if len(all) > 8 {
			fmt.Fprintf(w, " (and %d more — run `claude mcp list` to see all)", len(all)-8)
		}
	}
	if forGet && hasPending {
		fmt.Fprint(w, " (.mcp.json servers are awaiting approval — run `claude` in this directory to review them.)")
	}
	fmt.Fprint(w, "\n")
}

// mcpRenderRemoveMulti writes the multi-scope removal hint block.
func mcpRenderRemoveMulti(w io.Writer, name string, scopes []string, s *config.MCPStore) {
	fmt.Fprintf(w, "MCP server \"%s\" exists in multiple scopes:\n", name)
	for _, scope := range scopes {
		fmt.Fprintf(w, "  - %s (%s)\n", config.ScopeLabel(scope), mcpLocationPath(s, scope))
	}
	fmt.Fprint(w, "\nTo remove from a specific scope, use:\n")
	for _, scope := range scopes {
		fmt.Fprintf(w, "  claude mcp remove %s -s %s\n", name, scope)
	}
}

// mcpRenderDiagnostics writes the "MCP config diagnostics ⚠" section appended
// to `list`: first the per-scope "[Contains warnings]" blocks for unknown-type
// entries (list order, each followed by a blank line), then the
// "[Conflicting scopes]" block for names defined in multiple scopes with
// different endpoints (probes cf/cg/ch: user, project, local order; every
// participating scope listed even when its endpoint equals another's).
func mcpRenderDiagnostics(w io.Writer, s *config.MCPStore) {
	fmt.Fprint(w, "\nMCP config diagnostics ⚠\n\n")
	fmt.Fprint(w, "For help configuring MCP servers, see: https://code.claude.com/docs/en/mcp\n\n")
	for _, scope := range config.ListScopeOrder {
		entries, err := mcpUnknownTypeEntries(s, scope)
		if err != nil || len(entries) == 0 {
			continue
		}
		fmt.Fprintf(w, "[Contains warnings] %s\n", config.ScopeLabel(scope))
		fmt.Fprintf(w, "Location: %s\n", mcpLocationPath(s, scope))
		for _, e := range entries {
			fmt.Fprintf(w, " └ [Warning] [%s] mcpServers.%s: Skipped — unknown MCP server type \"%s\" for server \"%s\"\n",
				e.Name, e.Name, e.Type(), e.Name)
		}
		fmt.Fprint(w, "\n")
	}
	conflicts := mcpConflicts(s)
	if len(conflicts) == 0 {
		return
	}
	fmt.Fprint(w, "[Conflicting scopes]\n")
	for i, c := range conflicts {
		parts := make([]string, len(c.Scopes))
		removes := make([]string, len(c.Scopes))
		for j, scope := range c.Scopes {
			parts[j] = fmt.Sprintf("%s (%s)", scope, c.Endpoints[j])
			removes[j] = fmt.Sprintf("`claude mcp remove %s -s %s`", c.Name, scope)
		}
		fmt.Fprintf(w, "├ Server %q is defined in multiple scopes with different endpoints: %s. OAuth tokens are stored per endpoint, so authenticating in one context will not carry over.\n",
			c.Name, strings.Join(parts, ", "))
		branch := "├"
		if i == len(conflicts)-1 {
			branch = "└"
		}
		fmt.Fprintf(w, "%s Keep the correct endpoint and remove the others: %s\n", branch, strings.Join(removes, " or "))
	}
	fmt.Fprint(w, "\n")
}
