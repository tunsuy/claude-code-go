package bootstrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/config"
)

// RunE bodies for the `claude mcp` subcommands.  Each takes an mcpDeps bundle
// (writers + store + health checker) and the raw argv tail, so the full
// command surface is testable without a TTY, network or the real home dir.
// Every user-visible string here is byte-pinned against the oracle
// (claude v2.1.261); see the probes recorded in internal/bootstrap/CONTEXT.md.

// mcpDeps carries everything a RunE body needs.
type mcpDeps struct {
	Stdout io.Writer
	Stderr io.Writer
	Store  *config.MCPStore
	Health mcpHealthChecker
}

// newMCPDepsWith builds deps over explicit streams (the cobra wiring passes
// cmd.OutOrStdout()/cmd.ErrOrStderr() so tests can capture output).
func newMCPDepsWith(stdout, stderr io.Writer) (*mcpDeps, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("mcp: get working directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("mcp: home directory: %w", err)
	}
	return &mcpDeps{
		Stdout: stdout,
		Stderr: stderr,
		Store:  config.NewMCPStore(home, cwd),
		Health: newMCPHealthChecker(),
	}, nil
}

// errMCPAction is a user-facing action error: printed verbatim to stderr (no
// "Error:" prefix), exit code 1.
type errMCPAction struct{ msg string }

func (e *errMCPAction) Error() string { return e.msg }

func mcpActionErrorf(format string, args ...any) error {
	return &errMCPAction{msg: fmt.Sprintf(format, args...)}
}

// errMCPExit is a silent exit (all diagnostics were already printed by the
// body); only the exit code matters.
type errMCPExit struct{ code int }

func (e *errMCPExit) Error() string { return fmt.Sprintf("mcp: exit %d", e.code) }

// runMCPAdd implements `mcp add`.  Validation order (oracle-verified):
// scope word → transport word → env format (stdio only) / header format
// (http/sse only) → callback-port integer+range (http/sse only) →
// client-secret availability gate → duplicate check → write.
func runMCPAdd(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpAddFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpAddHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, mcpAddPositionals); err != nil {
		return err
	}
	name := p.Positionals[0]
	commandOrURL := p.Positionals[1]
	cmdArgs := p.Positionals[2:]

	scope := p.Values["scope"]
	if scope == "" {
		scope = config.MCPScopeLocal
	}
	if err := config.ValidateMCPScope(scope); err != nil {
		return mcpActionErrorf("%s", err.Error())
	}

	transport := p.Values["transport"]
	switch transport {
	case "", "stdio", "sse", "http":
	case "streamable-http":
		transport = "http"
	default:
		return mcpActionErrorf("Invalid transport type: %s. Must be one of: stdio, sse, http (or streamable-http)", transport)
	}
	// Without -t the commandOrUrl is ALWAYS a stdio command — even when it
	// looks like a URL, which is exactly what the URL warning below is about.
	effective := transport
	if effective == "" {
		effective = "stdio"
	}
	isURL := strings.HasPrefix(commandOrURL, "http://") || strings.HasPrefix(commandOrURL, "https://")
	display := mcpDisplayCommand(commandOrURL)

	var envPairs, headerPairs []config.EnvPair
	callbackPort := 0
	clientSecretWanted := false
	switch effective {
	case "http", "sse":
		headerPairs, err = mcpParseHeaderList(p.Lists["header"])
		if err != nil {
			return err
		}
		if v := p.Values["callback-port"]; v != "" {
			port, convErr := strconv.Atoi(v)
			if convErr != nil || port < 1 || port > 65535 {
				return mcpActionErrorf("Error: --callback-port must be an integer in [1, 65535]")
			}
			callbackPort = port
		}
		// The secret prompt only triggers when OAuth is actually being
		// configured; --client-secret alone is silently ignored (capture:
		// flows5 add-secret-flag exits 0 with no warning and no oauth key).
		clientSecretWanted = p.Bools["client-secret"] &&
			(p.Values["client-id"] != "" || p.Values["callback-port"] != "")
		if clientSecretWanted && !mcpHasClientSecretEnv() {
			// Divergence: the oracle prompts interactively on a TTY; we have
			// no prompt implementation, so the env var is the only path.
			return mcpActionErrorf("No TTY available to prompt for client secret. Set MCP_CLIENT_SECRET env var instead.")
		}
	default: // stdio
		envPairs, err = mcpParseEnvList(p.Lists["env"])
		if err != nil {
			return err
		}
	}

	entry := config.NewOrderedMap()
	switch effective {
	case "http", "sse":
		entry.Set("type", effective)
		entry.Set("url", commandOrURL)
		if len(headerPairs) > 0 {
			hdrs := config.NewOrderedMap()
			for _, hp := range headerPairs {
				hdrs.Set(hp.Key, hp.Value)
			}
			entry.Set("headers", hdrs)
		}
		oauth := config.NewOrderedMap()
		if v := p.Values["client-id"]; v != "" {
			oauth.Set("clientId", v)
		}
		if callbackPort != 0 {
			oauth.Set("callbackPort", callbackPort)
		}
		if oauth.Len() > 0 {
			entry.Set("oauth", oauth)
		}
	default: // stdio — env is always stored, even empty (capture: flows.txt storage)
		entry.Set("type", "stdio")
		entry.Set("command", commandOrURL)
		arr := make([]any, len(cmdArgs))
		for i, a := range cmdArgs {
			arr[i] = a
		}
		entry.Set("args", arr)
		envMap := config.NewOrderedMap()
		for _, ep := range envPairs {
			envMap.Set(ep.Key, ep.Value)
		}
		entry.Set("env", envMap)
	}

	if err := d.Store.AddMCPServer(scope, name, entry); err != nil {
		return mcpActionErrorf("%s", err.Error())
	}

	// Warnings (stderr, before the stdout success block; ordering between the
	// oauth and URL warnings captured in flows o4: oauth first, then the
	// blank-line-led URL warning).
	if effective == "stdio" &&
		(p.Values["client-id"] != "" || p.Bools["client-secret"] || p.Values["callback-port"] != "") {
		fmt.Fprint(d.Stderr, "Warning: --client-id, --client-secret, --callback-port, and --xaa are only supported for HTTP/SSE transports and will be ignored for stdio.\n")
	}
	if clientSecretWanted && mcpHasClientSecretEnv() {
		fmt.Fprint(d.Stderr, "Server added, but the client secret could not be stored. Re-run with --client-secret once secure storage is available.\n")
	}
	if transport == "" && isURL {
		mcpRenderURLWarning(d.Stderr, name, display)
	}

	mcpRenderAdded(d.Stdout, effective, name, display, cmdArgs, scope)
	if effective == "http" || effective == "sse" {
		mcpRenderHeaders(d.Stdout, headerPairs)
	}
	fmt.Fprint(d.Stdout, mcpFileModifiedLine(d.Store, scope))
	return nil
}

// mcpParseEnvList validates -e values (KEY=VALUE, non-empty key).
func mcpParseEnvList(vals []string) ([]config.EnvPair, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	pairs := make([]config.EnvPair, 0, len(vals))
	for _, v := range vals {
		if !strings.Contains(v, "=") || strings.SplitN(v, "=", 2)[0] == "" {
			return nil, mcpActionErrorf("Invalid environment variable format: %s, environment variables should be added as: -e KEY1=value1 -e KEY2=value2", v)
		}
		kv := strings.SplitN(v, "=", 2)
		pairs = append(pairs, config.EnvPair{Key: kv[0], Value: kv[1]})
	}
	return pairs, nil
}

// mcpParseHeaderList validates -H values ("Name: value" with non-empty name).
func mcpParseHeaderList(vals []string) ([]config.EnvPair, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	pairs := make([]config.EnvPair, 0, len(vals))
	for _, v := range vals {
		idx := strings.Index(v, ":")
		if idx < 0 {
			return nil, mcpActionErrorf("Invalid header format: %q. Expected format: \"Header-Name: value\"", v)
		}
		hname := strings.TrimSpace(v[:idx])
		if hname == "" {
			return nil, mcpActionErrorf("Invalid header: %q. Header name cannot be empty.", v)
		}
		pairs = append(pairs, config.EnvPair{Key: hname, Value: strings.TrimSpace(v[idx+1:])})
	}
	return pairs, nil
}

// runMCPAddJSON implements `mcp add-json`.  --client-secret is accepted and
// silently ignored here (capture o6: exit 0, no warning, no oauth key).
func runMCPAddJSON(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpAddJSONFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpAddJSONHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, mcpAddJSONPositionals); err != nil {
		return err
	}
	name, rawJSON := p.Positionals[0], p.Positionals[1]

	scope := p.Values["scope"]
	if scope == "" {
		scope = config.MCPScopeLocal
	}
	if err := config.ValidateMCPScope(scope); err != nil {
		return mcpActionErrorf("%s", err.Error())
	}

	entry, storedType, err := mcpValidateAddJSON(rawJSON)
	if err != nil {
		return err
	}
	if err := d.Store.AddMCPServer(scope, name, entry); err != nil {
		return mcpActionErrorf("%s", err.Error())
	}
	fmt.Fprintf(d.Stdout, "Added %s MCP server %s to %s config\n", storedType, name, mcpScopeWord(scope))
	return nil
}

// mcpValidateAddJSON parses and validates the add-json payload, returning the
// normalized OrderedMap to store plus the display type.  The oracle's schema:
// type is optional (default stdio, and a typeless entry stores NO type key);
// stdio needs a string command and stores args (defaulted to []) plus env
// only when present in the input; sse/http/streamable-http(→http) and ws
// (stored verbatim; wss/websocket are rejected) need a string url and store
// headers only when present; every other shape fails with the single generic
// "Invalid configuration" message.
func mcpValidateAddJSON(raw string) (*config.OrderedMap, string, error) {
	invalid := mcpActionErrorf("Invalid configuration: : Invalid input")
	m := config.NewOrderedMap()
	if err := mcpJSONUnmarshalObject(raw, m); err != nil {
		return nil, "", invalid
	}

	out := config.NewOrderedMap()
	switch m.GetString("type") {
	case "", "stdio":
		cmd, ok := mcpJSONString(m, "command")
		if !ok {
			return nil, "", invalid
		}
		if m.GetString("type") == "stdio" {
			out.Set("type", "stdio")
		}
		out.Set("command", cmd)
		args, err := mcpJSONStringArray(m, "args")
		if err != nil {
			return nil, "", invalid
		}
		out.Set("args", args)
		if _, hasEnv := m.Get("env"); hasEnv {
			env, err := mcpJSONStringMap(m, "env")
			if err != nil {
				return nil, "", invalid
			}
			out.Set("env", env)
		}
		return out, "stdio", nil
	case "sse", "http", "streamable-http", "ws":
		url, ok := mcpJSONString(m, "url")
		if !ok {
			return nil, "", invalid
		}
		stored := m.GetString("type")
		if stored == "streamable-http" {
			stored = "http"
		}
		out.Set("type", stored)
		out.Set("url", url)
		if _, hasHeaders := m.Get("headers"); hasHeaders {
			headers, err := mcpJSONStringMap(m, "headers")
			if err != nil {
				return nil, "", invalid
			}
			out.Set("headers", headers)
		}
		return out, stored, nil
	}
	return nil, "", invalid
}

// runMCPGet implements `mcp get`.  Extra positionals are ignored (capture:
// flows4 get-extra-args).  A project-scope winner renders ⏸ Pending (or ✘
// Rejected when disabledMcpjsonServers lists it) without a health check.
func runMCPGet(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpGetFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpGetHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, mcpNamePositional); err != nil {
		return err
	}
	name := p.Positionals[0]

	e, scope, ok := d.Store.GetMCPServer(name)
	if !ok {
		return mcpMissError(d.Store, name, true)
	}
	if scope == config.MCPScopeProject {
		if d.Store.ProjectApprovalState(name) == "rejected" {
			mcpRenderGet(d.Stdout, name, scope, e, mcpStatusRejected, "")
		} else {
			mcpRenderGet(d.Stdout, name, scope, e, mcpStatusPending, "")
		}
		return nil
	}
	res := d.Health.Check(context.Background(), name, e)
	if res.Connected {
		mcpRenderGet(d.Stdout, name, scope, e, mcpStatusConnected, "")
	} else {
		mcpRenderGet(d.Stdout, name, scope, e, "✘ Failed to connect", res.Issue)
	}
	return nil
}

// runMCPRemove implements `mcp remove`.  With -s the miss error names the
// scope; bare removal suggests across all servers (capture rm-prjx: a
// project-only name is suggested) and prints the miss without the get-only
// pending parenthetical (capture rm-alpga2).
func runMCPRemove(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpRemoveFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpRemoveHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, mcpNamePositional); err != nil {
		return err
	}
	name := p.Positionals[0]
	scope := p.Values["scope"]

	if scope != "" {
		if err := config.ValidateMCPScope(scope); err != nil {
			return mcpActionErrorf("%s", err.Error())
		}
		removed, err := d.Store.RemoveMCPServer(scope, name)
		if err != nil {
			return mcpActionErrorf("%s", err.Error())
		}
		if !removed {
			return mcpActionErrorf("No MCP server named \"%s\" in %s scope", name, scope)
		}
		fmt.Fprintf(d.Stdout, "Removed MCP server %s from %s config\n", name, mcpScopeWord(scope))
		fmt.Fprint(d.Stdout, mcpFileModifiedLine(d.Store, scope))
		return nil
	}

	scopes := d.Store.ScopesWithServer(name)
	switch len(scopes) {
	case 0:
		return mcpMissError(d.Store, name, false)
	case 1:
		scope = scopes[0]
		removed, err := d.Store.RemoveMCPServer(scope, name)
		if err != nil || !removed {
			return mcpActionErrorf("No MCP server named \"%s\" in %s scope", name, scope)
		}
		fmt.Fprintf(d.Stdout, "Removed MCP server \"%s\" from %s config\n", name, mcpScopeWord(scope))
		fmt.Fprint(d.Stdout, mcpFileModifiedLine(d.Store, scope))
		return nil
	default:
		mcpRenderRemoveMulti(d.Stderr, name, scopes, d.Store)
		return &errMCPExit{code: 1}
	}
}

// runMCPResetProjectChoices implements `mcp reset-project-choices`.  The
// second line appears only when the project has .mcp.json servers.
func runMCPResetProjectChoices(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpResetFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpResetHelp)
		return nil
	}
	if err := d.Store.ResetProjectChoices(); err != nil {
		return mcpActionErrorf("%s", err.Error())
	}
	fmt.Fprint(d.Stdout, "Project-scoped (.mcp.json) server approvals and rejections stored for this project have been reset.\n")
	if d.Store.McpJsonHasServers() {
		fmt.Fprint(d.Stdout, "You will be prompted for approval next time you start Claude Code.\n")
	}
	return nil
}

// runMCPAddFromClaudeDesktop implements `mcp add-from-claude-desktop`.
func runMCPAddFromClaudeDesktop(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpAfcdFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpAfcdHelp)
		return nil
	}
	return mcpRunAfcd(d, p)
}

// mcpMissError renders the get/remove miss message and wraps it as a silent
// exit-1.  get suggests and lists visible (local+user, known-type) names and
// may carry the pending-.mcp.json hints; remove lists all names and never
// does (the oracle's remove path calls the plain list renderer directly —
// captures: flows2 get-miss vs remove-miss-bare, rm-prjx, rm-alpga2,
// get-alpga2).
func mcpMissError(s *config.MCPStore, name string, forGet bool) error {
	names := s.AllServerNames()
	if forGet {
		names = s.VisibleServerNames()
	}
	var out strings.Builder
	suggestion := suggestServerName(name, names)
	mcpRenderMiss(&out, name, suggestion, names, forGet, s.HasPendingProjectServers())
	return &mcpMiser{msg: out.String()}
}

// mcpMiser wraps a pre-rendered miss message (already newline-terminated).
type mcpMiser struct{ msg string }

func (e *mcpMiser) Error() string { return e.msg }
