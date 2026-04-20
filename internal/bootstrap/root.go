package bootstrap

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/tunsuy/claude-code-go/internal/tui"
)

// rootFlags groups all flags declared on the root cobra command.
type rootFlags struct {
	// Core execution control
	print          bool
	continueSession bool
	resume         string
	model          string
	outputFormat   string
	maxTurns       int
	maxBudgetUSD   float64

	// Permission control
	dangerouslySkipPermissions bool
	permissionMode             string
	allowedTools               []string
	disallowedTools            []string

	// Context & config
	systemPrompt       string
	appendSystemPrompt string
	addDirs            []string
	mcpConfig          []string
	settings           string
	settingSources     string
	agents             string

	// Session management
	sessionID          string
	sessionName        string
	noSessionPersist   bool
	forkSession        bool

	// Debug & diagnostics
	debug        bool
	debugToStderr bool
	debugFile    string
	verbose      bool
	bare         bool

	// Mode extensions (interactive only)
	worktree     string
	tmux         bool
	effort       string
	thinking     string
	ide          bool
	fallbackModel string
}

// newRootCmd builds and returns the configured cobra root command.
func newRootCmd() *cobra.Command {
	f := &rootFlags{}

	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code — AI coding assistant",
		Long: longDesc(`
Claude Code is an AI-powered coding assistant that runs in your terminal.

Without any sub-command, claude opens an interactive REPL session in the
current directory.  Use -p / --print for non-interactive single-shot mode.
`),
		// SilenceUsage suppresses the usage message on error (cleaner UX).
		SilenceUsage: true,
		// SilenceErrors lets us print our own error format.
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInteractiveOrHeadless(cmd, f, args)
		},
	}

	// ── Core execution control ─────────────────────────────────────────────
	cmd.Flags().BoolVarP(&f.print, "print", "p", false,
		"Non-interactive single-shot output mode (skip TUI)")
	cmd.Flags().BoolVarP(&f.continueSession, "continue", "c", false,
		"Resume the most recent session in the current directory")
	cmd.Flags().StringVarP(&f.resume, "resume", "r", "",
		"Resume a specific session ID (omit value to show picker)")
	cmd.Flags().StringVar(&f.model, "model", "",
		"Override the model for this session (e.g. sonnet, opus)")
	cmd.Flags().StringVar(&f.outputFormat, "output-format", "text",
		"Output format for -p mode: text | json | stream-json")
	cmd.Flags().IntVar(&f.maxTurns, "max-turns", 0,
		"Maximum number of agentic turns (0 = unlimited, -p only)")
	cmd.Flags().Float64Var(&f.maxBudgetUSD, "max-budget-usd", 0,
		"Maximum API spend in USD (0 = unlimited, -p only)")

	// ── Permission control ─────────────────────────────────────────────────
	cmd.Flags().BoolVar(&f.dangerouslySkipPermissions, "dangerously-skip-permissions", false,
		"Skip ALL permission checks (recommended only in sandboxed environments)")
	cmd.Flags().StringVar(&f.permissionMode, "permission-mode", "",
		"Permission mode: default | auto | bypassPermissions | acceptEdits | plan")
	cmd.Flags().StringSliceVar(&f.allowedTools, "allowed-tools", nil,
		"Comma-separated list of tool names / patterns to allow")
	cmd.Flags().StringSliceVar(&f.disallowedTools, "disallowed-tools", nil,
		"Comma-separated list of tool names / patterns to deny")

	// ── Context & config ───────────────────────────────────────────────────
	cmd.Flags().StringVar(&f.systemPrompt, "system-prompt", "",
		"Override the system prompt (replaces default)")
	cmd.Flags().StringVar(&f.appendSystemPrompt, "append-system-prompt", "",
		"Append text to the default system prompt")
	cmd.Flags().StringSliceVar(&f.addDirs, "add-dir", nil,
		"Additional directories tools are allowed to access")
	cmd.Flags().StringSliceVar(&f.mcpConfig, "mcp-config", nil,
		"MCP server config(s): JSON file path or inline JSON string")
	cmd.Flags().StringVar(&f.settings, "settings", "",
		"Extra settings: file path or inline JSON string")
	cmd.Flags().StringVar(&f.settingSources, "setting-sources", "",
		"Restrict settings sources loaded: user,project,local")
	cmd.Flags().StringVar(&f.agents, "agents", "",
		"JSON definition of custom sub-agents")

	// ── Session management ─────────────────────────────────────────────────
	cmd.Flags().StringVar(&f.sessionID, "session-id", "",
		"Specify a session UUID (must be a valid UUID)")
	cmd.Flags().StringVarP(&f.sessionName, "name", "n", "",
		"Display name for the session")
	cmd.Flags().BoolVar(&f.noSessionPersist, "no-session-persistence", false,
		"Disable session persistence to disk (-p only)")
	cmd.Flags().BoolVar(&f.forkSession, "fork-session", false,
		"Create a new session ID when resuming (fork)")

	// ── Debug & diagnostics ────────────────────────────────────────────────
	cmd.Flags().BoolVar(&f.debug, "debug", false,
		"Enable debug logging")
	cmd.Flags().BoolVar(&f.debugToStderr, "debug-to-stderr", false,
		"Write debug output to stderr (hidden flag)")
	_ = cmd.Flags().MarkHidden("debug-to-stderr")
	cmd.Flags().StringVar(&f.debugFile, "debug-file", "",
		"Write debug log to file at this path")
	cmd.Flags().BoolVar(&f.verbose, "verbose", false,
		"Enable verbose output")
	cmd.Flags().BoolVar(&f.bare, "bare", false,
		"Minimal mode: skip hooks, plugins, LSP sync, background prefetch")

	// ── Mode extensions (interactive only) ────────────────────────────────
	cmd.Flags().StringVarP(&f.worktree, "worktree", "w", "",
		"Create a new git worktree for this session")
	cmd.Flags().BoolVar(&f.tmux, "tmux", false,
		"Attach a tmux session to the worktree")
	cmd.Flags().StringVar(&f.effort, "effort", "",
		"Effort level: low | medium | high | max")
	cmd.Flags().StringVar(&f.thinking, "thinking", "",
		"Thinking mode: enabled | adaptive | disabled")
	cmd.Flags().BoolVar(&f.ide, "ide", false,
		"Auto-connect to the IDE plugin")
	cmd.Flags().StringVar(&f.fallbackModel, "fallback-model", "",
		"Fallback model when primary is overloaded (-p only)")

	return cmd
}

// runInteractiveOrHeadless dispatches to the correct run path based on flags.
func runInteractiveOrHeadless(cmd *cobra.Command, f *rootFlags, args []string) error {
	// Build the application container (config → auth → engine → TUI).
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts := ContainerOptions{
		HomeDir:    homeDir,
		WorkingDir: cwd,
		ModelOverride: f.model,
		Verbose:    f.verbose,
		Debug:      f.debug,
	}

	if f.print {
		return runHeadless(f, args, opts)
	}
	return runInteractive(f, opts)
}

// runInteractive starts the full BubbleTea TUI.
func runInteractive(f *rootFlags, opts ContainerOptions) error {
	container, err := BuildContainer(opts)
	if err != nil {
		return fmt.Errorf("bootstrap: build container: %w", err)
	}

	// Apply permission flags to the app state.
	applyPermissionFlags(container, f)

	// Restore session history if --resume or --continue was requested.
	if msgs, err := loadSessionMessages(opts.WorkingDir, f); err != nil {
		return err
	} else if len(msgs) > 0 {
		container.QueryEngine.SetMessages(msgs)
	}

	// Set up graceful shutdown on SIGINT / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		// Let BubbleTea handle its own cleanup; the engine interrupt will
		// propagate via context cancellation.
	}()

	// Build and run the TUI.
	m := tui.New(container.QueryEngine, container.AppStateStore, false, true)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

// runHeadless executes a single query without the TUI and writes the result
// to stdout in the requested format.
func runHeadless(f *rootFlags, extraArgs []string, opts ContainerOptions) error {
	container, err := BuildHeadlessContainer(opts)
	if err != nil {
		return fmt.Errorf("bootstrap: build headless container: %w", err)
	}

	applyPermissionFlags(container, f)

	// Restore session history if --resume or --continue was requested.
	if msgs, err := loadSessionMessages(opts.WorkingDir, f); err != nil {
		return err
	} else if len(msgs) > 0 {
		container.QueryEngine.SetMessages(msgs)
	}

	// Collect the prompt: positional args, then fall back to stdin.
	prompt, err := collectHeadlessPrompt(extraArgs)
	if err != nil {
		return err
	}
	if prompt == "" {
		return fmt.Errorf("no prompt provided: pass text as argument or pipe via stdin")
	}

	// Install SIGINT handler to abort the query.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)

	return headlessRun(container, prompt, f, sigCh)
}
