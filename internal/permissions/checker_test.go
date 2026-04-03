package permissions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// ─────────────────────────────────────────────────────────────────
// DenialTrackingState tests
// ─────────────────────────────────────────────────────────────────

func TestDenialTrackingState_Record(t *testing.T) {
	d := &DenialTrackingState{}
	d.Record(DenialRecord{ToolName: "Bash", ToolUseID: "u1", Reason: "denied by test"})
	if d.DenialCount != 1 {
		t.Errorf("expected DenialCount=1, got %d", d.DenialCount)
	}
	if d.LastDeniedAt.IsZero() {
		t.Error("expected LastDeniedAt to be set")
	}
	if len(d.RecentDenials) != 1 {
		t.Errorf("expected 1 recent denial, got %d", len(d.RecentDenials))
	}
}

func TestDenialTrackingState_RecordPreservesTimestamp(t *testing.T) {
	d := &DenialTrackingState{}
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	d.Record(DenialRecord{ToolName: "Edit", DeniedAt: ts})
	if d.LastDeniedAt != ts {
		t.Errorf("expected timestamp %v, got %v", ts, d.LastDeniedAt)
	}
}

func TestDenialTrackingState_MultipleRecords(t *testing.T) {
	d := &DenialTrackingState{}
	for i := 0; i < 5; i++ {
		d.Record(DenialRecord{ToolName: "Write"})
	}
	if d.DenialCount != 5 {
		t.Errorf("expected 5 denials, got %d", d.DenialCount)
	}
	if len(d.RecentDenials) != 5 {
		t.Errorf("expected 5 recent records, got %d", len(d.RecentDenials))
	}
}

// ─────────────────────────────────────────────────────────────────
// matchPattern tests
// ─────────────────────────────────────────────────────────────────

func TestMatchPattern_ExactName(t *testing.T) {
	if !matchPattern("Bash", "Bash", nil) {
		t.Error("expected exact name match")
	}
	if matchPattern("Bash", "Read", nil) {
		t.Error("expected no match for different name")
	}
}

func TestMatchPattern_CaseInsensitive(t *testing.T) {
	if !matchPattern("bash", "Bash", nil) {
		t.Error("expected case-insensitive match")
	}
}

func TestMatchPattern_WithContentGlob(t *testing.T) {
	// matcherFn returns true if pattern contains "*".
	matcher := func(pattern string) bool {
		return pattern == "*"
	}
	if !matchPattern("Bash(*)", "Bash", matcher) {
		t.Error("expected match for Bash(*)")
	}
	if matchPattern("Bash(specific_cmd)", "Bash", matcher) {
		t.Error("expected no match for specific_cmd pattern")
	}
}

func TestMatchPattern_WrongToolName(t *testing.T) {
	matcher := func(_ string) bool { return true }
	if matchPattern("Write(*)", "Read", matcher) {
		t.Error("expected no match when tool name differs")
	}
}

// ─────────────────────────────────────────────────────────────────
// Checker / CanUseTool tests
// ─────────────────────────────────────────────────────────────────

// stubPermTool is a minimal tool.Tool for permission testing.
type stubPermTool struct {
	name         string
	readOnly     bool
	permBehavior tool.PermissionBehavior
}

func (s *stubPermTool) Name() string    { return s.name }
func (s *stubPermTool) Aliases() []string { return nil }
func (s *stubPermTool) Description(_ tool.Input, _ tool.PermissionContext) string { return "" }
func (s *stubPermTool) InputSchema() tool.InputSchema { return tool.InputSchema{Type: "object"} }
func (s *stubPermTool) Prompt(_ context.Context, _ tool.PermissionContext) (string, error) {
	return "", nil
}
func (s *stubPermTool) MaxResultSizeChars() int                     { return -1 }
func (s *stubPermTool) SearchHint() string                         { return "" }
func (s *stubPermTool) IsConcurrencySafe(_ tool.Input) bool        { return s.readOnly }
func (s *stubPermTool) IsReadOnly(_ tool.Input) bool               { return s.readOnly }
func (s *stubPermTool) IsDestructive(_ tool.Input) bool            { return false }
func (s *stubPermTool) IsEnabled() bool                            { return true }
func (s *stubPermTool) InterruptBehavior() tool.InterruptBehavior  { return tool.InterruptBehaviorCancel }
func (s *stubPermTool) ValidateInput(_ tool.Input, _ *tool.UseContext) (tool.ValidationResult, error) {
	return tool.ValidationResult{OK: true}, nil
}
func (s *stubPermTool) CheckPermissions(_ tool.Input, _ *tool.UseContext) (tool.PermissionResult, error) {
	if s.permBehavior == "" {
		return tool.PermissionResult{Behavior: tool.PermissionPassthrough}, nil
	}
	return tool.PermissionResult{Behavior: s.permBehavior}, nil
}
func (s *stubPermTool) PreparePermissionMatcher(_ tool.Input) (func(string) bool, error) {
	return nil, nil
}
func (s *stubPermTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	return &tool.Result{Content: "ok"}, nil
}
func (s *stubPermTool) MapResultToToolResultBlock(_ any, _ string) (json.RawMessage, error) {
	return json.RawMessage(`"ok"`), nil
}
func (s *stubPermTool) ToAutoClassifierInput(_ tool.Input) string { return "" }
func (s *stubPermTool) UserFacingName(_ tool.Input) string        { return s.name }

type stubRegistry struct {
	tools map[string]tool.Tool
}

func (r *stubRegistry) Get(name string) (tool.Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func newStubRegistry(tools ...tool.Tool) *stubRegistry {
	r := &stubRegistry{tools: make(map[string]tool.Tool)}
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
	return r
}

func TestCanUseTool_BypassPermissions(t *testing.T) {
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{Mode: types.PermissionModeBypassPermissions},
	})
	result, err := c.CanUseTool(context.Background(), "Bash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionAllow {
		t.Errorf("expected Allow in bypass mode, got %q", result.Behavior)
	}
}

func TestCanUseTool_AlwaysDeny(t *testing.T) {
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysDenyRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"Bash"},
			},
		},
	})
	result, err := c.CanUseTool(context.Background(), "Bash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionDeny {
		t.Errorf("expected Deny from alwaysDeny rule, got %q", result.Behavior)
	}
}

func TestCanUseTool_AlwaysAllow(t *testing.T) {
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysAllowRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"Read"},
			},
		},
	})
	result, err := c.CanUseTool(context.Background(), "Read", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionAllow {
		t.Errorf("expected Allow from alwaysAllow rule, got %q", result.Behavior)
	}
}

func TestCanUseTool_AlwaysAsk(t *testing.T) {
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysAskRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"Write"},
			},
		},
	})
	result, err := c.CanUseTool(context.Background(), "Write", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionAsk {
		t.Errorf("expected Ask from alwaysAsk rule, got %q", result.Behavior)
	}
}

func TestCanUseTool_DontAskMode(t *testing.T) {
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{Mode: types.PermissionModeDontAsk},
	})
	result, err := c.CanUseTool(context.Background(), "Bash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionAllow {
		t.Errorf("expected Allow in dontAsk mode, got %q", result.Behavior)
	}
}

func TestCanUseTool_PlanMode_ReadOnly(t *testing.T) {
	reg := newStubRegistry(&stubPermTool{name: "Read", readOnly: true})
	c := NewChecker(CheckerConfig{
		PermCtx:  types.ToolPermissionContext{Mode: types.PermissionModePlan},
		Registry: reg,
	})
	result, err := c.CanUseTool(context.Background(), "Read", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionAllow {
		t.Errorf("expected Allow for read tool in plan mode, got %q", result.Behavior)
	}
}

func TestCanUseTool_PlanMode_WriteBlocked(t *testing.T) {
	reg := newStubRegistry(&stubPermTool{name: "Write", readOnly: false})
	c := NewChecker(CheckerConfig{
		PermCtx:  types.ToolPermissionContext{Mode: types.PermissionModePlan},
		Registry: reg,
	})
	result, err := c.CanUseTool(context.Background(), "Write", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionDeny {
		t.Errorf("expected Deny for write tool in plan mode, got %q", result.Behavior)
	}
}

func TestGetDenialCount(t *testing.T) {
	askCh := make(chan AskRequest, 1)
	respCh := make(chan AskResponse, 1)
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysDenyRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"Bash"},
			},
		},
		AskCh:  askCh,
		RespCh: respCh,
	})
	c.RequestPermission(context.Background(), PermissionRequest{
		ToolName:  "Bash",
		ToolUseID: "u1",
	}, nil)
	if c.GetDenialCount() != 1 {
		t.Errorf("expected 1 denial, got %d", c.GetDenialCount())
	}
}

func TestRequestPermission_AskNilChannels(t *testing.T) {
	// Ask with no TUI channels → downgrade to deny.
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysAskRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"Write"},
			},
		},
	})
	result, err := c.RequestPermission(context.Background(), PermissionRequest{
		ToolName:  "Write",
		ToolUseID: "u2",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionDeny {
		t.Errorf("expected Deny when ask channels are nil, got %q", result.Behavior)
	}
	if c.GetDenialCount() != 1 {
		t.Errorf("expected denial recorded, got %d", c.GetDenialCount())
	}
}

func TestRequestPermission_AskUserApproves(t *testing.T) {
	askCh := make(chan AskRequest, 1)
	respCh := make(chan AskResponse, 1)

	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysAskRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"Write"},
			},
		},
		AskCh:  askCh,
		RespCh: respCh,
	})

	// Pre-feed the response before calling (buffered channels).
	respCh <- AskResponse{ID: "u3", Decision: tool.PermissionAllow}

	result, err := c.RequestPermission(context.Background(), PermissionRequest{
		ToolName:  "Write",
		ToolUseID: "u3",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tool.PermissionAllow {
		t.Errorf("expected Allow after user approval, got %q", result.Behavior)
	}
}
