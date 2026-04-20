package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	// DefaultBashTimeoutMs is the default Bash execution timeout (2 minutes).
	DefaultBashTimeoutMs = 120_000
	// MaxBashTimeoutMs is the maximum allowed timeout (10 minutes).
	MaxBashTimeoutMs = 600_000
)

// ── Input / Output types ──────────────────────────────────────────────────────

// BashInput is the input schema for the Bash tools.
type BashInput struct {
	// Command is the shell command to execute (required).
	Command string `json:"command"`
	// Timeout is the execution timeout in milliseconds (optional, max 600000).
	Timeout *int `json:"timeout,omitempty"`
	// DangerouslyDisableSandbox bypasses sandbox when permitted by policy (optional).
	DangerouslyDisableSandbox bool `json:"dangerously_disable_sandbox,omitempty"`
}

// BashOutput is the structured output of Bash.
type BashOutput struct {
	// Stdout is the standard output of the command.
	Stdout string `json:"stdout"`
	// Stderr is the standard error output.
	Stderr string `json:"stderr"`
	// ExitCode is the command exit status.
	ExitCode int `json:"exit_code"`
	// TimedOut is true if the command was killed due to timeout.
	TimedOut bool `json:"timed_out,omitempty"`
}

// ── Tool implementation ───────────────────────────────────────────────────────

// BashTool is the exported singleton instance.
// TODO(dep): Full sandbox implementation requires Agent-Core sandbox manager.
// Phase 1 MVP uses exec.Command directly; Phase 2 will inject a BashExecutor
// interface for sandboxed execution.
var BashTool tools.Tool = &bashTool{}

type bashTool struct{ tools.BaseTool }

func (t *bashTool) Name() string { return "Bash" }

func (t *bashTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Executes a given bash command and returns its output.

The working directory persists between commands, but shell state does not.

IMPORTANT:
- Avoid using this tool to run find, grep, cat, head, tail, sed, awk, or echo commands unless explicitly instructed; use the dedicated tools instead
- You can specify an optional timeout in milliseconds (up to 600000ms / 10 minutes); default is 120000ms (2 minutes)
- Use the run_in_background parameter for long-running commands`
}

func (t *bashTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"command": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The bash command to execute",
			}),
			"timeout": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": fmt.Sprintf("Optional timeout in milliseconds (max %d)", MaxBashTimeoutMs),
			}),
			"dangerously_disable_sandbox": tools.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "Set to true to disable sandbox mode when policy permits",
			}),
		},
		[]string{"command"},
	)
}

func (t *bashTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *bashTool) IsReadOnly(_ tools.Input) bool         { return false }
func (t *bashTool) IsDestructive(_ tools.Input) bool      { return true }

func (t *bashTool) InterruptBehavior() tools.InterruptBehavior {
	return tools.InterruptBehaviorCancel
}

func (t *bashTool) UserFacingName(input tools.Input) string {
	var in BashInput
	if json.Unmarshal(input, &in) == nil && in.Command != "" {
		cmd := in.Command
		if len(cmd) > 60 {
			cmd = cmd[:60] + "…"
		}
		return fmt.Sprintf("Bash(%s)", cmd)
	}
	return "Bash"
}

func (t *bashTool) ValidateInput(input tools.Input, _ *tools.UseContext) (tools.ValidationResult, error) {
	var in BashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.ValidationResult{OK: false, Reason: "invalid JSON: " + err.Error()}, nil
	}
	if strings.TrimSpace(in.Command) == "" {
		return tools.ValidationResult{OK: false, Reason: "command is required"}, nil
	}
	if in.Timeout != nil && *in.Timeout > MaxBashTimeoutMs {
		return tools.ValidationResult{
			OK:     false,
			Reason: fmt.Sprintf("timeout exceeds maximum allowed value of %d ms", MaxBashTimeoutMs),
		}, nil
	}
	return tools.ValidationResult{OK: true}, nil
}

// Call executes the Bash tools.
// TODO(dep): sandbox support, permission checks, and security rule matching
// require Agent-Core's SandboxManager and permission system.
func (t *bashTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in BashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}

	timeoutMs := DefaultBashTimeoutMs
	if in.Timeout != nil && *in.Timeout > 0 {
		timeoutMs = *in.Timeout
		if timeoutMs > MaxBashTimeoutMs {
			timeoutMs = MaxBashTimeoutMs
		}
	}

	// TODO(dep): ShouldUseSandbox(in.Command, in.DangerouslyDisableSandbox, settings)

	var parentCtx context.Context = context.Background()
	if ctx != nil && ctx.Ctx != nil {
		parentCtx = ctx.Ctx
	}

	timeoutCtx, cancel := context.WithTimeout(parentCtx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", in.Command)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	out := BashOutput{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			out.TimedOut = true
			out.ExitCode = -1
			return &tools.Result{
				IsError: true,
				Content: fmt.Sprintf("Command timed out after %d ms.\nstdout: %s\nstderr: %s",
					timeoutMs, out.Stdout, out.Stderr),
			}, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			out.ExitCode = exitErr.ExitCode()
		} else {
			out.ExitCode = -1
		}
	}

	return &tools.Result{Content: out}, nil
}

func (t *bashTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	out, ok := output.(BashOutput)
	if !ok {
		return t.BaseTool.MapResultToToolResultBlock(output, toolUseID)
	}

	var sb strings.Builder
	if out.Stdout != "" {
		sb.WriteString(out.Stdout)
	}
	if out.Stderr != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("[stderr]\n")
		sb.WriteString(out.Stderr)
	}
	if out.ExitCode != 0 {
		sb.WriteString(fmt.Sprintf("\n[Exit code: %d]", out.ExitCode))
	}

	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     sb.String(),
	}
	return json.Marshal(block)
}
