package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	tool "github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/internal/tools/agent"
)

// ── AgentTool ─────────────────────────────────────────────────────────────────

func TestAgentTool_Name(t *testing.T) {
	if agent.AgentTool.Name() != "Agent" {
		t.Errorf("expected Agent, got %q", agent.AgentTool.Name())
	}
}

func TestAgentTool_IsConcurrencySafe_False(t *testing.T) {
	// P1-1: must be false (state-mutating tool)
	if agent.AgentTool.IsConcurrencySafe(nil) {
		t.Error("AgentTool.IsConcurrencySafe must return false")
	}
}

func TestAgentTool_IsReadOnly_False(t *testing.T) {
	if agent.AgentTool.IsReadOnly(nil) {
		t.Error("AgentTool.IsReadOnly must return false")
	}
}

func TestAgentTool_InputSchema_HasAllowedTools(t *testing.T) {
	// P1-2: schema must contain allowed_tools
	schema := agent.AgentTool.InputSchema()
	if _, ok := schema.Properties["allowed_tools"]; !ok {
		t.Error("AgentTool schema missing 'allowed_tools'")
	}
}

func TestAgentTool_InputSchema_HasMaxTurns(t *testing.T) {
	// P1-2: schema must contain max_turns
	schema := agent.AgentTool.InputSchema()
	if _, ok := schema.Properties["max_turns"]; !ok {
		t.Error("AgentTool schema missing 'max_turns'")
	}
}

func TestAgentTool_InputSchema_RequiredIsPrompt(t *testing.T) {
	schema := agent.AgentTool.InputSchema()
	if len(schema.Required) != 1 || schema.Required[0] != "prompt" {
		t.Errorf("expected Required=[prompt], got %v", schema.Required)
	}
}

func TestAgentTool_UserFacingName_WithPrompt(t *testing.T) {
	in, _ := json.Marshal(agent.AgentInput{Prompt: "do something"})
	name := agent.AgentTool.UserFacingName(in)
	if !strings.Contains(name, "do something") {
		t.Errorf("unexpected UserFacingName: %q", name)
	}
}

func TestAgentTool_UserFacingName_LongTruncated(t *testing.T) {
	prompt := strings.Repeat("x", 80)
	in, _ := json.Marshal(agent.AgentInput{Prompt: prompt})
	name := agent.AgentTool.UserFacingName(in)
	// Should be truncated at 60 chars + "…" (3 UTF-8 bytes, 1 rune)
	// Use rune count to handle multi-byte ellipsis correctly.
	runes := []rune(name)
	// "Agent(" = 6 runes, 60 content runes, "…" = 1 rune, ")" = 1 rune → max 68 runes
	if len(runes) > 68 {
		t.Errorf("UserFacingName not truncated properly: %q (rune len %d)", name, len(runes))
	}
}

func TestAgentTool_UserFacingName_NoInput(t *testing.T) {
	name := agent.AgentTool.UserFacingName(nil)
	if name != "Agent" {
		t.Errorf("expected Agent, got %q", name)
	}
}

func TestAgentTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(agent.AgentInput{Prompt: "hello"})
	result, err := agent.AgentTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

func TestAgentTool_AgentInput_AllowedToolsField(t *testing.T) {
	// P1-2: JSON round-trip for AllowedTools
	in := agent.AgentInput{
		Prompt:       "do it",
		AllowedTools: []string{"Bash", "Read"},
	}
	data, _ := json.Marshal(in)
	var out agent.AgentInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.AllowedTools) != 2 || out.AllowedTools[0] != "Bash" {
		t.Errorf("AllowedTools round-trip failed: %v", out.AllowedTools)
	}
}

func TestAgentTool_AgentInput_MaxTurnsField(t *testing.T) {
	// P1-2: JSON round-trip for MaxTurns
	maxTurns := 5
	in := agent.AgentInput{Prompt: "do it", MaxTurns: &maxTurns}
	data, _ := json.Marshal(in)
	var out agent.AgentInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.MaxTurns == nil || *out.MaxTurns != 5 {
		t.Errorf("MaxTurns round-trip failed: %v", out.MaxTurns)
	}
}

func TestAgentTool_ImplementsToolInterface(t *testing.T) {
	var _ tool.Tool = agent.AgentTool
}

// ── SendMessageTool ───────────────────────────────────────────────────────────

func TestSendMessageTool_Name(t *testing.T) {
	if agent.SendMessageTool.Name() != "SendMessage" {
		t.Errorf("expected SendMessage, got %q", agent.SendMessageTool.Name())
	}
}

func TestSendMessageTool_IsConcurrencySafe_False(t *testing.T) {
	// P1-1: must be false
	if agent.SendMessageTool.IsConcurrencySafe(nil) {
		t.Error("SendMessageTool.IsConcurrencySafe must return false")
	}
}

func TestSendMessageTool_IsReadOnly_False(t *testing.T) {
	if agent.SendMessageTool.IsReadOnly(nil) {
		t.Error("SendMessageTool.IsReadOnly must return false")
	}
}

func TestSendMessageTool_InputSchema_ContentField(t *testing.T) {
	// P1-3: schema must use "content" not "message"
	schema := agent.SendMessageTool.InputSchema()
	if _, ok := schema.Properties["content"]; !ok {
		t.Error("SendMessageTool schema missing 'content' field")
	}
	if _, ok := schema.Properties["message"]; ok {
		t.Error("SendMessageTool schema must not have 'message' field (should be 'content')")
	}
}

func TestSendMessageTool_InputSchema_Required(t *testing.T) {
	schema := agent.SendMessageTool.InputSchema()
	reqMap := make(map[string]bool)
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	if !reqMap["agent_id"] {
		t.Error("SendMessageTool schema must require 'agent_id'")
	}
	if !reqMap["content"] {
		t.Error("SendMessageTool schema must require 'content'")
	}
}

func TestSendMessageInput_ContentJsonTag(t *testing.T) {
	// P1-3: JSON tag must be "content"
	in := agent.SendMessageInput{AgentID: "a1", Content: "hello"}
	data, _ := json.Marshal(in)
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["content"]; !ok {
		t.Error("SendMessageInput.Content must serialize as 'content'")
	}
	if _, ok := m["message"]; ok {
		t.Error("SendMessageInput must not serialize as 'message'")
	}
}

func TestSendMessageTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(agent.SendMessageInput{AgentID: "bot1", Content: "hi"})
	name := agent.SendMessageTool.UserFacingName(in)
	if !strings.Contains(name, "bot1") {
		t.Errorf("UserFacingName should contain agent_id: %q", name)
	}
}

func TestSendMessageTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(agent.SendMessageInput{AgentID: "a1", Content: "hello"})
	result, err := agent.SendMessageTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

func TestSendMessageTool_ImplementsToolInterface(t *testing.T) {
	var _ tool.Tool = agent.SendMessageTool
}
