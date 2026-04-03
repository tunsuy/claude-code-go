module github.com/anthropics/claude-code-go

go 1.21

// External dependencies will be added by each responsible layer agent:
//   Agent-CLI    : github.com/spf13/cobra
//   Agent-TUI    : github.com/charmbracelet/bubbletea, lipgloss, bubbles
//   Agent-Services: github.com/anthropics/anthropic-sdk-go, github.com/mark3labs/mcp-go, golang.org/x/oauth2, github.com/golang-jwt/jwt/v5
// The infra layer (pkg/types, pkg/utils, internal/config, internal/state,
// internal/session, internal/hooks, internal/plugin) uses only the Go standard library.

require (
	github.com/alecthomas/chroma/v2 v2.14.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/charmbracelet/bubbles v0.20.0 // indirect
	github.com/charmbracelet/bubbletea v1.1.0 // indirect
	github.com/charmbracelet/glamour v0.8.0 // indirect
	github.com/charmbracelet/lipgloss v0.13.0 // indirect
	github.com/charmbracelet/x/ansi v0.2.3 // indirect
	github.com/charmbracelet/x/input v0.1.0 // indirect
	github.com/charmbracelet/x/term v0.2.0 // indirect
	github.com/charmbracelet/x/windows v0.1.0 // indirect
	github.com/dlclark/regexp2 v1.11.0 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.15.3-0.20240618155329-98d742f6907a // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yuin/goldmark v1.7.4 // indirect
	github.com/yuin/goldmark-emoji v1.0.3 // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/term v0.22.0 // indirect
	golang.org/x/text v0.16.0 // indirect
)
