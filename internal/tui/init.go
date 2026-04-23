package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tunsuy/claude-code-go/internal/commands"
	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/internal/memdir"
	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// New creates a fully-initialised AppModel and returns it as a tea.Model.
// This is the public constructor for the TUI layer.
//
// permAskCh receives permission requests from the engine; if nil, HIL is disabled.
// permRespCh is used to send permission responses back to the engine.
func New(
	qe engine.QueryEngine,
	appStore *state.AppStateStore,
	vimEnabled bool,
	dark bool,
	permAskCh <-chan permissions.AskRequest,
	permRespCh chan<- permissions.AskResponse,
) tea.Model {
	// DEBUG log
	if f, err := os.OpenFile("/tmp/claude-code-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintf(f, "[DEBUG] TUI New: permAskCh=%v, permRespCh=%v\n", permAskCh != nil, permRespCh != nil)
		f.Close()
	}
	
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)

	m := newAppModel(qe, appStore, vimEnabled, dark, reg, permAskCh, permRespCh)
	return m
}

// Init is the BubbleTea Init method. It returns the initial Cmd batch:
//   - Start the spinner ticker.
//   - Load CLAUDE.md files from the working directory.
//   - Start the permission request listener (if HIL is enabled).
func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.input.Init(),
		tickCmd(),
		loadMemdirCmd(m.appState.GetState().WorkingDir),
	}
	// Start listening for permission requests if HIL is enabled.
	if m.permAskCh != nil {
		cmds = append(cmds, listenForPermissionRequest(m.permAskCh, m.permRespCh))
	}
	return tea.Batch(cmds...)
}

// loadMemdirCmd discovers and returns a MemdirLoadedMsg with the found paths.
func loadMemdirCmd(workingDir string) tea.Cmd {
	return func() tea.Msg {
		paths := memdir.DiscoverClaudeMd(workingDir)
		return MemdirLoadedMsg{Paths: paths}
	}
}

// listenForPermissionRequest returns a Cmd that listens for permission requests
// from the engine and converts them to PermissionRequestMsg for the TUI.
func listenForPermissionRequest(
	askCh <-chan permissions.AskRequest,
	respCh chan<- permissions.AskResponse,
) tea.Cmd {
	return func() tea.Msg {
		req, ok := <-askCh
		if !ok {
			// Channel closed, stop listening.
			return nil
		}
		// Convert the permissions.AskRequest to a TUI PermissionRequestMsg.
		// The RespFn callback will send the response back through respCh.
		return PermissionRequestMsg{
			RequestID:   req.ID,
			ToolName:    req.ToolName,
			ToolUseID:   req.ToolUseID,
			Message:     req.Message,
			Input:       string(req.Input),
			ProjectPath: req.ProjectPath,
			RespFn: func(allow bool) {
				decision := tools.PermissionDeny
				if allow {
					decision = tools.PermissionAllow
				}
				respCh <- permissions.AskResponse{
					ID:       req.ID,
					Decision: decision,
				}
			},
		}
	}
}
