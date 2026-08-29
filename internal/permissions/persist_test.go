package permissions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/config"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// readProjectAllowRules reads permissions.allow from dir/.claude/settings.json.
func readProjectAllowRules(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.ClaudeDir, config.SettingsFile))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var doc struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return doc.Permissions.Allow
}

// newAlwaysAskChecker builds a checker that asks for WebFetch and persists
// always-allow rules into dir's project settings.
func newAlwaysAskChecker(t *testing.T, dir string) (Checker, chan AskRequest, chan AskResponse) {
	t.Helper()
	askCh := make(chan AskRequest, 1)
	respCh := make(chan AskResponse, 1)
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysAskRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"WebFetch"},
			},
		},
		AskCh:  askCh,
		RespCh: respCh,
		Persist: &PersistConfig{
			ProjectDir: dir,
		},
	})
	return c, askCh, respCh
}

func TestRequestPermission_AlwaysAllowPersistsRule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, _, respCh := newAlwaysAskChecker(t, dir)

	// Pre-feed the always-allow response (buffered channels).
	respCh <- AskResponse{ID: "u1", Decision: tools.PermissionAllow, AlwaysAllow: true}

	result, err := c.RequestPermission(context.Background(), PermissionRequest{
		ToolName:  "WebFetch",
		ToolUseID: "u1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tools.PermissionAllow {
		t.Fatalf("expected Allow, got %q", result.Behavior)
	}

	got := readProjectAllowRules(t, dir)
	if len(got) != 1 || got[0] != "WebFetch" {
		t.Errorf("settings allow = %v, want [WebFetch]", got)
	}
}

func TestRequestPermission_AlwaysAllowSubsequentCallAllowedWithoutAsking(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, askCh, respCh := newAlwaysAskChecker(t, dir)
	respCh <- AskResponse{ID: "u1", Decision: tools.PermissionAllow, AlwaysAllow: true}
	if _, err := c.RequestPermission(context.Background(), PermissionRequest{
		ToolName:  "WebFetch",
		ToolUseID: "u1",
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Second call: the in-memory rule must short-circuit to Allow before the
	// ask layer — CanUseTool alone should confirm it.
	result, err := c.CanUseTool(context.Background(), "WebFetch", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tools.PermissionAllow {
		t.Errorf("expected Allow on repeat call, got %q", result.Behavior)
	}

	// Drain the first call's AskRequest from the buffered ask channel.
	select {
	case <-askCh:
	default:
		t.Fatal("expected an AskRequest from the first call")
	}

	// And the full flow must not emit a new AskRequest.
	done := make(chan tools.PermissionResult, 1)
	go func() {
		r, _ := c.RequestPermission(context.Background(), PermissionRequest{
			ToolName:  "WebFetch",
			ToolUseID: "u2",
		}, nil)
		done <- r
	}()
	select {
	case r := <-done:
		if r.Behavior != tools.PermissionAllow {
			t.Errorf("expected Allow without asking, got %q", r.Behavior)
		}
	case req := <-askCh:
		t.Errorf("unexpected ask request: %+v", req)
	}
}

func TestRequestPermission_AlwaysAllowIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, _, respCh := newAlwaysAskChecker(t, dir)

	for _, id := range []string{"u1", "u2"} {
		respCh <- AskResponse{ID: id, Decision: tools.PermissionAllow, AlwaysAllow: true}
		if _, err := c.RequestPermission(context.Background(), PermissionRequest{
			ToolName:  "WebFetch",
			ToolUseID: id,
		}, nil); err != nil {
			t.Fatal(err)
		}
	}

	got := readProjectAllowRules(t, dir)
	if len(got) != 1 || got[0] != "WebFetch" {
		t.Errorf("settings allow = %v, want single [WebFetch] entry", got)
	}
}

func TestRequestPermission_PlainAllowDoesNotPersist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c, _, respCh := newAlwaysAskChecker(t, dir)

	respCh <- AskResponse{ID: "u1", Decision: tools.PermissionAllow}
	if _, err := c.RequestPermission(context.Background(), PermissionRequest{
		ToolName:  "WebFetch",
		ToolUseID: "u1",
	}, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, config.ClaudeDir, config.SettingsFile)); !os.IsNotExist(err) {
		t.Error("plain allow should not write settings.json")
	}

	// The next call must still ask (in-memory rules untouched).
	result, err := c.CanUseTool(context.Background(), "WebFetch", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != tools.PermissionAsk {
		t.Errorf("expected Ask on repeat call after plain allow, got %q", result.Behavior)
	}
}

// stubPathTool is a PathTool stub so ruleForTool produces path rules.
type stubPathTool struct {
	stubPermTool
	path string
}

func (s *stubPathTool) GetPath(_ tools.Input) string { return s.path }

func TestRuleForTool_PathToolProducesPathRule(t *testing.T) {
	reg := newStubRegistry(&stubPathTool{
		stubPermTool: stubPermTool{name: "Read"},
		path:         "/tmp/project",
	})
	if got := ruleForTool("Read", nil, reg); got != "Read(/tmp/project/**)" {
		t.Errorf("ruleForTool = %q, want Read(/tmp/project/**)", got)
	}
}

func TestRuleForTool_NonPathToolProducesBareName(t *testing.T) {
	reg := newStubRegistry(&stubPermTool{name: "WebFetch"})
	if got := ruleForTool("WebFetch", nil, reg); got != "WebFetch" {
		t.Errorf("ruleForTool = %q, want WebFetch", got)
	}
}

func TestRuleForTool_NilRegistry(t *testing.T) {
	if got := ruleForTool("WebFetch", nil, nil); got != "WebFetch" {
		t.Errorf("ruleForTool = %q, want WebFetch", got)
	}
}

func TestSetProjectDir(t *testing.T) {
	dir := t.TempDir()
	c, _, respCh := newAlwaysAskChecker(t, "")

	// Persistence was disabled at construction; enable it via SetProjectDir.
	impl := c.(*checker)
	impl.SetProjectDir(dir)

	respCh <- AskResponse{ID: "u1", Decision: tools.PermissionAllow, AlwaysAllow: true}
	if _, err := c.RequestPermission(context.Background(), PermissionRequest{
		ToolName:  "WebFetch",
		ToolUseID: "u1",
	}, nil); err != nil {
		t.Fatal(err)
	}

	got := readProjectAllowRules(t, dir)
	if len(got) != 1 || got[0] != "WebFetch" {
		t.Errorf("settings allow = %v, want [WebFetch]", got)
	}
}

func TestUniqueStrings(t *testing.T) {
	rules := uniqueStrings(nil, "a")
	rules = uniqueStrings(rules, "b")
	rules = uniqueStrings(rules, "a")
	if len(rules) != 2 || rules[0] != "a" || rules[1] != "b" {
		t.Errorf("uniqueStrings = %v, want [a b]", rules)
	}
}
