package agentctx

import (
	"context"
	"testing"
)

func TestWithAgentID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		agentID string
		wantID  string
	}{
		{name: "set and get", agentID: "agent-123", wantID: "agent-123"},
		{name: "empty string", agentID: "", wantID: ""},
		{name: "uuid format", agentID: "aworker-abc123", wantID: "aworker-abc123"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := WithAgentID(context.Background(), tt.agentID)
			got := AgentID(ctx)
			if got != tt.wantID {
				t.Errorf("AgentID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestAgentID_NoValue(t *testing.T) {
	t.Parallel()
	got := AgentID(context.Background())
	if got != "" {
		t.Errorf("AgentID() on empty context = %q, want empty", got)
	}
}

func TestAgentID_Nested(t *testing.T) {
	t.Parallel()
	ctx := WithAgentID(context.Background(), "parent-agent")
	childCtx := WithAgentID(ctx, "child-agent")

	// Child context should return child ID.
	if got := AgentID(childCtx); got != "child-agent" {
		t.Errorf("AgentID(child) = %q, want %q", got, "child-agent")
	}
	// Parent context should still return parent ID.
	if got := AgentID(ctx); got != "parent-agent" {
		t.Errorf("AgentID(parent) = %q, want %q", got, "parent-agent")
	}
}
