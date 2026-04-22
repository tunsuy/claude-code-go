// Package commands implements the slash command registry and built-in commands
// for the Claude Code TUI.
package commands

import (
	"sort"
	"strings"
)

// ResultDisplay controls how a command result is displayed.
type ResultDisplay int

const (
	// DisplayMessage shows the result as a muted system message in the chat.
	DisplayMessage ResultDisplay = iota
	// DisplayError shows the result as an error.
	DisplayError
	// DisplayDialog shows the result in a modal dialog.
	DisplayDialog
	// DisplayNone suppresses output (command has side effects only).
	DisplayNone
)

// Result is returned by a command's Execute function.
type Result struct {
	// Text is the display text (may be empty for DisplayNone).
	Text string
	// Display controls how the result is rendered.
	Display ResultDisplay
	// ShouldExit signals the TUI to quit.
	ShouldExit bool
	// NewMessages replaces the conversation history (used by /clear, /compact).
	NewMessages interface{} // []types.Message — typed as interface{} to avoid import cycle
	// NewTheme is set by /theme to change the active theme.
	NewTheme string
	// NewModel is set by /model.
	NewModel string
	// NewEffort is set by /effort to change the effort level.
	// Valid values: "low", "medium", "high"
	NewEffort string
	// ToggleVim is set by /vim.
	ToggleVim bool
	// OpenDialog is set to a dialog name to open a specific modal dialog.
	// Recognised values: "compact", "exit", "config".
	OpenDialog string
	// ClearHistory signals the TUI to wipe the conversation history.
	ClearHistory bool
}

// CommandContext carries the read-only application context available to commands.
type CommandContext struct {
	// WorkingDir is the current working directory.
	WorkingDir string
	// Model is the active model identifier.
	Model string
	// SessionID is the current session ID.
	SessionID string
	// DarkMode is true when the TUI is in dark mode.
	DarkMode bool
	// VimEnabled is true when Vim key bindings are enabled.
	VimEnabled bool
	// Effort is the current effort level (low, medium, high).
	Effort string
	// MessageCount is the number of messages in the current conversation.
	MessageCount int
	// TokensInput is the number of input tokens used in the session so far.
	TokensInput int
	// TokensOutput is the number of output tokens used in the session so far.
	TokensOutput int
}

// Command is a single slash command registration.
type Command struct {
	// Name is the command name without the leading slash (e.g. "clear").
	Name string
	// Description is shown in /help and Tab-completion tooltips.
	Description string
	// Execute runs the command and returns a Result.
	// args is the space-trimmed text after the command name.
	Execute func(ctx CommandContext, args string) Result
}

// Registry holds all registered slash commands.
type Registry struct {
	commands map[string]*Command
	order    []string // insertion order for /help display
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]*Command),
	}
}

// Register adds a command to the registry.
// If a command with the same name already exists it is overwritten.
func (r *Registry) Register(cmd *Command) {
	if _, exists := r.commands[cmd.Name]; !exists {
		r.order = append(r.order, cmd.Name)
	}
	r.commands[cmd.Name] = cmd
}

// Lookup returns the command with the given name, or nil if not found.
func (r *Registry) Lookup(name string) *Command {
	return r.commands[name]
}

// All returns all registered commands in insertion order.
func (r *Registry) All() []*Command {
	result := make([]*Command, 0, len(r.order))
	for _, name := range r.order {
		if cmd, ok := r.commands[name]; ok {
			result = append(result, cmd)
		}
	}
	return result
}

// CompletePrefix returns all commands whose name starts with prefix (without '/').
// Results are sorted alphabetically.
func (r *Registry) CompletePrefix(prefix string) []*Command {
	prefix = strings.TrimPrefix(prefix, "/")
	var matches []*Command
	for _, cmd := range r.commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})
	return matches
}
