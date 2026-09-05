package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/tunsuy/claude-code-go/internal/config"
	"github.com/tunsuy/claude-code-go/internal/mcp"
)

// Health checking for `mcp list` / `mcp get`.  The connection is attempted with
// the real MCP client stack; failures are then translated into the exact
// strings the oracle (claude v2.1.261) prints.  The translation table:
//
//	stdio, command not on $PATH (no slash):
//	    ENOENT: Executable not found in $PATH: "cmd"
//	stdio, command path missing (contains a slash):
//	    ENOENT: ENOENT: no such file or directory, posix_spawn 'cmd'
//	http DNS failure:   ENOTFOUND: getaddrinfo ENOTFOUND <host>
//	sse  DNS failure:   SSE error: getaddrinfo ENOTFOUND <host>
//	http refused:       ConnectionRefused: Unable to connect. Is the computer able to access the url?
//	sse  refused:       SSE error: Unable to connect. Is the computer able to access the url?
//	everything else:    -32000: MCP error -32000: Connection closed
//
// The last row is an approximation: uncaptured oracle failure classes (HTTP
// error responses, timeouts) are folded into the generic MCP error, which is
// what the TS SDK reports when the transport closes without a result.

// mcpHealthTimeout bounds a single health-check connection attempt.
const mcpHealthTimeout = 30 * time.Second

// mcpHealthResult is the outcome of one health check.
type mcpHealthResult struct {
	Connected bool
	Issue     string // oracle-format failure string, empty when connected
}

// mcpHealthChecker connects to a server entry and reports the oracle-format
// result.  It is an interface so tests can inject deterministic results.
type mcpHealthChecker interface {
	Check(ctx context.Context, name string, e config.MCPServerEntry) mcpHealthResult
}

// mcpPoolChecker is the production checker, backed by the MCP client pool.
type mcpPoolChecker struct{}

// newMCPHealthChecker returns the default pool-backed checker.
func newMCPHealthChecker() mcpHealthChecker { return mcpPoolChecker{} }

// Check dials the server with a fresh pool and translates the outcome.
// ws entries are never dialed: the transport is unimplemented in this CLI, so
// the check always reports the oracle's ws failure string (capture: `mcp get
// w1` → "WebSocket connection to 'ws://127.0.0.1:1/' failed: Failed to
// connect" — documented divergence, no connection is attempted).
func (mcpPoolChecker) Check(ctx context.Context, name string, e config.MCPServerEntry) mcpHealthResult {
	if e.Type() == "ws" {
		return mcpHealthResult{Issue: fmt.Sprintf("WebSocket connection to '%s' failed: Failed to connect", mcpWSHref(e.URL()))}
	}
	cfg, err := mcpServerConfigFromEntry(e)
	if err != nil {
		return mcpHealthResult{Issue: "-32000: MCP error -32000: Connection closed"}
	}
	ctx, cancel := context.WithTimeout(ctx, mcpHealthTimeout)
	defer cancel()

	pool := mcp.NewPool()
	defer func() { _ = pool.Disconnect(name) }()
	if err := pool.Connect(ctx, name, cfg); err != nil {
		return mcpHealthResult{Issue: translateMCPConnectError(err, e)}
	}
	return mcpHealthResult{Connected: true}
}

// mcpServerConfigFromEntry converts a stored entry into a pool ServerConfig.
func mcpServerConfigFromEntry(e config.MCPServerEntry) (mcp.ServerConfig, error) {
	switch e.Type() {
	case "http":
		cfg := mcp.ServerConfig{
			Transport: mcp.TransportHTTP,
			HTTP:      &mcp.HTTPTransportConfig{URL: e.URL(), Headers: envPairMap(e.Headers())},
		}
		setMCPOAuth(cfg.HTTP.OAuth, e)
		return cfg, nil
	case "sse":
		cfg := mcp.ServerConfig{
			Transport: mcp.TransportSSE,
			SSE:       &mcp.SSETransportConfig{URL: e.URL(), Headers: envPairMap(e.Headers())},
		}
		setMCPOAuth(cfg.SSE.OAuth, e)
		return cfg, nil
	case "", "stdio":
		env := map[string]string{}
		for _, p := range e.EnvPairs() {
			env[p.Key] = p.Value
		}
		return mcp.ServerConfig{
			Transport: mcp.TransportStdio,
			Stdio:     &mcp.StdioTransportConfig{Command: e.Command(), Args: e.Args(), Env: env},
		}, nil
	}
	return mcp.ServerConfig{}, fmt.Errorf("mcp: unsupported transport type %q", e.Type())
}

// envPairMap converts ordered pairs into a plain map.
func envPairMap(pairs []config.EnvPair) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[p.Key] = p.Value
	}
	return m
}

// setMCPOAuth copies a stored oauth block into an transport OAuth config.
func setMCPOAuth(_ *mcp.MCPOAuthConfig, _ config.MCPServerEntry) {
	// OAuth handshake during health checks is not implemented; the oracle
	// health-checks without stored credentials too (connect failures surface
	// as the generic MCP error).
}

// translateMCPConnectError maps a Go connection error to the oracle string.
func translateMCPConnectError(err error, e config.MCPServerEntry) string {
	var dnsErr *net.DNSError
	var execErr *exec.Error
	var pathErr *fs.PathError

	switch e.Type() {
	case "http", "sse":
		prefix := ""
		if e.Type() == "sse" {
			prefix = "SSE error: "
		}
		if errors.As(err, &dnsErr) {
			if prefix != "" {
				return fmt.Sprintf("SSE error: getaddrinfo ENOTFOUND %s", dnsErr.Name)
			}
			return fmt.Sprintf("ENOTFOUND: getaddrinfo ENOTFOUND %s", dnsErr.Name)
		}
		if errors.Is(err, syscall.ECONNREFUSED) {
			if prefix != "" {
				return "SSE error: Unable to connect. Is the computer able to access the url?"
			}
			return "ConnectionRefused: Unable to connect. Is the computer able to access the url?"
		}
		return "-32000: MCP error -32000: Connection closed"

	default: // stdio and typeless
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return fmt.Sprintf("ENOENT: Executable not found in $PATH: %q", execErr.Name)
		}
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, syscall.ENOENT) {
			return fmt.Sprintf("ENOENT: ENOENT: no such file or directory, posix_spawn '%s'", e.Command())
		}
		return "-32000: MCP error -32000: Connection closed"
	}
}
