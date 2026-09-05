package bootstrap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/config"
	"github.com/tunsuy/claude-code-go/internal/mcp"
)

// Health-check tests.  Every row of the translation table documented in
// mcp_health.go is exercised through the real mcpPoolChecker: the failure rows
// dial real unreachable endpoints (missing commands, loopback port 1, RFC 6761
// .invalid DNS), and the connected row runs a genuine stdio handshake against
// this test binary re-executed as a helper process.
//
// No t.Parallel: the package runs sequentially (TestMain pins HOME).

// mcpHelperEnv is the marker that switches a re-executed test binary into MCP
// helper mode (see helperStdioEntry / TestMCPHelperProcess).
const mcpHelperEnv = "CCG_MCP_HEALTH_HELPER"

// TestMCPHelperProcess is not a test: when the binary is re-executed with
// mcpHelperEnv=1 it serves the MCP stdio handshake — responding to
// `initialize` and returning once `notifications/initialized` arrives — over
// newline-delimited JSON-RPC on stdin/stdout.
func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv(mcpHelperEnv) != "1" {
		return
	}
	in := bufio.NewReader(os.Stdin)
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return
		}
		var msg struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &msg) != nil {
			continue
		}
		switch msg.Method {
		case "initialize":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      msg.ID,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]any{"name": "helper", "version": "1.0.0"},
					"capabilities":    map[string]any{},
				},
			}
			data, err := json.Marshal(resp)
			if err != nil {
				return
			}
			if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
				return
			}
		case "notifications/initialized":
			return
		}
	}
}

// helperStdioEntry builds a stdio entry that re-executes this test binary as
// the MCP helper.  The marker rides in the entry's env map: the transport
// appends entry env to the parent environ, so the re-executed binary sees it.
func helperStdioEntry() *config.OrderedMap {
	e := config.NewOrderedMap()
	e.Set("type", "stdio")
	e.Set("command", os.Args[0])
	e.Set("args", []any{"-test.run=TestMCPHelperProcess", "--"})
	env := config.NewOrderedMap()
	env.Set(mcpHelperEnv, "1")
	e.Set("env", env)
	return e
}

func TestMCPPoolChecker(t *testing.T) {
	cases := []struct {
		name  string
		entry *config.OrderedMap
		want  string // oracle issue string, "" for connected
	}{
		// The one connected row: a real initialize/initialized handshake.
		{"stdio-connected", helperStdioEntry(), ""},
		// stdio, command not on $PATH (no slash in the name).
		{"stdio-not-on-path", rawStdioEntry("definitely-not-a-command-xyz"),
			`ENOENT: Executable not found in $PATH: "definitely-not-a-command-xyz"`},
		// stdio, command path missing (contains a slash).  The oracle wording
		// says posix_spawn (libuv on macOS); Go's fork/exec error maps onto the
		// same string.
		{"stdio-path-missing", rawStdioEntry("./no/such/file"),
			`ENOENT: ENOENT: no such file or directory, posix_spawn './no/such/file'`},
		// Loopback port 1: refused immediately (unprivileged processes cannot
		// listen below 1024).
		{"http-refused", rawURLEntry("http", "http://127.0.0.1:1/mcp"),
			"ConnectionRefused: Unable to connect. Is the computer able to access the url?"},
		{"sse-refused", rawURLEntry("sse", "http://127.0.0.1:1/sse"),
			"SSE error: Unable to connect. Is the computer able to access the url?"},
		// .invalid is reserved by RFC 6761 to never resolve.
		{"http-dns", rawURLEntry("http", "http://ccg-mcp-health.invalid/mcp"),
			"ENOTFOUND: getaddrinfo ENOTFOUND ccg-mcp-health.invalid"},
		{"sse-dns", rawURLEntry("sse", "http://ccg-mcp-health.invalid/sse"),
			"SSE error: getaddrinfo ENOTFOUND ccg-mcp-health.invalid"},
		// ws is never dialed (documented divergence): the fixed failure string
		// regardless of reachability — mcpWSHref adds the trailing slash.
		{"ws-never-dials", rawURLEntry("ws", "ws://127.0.0.1:1"),
			"WebSocket connection to 'ws://127.0.0.1:1/' failed: Failed to connect"},
		// Entries the config converter rejects fold into the generic MCP error.
		{"unknown-type", rawURLEntry("bogus", "x"),
			"-32000: MCP error -32000: Connection closed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := config.MCPServerEntry{Name: tc.name, Raw: tc.entry}
			got := mcpPoolChecker{}.Check(context.Background(), tc.name, e)
			if tc.want == "" {
				if !got.Connected || got.Issue != "" {
					t.Fatalf("Check = %+v, want connected", got)
				}
				return
			}
			if got.Connected || got.Issue != tc.want {
				t.Fatalf("Check = %+v, want issue %q", got, tc.want)
			}
		})
	}
}

func TestTranslateMCPConnectError(t *testing.T) {
	// A real refused dial supplies the exact error chain the transports wrap
	// (url.Error → net.OpError → ECONNREFUSED); the other rows are synthesized
	// with the same shapes the net/os/exec packages produce.
	_, dialErr := net.Dial("tcp", "127.0.0.1:1")
	if dialErr == nil {
		t.Fatal("dial to 127.0.0.1:1 unexpectedly succeeded")
	}
	cases := []struct {
		name  string
		err   error
		entry config.MCPServerEntry
		want  string
	}{
		{"http-dns", &net.DNSError{Err: "no such host", Name: "ccg.invalid"},
			config.MCPServerEntry{Raw: rawURLEntry("http", "http://ccg.invalid/m")},
			"ENOTFOUND: getaddrinfo ENOTFOUND ccg.invalid"},
		{"sse-dns", &net.DNSError{Err: "no such host", Name: "ccg.invalid"},
			config.MCPServerEntry{Raw: rawURLEntry("sse", "http://ccg.invalid/sse")},
			"SSE error: getaddrinfo ENOTFOUND ccg.invalid"},
		{"http-refused",
			fmt.Errorf("mcp: http: post: %w", &url.Error{Op: "Post", URL: "http://127.0.0.1:1/mcp", Err: dialErr}),
			config.MCPServerEntry{Raw: rawURLEntry("http", "http://127.0.0.1:1/mcp")},
			"ConnectionRefused: Unable to connect. Is the computer able to access the url?"},
		{"sse-refused",
			fmt.Errorf("mcp: sse: post: %w", &url.Error{Op: "Post", URL: "http://127.0.0.1:1/message", Err: dialErr}),
			config.MCPServerEntry{Raw: rawURLEntry("sse", "http://127.0.0.1:1/sse")},
			"SSE error: Unable to connect. Is the computer able to access the url?"},
		{"http-generic", errors.New("boom"),
			config.MCPServerEntry{Raw: rawURLEntry("http", "http://h/m")},
			"-32000: MCP error -32000: Connection closed"},
		{"stdio-not-found", &exec.Error{Name: "gone", Err: exec.ErrNotFound},
			config.MCPServerEntry{Raw: rawStdioEntry("gone")},
			`ENOENT: Executable not found in $PATH: "gone"`},
		{"stdio-path-enoent", &fs.PathError{Op: "fork/exec", Path: "./no/such/file", Err: syscall.ENOENT},
			config.MCPServerEntry{Raw: rawStdioEntry("./no/such/file")},
			`ENOENT: ENOENT: no such file or directory, posix_spawn './no/such/file'`},
		{"stdio-generic", errors.New("boom"),
			config.MCPServerEntry{Raw: rawStdioEntry("echo")},
			"-32000: MCP error -32000: Connection closed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := translateMCPConnectError(tc.err, tc.entry); got != tc.want {
				t.Fatalf("translateMCPConnectError = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMCPServerConfigFromEntry(t *testing.T) {
	t.Run("http", func(t *testing.T) {
		e := rawURLEntry("http", "http://h.test/mcp")
		hdrs := config.NewOrderedMap()
		hdrs.Set("Authorization", "Bearer x")
		e.Set("headers", hdrs)
		cfg, err := mcpServerConfigFromEntry(config.MCPServerEntry{Raw: e})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Transport != mcp.TransportHTTP || cfg.HTTP == nil || cfg.HTTP.URL != "http://h.test/mcp" {
			t.Fatalf("cfg = %+v", cfg)
		}
		if cfg.HTTP.Headers["Authorization"] != "Bearer x" {
			t.Fatalf("headers = %v", cfg.HTTP.Headers)
		}
	})
	t.Run("sse", func(t *testing.T) {
		cfg, err := mcpServerConfigFromEntry(config.MCPServerEntry{Raw: rawURLEntry("sse", "http://s.test/sse")})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Transport != mcp.TransportSSE || cfg.SSE == nil || cfg.SSE.URL != "http://s.test/sse" {
			t.Fatalf("cfg = %+v", cfg)
		}
		if cfg.SSE.Headers != nil {
			t.Fatalf("no stored headers must map to nil, got %v", cfg.SSE.Headers)
		}
	})
	t.Run("stdio", func(t *testing.T) {
		e := rawStdioEntry("echo", "hello", "world")
		env := config.NewOrderedMap()
		env.Set("K", "V")
		e.Set("env", env)
		cfg, err := mcpServerConfigFromEntry(config.MCPServerEntry{Raw: e})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Transport != mcp.TransportStdio || cfg.Stdio == nil {
			t.Fatalf("cfg = %+v", cfg)
		}
		if cfg.Stdio.Command != "echo" || !reflect.DeepEqual(cfg.Stdio.Args, []string{"hello", "world"}) {
			t.Fatalf("stdio = %+v", cfg.Stdio)
		}
		if cfg.Stdio.Env["K"] != "V" {
			t.Fatalf("env = %v", cfg.Stdio.Env)
		}
	})
	t.Run("typeless", func(t *testing.T) {
		// Legacy entries without a "type" key connect as stdio.
		e := config.NewOrderedMap()
		e.Set("command", "echo")
		e.Set("args", []any{})
		cfg, err := mcpServerConfigFromEntry(config.MCPServerEntry{Raw: e})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Transport != mcp.TransportStdio || cfg.Stdio == nil || cfg.Stdio.Command != "echo" {
			t.Fatalf("cfg = %+v", cfg)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		_, err := mcpServerConfigFromEntry(config.MCPServerEntry{Raw: rawURLEntry("bogus", "x")})
		if err == nil || err.Error() != `mcp: unsupported transport type "bogus"` {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("env-pair-map-empty", func(t *testing.T) {
		if envPairMap(nil) != nil {
			t.Fatal("empty pairs must map to a nil map")
		}
	})
}
