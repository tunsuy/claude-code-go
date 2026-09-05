package bootstrap

// Oracle-verified help texts for the `claude mcp` command tree (claude v2.1.261,
// non-TTY 80-column rendering).  These are byte-exact literals — including the
// wrapped lines and the two-space continuation indent inside the parent's
// Commands column — and must not be reflowed.
//
// Documented divergence: `mcp login` / `mcp logout` are not implemented in this
// CLI, so their rows are dropped from the parent help (the oracle lists them).

const mcpParentHelp = `Usage: claude mcp [options] [command]

Configure and manage MCP servers

Options:
  -h, --help                            Display help for command

Commands:
  add [options] <name> <commandOrUrl> [args...]  Add an MCP server to Claude Code.
  
  Examples:
    # Add HTTP server:
    claude mcp add --transport http sentry https://mcp.sentry.dev/mcp
  
    # Add HTTP server with headers:
    claude mcp add --transport http corridor https://app.corridor.dev/api/mcp --header "Authorization: Bearer ..."
  
    # Add stdio server with environment variables:
    claude mcp add my-server -e API_KEY=xxx -- npx my-mcp-server
  
    # Add stdio server with subprocess flags:
    claude mcp add my-server -- my-command --some-flag arg1
  add-from-claude-desktop [options]     Import MCP servers from Claude Desktop
                                        (Mac and WSL only)
  add-json [options] <name> <json>      Add an MCP server (stdio, SSE, HTTP, or
                                        WebSocket) with a JSON string
  get <name>                            Get details about an MCP server.
                                        Unapproved .mcp.json servers are shown
                                        as ⏸ Pending approval and not connected
                                        to; approved servers are health-checked
                                        unless disabled for this project.
  help [command]                        display help for command
  list                                  List configured MCP servers. Unapproved
                                        .mcp.json servers are shown as ⏸ Pending
                                        approval and not connected to; approved
                                        servers are health-checked unless
                                        disabled for this project.
  remove [options] <name>               Remove an MCP server
  reset-project-choices                 Reset all approved and rejected
                                        project-scoped (.mcp.json) servers
                                        within this project
  serve [options]                       Start the Claude Code MCP server
`

const mcpAddHelp = `Usage: claude mcp add [options] <name> <commandOrUrl> [args...]

Add an MCP server to Claude Code.

Examples:
  # Add HTTP server:
  claude mcp add --transport http sentry https://mcp.sentry.dev/mcp

  # Add HTTP server with headers:
  claude mcp add --transport http corridor https://app.corridor.dev/api/mcp
--header "Authorization: Bearer ..."

  # Add stdio server with environment variables:
  claude mcp add my-server -e API_KEY=xxx -- npx my-mcp-server

  # Add stdio server with subprocess flags:
  claude mcp add my-server -- my-command --some-flag arg1

Options:
  --callback-port <port>       Fixed port for OAuth callback (for servers
                               requiring pre-registered redirect URIs)
  --client-id <clientId>       OAuth client ID for HTTP/SSE servers
  --client-secret              Prompt for OAuth client secret (or set
                               MCP_CLIENT_SECRET env var)
  -e, --env <env...>           Set environment variables (e.g. -e KEY=value)
  -H, --header <header...>     Set headers for HTTP/SSE servers (e.g. -H
                               "X-Api-Key: abc123" -H "X-Custom: value")
  -h, --help                   Display help for command
  -s, --scope <scope>          Configuration scope (local, user, or project)
                               (default: "local")
  -t, --transport <transport>  Transport type (stdio, sse, http). Defaults to
                               stdio if not specified.
`

const mcpRemoveHelp = `Usage: claude mcp remove [options] <name>

Remove an MCP server

Options:
  -h, --help           Display help for command
  -s, --scope <scope>  Configuration scope (local, user, or project) - if not
                       specified, removes from whichever scope it exists in
`

const mcpListHelp = `Usage: claude mcp list [options]

List configured MCP servers. Unapproved .mcp.json servers are shown as ⏸ Pending
approval and not connected to; approved servers are health-checked unless
disabled for this project.

Options:
  -h, --help  Display help for command
`

const mcpGetHelp = `Usage: claude mcp get [options] <name>

Get details about an MCP server. Unapproved .mcp.json servers are shown as ⏸
Pending approval and not connected to; approved servers are health-checked
unless disabled for this project.

Options:
  -h, --help  Display help for command
`

const mcpAddJSONHelp = `Usage: claude mcp add-json [options] <name> <json>

Add an MCP server (stdio, SSE, HTTP, or WebSocket) with a JSON string

Options:
  --client-secret      Prompt for OAuth client secret (or set MCP_CLIENT_SECRET
                       env var)
  -h, --help           Display help for command
  -s, --scope <scope>  Configuration scope (local, user, or project) (default:
                       "local")
`

const mcpAfcdHelp = `Usage: claude mcp add-from-claude-desktop [options]

Import MCP servers from Claude Desktop (Mac and WSL only)

Options:
  -h, --help           Display help for command
  -s, --scope <scope>  Configuration scope (local, user, or project) (default:
                       "local")
`

const mcpResetHelp = `Usage: claude mcp reset-project-choices [options]

Reset all approved and rejected project-scoped (.mcp.json) servers within this
project

Options:
  -h, --help  Display help for command
`

const mcpServeHelp = `Usage: claude mcp serve [options]

Start the Claude Code MCP server

Options:
  -d, --debug  Enable debug mode
  -h, --help   Display help for command
  --verbose    Override verbose mode setting from config
`
