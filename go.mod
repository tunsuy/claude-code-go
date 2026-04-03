module github.com/anthropics/claude-code-go

go 1.21

// External dependencies will be added by each responsible layer agent:
//   Agent-CLI    : github.com/spf13/cobra
//   Agent-TUI    : github.com/charmbracelet/bubbletea, lipgloss, bubbles
//   Agent-Services: github.com/anthropics/anthropic-sdk-go, github.com/mark3labs/mcp-go, golang.org/x/oauth2, github.com/golang-jwt/jwt/v5
// The infra layer (pkg/types, pkg/utils, internal/config, internal/state,
// internal/session, internal/hooks, internal/plugin) uses only the Go standard library.
