package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/tunsuy/claude-code-go/internal/commands"
	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// dialogKind enumerates active modal dialogs.
type dialogKind int

const (
	dialogNone       dialogKind = iota
	dialogPermission            // tool permission request
	dialogCompact               // /compact confirmation
	dialogExit                  // exit confirmation
	dialogConfig                // /config
)

// AppModel is the root BubbleTea Model for the entire TUI application.
// It corresponds to TS REPL.tsx, with all useState/useRef fields unified here.
//
// NOTE: AppModel is passed by value through Update(). The messages slice grows
// over time; pointer fields (permReq, appState) remain stable across copies.
type AppModel struct {
	// --- Session state ---
	sessionID string
	// messages is the full conversation history.
	// NOTE: abortFn may still point to a previous context when a new query
	// starts; startQueryCmd resets it before creating the new context.
	messages  []types.Message
	isLoading bool
	abortFn   context.CancelFunc // nil when idle
	abortCtx  context.Context    // current query context

	// --- UI sub-view state ---
	activeDialog dialogKind
	permReq      *PermissionDialog // non-nil only while dialogPermission is active
	showSpinner  bool
	spinner      SpinnerModel

	// --- Tool result expansion state ---
	// expandedToolResults tracks which tool results should be shown expanded.
	// Key is ToolUseID, value is true if expanded.
	expandedToolResults map[string]bool

	// --- Streaming accumulation ---
	// streamingText accumulates the assistant's text as tokens arrive.
	// When StreamDoneMsg arrives its FinalMessage replaces the last partial message.
	streamingText   string
	streamingHasMsg bool // true if we have appended a partial message

	// --- Scroll & layout ---
	termWidth      int
	termHeight     int
	viewport       viewport.Model
	pinnedToBottom bool

	// --- Slash commands ---
	commandRegistry *commands.Registry
	commandResult   *CommandResultMsg

	// --- Multi-agent / coordinator ---
	coordinatorMode  bool
	coordinatorPanel CoordinatorPanel
	lastTickTime     time.Time

	// --- Input ---
	input InputModel

	// --- Style ---
	theme    Theme
	darkMode bool

	// --- Effort level ---
	// effort controls the speed vs. intelligence tradeoff.
	// Valid values: "low", "medium", "high". Default is "medium".
	effort string

	// --- Status bar ---
	statusBar StatusBar

	// --- Welcome header (shown once on startup) ---
	welcomeHeader WelcomeHeader

	// --- Dependencies (injected via New()) ---
	queryEngine engine.QueryEngine
	appState    *state.AppStateStore

	// --- Active stream channel (for pull-loop) ---
	streamCh <-chan engine.Msg

	// --- CLAUDE.md memory ---
	// memdirPaths holds the discovered CLAUDE.md file paths (set by MemdirLoadedMsg).
	memdirPaths []string
	// memdirPrompt is the concatenated text of all CLAUDE.md files.
	memdirPrompt string

	// --- Markdown renderer cache (P1-C) ---
	// mdRenderer is rebuilt whenever termWidth or darkMode changes.
	mdRenderer      *glamour.TermRenderer
	mdRendererWidth int
	mdRendererDark  bool
}

// newAppModel is the internal constructor; callers use New().
func newAppModel(
	qe engine.QueryEngine,
	appStore *state.AppStateStore,
	vimEnabled bool,
	dark bool,
	reg *commands.Registry,
) AppModel {
	st := appStore.GetState()
	cwd := st.WorkingDir
	model := st.MainLoopModel.ModelID

	t := DefaultDarkTheme
	if !dark {
		t = DefaultLightTheme
	}

	m := AppModel{
		theme:               t,
		darkMode:            dark,
		effort:              "medium", // default effort level
		queryEngine:         qe,
		appState:            appStore,
		commandRegistry:     reg,
		spinner:             newSpinner(),
		input:               NewInput(vimEnabled),
		lastTickTime:        time.Now(),
		expandedToolResults: make(map[string]bool),
		coordinatorPanel: CoordinatorPanel{
			Tasks:     make(map[string]AgentTaskState),
			TaskOrder: []string{},
		},
		statusBar: StatusBar{
			model: model,
			cwd:   cwd,
		},
		welcomeHeader:  NewWelcomeHeader(model, cwd),
		pinnedToBottom: true,
		viewport:       viewport.New(80, 20),
	}
	return m
}

// appendStreamDelta appends a streaming text delta.
func (m AppModel) appendStreamDelta(delta string) AppModel {
	m.streamingText += delta
	return m
}

// inProgressAssistantMessage builds a partial types.Message from the accumulated text.
func (m AppModel) inProgressAssistantMessage() types.Message {
	text := m.streamingText
	return types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: &text},
		},
	}
}

// visibleMessages returns messages to display, appending any in-progress partial.
func (m AppModel) visibleMessages() []types.Message {
	if !m.streamingHasMsg || m.streamingText == "" {
		return m.messages
	}
	result := make([]types.Message, len(m.messages)+1)
	copy(result, m.messages)
	result[len(m.messages)] = m.inProgressAssistantMessage()
	return result
}

// newSystemMessage creates a system-role message with text.
func newSystemMessage(text string) types.Message {
	return types.Message{
		Role: types.RoleAssistant, // displayed as assistant but muted
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr(text)},
		},
	}
}

// newUserMessage creates a user-role message with text.
func newUserMessage(text string) types.Message {
	return types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr(text)},
		},
	}
}

func strPtr(s string) *string { return &s }

// markdownRenderer returns a cached *glamour.TermRenderer, rebuilding it only
// when termWidth or darkMode have changed since the last call (P1-C fix).
func (m *AppModel) markdownRenderer() *glamour.TermRenderer {
	if m.mdRenderer != nil &&
		m.mdRendererWidth == m.termWidth &&
		m.mdRendererDark == m.darkMode {
		return m.mdRenderer
	}
	width := m.termWidth - 2
	if width <= 0 {
		width = 80
	}
	// Use custom style with no padding to avoid interfering with our layout
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(newGlamourStyle(m.darkMode)),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	m.mdRenderer = r
	m.mdRendererWidth = m.termWidth
	m.mdRendererDark = m.darkMode
	return m.mdRenderer
}

// IsSlashCommand returns true if text starts with '/'.
func IsSlashCommand(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/")
}

// parseSlashInput splits "/name args" into name and args.
func parseSlashInput(text string) (name, args string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", text
	}
	text = text[1:] // strip leading /
	if idx := strings.Index(text, " "); idx >= 0 {
		return text[:idx], strings.TrimSpace(text[idx+1:])
	}
	return text, ""
}

// tickInterval is the duration between spinner ticks.
const tickInterval = 100 * time.Millisecond

// tickCmd returns a Cmd that fires after tickInterval.
func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}
