package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tunsuy/claude-code-go/internal/mcp"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// The `claude mcp` command tree.  The whole tree reproduces the upstream TS
// CLI's commander.js semantics rather than cobra's: every command disables
// flag parsing and receives the raw argv tail, which parseMCPFlags
// (mcp_parse.go) walks with commander's rules (-h wins mid-walk, unknown
// options error on the raw token, greedy -e/-H, `--` switches to
// positionals-only).  The dispatch layer below prints failures itself and
// returns ErrSilent, so main exits 1 without prefixing anything.

// newMCPCmd creates the `claude mcp` subcommand tree.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "mcp",
		Short:              "Manage Model Context Protocol (MCP) server integrations",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPParent(cmd, args)
		},
	}
	cmd.AddCommand(
		newMCPServeCmd(),
		newMCPAddCmd(),
		newMCPAddFromClaudeDesktopCmd(),
		newMCPAddJSONCmd(),
		newMCPGetCmd(),
		newMCPListCmd(),
		newMCPRemoveCmd(),
		newMCPResetProjectChoicesCmd(),
		newMCPHelpCmd(),
	)
	return cmd
}

// mcpDepsBuilder builds the deps bundle from a command's streams.  Swapped in
// tests to pin the home/project directories.
var mcpDepsBuilder = newMCPDepsWith

// mcpDispatchRun wires one subcommand: build deps from the command's streams,
// run fn, then translate a typed failure into the oracle's reporting
// convention (stderr, no "Error:" prefix, silent exit 1).
func mcpDispatchRun(cmd *cobra.Command, args []string, fn func(*mcpDeps, []string) error) error {
	d, err := mcpDepsBuilder(cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if err := fn(d, args); err != nil {
		return mcpReportError(d, err)
	}
	return nil
}

// mcpReportError prints a run failure the way the oracle does and returns
// ErrSilent for a silent exit 1.  Internal (untyped) errors pass through for
// the generic handler.
func mcpReportError(d *mcpDeps, err error) error {
	switch e := err.(type) {
	case *mcpUsageError:
		fmt.Fprintln(d.Stderr, e.Line1)
		if e.Line2 != "" {
			fmt.Fprintln(d.Stderr, e.Line2)
		}
	case *mcpMissingArgError:
		fmt.Fprintln(d.Stderr, e.Error())
	case *errMCPAction:
		fmt.Fprintln(d.Stderr, e.msg)
	case *mcpMiser:
		// Pre-rendered miss message, already newline-terminated.
		fmt.Fprint(d.Stderr, e.msg)
	case *errMCPExit:
		// Diagnostics were already printed; only the exit code matters.
	default:
		return err
	}
	return ErrSilent
}

// mcpCommandNames lists the subcommand names for unknown-command suggestions.
// The oracle also offers login/logout (documented divergence: not implemented
// here, so e.g. `mcp bogus` suggests nothing where it would suggest logout).
var mcpCommandNames = []string{
	"add", "add-from-claude-desktop", "add-json", "get", "help", "list",
	"remove", "reset-project-choices", "serve",
}

// mcpUnknownCommandError builds the commander unknown-command error with its
// "(Did you mean …?)" suggestion line.
func mcpUnknownCommandError(name string) error {
	e := &mcpUsageError{Line1: fmt.Sprintf("error: unknown command '%s'", name)}
	if s := suggestSimilar(name, mcpCommandNames); len(s) > 0 {
		e.Line2 = "(Did you mean " + strings.Join(s, " or ") + "?)"
	}
	return e
}

// mcpSubcommands maps subcommand names to run functions for the parent's
// post-`--` dispatch (cobra matches pre-`--` names itself, since its matcher
// stops at `--`).
var mcpSubcommands = map[string]func(*mcpDeps, []string) error{
	"add":                     runMCPAdd,
	"add-from-claude-desktop": runMCPAddFromClaudeDesktop,
	"add-json":                runMCPAddJSON,
	"get":                     runMCPGet,
	"list":                    runMCPList,
	"remove":                  runMCPRemove,
	"reset-project-choices":   runMCPResetProjectChoices,
	"serve":                   runMCPServeArgs,
	"help":                    runMCPHelpArgs,
}

// runMCPParent handles the `mcp` command itself.  Cobra reaches it only when
// the first raw token is not a known subcommand: bare `mcp` (help to stderr,
// exit 1), `-h` (help to stdout, exit 0), an unknown command (deferred error
// on the first positional — `-h` later in the walk still wins), or tokens
// after `--`, which cobra's matcher skips but the oracle still dispatches
// (capture: `mcp -- add` → add's "missing required argument 'name'").
func runMCPParent(cmd *cobra.Command, args []string) error {
	d, err := mcpDepsBuilder(cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	p, perr := parseMCPFlags(args, mcpParentFlagSpecs)
	if perr != nil {
		return mcpReportError(d, perr)
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpParentHelp)
		return nil
	}
	if len(p.Positionals) > 0 {
		name := p.Positionals[0]
		if fn, ok := mcpSubcommands[name]; ok {
			if err := fn(d, p.Positionals[1:]); err != nil {
				return mcpReportError(d, err)
			}
			return nil
		}
		return mcpReportError(d, mcpUnknownCommandError(name))
	}
	fmt.Fprint(d.Stderr, mcpParentHelp)
	return ErrSilent
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "serve [options]",
		Short:              "Start the Claude Code MCP server",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPServeArgs)
		},
	}
}

// runMCPServeArgs implements `mcp serve`: -h shows help; the debug/verbose
// flags are accepted and ignored (debug logging is not wired up).
func runMCPServeArgs(d *mcpDeps, args []string) error {
	p, err := parseMCPFlags(args, mcpServeFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, mcpServeHelp)
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("mcp serve: get working directory: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("mcp serve: home directory: %w", err)
	}
	container, err := BuildContainer(ContainerOptions{
		HomeDir:    homeDir,
		WorkingDir: cwd,
	})
	if err != nil {
		return fmt.Errorf("mcp serve: build container: %w", err)
	}
	return runMCPServe(container)
}

func newMCPAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "add [options] <name> <commandOrUrl> [args...]",
		Short:              "Add an MCP server to Claude Code.",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPAdd)
		},
	}
}

func newMCPAddFromClaudeDesktopCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "add-from-claude-desktop [options]",
		Short:              "Import MCP servers from Claude Desktop (Mac and WSL only)",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPAddFromClaudeDesktop)
		},
	}
}

func newMCPAddJSONCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "add-json [options] <name> <json>",
		Short:              "Add an MCP server (stdio, SSE, HTTP, or WebSocket) with a JSON string",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPAddJSON)
		},
	}
}

func newMCPGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "get <name>",
		Short:              "Get details about an MCP server.",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPGet)
		},
	}
}

func newMCPListCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "list",
		Short:              "List configured MCP servers",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPList)
		},
	}
}

func newMCPRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "remove [options] <name>",
		Short:              "Remove an MCP server",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPRemove)
		},
	}
}

func newMCPResetProjectChoicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "reset-project-choices",
		Short:              "Reset all approved and rejected project-scoped (.mcp.json) servers within this project",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPResetProjectChoices)
		},
	}
}

// mcpHelpTopics maps `mcp help <topic>` to its help text.  "help" itself is
// deliberately absent: `mcp help help` shows the parent help on stderr
// (oracle-verified).
var mcpHelpTopics = map[string]string{
	"add":                     mcpAddHelp,
	"add-from-claude-desktop": mcpAfcdHelp,
	"add-json":                mcpAddJSONHelp,
	"get":                     mcpGetHelp,
	"list":                    mcpListHelp,
	"remove":                  mcpRemoveHelp,
	"reset-project-choices":   mcpResetHelp,
	"serve":                   mcpServeHelp,
}

func newMCPHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "help [command]",
		Short:              "display help for command",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpDispatchRun(cmd, args, runMCPHelpArgs)
		},
	}
}

// runMCPHelpArgs implements `mcp help [command]`.  Every argument is a topic
// name — "-h" included: `mcp help -h` shows the parent help on stderr and
// exits 1, exactly like `mcp help bogus` (oracle-verified).
func runMCPHelpArgs(d *mcpDeps, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(d.Stdout, mcpParentHelp)
		return nil
	}
	if help, ok := mcpHelpTopics[args[0]]; ok {
		fmt.Fprint(d.Stdout, help)
		return nil
	}
	fmt.Fprint(d.Stderr, mcpParentHelp)
	return ErrSilent
}

// runMCPServe serves Claude's built-in tool registry as an MCP server over stdin/stdout.
func runMCPServe(container *AppContainer) error {
	// P1-D: Set ENTRYPOINT so all downstream components know we are running
	// as an MCP server.  This must happen before any tool or engine code is
	// reached so that entrypoint-specific behaviour (e.g. logging, permission
	// defaults) is applied consistently.
	os.Setenv("CLAUDE_CODE_ENTRYPOINT", "mcp") //nolint:errcheck

	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)
	for {
		var req mcp.JSONRPCMessage
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("mcp serve: decode: %w", err)
		}
		switch req.Method {
		case "initialize":
			resp := &mcp.JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: mustMarshalJSON(map[string]any{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]any{"name": "claude-code-go", "version": appVersion},
					"capabilities":    map[string]any{"tools": map[string]any{}},
				}),
			}
			enc.Encode(resp) //nolint:errcheck

		case "tools/list":
			// P1-E: tools/list — return the registry's full tool list formatted
			// according to the MCP protocol schema.
			toolList := buildMCPToolList(container.ToolRegistry)
			resp := &mcp.JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  mustMarshalJSON(map[string]any{"tools": toolList}),
			}
			enc.Encode(resp) //nolint:errcheck

		case "tools/call":
			// P1-E: tools/call — dispatch to the matching registered tool and
			// return the result (or a structured JSON-RPC error on failure).
			result := handleMCPToolCall(req, container.ToolRegistry)
			enc.Encode(result) //nolint:errcheck

		default:
			resp := &mcp.JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcp.JSONRPCError{Code: -32601, Message: "Method not found: " + req.Method},
			}
			enc.Encode(resp) //nolint:errcheck
		}
	}
}

// mcpToolEntry is the MCP-protocol representation of a single tool.
type mcpToolEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// buildMCPToolList converts the tool registry into the MCP tools/list response payload.
func buildMCPToolList(reg *tools.Registry) []mcpToolEntry {
	all := reg.All()
	entries := make([]mcpToolEntry, 0, len(all))
	for _, t := range all {
		schema := t.InputSchema()
		schemaBytes, _ := json.Marshal(schema)
		entries = append(entries, mcpToolEntry{
			Name:        t.Name(),
			Description: t.Description(nil, nil),
			InputSchema: schemaBytes,
		})
	}
	return entries
}

// handleMCPToolCall dispatches a tools/call request to the registered tool.
func handleMCPToolCall(req mcp.JSONRPCMessage, reg *tools.Registry) *mcp.JSONRPCMessage {
	// Decode the params: { "name": "...", "arguments": {...} }
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &mcp.JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcp.JSONRPCError{Code: -32600, Message: "Invalid params: " + err.Error()},
		}
	}

	t, ok := reg.Get(params.Name)
	if !ok {
		return &mcp.JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcp.JSONRPCError{Code: -32601, Message: "Tool not found: " + params.Name},
		}
	}

	useCtx := &tools.UseContext{
		Ctx:     context.Background(),
		AbortCh: make(chan struct{}),
	}

	result, err := t.Call(params.Arguments, useCtx, nil)
	if err != nil {
		return &mcp.JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcp.JSONRPCError{Code: -32603, Message: "Tool execution error: " + err.Error()},
		}
	}

	// Format the result as an MCP tool_result content block.
	isError := result.IsError
	contentStr := fmt.Sprintf("%v", result.Content)
	if raw, ok := result.Content.(string); ok {
		contentStr = raw
	}

	return &mcp.JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: mustMarshalJSON(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": contentStr},
			},
			"isError": isError,
		}),
	}
}

func mustMarshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
