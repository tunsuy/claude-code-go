package shell_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/shell"
)

func TestBashTool_Name(t *testing.T) {
	if shell.BashTool.Name() != "Bash" {
		t.Errorf("expected Bash, got %q", shell.BashTool.Name())
	}
}

func TestBashTool_IsConcurrencySafe_False(t *testing.T) {
	if shell.BashTool.IsConcurrencySafe(nil) {
		t.Error("BashTool must not be concurrency-safe")
	}
}

func TestBashTool_IsReadOnly_False(t *testing.T) {
	if shell.BashTool.IsReadOnly(nil) {
		t.Error("BashTool must not be read-only")
	}
}

func TestBashTool_IsDestructive_True(t *testing.T) {
	if !shell.BashTool.IsDestructive(nil) {
		t.Error("BashTool must be destructive")
	}
}

func TestBashTool_InputSchema(t *testing.T) {
	schema := shell.BashTool.InputSchema()
	if schema.Type != "object" {
		t.Errorf("expected schema.Type=object, got %q", schema.Type)
	}
	if _, ok := schema.Properties["command"]; !ok {
		t.Error("schema missing 'command'")
	}
	if _, ok := schema.Properties["timeout"]; !ok {
		t.Error("schema missing 'timeout'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "command" {
		t.Errorf("expected Required=[command], got %v", schema.Required)
	}
}

func TestBashTool_ValidateInput_EmptyCommand(t *testing.T) {
	in, _ := json.Marshal(shell.BashInput{Command: "  "})
	vr, err := shell.BashTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for empty command")
	}
}

func TestBashTool_ValidateInput_TimeoutExceedsMax(t *testing.T) {
	tooLong := shell.MaxBashTimeoutMs + 1
	in, _ := json.Marshal(shell.BashInput{Command: "ls", Timeout: &tooLong})
	vr, err := shell.BashTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for timeout > max")
	}
}

func TestBashTool_ValidateInput_Valid(t *testing.T) {
	in, _ := json.Marshal(shell.BashInput{Command: "echo hello"})
	vr, err := shell.BashTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.OK {
		t.Errorf("expected validation OK, got reason: %q", vr.Reason)
	}
}

func TestBashTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(shell.BashInput{Command: "echo hello"})
	name := shell.BashTool.UserFacingName(in)
	if name != "Bash(echo hello)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestBashTool_UserFacingName_LongTruncated(t *testing.T) {
	cmd := strings.Repeat("x", 80)
	in, _ := json.Marshal(shell.BashInput{Command: cmd})
	name := shell.BashTool.UserFacingName(in)
	if !strings.HasSuffix(name, "…)") {
		t.Errorf("expected truncation with …: %q", name)
	}
}

func TestBashTool_UserFacingName_NoInput(t *testing.T) {
	if shell.BashTool.UserFacingName(nil) != "Bash" {
		t.Error("expected fallback name Bash")
	}
}

func TestBashTool_Call_EchoCommand(t *testing.T) {
	in, _ := json.Marshal(shell.BashInput{Command: "echo hello"})
	result, err := shell.BashTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	out, ok := result.Content.(shell.BashOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if !strings.Contains(out.Stdout, "hello") {
		t.Errorf("expected 'hello' in stdout, got %q", out.Stdout)
	}
	if out.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", out.ExitCode)
	}
}

func TestBashTool_Call_NonZeroExitCode(t *testing.T) {
	in, _ := json.Marshal(shell.BashInput{Command: "exit 42"})
	result, err := shell.BashTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := result.Content.(shell.BashOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if out.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", out.ExitCode)
	}
}

func TestBashTool_Call_Stderr(t *testing.T) {
	in, _ := json.Marshal(shell.BashInput{Command: "echo err >&2"})
	result, err := shell.BashTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := result.Content.(shell.BashOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if !strings.Contains(out.Stderr, "err") {
		t.Errorf("expected 'err' in stderr, got %q", out.Stderr)
	}
}

func TestBashTool_Call_InvalidJSON(t *testing.T) {
	result, err := shell.BashTool.Call([]byte("not-json"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
}

func TestBashTool_Call_Timeout(t *testing.T) {
	timeoutMs := 100 // 100ms
	in, _ := json.Marshal(shell.BashInput{Command: "sleep 10", Timeout: &timeoutMs})
	result, err := shell.BashTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for timed-out command")
	}
	content, ok := result.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result.Content)
	}
	if !strings.Contains(content, "timed out") {
		t.Errorf("expected 'timed out' in error message, got %q", content)
	}
}

func TestBashTool_MapResultToToolResultBlock(t *testing.T) {
	out := shell.BashOutput{Stdout: "hello\n", Stderr: "", ExitCode: 0}
	raw, err := shell.BashTool.MapResultToToolResultBlock(out, "tid1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	if block["type"] != "tool_result" {
		t.Errorf("expected type=tool_result")
	}
	content := block["content"].(string)
	if !strings.Contains(content, "hello") {
		t.Errorf("expected stdout in content: %q", content)
	}
}

func TestBashTool_MapResultToToolResultBlock_WithStderr(t *testing.T) {
	out := shell.BashOutput{Stdout: "out\n", Stderr: "err\n", ExitCode: 1}
	raw, err := shell.BashTool.MapResultToToolResultBlock(out, "tid2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	content := block["content"].(string)
	if !strings.Contains(content, "[stderr]") {
		t.Errorf("expected [stderr] in content: %q", content)
	}
	if !strings.Contains(content, "Exit code: 1") {
		t.Errorf("expected exit code in content: %q", content)
	}
}

func TestBashTool_Constants(t *testing.T) {
	if shell.DefaultBashTimeoutMs != 120_000 {
		t.Errorf("expected DefaultBashTimeoutMs=120000, got %d", shell.DefaultBashTimeoutMs)
	}
	if shell.MaxBashTimeoutMs != 600_000 {
		t.Errorf("expected MaxBashTimeoutMs=600000, got %d", shell.MaxBashTimeoutMs)
	}
}

func TestBashTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = shell.BashTool
}
