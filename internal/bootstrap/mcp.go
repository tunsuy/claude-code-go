package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/anthropics/claude-code-go/internal/mcp"
	"github.com/anthropics/claude-code-go/internal/tools"
)

// newMCPCmd creates the `claude mcp` subcommand tree.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mcp",
		Short:        "Manage Model Context Protocol (MCP) server integrations",
		SilenceUsage: true,
	}

	cmd.AddCommand(
		newMCPServeCmd(),
		newMCPAddCmd(),
		newMCPRemoveCmd(),
		newMCPListCmd(),
		newMCPGetCmd(),
		newMCPAddJSONCmd(),
		newMCPAddFromClaudeDesktopCmd(),
		newMCPResetProjectChoicesCmd(),
	)
	return cmd
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start Claude as an MCP server on stdin/stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Build a minimal container so we can expose the tool registry.
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("mcp serve: get working directory: %w", err)
			}
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("mcp serve: get home directory: %w", err)
			}
			container, err := BuildContainer(ContainerOptions{
				HomeDir:    homeDir,
				WorkingDir: cwd,
			})
			if err != nil {
				return fmt.Errorf("mcp serve: build container: %w", err)
			}
			return runMCPServe(container)
		},
	}
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

func newMCPAddCmd() *cobra.Command {
	var (
		scope     string
		transport string
		envVars   []string
	)
	cmd := &cobra.Command{
		Use:   "add <name> <command> [args...]",
		Short: "Add an MCP stdio server",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = scope
			_ = transport
			_ = envVars
			return fmt.Errorf("mcp add: not yet implemented")
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "local", "Config scope: local | project | user")
	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport: stdio | sse | http")
	cmd.Flags().StringArrayVar(&envVars, "env", nil, "Environment variables KEY=VALUE")
	return cmd
}

func newMCPRemoveCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an MCP server configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = scope
			return fmt.Errorf("mcp remove: not yet implemented")
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "local", "Config scope: local | project | user")
	return cmd
}

func newMCPListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured MCP servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp list: not yet implemented")
		},
	}
}

func newMCPGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show details for a single MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp get: not yet implemented")
		},
	}
}

func newMCPAddJSONCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "add-json <name> <json>",
		Short: "Add an MCP server from a raw JSON definition",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = scope
			return fmt.Errorf("mcp add-json: not yet implemented")
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "local", "Config scope: local | project | user")
	return cmd
}

func newMCPAddFromClaudeDesktopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-from-claude-desktop",
		Short: "Import MCP servers from the Claude desktop app configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp add-from-claude-desktop: not yet implemented")
		},
	}
}

func newMCPResetProjectChoicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-project-choices",
		Short: "Reset project-level MCP server approval choices",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("mcp reset-project-choices: not yet implemented")
		},
	}
}
