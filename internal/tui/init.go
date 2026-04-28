package tui

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tunsuy/claude-code-go/internal/commands"
	"github.com/tunsuy/claude-code-go/internal/coordinator"
	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/internal/memdir"
	"github.com/tunsuy/claude-code-go/internal/msgqueue"
	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// New creates a fully-initialised AppModel and returns it as a tea.Model.
// This is the public constructor for the TUI layer.
//
// permAskCh receives permission requests from the engine; if nil, HIL is disabled.
// permRespCh is used to send permission responses back to the engine.
// agentCoord is the multi-agent coordinator adapter; if nil, agent tools are disabled.
// agentEventCh receives sub-agent progress/status events; if nil, coordinator panel is disabled.
// mq is the unified command queue for mid-session message processing; may be nil.
// qg is the query dispatch guard; may be nil.
func New(
	qe engine.QueryEngine,
	appStore *state.AppStateStore,
	vimEnabled bool,
	dark bool,
	permAskCh <-chan permissions.AskRequest,
	permRespCh chan<- permissions.AskResponse,
	agentCoord tools.AgentCoordinator,
	agentEventCh <-chan coordinator.Event,
	mq *msgqueue.MessageQueue,
	qg *msgqueue.QueryGuard,
) tea.Model {
	// DEBUG log
	if f, err := os.OpenFile("/tmp/claude-code-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintf(f, "[DEBUG] TUI New: permAskCh=%v, permRespCh=%v, agentCoord=%v, agentEventCh=%v\n",
			permAskCh != nil, permRespCh != nil, agentCoord != nil, agentEventCh != nil)
		f.Close()
	}

	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)

	m := newAppModel(qe, appStore, vimEnabled, dark, reg, permAskCh, permRespCh, agentCoord, agentEventCh, mq, qg)
	return m
}

// Init is the BubbleTea Init method. It returns the initial Cmd batch:
//   - Start the spinner ticker.
//   - Load CLAUDE.md files from the working directory.
//   - Start the permission request listener (if HIL is enabled).
//   - Start the agent event listener (if coordinator is enabled).
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
	// Start listening for agent events if coordinator is enabled.
	if m.agentEventCh != nil {
		cmds = append(cmds, listenForAgentEvent(m.agentEventCh))
	}
	// Start listening for message queue changes if mid-session messaging is enabled.
	if m.msgQueue != nil {
		cmds = append(cmds, listenForQueueChange(m.msgQueue))
	}
	return tea.Batch(cmds...)
}

// loadMemdirCmd discovers and returns a MemdirLoadedMsg with the found paths.
func loadMemdirCmd(workingDir string) tea.Cmd {
	return func() tea.Msg {
		scopedFiles, err := memdir.DiscoverAll(workingDir)
		if err != nil {
			// Fallback to legacy discovery.
			paths := memdir.DiscoverClaudeMd(workingDir)
			return MemdirLoadedMsg{Paths: paths}
		}
		// Extract paths for backward compatibility.
		paths := make([]string, len(scopedFiles))
		for i, f := range scopedFiles {
			paths[i] = f.Path
		}
		return MemdirLoadedMsg{Paths: paths, ScopedFiles: scopedFiles}
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

// listenForAgentEvent returns a Cmd that reads the next coordinator.Event from
// the channel and converts it into an AgentProgressMsg or AgentStatusMsg for
// the BubbleTea update loop. After dispatching one message it re-subscribes
// itself (the Update handler calls listenForAgentEvent again).
func listenForAgentEvent(ch <-chan coordinator.Event) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			// Channel closed, stop listening.
			return nil
		}
		switch evt.Kind {
		case coordinator.EventProgress:
			return AgentProgressMsg{
				TaskID:   evt.AgentID,
				Activity: evt.Activity,
				Detail:   evt.Detail,
			}
		case coordinator.EventStatusChange:
			status := mapCoordinatorStatus(evt.Status)
			return AgentStatusMsg{
				TaskID:      evt.AgentID,
				Status:      status,
				Description: evt.Description,
			}
		default:
			return nil
		}
	}
}

// mapCoordinatorStatus converts a coordinator status string to a TUI AgentStatus.
func mapCoordinatorStatus(s string) AgentStatus {
	switch s {
	case "running":
		return AgentRunning
	case "completed":
		return AgentCompleted
	case "failed":
		return AgentFailed
	case "killed":
		return AgentFailed // treat killed as failed in TUI
	default:
		return AgentRunning
	}
}

// agentEvictDelay is the duration a completed/failed task remains visible
// before being evicted from the coordinator panel.
const agentEvictDelay = 30 * time.Second

// listenForQueueChange subscribes to the message queue and blocks until a
// mutation occurs, then returns queueChangedMsg to the BubbleTea Update loop.
// The Update handler must re-invoke this function after each receive to
// continue listening (same pattern as listenForPermissionRequest).
func listenForQueueChange(q *msgqueue.MessageQueue) tea.Cmd {
	if q == nil {
		return nil
	}
	return func() tea.Msg {
		ch, id := q.Subscribe()
		defer q.Unsubscribe(id)
		_, ok := <-ch
		if !ok {
			// Channel closed (signal teardown) — stop listening.
			return nil
		}
		return queueChangedMsg{}
	}
}
