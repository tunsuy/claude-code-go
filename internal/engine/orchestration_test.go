package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/claude-code-go/internal/tool"
)

// stubTool is a minimal tool.Tool implementation for testing.
type stubTool struct {
	name   string
	safe   bool
	readOnly bool
}

func (s *stubTool) Name() string                                     { return s.name }
func (s *stubTool) Aliases() []string                                { return nil }
func (s *stubTool) Description(_ tool.Input, _ tool.PermissionContext) string { return "" }
func (s *stubTool) InputSchema() tool.InputSchema                    { return tool.InputSchema{Type: "object"} }
func (s *stubTool) Prompt(_ context.Context, _ tool.PermissionContext) (string, error) {
	return "", nil
}
func (s *stubTool) MaxResultSizeChars() int                        { return -1 }
func (s *stubTool) SearchHint() string                             { return "" }
func (s *stubTool) IsConcurrencySafe(_ tool.Input) bool            { return s.safe }
func (s *stubTool) IsReadOnly(_ tool.Input) bool                   { return s.readOnly }
func (s *stubTool) IsDestructive(_ tool.Input) bool                { return false }
func (s *stubTool) IsEnabled() bool                                { return true }
func (s *stubTool) InterruptBehavior() tool.InterruptBehavior      { return tool.InterruptBehaviorCancel }
func (s *stubTool) ValidateInput(_ tool.Input, _ *tool.UseContext) (tool.ValidationResult, error) {
	return tool.ValidationResult{OK: true}, nil
}
func (s *stubTool) CheckPermissions(_ tool.Input, _ *tool.UseContext) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: tool.PermissionPassthrough}, nil
}
func (s *stubTool) PreparePermissionMatcher(_ tool.Input) (func(string) bool, error) {
	return nil, nil
}
func (s *stubTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	return &tool.Result{Content: "ok"}, nil
}
func (s *stubTool) MapResultToToolResultBlock(_ any, _ string) (json.RawMessage, error) {
	return json.RawMessage(`"ok"`), nil
}
func (s *stubTool) ToAutoClassifierInput(_ tool.Input) string { return "" }
func (s *stubTool) UserFacingName(_ tool.Input) string        { return s.name }

// newTestRegistry builds a *tool.Registry populated with the provided tools.
func newTestRegistry(tools ...tool.Tool) *tool.Registry {
	r := tool.NewRegistry()
	for _, t := range tools {
		r.Register(t)
	}
	return r
}

// newInput marshals v into a json.RawMessage, panicking on error.
func newInput(v any) tool.Input {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestPartitionToolCalls_AllConcurrent(t *testing.T) {
	registry := newTestRegistry(
		&stubTool{name: "Read", safe: true},
		&stubTool{name: "Glob", safe: true},
	)

	calls := []toolCall{
		{id: "1", name: "Read", input: newInput(map[string]string{"path": "/a"})},
		{id: "2", name: "Glob", input: newInput(map[string]string{"pattern": "*.go"})},
		{id: "3", name: "Read", input: newInput(map[string]string{"path": "/b"})},
	}

	batches := partitionToolCalls(calls, registry)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if !batches[0].concurrent {
		t.Error("expected concurrent batch")
	}
	if len(batches[0].calls) != 3 {
		t.Fatalf("expected 3 calls in batch, got %d", len(batches[0].calls))
	}
}

func TestPartitionToolCalls_AllSerial(t *testing.T) {
	registry := newTestRegistry(
		&stubTool{name: "Write", safe: false},
		&stubTool{name: "Edit", safe: false},
	)

	calls := []toolCall{
		{id: "1", name: "Write", input: newInput(nil)},
		{id: "2", name: "Edit", input: newInput(nil)},
	}

	batches := partitionToolCalls(calls, registry)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches (each serial), got %d", len(batches))
	}
	for i, b := range batches {
		if b.concurrent {
			t.Errorf("batch %d: expected serial, got concurrent", i)
		}
	}
}

func TestPartitionToolCalls_Mixed(t *testing.T) {
	registry := newTestRegistry(
		&stubTool{name: "Read", safe: true},
		&stubTool{name: "Write", safe: false},
		&stubTool{name: "Glob", safe: true},
	)

	// Read Read Write Glob → [Read,Read] [Write] [Glob]
	calls := []toolCall{
		{id: "1", name: "Read"},
		{id: "2", name: "Read"},
		{id: "3", name: "Write"},
		{id: "4", name: "Glob"},
	}

	batches := partitionToolCalls(calls, registry)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if !batches[0].concurrent || len(batches[0].calls) != 2 {
		t.Errorf("batch 0: expected concurrent with 2 calls, got concurrent=%v len=%d",
			batches[0].concurrent, len(batches[0].calls))
	}
	if batches[1].concurrent || len(batches[1].calls) != 1 {
		t.Errorf("batch 1: expected serial with 1 call, got concurrent=%v len=%d",
			batches[1].concurrent, len(batches[1].calls))
	}
	if !batches[2].concurrent || len(batches[2].calls) != 1 {
		t.Errorf("batch 2: expected concurrent with 1 call, got concurrent=%v len=%d",
			batches[2].concurrent, len(batches[2].calls))
	}
}

func TestPartitionToolCalls_Empty(t *testing.T) {
	registry := newTestRegistry()
	batches := partitionToolCalls(nil, registry)
	if len(batches) != 0 {
		t.Fatalf("expected 0 batches for nil input, got %d", len(batches))
	}
}

func TestPartitionToolCalls_UnknownTool(t *testing.T) {
	// Unknown tools (not in registry) are treated as unsafe (serial).
	registry := newTestRegistry()
	calls := []toolCall{
		{id: "1", name: "UnknownTool"},
		{id: "2", name: "AnotherUnknown"},
	}
	// Both are unsafe — two serial batches.
	batches := partitionToolCalls(calls, registry)
	if len(batches) != 2 {
		t.Fatalf("expected 2 serial batches for unknown tools, got %d", len(batches))
	}
	for i, b := range batches {
		if b.concurrent {
			t.Errorf("batch %d: expected serial for unknown tool", i)
		}
	}
}
