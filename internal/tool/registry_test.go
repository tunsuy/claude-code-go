package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// mockTool is a minimal Tool implementation for testing the Registry.
type mockTool struct {
	tool.BaseTool
	name    string
	aliases []string
	enabled bool
}

func newMock(name string, aliases ...string) *mockTool {
	return &mockTool{name: name, aliases: aliases, enabled: true}
}

func (m *mockTool) Name() string    { return m.name }
func (m *mockTool) Aliases() []string { return m.aliases }
func (m *mockTool) IsEnabled() bool { return m.enabled }

func (m *mockTool) Description(_ tool.Input, _ tool.PermissionContext) string { return "" }
func (m *mockTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(nil, nil)
}
func (m *mockTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (m *mockTool) IsReadOnly(_ tool.Input) bool        { return true }
func (m *mockTool) UserFacingName(_ tool.Input) string  { return m.name }
func (m *mockTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	return &tool.Result{Content: "ok"}, nil
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestRegistry_Register_BasicLookup(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("Bash"))

	got, ok := r.Get("Bash")
	if !ok {
		t.Fatal("expected tool to be found after Register")
	}
	if got.Name() != "Bash" {
		t.Fatalf("expected name Bash, got %q", got.Name())
	}
}

func TestRegistry_Register_AliasLookup(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("Read", "FileRead", "ReadFile"))

	for _, alias := range []string{"Read", "FileRead", "ReadFile"} {
		got, ok := r.Get(alias)
		if !ok {
			t.Fatalf("expected alias %q to resolve", alias)
		}
		if got.Name() != "Read" {
			t.Fatalf("alias %q: expected name Read, got %q", alias, got.Name())
		}
	}
}

func TestRegistry_Register_PanicOnDuplicate(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("Bash"))

	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate Register, got none")
		}
	}()
	r.Register(newMock("Bash"))
}

func TestRegistry_Register_PreservesOrder(t *testing.T) {
	r := tool.NewRegistry()
	names := []string{"Alpha", "Beta", "Gamma", "Delta"}
	for _, n := range names {
		r.Register(newMock(n))
	}

	got := r.Names()
	if len(got) != len(names) {
		t.Fatalf("expected %d names, got %d", len(names), len(got))
	}
	for i, want := range names {
		if got[i] != want {
			t.Fatalf("order[%d]: expected %q, got %q", i, want, got[i])
		}
	}
}

// ── Deregister ────────────────────────────────────────────────────────────────

func TestRegistry_Deregister_RemovesCanonical(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("Bash"))

	if err := r.Deregister("Bash"); err != nil {
		t.Fatalf("unexpected Deregister error: %v", err)
	}
	if _, ok := r.Get("Bash"); ok {
		t.Error("expected tool to be absent after Deregister")
	}
	if r.Len() != 0 {
		t.Errorf("expected Len 0 after Deregister, got %d", r.Len())
	}
}

func TestRegistry_Deregister_RemovesAliases(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("Read", "FileRead", "ReadFile"))

	if err := r.Deregister("Read"); err != nil {
		t.Fatalf("unexpected Deregister error: %v", err)
	}
	for _, alias := range []string{"Read", "FileRead", "ReadFile"} {
		if _, ok := r.Get(alias); ok {
			t.Errorf("alias %q should be absent after Deregister", alias)
		}
	}
}

func TestRegistry_Deregister_UpdatesOrder(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("A"))
	r.Register(newMock("B"))
	r.Register(newMock("C"))

	if err := r.Deregister("B"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"A", "C"}
	got := r.Names()
	if len(got) != len(want) {
		t.Fatalf("expected names %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names[%d]: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestRegistry_Deregister_ErrorIfNotFound(t *testing.T) {
	r := tool.NewRegistry()
	err := r.Deregister("NonExistent")
	if err == nil {
		t.Error("expected error when deregistering non-existent tool")
	}
}

// ── Replace ───────────────────────────────────────────────────────────────────

func TestRegistry_Replace_UpdatesExisting(t *testing.T) {
	r := tool.NewRegistry()
	old := newMock("Bash")
	r.Register(old)

	updated := newMock("Bash")
	r.Replace(updated) // must not panic

	got, ok := r.Get("Bash")
	if !ok {
		t.Fatal("expected tool to be found after Replace")
	}
	if got != updated {
		t.Error("expected registry to hold the replaced instance")
	}
	if r.Len() != 1 {
		t.Errorf("expected Len 1 after Replace, got %d", r.Len())
	}
}

func TestRegistry_Replace_RegistersIfAbsent(t *testing.T) {
	r := tool.NewRegistry()
	r.Replace(newMock("NewTool"))

	if _, ok := r.Get("NewTool"); !ok {
		t.Error("Replace should register tool if not already present")
	}
}

func TestRegistry_Replace_UpdatesAliases(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("MyTool", "OldAlias"))

	// Replace with new aliases — OldAlias should be gone.
	r.Replace(newMock("MyTool", "NewAlias"))

	if _, ok := r.Get("OldAlias"); ok {
		t.Error("OldAlias should have been removed after Replace")
	}
	if _, ok := r.Get("NewAlias"); !ok {
		t.Error("NewAlias should be accessible after Replace")
	}
}

func TestRegistry_Replace_PreservesOrder(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("A"))
	r.Register(newMock("B"))
	r.Register(newMock("C"))
	r.Replace(newMock("B")) // replace the middle tool

	got := r.Names()
	want := []string{"A", "B", "C"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order after Replace: expected %v, got %v", want, got)
		}
	}
}

// ── All / Filter ──────────────────────────────────────────────────────────────

func TestRegistry_All_SkipsDisabled(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("Enabled"))

	disabled := newMock("Disabled")
	disabled.enabled = false
	r.Register(disabled)

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 enabled tool, got %d", len(all))
	}
	if all[0].Name() != "Enabled" {
		t.Errorf("expected Enabled, got %q", all[0].Name())
	}
}

func TestRegistry_Filter_SubsetByName(t *testing.T) {
	r := tool.NewRegistry()
	for _, n := range []string{"A", "B", "C", "D"} {
		r.Register(newMock(n))
	}

	got := r.Filter([]string{"B", "D"})
	if len(got) != 2 {
		t.Fatalf("expected 2 tools from Filter, got %d", len(got))
	}
	if got[0].Name() != "B" || got[1].Name() != "D" {
		t.Errorf("unexpected filter result: %v", got)
	}
}

func TestRegistry_Filter_EmptyReturnsAll(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(newMock("X"))
	r.Register(newMock("Y"))

	got := r.Filter(nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 tools from empty Filter, got %d", len(got))
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestRegistry_ConcurrentReadsSafe(t *testing.T) {
	r := tool.NewRegistry()
	for i := 0; i < 20; i++ {
		r.Register(newMock(string(rune('A' + i))))
	}

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			_ = r.All()
			_, _ = r.Get("A")
			_ = r.Len()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

// ── PropSchema / NewInputSchema ───────────────────────────────────────────────

func TestPropSchema_RoundTrip(t *testing.T) {
	def := map[string]any{"type": "string", "description": "test"}
	raw := tool.PropSchema(def)

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("PropSchema produced invalid JSON: %v", err)
	}
	if got["type"] != "string" {
		t.Errorf("expected type=string, got %v", got["type"])
	}
}

func TestNewInputSchema_TypeIsObject(t *testing.T) {
	schema := tool.NewInputSchema(nil, nil)
	if schema.Type != "object" {
		t.Errorf("expected Type=object, got %q", schema.Type)
	}
}

// ── BaseTool defaults ─────────────────────────────────────────────────────────

func TestBaseTool_Defaults(t *testing.T) {
	m := newMock("X")

	if len(m.Aliases()) != 0 {
		t.Error("expected Aliases() == nil/empty")
	}
	if m.MaxResultSizeChars() != -1 {
		t.Errorf("expected MaxResultSizeChars -1, got %d", m.MaxResultSizeChars())
	}
	if m.SearchHint() != "" {
		t.Errorf("expected empty SearchHint, got %q", m.SearchHint())
	}
	if m.IsDestructive(nil) {
		t.Error("expected IsDestructive false by default")
	}
	if !m.IsEnabled() {
		t.Error("expected IsEnabled true by default")
	}
	if m.InterruptBehavior() != tool.InterruptBehaviorCancel {
		t.Errorf("expected InterruptBehaviorCancel, got %q", m.InterruptBehavior())
	}
	prompt, err := m.Prompt(context.Background(), nil)
	if err != nil || prompt != "" {
		t.Errorf("expected empty Prompt, got (%q, %v)", prompt, err)
	}
	vr, err := m.ValidateInput(nil, nil)
	if err != nil || !vr.OK {
		t.Errorf("expected ValidateInput OK, got (%v, %v)", vr, err)
	}
	pr, err := m.CheckPermissions(nil, nil)
	if err != nil || pr.Behavior != tool.PermissionPassthrough {
		t.Errorf("expected CheckPermissions Passthrough, got (%v, %v)", pr, err)
	}
	matcher, err := m.PreparePermissionMatcher(nil)
	if err != nil || matcher != nil {
		t.Errorf("expected PreparePermissionMatcher nil, got (non-nil matcher, %v)", err)
	}
	if m.ToAutoClassifierInput(nil) != "" {
		t.Error("expected empty ToAutoClassifierInput")
	}
}
