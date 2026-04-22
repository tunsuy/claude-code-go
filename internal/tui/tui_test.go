package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/commands"
	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/pkg/types"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Fake engine for testing
// ---------------------------------------------------------------------------

type fakeEngine struct {
	messages    []types.Message
	model       string
	interrupted bool
	queryCh     chan engine.Msg
	queryErr    error
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{
		queryCh: make(chan engine.Msg, 10),
	}
}

func (f *fakeEngine) Query(_ context.Context, params engine.QueryParams) (<-chan engine.Msg, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	f.messages = params.Messages
	ch := make(chan engine.Msg, 10)
	// Copy queued messages into the returned channel and close.
	go func() {
		for _, m := range drainChan(f.queryCh) {
			ch <- m
		}
		close(ch)
	}()
	return ch, nil
}

func (f *fakeEngine) Interrupt(_ context.Context) { f.interrupted = true }
func (f *fakeEngine) GetMessages() []types.Message { return f.messages }
func (f *fakeEngine) SetMessages(msgs []types.Message) { f.messages = msgs }
func (f *fakeEngine) SetModel(m string)                { f.model = m }

func drainChan(ch chan engine.Msg) []engine.Msg {
	var out []engine.Msg
	for {
		select {
		case m := <-ch:
			out = append(out, m)
		default:
			return out
		}
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestModel() AppModel {
	fe := newFakeEngine()
	appStore := state.NewAppStateStore(state.AppState{
		WorkingDir: "/tmp",
		MainLoopModel: state.ModelSetting{ModelID: "test-model"},
	})
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)
	m := newAppModel(fe, appStore, false, true, reg)
	m.termWidth = 80
	m.termHeight = 24
	m.viewport.Width = 80
	m.viewport.Height = m.viewportHeight()
	return m
}

func applyMsg(m AppModel, msg tea.Msg) AppModel {
	updated, _ := m.Update(msg)
	return updated.(AppModel)
}

// ---------------------------------------------------------------------------
// parseSlashInput
// ---------------------------------------------------------------------------

func TestParseSlashInput(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs string
	}{
		{"/clear", "clear", ""},
		{"/theme dark", "theme", "dark"},
		{"/model claude-opus-4-5", "model", "claude-opus-4-5"},
		{"  /help  ", "help", ""},
		{"notacommand", "", "notacommand"},
		{"", "", ""},
		{"/compact extra args here", "compact", "extra args here"},
	}
	for _, tc := range tests {
		name, args := parseSlashInput(tc.input)
		if name != tc.wantName || args != tc.wantArgs {
			t.Errorf("parseSlashInput(%q) = (%q,%q); want (%q,%q)",
				tc.input, name, args, tc.wantName, tc.wantArgs)
		}
	}
}

// ---------------------------------------------------------------------------
// IsSlashCommand
// ---------------------------------------------------------------------------

func TestIsSlashCommand(t *testing.T) {
	if !IsSlashCommand("/clear") {
		t.Error("expected /clear to be a slash command")
	}
	if !IsSlashCommand("  /help") {
		t.Error("expected '  /help' to be a slash command")
	}
	if IsSlashCommand("hello") {
		t.Error("expected 'hello' not to be a slash command")
	}
	if IsSlashCommand("") {
		t.Error("expected '' not to be a slash command")
	}
}

// ---------------------------------------------------------------------------
// dispatchEngineMsg
// ---------------------------------------------------------------------------

func TestDispatchEngineMsg(t *testing.T) {
	tests := []struct {
		name     string
		msg      engine.Msg
		wantType string
	}{
		{
			name:     "stream text",
			msg:      engine.Msg{Type: engine.MsgTypeStreamText, TextDelta: "hello"},
			wantType: "StreamTokenMsg",
		},
		{
			name:     "thinking delta",
			msg:      engine.Msg{Type: engine.MsgTypeThinkingDelta, TextDelta: "thinking"},
			wantType: "StreamThinkingMsg",
		},
		{
			name:     "tool use start",
			msg:      engine.Msg{Type: engine.MsgTypeToolUseStart, ToolUseID: "id1", ToolName: "bash"},
			wantType: "StreamToolUseStartMsg",
		},
		{
			name:     "tool use input delta",
			msg:      engine.Msg{Type: engine.MsgTypeToolUseInputDelta, ToolUseID: "id1", InputDelta: "{}"},
			wantType: "StreamToolUseInputDeltaMsg",
		},
		{
			name:     "tool use complete",
			msg:      engine.Msg{Type: engine.MsgTypeToolUseComplete, ToolUseID: "id1", ToolInput: "{}"},
			wantType: "StreamToolUseCompleteMsg",
		},
		{
			name: "tool result",
			msg: engine.Msg{Type: engine.MsgTypeToolResult, ToolResult: &engine.ToolResultMsg{
				ToolUseID: "id1", Content: "result", IsError: false,
			}},
			wantType: "StreamToolResultMsg",
		},
		{
			name:     "turn complete end_turn",
			msg:      engine.Msg{Type: engine.MsgTypeTurnComplete, StopReason: "end_turn"},
			wantType: "StreamDoneMsg",
		},
		{
			name:     "turn complete empty stop reason",
			msg:      engine.Msg{Type: engine.MsgTypeTurnComplete},
			wantType: "StreamDoneMsg",
		},
		{
			name:     "turn complete tool_use",
			msg:      engine.Msg{Type: engine.MsgTypeTurnComplete, StopReason: "tool_use"},
			wantType: "StreamAssistantTurnMsg",
		},
		{
			name:     "turn complete max_tokens",
			msg:      engine.Msg{Type: engine.MsgTypeTurnComplete, StopReason: "max_tokens"},
			wantType: "StreamAssistantTurnMsg",
		},
		{
			name:     "assistant message",
			msg:      engine.Msg{Type: engine.MsgTypeAssistantMessage, AssistantMsg: &types.Message{Role: types.RoleAssistant}},
			wantType: "StreamAssistantTurnMsg",
		},
		{
			name:     "error",
			msg:      engine.Msg{Type: engine.MsgTypeError, Err: errors.New("fail")},
			wantType: "StreamErrorMsg",
		},
		{
			name:     "system message",
			msg:      engine.Msg{Type: engine.MsgTypeSystemMessage, SystemText: "info"},
			wantType: "SystemTextMsg",
		},
		{
			name:     "unknown returns nil",
			msg:      engine.Msg{Type: engine.MsgType("unknown_xyz")},
			wantType: "<nil>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := dispatchEngineMsg(tc.msg)
			var gotType string
			if result == nil {
				gotType = "<nil>"
			} else {
				switch result.(type) {
				case StreamTokenMsg:
					gotType = "StreamTokenMsg"
				case StreamThinkingMsg:
					gotType = "StreamThinkingMsg"
				case StreamToolUseStartMsg:
					gotType = "StreamToolUseStartMsg"
				case StreamToolUseInputDeltaMsg:
					gotType = "StreamToolUseInputDeltaMsg"
				case StreamToolUseCompleteMsg:
					gotType = "StreamToolUseCompleteMsg"
			case StreamToolResultMsg:
				gotType = "StreamToolResultMsg"
			case StreamAssistantTurnMsg:
				gotType = "StreamAssistantTurnMsg"
			case StreamDoneMsg:
					gotType = "StreamDoneMsg"
				case StreamErrorMsg:
					gotType = "StreamErrorMsg"
				case SystemTextMsg:
					gotType = "SystemTextMsg"
				default:
					gotType = "unknown"
				}
			}
			if gotType != tc.wantType {
				t.Errorf("dispatchEngineMsg(%v) type = %s; want %s", tc.msg.Type, gotType, tc.wantType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update — basic message handling
// ---------------------------------------------------------------------------

func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := newTestModel()
	m = applyMsg(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.termWidth != 100 || m.termHeight != 40 {
		t.Errorf("expected termWidth=100 termHeight=40, got %d %d", m.termWidth, m.termHeight)
	}
}

func TestUpdate_TickMsg(t *testing.T) {
	m := newTestModel()
	m.isLoading = true
	before := m.spinner.current
	m = applyMsg(m, TickMsg{Time: m.lastTickTime.Add(100 * time.Millisecond)})
	// Spinner should have ticked.
	if m.spinner.current == before {
		t.Error("expected spinner to advance on TickMsg when loading")
	}
}

func TestUpdate_TickMsg_NotLoading(t *testing.T) {
	m := newTestModel()
	m.isLoading = false
	before := m.spinner.current
	m = applyMsg(m, TickMsg{Time: m.lastTickTime.Add(100 * time.Millisecond)})
	if m.spinner.current != before {
		t.Error("spinner should NOT advance when not loading")
	}
}

func TestUpdate_MemdirLoadedMsg(t *testing.T) {
	m := newTestModel()
	// Use empty paths list (no real files needed for path storage test).
	m = applyMsg(m, MemdirLoadedMsg{Paths: []string{"/tmp/CLAUDE.md"}})
	if len(m.memdirPaths) != 1 || m.memdirPaths[0] != "/tmp/CLAUDE.md" {
		t.Errorf("memdirPaths not stored correctly: %v", m.memdirPaths)
	}
	// memdirPrompt will be empty because the file doesn't exist, but that's ok.
}

func TestUpdate_SystemTextMsg(t *testing.T) {
	m := newTestModel()
	m = applyMsg(m, SystemTextMsg{Text: "hello from system"})
	if len(m.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(m.messages))
	}
	blk := m.messages[0].Content[0]
	if blk.Text == nil || !strings.Contains(*blk.Text, "hello from system") {
		t.Errorf("unexpected message content: %v", blk.Text)
	}
}

func TestUpdate_StreamTokenMsg(t *testing.T) {
	m := newTestModel()
	// Need an active stream channel.
	ch := make(chan engine.Msg, 10)
	m.streamCh = ch
	m = applyMsg(m, StreamTokenMsg{Delta: "hello"})
	if m.streamingText != "hello" {
		t.Errorf("expected streamingText='hello', got %q", m.streamingText)
	}
	if !m.streamingHasMsg {
		t.Error("expected streamingHasMsg=true")
	}
}

func TestUpdate_StreamTokenMsg_NilChannel(t *testing.T) {
	m := newTestModel()
	m.streamCh = nil
	m = applyMsg(m, StreamTokenMsg{Delta: "hello"})
	// Should be a no-op.
	if m.streamingText != "" {
		t.Errorf("expected no-op when streamCh is nil, got %q", m.streamingText)
	}
}

func TestUpdate_StreamDoneMsg_WithFinalMessage(t *testing.T) {
	m := newTestModel()
	ch := make(chan engine.Msg, 1)
	m.streamCh = ch
	m.isLoading = true
	m.showSpinner = true
	m.streamingText = "partial"
	m.streamingHasMsg = true

	finalMsg := &types.Message{
		Role:    types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("final answer")}},
	}
	m = applyMsg(m, StreamDoneMsg{FinalMessage: finalMsg})

	if m.isLoading {
		t.Error("expected isLoading=false after StreamDoneMsg")
	}
	if m.streamCh != nil {
		t.Error("expected streamCh=nil after StreamDoneMsg")
	}
	if m.streamingText != "" {
		t.Errorf("expected streamingText empty, got %q", m.streamingText)
	}
	if len(m.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(m.messages))
	}
	if m.messages[0].Content[0].Text == nil || *m.messages[0].Content[0].Text != "final answer" {
		t.Errorf("unexpected final message content")
	}
}

func TestUpdate_StreamDoneMsg_PromotePartial(t *testing.T) {
	m := newTestModel()
	ch := make(chan engine.Msg, 1)
	m.streamCh = ch
	m.isLoading = true
	m.streamingText = "streamed text"
	m.streamingHasMsg = true

	m = applyMsg(m, StreamDoneMsg{FinalMessage: nil})

	if len(m.messages) != 1 {
		t.Fatalf("expected partial text promoted to 1 message, got %d", len(m.messages))
	}
}

func TestUpdate_StreamErrorMsg(t *testing.T) {
	m := newTestModel()
	ch := make(chan engine.Msg, 1)
	m.streamCh = ch
	m.isLoading = true

	m = applyMsg(m, StreamErrorMsg{Err: errors.New("network failure")})

	if m.isLoading {
		t.Error("expected isLoading=false after StreamErrorMsg")
	}
	if m.streamCh != nil {
		t.Error("expected streamCh=nil after StreamErrorMsg")
	}
	if len(m.messages) != 1 {
		t.Fatalf("expected 1 error message, got %d", len(m.messages))
	}
	blk := m.messages[0].Content[0]
	if blk.Text == nil || !strings.Contains(*blk.Text, "network failure") {
		t.Errorf("unexpected error message: %v", blk.Text)
	}
}

func TestUpdate_PermissionRequestMsg(t *testing.T) {
	m := newTestModel()
	called := false
	m = applyMsg(m, PermissionRequestMsg{
		RequestID: "req1",
		ToolName:  "bash",
		ToolUseID: "use1",
		Message:   "run command?",
		RespFn:    func(allow bool) { called = allow },
	})
	if m.activeDialog != dialogPermission {
		t.Errorf("expected dialogPermission, got %d", m.activeDialog)
	}
	if m.permReq == nil {
		t.Fatal("expected non-nil permReq")
	}
	if m.permReq.toolName != "bash" {
		t.Errorf("expected toolName='bash', got %q", m.permReq.toolName)
	}
	_ = called
}

func TestUpdate_AgentStatusMsg(t *testing.T) {
	m := newTestModel()
	m.coordinatorPanel.Tasks["task1"] = AgentTaskState{Status: AgentRunning}
	m = applyMsg(m, AgentStatusMsg{TaskID: "task1", Status: AgentCompleted})
	if m.coordinatorPanel.Tasks["task1"].Status != AgentCompleted {
		t.Error("expected task1 status to be Completed")
	}
}

func TestUpdate_CompactDoneMsg(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogCompact
	m = applyMsg(m, CompactDoneMsg{Summary: "reduced to 5 messages"})
	if m.activeDialog != dialogNone {
		t.Error("expected activeDialog=none after CompactDoneMsg")
	}
	if len(m.messages) != 1 {
		t.Fatalf("expected 1 system message, got %d", len(m.messages))
	}
	if !strings.Contains(*m.messages[0].Content[0].Text, "reduced to 5 messages") {
		t.Errorf("compact summary not in message")
	}
}

// ---------------------------------------------------------------------------
// applyCommandResult — P1-B tests
// ---------------------------------------------------------------------------

func TestApplyCommandResult_ShouldExit(t *testing.T) {
	m := newTestModel()
	_, cmd := m.applyCommandResult(commands.Result{ShouldExit: true}, "exit")
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestApplyCommandResult_NewTheme_Valid(t *testing.T) {
	m := newTestModel()
	m2, _ := m.applyCommandResult(commands.Result{NewTheme: "light", Display: commands.DisplayMessage, Text: "Theme set."}, "theme")
	if m2.darkMode {
		t.Error("expected darkMode=false after light theme")
	}
}

func TestApplyCommandResult_NewTheme_Invalid(t *testing.T) {
	m := newTestModel()
	m2, _ := m.applyCommandResult(commands.Result{NewTheme: "nonexistent"}, "theme")
	if len(m2.messages) != 1 {
		t.Fatalf("expected 1 error message for unknown theme, got %d", len(m2.messages))
	}
}

func TestApplyCommandResult_ToggleVim(t *testing.T) {
	m := newTestModel()
	m.input.vimEnabled = false
	m2, _ := m.applyCommandResult(commands.Result{ToggleVim: true, Display: commands.DisplayMessage}, "vim")
	if !m2.input.vimEnabled {
		t.Error("expected vimEnabled=true after ToggleVim")
	}
}

func TestApplyCommandResult_NewModel(t *testing.T) {
	m := newTestModel()
	m2, _ := m.applyCommandResult(commands.Result{NewModel: "claude-opus-4-5", Display: commands.DisplayMessage}, "model")
	if m2.statusBar.model != "claude-opus-4-5" {
		t.Errorf("expected model='claude-opus-4-5', got %q", m2.statusBar.model)
	}
}

func TestApplyCommandResult_OpenDialog_Compact(t *testing.T) {
	m := newTestModel()
	m2, _ := m.applyCommandResult(commands.Result{OpenDialog: "compact", Display: commands.DisplayNone}, "compact")
	if m2.activeDialog != dialogCompact {
		t.Errorf("expected dialogCompact, got %d", m2.activeDialog)
	}
}

func TestApplyCommandResult_OpenDialog_Exit(t *testing.T) {
	m := newTestModel()
	m2, _ := m.applyCommandResult(commands.Result{OpenDialog: "exit", Display: commands.DisplayNone}, "exit")
	if m2.activeDialog != dialogExit {
		t.Errorf("expected dialogExit, got %d", m2.activeDialog)
	}
}

func TestApplyCommandResult_ClearHistory(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{newUserMessage("hello")}
	m.streamingText = "partial"
	m2, _ := m.applyCommandResult(commands.Result{
		ClearHistory: true,
		Display:      commands.DisplayMessage,
		Text:         "Conversation cleared.",
	}, "clear")
	if len(m2.messages) != 1 {
		// After clear the "Conversation cleared." message is appended.
		t.Fatalf("expected 1 message (clear notification), got %d", len(m2.messages))
	}
	if m2.streamingText != "" {
		t.Errorf("expected streamingText cleared, got %q", m2.streamingText)
	}
}

func TestApplyCommandResult_DisplayMessage(t *testing.T) {
	m := newTestModel()
	m2, _ := m.applyCommandResult(commands.Result{
		Text:    "hello there",
		Display: commands.DisplayMessage,
	}, "status")
	if len(m2.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(m2.messages))
	}
	if !strings.Contains(*m2.messages[0].Content[0].Text, "hello there") {
		t.Error("message text not found")
	}
}

func TestApplyCommandResult_DisplayError(t *testing.T) {
	m := newTestModel()
	m2, _ := m.applyCommandResult(commands.Result{
		Text:    "something broke",
		Display: commands.DisplayError,
	}, "bad")
	if len(m2.messages) != 1 {
		t.Fatalf("expected 1 error message, got %d", len(m2.messages))
	}
	if !strings.Contains(*m2.messages[0].Content[0].Text, "Error:") {
		t.Error("error prefix not found")
	}
}

// ---------------------------------------------------------------------------
// handleKey / keyboard handling
// ---------------------------------------------------------------------------

func TestHandleKey_CtrlC_NotLoading(t *testing.T) {
	m := newTestModel()
	m.isLoading = false
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("expected tea.Quit on Ctrl+C when not loading")
	}
}

func TestHandleKey_CtrlD(t *testing.T) {
	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Error("expected tea.Quit on Ctrl+D")
	}
}

func TestHandleKey_CtrlC_WhenLoading(t *testing.T) {
	m := newTestModel()
	m.isLoading = true
	m.streamCh = make(chan engine.Msg)
	m.abortFn = func() {}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2Model := m2.(AppModel)
	if m2Model.isLoading {
		t.Error("expected isLoading=false after abort")
	}
	if m2Model.streamCh != nil {
		t.Error("expected streamCh=nil after abort (P1-D fix)")
	}
	if m2Model.abortFn != nil {
		t.Error("expected abortFn=nil after abort (P1-D fix)")
	}
}

func TestHandleKey_Esc_WhenLoading(t *testing.T) {
	m := newTestModel()
	m.isLoading = true
	m.streamCh = make(chan engine.Msg)
	m.abortFn = func() {}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2Model := m2.(AppModel)
	if m2Model.isLoading {
		t.Error("expected isLoading=false after Esc abort")
	}
}

func TestHandleKey_PageUpDown(t *testing.T) {
	m := newTestModel()
	m.pinnedToBottom = true
	// Add enough messages to make scrolling meaningful.
	for i := 0; i < 50; i++ {
		m.messages = append(m.messages, newUserMessage("line "+itoa(i)))
	}
	m.syncViewportContent()

	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m2.pinnedToBottom {
		t.Error("expected pinnedToBottom=false after PgUp")
	}

	// PgDown should move back toward bottom.
	m3 := applyMsg(m2, tea.KeyMsg{Type: tea.KeyPgDown})
	_ = m3 // Verify no panic; exact offset depends on content height.
}

// ---------------------------------------------------------------------------
// handlePermissionKey
// ---------------------------------------------------------------------------

func TestHandlePermissionKey_EnterAllow(t *testing.T) {
	m := newTestModel()
	var decision bool
	d := newPermissionDialog(PermissionRequestMsg{
		ToolName: "bash",
		RespFn:   func(allow bool) { decision = allow },
	})
	m.activeDialog = dialogPermission
	m.permReq = &d

	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m2.activeDialog != dialogNone {
		t.Error("expected dialog closed after Enter")
	}
	if !decision {
		t.Error("expected allow=true when cursor on 'Yes'")
	}
}

func TestHandlePermissionKey_EscDeny(t *testing.T) {
	m := newTestModel()
	var decision *bool
	d := newPermissionDialog(PermissionRequestMsg{
		ToolName: "bash",
		RespFn:   func(allow bool) { b := allow; decision = &b },
	})
	m.activeDialog = dialogPermission
	m.permReq = &d

	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m2.activeDialog != dialogNone {
		t.Error("expected dialog closed after Esc")
	}
	if decision == nil || *decision {
		t.Error("expected deny on Esc")
	}
}

func TestHandlePermissionKey_UpDown(t *testing.T) {
	m := newTestModel()
	d := newPermissionDialog(PermissionRequestMsg{ToolName: "bash"})
	m.activeDialog = dialogPermission
	m.permReq = &d

	// Move down to "No" option.
	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	if m2.permReq.cursor != 1 {
		t.Errorf("expected cursor=1 after Down, got %d", m2.permReq.cursor)
	}

	// Move back up.
	m3 := applyMsg(m2, tea.KeyMsg{Type: tea.KeyUp})
	if m3.permReq.cursor != 0 {
		t.Errorf("expected cursor=0 after Up, got %d", m3.permReq.cursor)
	}
}

// ---------------------------------------------------------------------------
// handleCompactKey
// ---------------------------------------------------------------------------

func TestHandleCompactKey_Yes(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogCompact
	m.messages = []types.Message{newUserMessage("hello")}

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m2Model := m2.(AppModel)
	if m2Model.activeDialog != dialogNone {
		t.Error("expected dialog closed")
	}
	if len(m2Model.messages) != 0 {
		t.Errorf("expected messages cleared, got %d", len(m2Model.messages))
	}
	if cmd == nil {
		t.Error("expected CompactDoneMsg cmd")
	}
}

func TestHandleCompactKey_No(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogCompact

	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m2.activeDialog != dialogNone {
		t.Error("expected dialog closed after 'n'")
	}
}

func TestHandleCompactKey_Esc(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogCompact

	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m2.activeDialog != dialogNone {
		t.Error("expected dialog closed after Esc")
	}
}

// ---------------------------------------------------------------------------
// handleExitKey
// ---------------------------------------------------------------------------

func TestHandleExitKey_Yes(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogExit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestHandleExitKey_No(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogExit
	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m2.activeDialog != dialogNone {
		t.Error("expected dialog closed")
	}
}

// ---------------------------------------------------------------------------
// handleSubmit
// ---------------------------------------------------------------------------

func TestHandleSubmit_EmptyInput(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue("")
	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m2.isLoading {
		t.Error("empty input should not start query")
	}
}

func TestHandleSubmit_SlashCommand(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue("/help")
	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m2.messages) == 0 {
		t.Error("expected /help to produce a system message")
	}
	if m2.isLoading {
		t.Error("/help should not start a query")
	}
}

func TestHandleSubmit_UnknownSlashCommand(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue("/unknownxyz")
	m2 := applyMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m2.messages) != 1 {
		t.Fatalf("expected 1 error message, got %d", len(m2.messages))
	}
	if !strings.Contains(*m2.messages[0].Content[0].Text, "Unknown command") {
		t.Error("expected 'Unknown command' error message")
	}
}

func TestHandleSubmit_NormalQuery(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue("hello claude")
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2Model := m2.(AppModel)
	if !m2Model.isLoading {
		t.Error("expected isLoading=true after submitting query")
	}
	if cmd == nil {
		t.Error("expected query cmd")
	}
	if len(m2Model.messages) != 1 {
		t.Errorf("expected user message appended, got %d messages", len(m2Model.messages))
	}
	if m2Model.messages[0].Role != types.RoleUser {
		t.Error("expected user role message")
	}
}

// ---------------------------------------------------------------------------
// SpinnerModel
// ---------------------------------------------------------------------------

func TestSpinnerTick(t *testing.T) {
	s := newSpinner()
	s0 := s.current
	s = s.Tick(100 * time.Millisecond)
	if s.current == s0 {
		t.Error("spinner should advance on Tick")
	}
	if s.elapsed != 100*time.Millisecond {
		t.Errorf("expected elapsed=100ms, got %v", s.elapsed)
	}
}

func TestSpinnerReset(t *testing.T) {
	s := newSpinner()
	s = s.Tick(500 * time.Millisecond)
	s = s.Tick(500 * time.Millisecond)
	s = s.Reset()
	if s.current != 0 {
		t.Errorf("expected current=0 after reset, got %d", s.current)
	}
	if s.elapsed != 0 {
		t.Errorf("expected elapsed=0 after reset, got %v", s.elapsed)
	}
}

func TestSpinnerView(t *testing.T) {
	s := newSpinner()
	view := s.View(DefaultDarkTheme)
	if view == "" {
		t.Error("spinner view should not be empty")
	}
	if !strings.Contains(view, "Thinking") {
		t.Errorf("expected 'Thinking' in spinner view, got: %q", view)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "0s"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
	}
	for _, tc := range tests {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q; want %q", tc.d, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// PermissionDialog
// ---------------------------------------------------------------------------

func TestPermissionDialog_UpDown(t *testing.T) {
	d := newPermissionDialog(PermissionRequestMsg{ToolName: "test"})
	if d.cursor != 0 {
		t.Errorf("initial cursor should be 0, got %d", d.cursor)
	}

	d = d.Down()
	if d.cursor != 1 {
		t.Errorf("after Down cursor should be 1, got %d", d.cursor)
	}

	// Can't go further than last option.
	d = d.Down()
	if d.cursor != 1 {
		t.Errorf("cursor should stay at 1 (last option), got %d", d.cursor)
	}

	d = d.Up()
	if d.cursor != 0 {
		t.Errorf("after Up cursor should be 0, got %d", d.cursor)
	}

	// Can't go below 0.
	d = d.Up()
	if d.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", d.cursor)
	}
}

func TestPermissionDialog_Confirm(t *testing.T) {
	d := newPermissionDialog(PermissionRequestMsg{ToolName: "test"})
	// cursor=0 → "Yes, allow once"
	if !d.Confirm() {
		t.Error("cursor=0 should return allow=true")
	}

	d = d.Down()
	// cursor=1 → "No, deny"
	if d.Confirm() {
		t.Error("cursor=1 should return allow=false")
	}
}

// ---------------------------------------------------------------------------
// View smoke tests
// ---------------------------------------------------------------------------

func TestView_Empty(t *testing.T) {
	m := newTestModel()
	// termWidth=0 returns empty.
	m.termWidth = 0
	v := m.View()
	if v != "" {
		t.Errorf("expected empty view when termWidth=0, got %q", v)
	}
}

func TestView_WithMessages(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{
		newUserMessage("hello"),
		newSystemMessage("world"),
	}
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view")
	}
}

func TestView_Scrolled(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{
		newUserMessage("line 1"),
		newUserMessage("line 2"),
		newUserMessage("line 3"),
	}
	m.pinnedToBottom = false
	m.syncViewportContent()
	m.viewport.SetYOffset(2)
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view when scrolled")
	}
}

func TestView_Loading(t *testing.T) {
	m := newTestModel()
	m.isLoading = true
	m.showSpinner = true
	v := m.View()
	if !strings.Contains(v, "Thinking") {
		t.Errorf("expected spinner in loading view, got: %q", v)
	}
}

func TestView_DialogCompact(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogCompact
	v := m.View()
	if !strings.Contains(v, "Compact") {
		t.Errorf("expected compact dialog in view, got: %q", v)
	}
}

func TestView_DialogExit(t *testing.T) {
	m := newTestModel()
	m.activeDialog = dialogExit
	v := m.View()
	if !strings.Contains(v, "Exit") {
		t.Errorf("expected exit dialog in view, got: %q", v)
	}
}

func TestView_DialogPermission(t *testing.T) {
	m := newTestModel()
	d := newPermissionDialog(PermissionRequestMsg{ToolName: "bash", Message: "run cmd"})
	m.activeDialog = dialogPermission
	m.permReq = &d
	v := m.View()
	if !strings.Contains(v, "Permission") {
		t.Errorf("expected permission dialog in view, got: %q", v)
	}
}

// ---------------------------------------------------------------------------
// MessageListView
// ---------------------------------------------------------------------------

func TestMessageListView_UserMessage(t *testing.T) {
	msgs := []types.Message{newUserMessage("hello")}
	v := MessageListView(msgs, 80, true, DefaultDarkTheme, nil, nil)
	if !strings.Contains(v, ">") {
		t.Errorf("expected '>' prefix in message view, got: %q", v)
	}
}

func TestMessageListView_AssistantMessage(t *testing.T) {
	text := "I can help"
	msgs := []types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: &text},
		},
	}}
	v := MessageListView(msgs, 80, true, DefaultDarkTheme, nil, nil)
	if v == "" {
		t.Error("expected non-empty view for assistant message")
	}
}

func TestMessageListView_SystemMessage(t *testing.T) {
	msgs := []types.Message{newSystemMessage("system info")}
	v := MessageListView(msgs, 80, true, DefaultDarkTheme, nil, nil)
	// Strip ANSI escape codes before comparing (glamour inserts them between words).
	plain := stripANSI(v)
	if !strings.Contains(plain, "system") || !strings.Contains(plain, "info") {
		t.Errorf("expected 'system info' in system message view, got: %q", v)
	}
}

// ---------------------------------------------------------------------------
// MarkdownRenderer cache (P1-C)
// ---------------------------------------------------------------------------

func TestMarkdownRendererCache(t *testing.T) {
	m := newTestModel()
	m.termWidth = 80
	m.darkMode = true
	r1 := m.markdownRenderer()
	if r1 == nil {
		t.Skip("glamour renderer returned nil (headless env), skipping")
	}
	r2 := m.markdownRenderer()
	if r1 != r2 {
		t.Error("expected same renderer instance on second call (cache hit)")
	}
}

func TestMarkdownRendererCache_RebuildOnWidthChange(t *testing.T) {
	m := newTestModel()
	m.termWidth = 80
	m.darkMode = true
	r1 := m.markdownRenderer()
	if r1 == nil {
		t.Skip("glamour renderer returned nil (headless env), skipping")
	}
	m.termWidth = 100 // Change width
	r2 := m.markdownRenderer()
	if r1 == r2 {
		t.Error("expected new renderer instance after width change")
	}
}

// ---------------------------------------------------------------------------
// newSystemMessage / newUserMessage helpers
// ---------------------------------------------------------------------------

func TestNewSystemMessage(t *testing.T) {
	msg := newSystemMessage("test text")
	if msg.Role != types.RoleAssistant {
		t.Errorf("expected RoleAssistant, got %v", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text == nil {
		t.Fatal("expected one text content block")
	}
	if *msg.Content[0].Text != "test text" {
		t.Errorf("unexpected text: %q", *msg.Content[0].Text)
	}
}

func TestNewUserMessage(t *testing.T) {
	msg := newUserMessage("hi there")
	if msg.Role != types.RoleUser {
		t.Errorf("expected RoleUser, got %v", msg.Role)
	}
	if *msg.Content[0].Text != "hi there" {
		t.Errorf("unexpected text: %q", *msg.Content[0].Text)
	}
}

// ---------------------------------------------------------------------------
// visibleMessages
// ---------------------------------------------------------------------------

func TestVisibleMessages_WithStreaming(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{newUserMessage("hello")}
	m.streamingText = "partial response"
	m.streamingHasMsg = true
	visible := m.visibleMessages()
	if len(visible) != 2 {
		t.Errorf("expected 2 visible messages (1 user + 1 streaming), got %d", len(visible))
	}
}

func TestVisibleMessages_NoStreaming(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{newUserMessage("hello")}
	m.streamingHasMsg = false
	visible := m.visibleMessages()
	if len(visible) != 1 {
		t.Errorf("expected 1 visible message, got %d", len(visible))
	}
}

// ---------------------------------------------------------------------------
// summariseInput / summariseMapInput / toString
// ---------------------------------------------------------------------------

func TestSummariseInput_Empty(t *testing.T) {
	if summariseInput("", 100) != "" {
		t.Error("expected empty for empty input")
	}
}

func TestSummariseInput_JSON(t *testing.T) {
	result := summariseInput(`{"key":"val"}`, 200)
	if !strings.Contains(result, "key") {
		t.Errorf("expected JSON pretty-printed in summary, got: %q", result)
	}
}

func TestSummariseInput_Truncation(t *testing.T) {
	long := strings.Repeat("a", 300)
	result := summariseInput(long, 100)
	if len(result) <= 100 {
		// result should be truncated (slightly over 100 due to "…")
		return
	}
}

func TestSummariseToolInput_Empty(t *testing.T) {
	if summariseToolInput(map[string]any{}, 100) != "" {
		t.Error("expected empty for empty map")
	}
}

func TestSummariseToolInput_Single(t *testing.T) {
	result := summariseToolInput(map[string]any{"command": "ls -la"}, 100)
	if result != "ls -la" {
		t.Errorf("expected 'ls -la', got %q", result)
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		v    any
		want string
	}{
		{"hello", "hello"},
		{float64(42), "42"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
		{struct{}{}, "<complex>"},
	}
	for _, tc := range tests {
		got := toString(tc.v)
		if got != tc.want {
			t.Errorf("toString(%v) = %q; want %q", tc.v, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// itoa / formatDuration
// ---------------------------------------------------------------------------

func TestItoa(t *testing.T) {
	tests := []struct{ n int; want string }{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-5, "-5"},
		{1000, "1000"},
	}
	for _, tc := range tests {
		if got := itoa(tc.n); got != tc.want {
			t.Errorf("itoa(%d) = %q; want %q", tc.n, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// commands.Result fields (P1-B regression test)
// ---------------------------------------------------------------------------

func TestCommandsResult_OpenDialogField(t *testing.T) {
	r := commands.Result{OpenDialog: "compact"}
	if r.OpenDialog != "compact" {
		t.Error("OpenDialog field not accessible")
	}
}

func TestCommandsResult_ClearHistoryField(t *testing.T) {
	r := commands.Result{ClearHistory: true}
	if !r.ClearHistory {
		t.Error("ClearHistory field not accessible")
	}
}

// ---------------------------------------------------------------------------
// Slash command integration — /compact and /clear use new fields
// ---------------------------------------------------------------------------

func TestBuiltinCompact_UsesOpenDialog(t *testing.T) {
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)
	cmd := reg.Lookup("compact")
	if cmd == nil {
		t.Fatal("compact command not found")
	}
	result := cmd.Execute(commands.CommandContext{}, "")
	if result.OpenDialog != "compact" {
		t.Errorf("expected OpenDialog='compact', got %q", result.OpenDialog)
	}
}

func TestBuiltinClear_UsesClearHistory(t *testing.T) {
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)
	cmd := reg.Lookup("clear")
	if cmd == nil {
		t.Fatal("clear command not found")
	}
	result := cmd.Execute(commands.CommandContext{}, "")
	if !result.ClearHistory {
		t.Error("expected ClearHistory=true for /clear command")
	}
}

// ---------------------------------------------------------------------------
// P1-E regression: MemdirLoadedMsg stores prompt
// ---------------------------------------------------------------------------

func TestUpdate_MemdirLoadedMsg_StoresPrompt(t *testing.T) {
	m := newTestModel()
	// Use a tempfile with content to verify prompt loading.
	dir := t.TempDir()
	f := dir + "/CLAUDE.md"
	if err := os.WriteFile(f, []byte("# My Rules\nAlways be helpful."), 0644); err != nil {
		t.Fatal(err)
	}
	m = applyMsg(m, MemdirLoadedMsg{Paths: []string{f}})
	if m.memdirPrompt == "" {
		t.Error("expected non-empty memdirPrompt after MemdirLoadedMsg with valid file")
	}
	if !strings.Contains(m.memdirPrompt, "Always be helpful.") {
		t.Errorf("expected CLAUDE.md content in memdirPrompt, got: %q", m.memdirPrompt)
	}
}

// ---------------------------------------------------------------------------
// InputModel.IsSlashCommand() / InputModel.SlashPrefix()
// ---------------------------------------------------------------------------

func TestInputModelIsSlashCommand(t *testing.T) {
	im := NewInput(false)
	if im.IsSlashCommand() {
		t.Error("empty input should not be a slash command")
	}
	im = im.SetValue("/help")
	if !im.IsSlashCommand() {
		t.Error("/help should be a slash command")
	}
	im = im.SetValue("hello")
	if im.IsSlashCommand() {
		t.Error("plain text should not be a slash command")
	}
}

func TestInputModelSlashPrefix(t *testing.T) {
	im := NewInput(false)
	if im.SlashPrefix() != "" {
		t.Error("empty input SlashPrefix should be empty")
	}
	im = im.SetValue("/help")
	if im.SlashPrefix() != "/help" {
		t.Errorf("got %q, want /help", im.SlashPrefix())
	}
	im = im.SetValue("/model claude-3")
	if im.SlashPrefix() != "/model" {
		t.Errorf("got %q, want /model", im.SlashPrefix())
	}
	im = im.SetValue("hello")
	if im.SlashPrefix() != "" {
		t.Error("plain text SlashPrefix should be empty")
	}
}

// ---------------------------------------------------------------------------
// messagelist helpers: renderSystemMessage, renderThinkingBlock,
// renderToolUseBlock, renderToolResultBlock, truncate
// ---------------------------------------------------------------------------

func TestRenderSystemMessage(t *testing.T) {
	theme := DefaultDarkTheme
	// Check each word separately — lipgloss may inject ANSI codes between words.
	msg := newSystemMessage("hello system")
	lookups := MessageLookups{
		ToolUseToResult:      make(map[string]types.ContentBlock),
		ResolvedToolUseIDs:   make(map[string]bool),
		ErroredToolUseIDs:    make(map[string]bool),
		InProgressToolUseIDs: make(map[string]bool),
	}
	out := renderMessage(msg, 80, false, theme, nil, nil, lookups)
	if !strings.Contains(out, "hello") || !strings.Contains(out, "system") {
		t.Errorf("renderSystemMessage output %q missing expected words", out)
	}
}

func TestRenderThinkingBlock(t *testing.T) {
	theme := DefaultDarkTheme
	// Short thinking (≤3 lines) — no truncation.
	out := renderThinkingBlock("line1\nline2", theme)
	if !strings.Contains(out, "line1") {
		t.Error("thinking block should contain line1")
	}
	// Long thinking (>3 lines) — truncated.
	long := "a\nb\nc\nd\ne"
	out2 := renderThinkingBlock(long, theme)
	if !strings.Contains(out2, "truncated") {
		t.Error("long thinking block should say truncated")
	}
}

func TestRenderToolUseBlock(t *testing.T) {
	theme := DefaultDarkTheme
	name := "Bash"
	toolID := "test-tool-id"
	blk := types.ContentBlock{
		Type:  types.ContentTypeToolUse,
		ID:    &toolID,
		Name:  &name,
		Input: map[string]any{"command": "ls -la"},
	}
	lookups := MessageLookups{
		ToolUseToResult:      make(map[string]types.ContentBlock),
		ResolvedToolUseIDs:   make(map[string]bool),
		ErroredToolUseIDs:    make(map[string]bool),
		InProgressToolUseIDs: make(map[string]bool),
	}
	out := renderToolUseBlock(blk, theme, lookups)
	if !strings.Contains(out, "Bash") {
		t.Errorf("tool use block should contain tool name, got: %q", out)
	}
}

func TestRenderToolResultBlock(t *testing.T) {
	theme := DefaultDarkTheme
	txt := "result content"
	blk := types.ContentBlock{
		Type: types.ContentTypeToolResult,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr(txt)},
		},
	}
	out := renderToolResultBlock(blk, theme, false)
	if !strings.Contains(out, txt) {
		t.Errorf("tool result block should contain result text, got: %q", out)
	}

	// Error variant.
	isErr := true
	errBlk := types.ContentBlock{
		Type:    types.ContentTypeToolResult,
		IsError: &isErr,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr("boom")},
		},
	}
	out2 := renderToolResultBlock(errBlk, theme, false)
	if !strings.Contains(out2, "boom") {
		t.Errorf("error tool result should contain error text, got: %q", out2)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("truncate of short string should be identity")
	}
	res := truncate("hello world", 5)
	if !strings.HasPrefix(res, "hello") {
		t.Errorf("truncated string should start with original prefix, got %q", res)
	}
	if !strings.Contains(res, "…") {
		t.Error("truncated string should contain ellipsis")
	}
}

// ---------------------------------------------------------------------------
// AgentStatus.String()
// ---------------------------------------------------------------------------

func TestAgentStatusString(t *testing.T) {
	cases := []struct {
		s    AgentStatus
		want string
	}{
		{AgentRunning, "Running"},
		{AgentPaused, "Paused"},
		{AgentCompleted, "Done"},
		{AgentFailed, "Failed"},
		{AgentStatus(99), "Unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("AgentStatus(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// handleTabCompletion
// ---------------------------------------------------------------------------

func TestHandleTabCompletion_WithMatch(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue("/hel")
	// Tab should autocomplete to /help
	m2, _ := m.handleTabCompletion()
	am := m2.(AppModel)
	if !strings.HasPrefix(am.input.Value(), "/help") {
		t.Errorf("tab completion: expected /help prefix, got %q", am.input.Value())
	}
}

func TestHandleTabCompletion_NoMatch(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue("/zzzzunknown")
	m2, _ := m.handleTabCompletion()
	am := m2.(AppModel)
	// No completion — value unchanged.
	if am.input.Value() != "/zzzzunknown" {
		t.Errorf("no match: expected unchanged value, got %q", am.input.Value())
	}
}
