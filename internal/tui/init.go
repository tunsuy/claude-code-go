package tui

import (
	"github.com/anthropics/claude-code-go/internal/commands"
	"github.com/anthropics/claude-code-go/internal/engine"
	"github.com/anthropics/claude-code-go/internal/memdir"
	"github.com/anthropics/claude-code-go/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

// New creates a fully-initialised AppModel and returns it as a tea.Model.
// This is the public constructor for the TUI layer.
func New(
	qe engine.QueryEngine,
	appStore *state.AppStateStore,
	vimEnabled bool,
	dark bool,
) tea.Model {
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)

	m := newAppModel(qe, appStore, vimEnabled, dark, reg)
	return m
}

// Init is the BubbleTea Init method. It returns the initial Cmd batch:
//   - Start the spinner ticker.
//   - Load CLAUDE.md files from the working directory.
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.input.Init(),
		tickCmd(),
		loadMemdirCmd(m.appState.GetState().WorkingDir),
	)
}

// loadMemdirCmd discovers and returns a MemdirLoadedMsg with the found paths.
func loadMemdirCmd(workingDir string) tea.Cmd {
	return func() tea.Msg {
		paths := memdir.DiscoverClaudeMd(workingDir)
		return MemdirLoadedMsg{Paths: paths}
	}
}
