// Package hooks provides the hook dispatch system.
// Hooks are shell commands or Go callbacks configured in settings.json that
// are executed at lifecycle points: PreToolUse, PostToolUse, Stop, etc.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/anthropics/claude-code-go/pkg/types"
)

const defaultTimeoutMs = 10_000 // 10 s

// Dispatcher executes hook definitions for a given lifecycle event.
type Dispatcher struct {
	// hooks maps hook type → list of definitions (from merged settings).
	hooks map[types.HookType][]types.HookDefinition
	// disabled is set when settings.DisableAllHooks == true.
	disabled bool
}

// NewDispatcher creates a Dispatcher from the merged settings hook map.
func NewDispatcher(hooks map[types.HookType][]types.HookDefinition, disabled bool) *Dispatcher {
	return &Dispatcher{hooks: hooks, disabled: disabled}
}

// Run executes all hook definitions registered for hookType.
// input is serialised to JSON and passed to the hook command via stdin.
// The aggregated result is returned; if any hook blocks, Blocked == true.
func (d *Dispatcher) Run(ctx context.Context, hookType types.HookType, input map[string]any) (*types.AggregatedHookResult, error) {
	agg := &types.AggregatedHookResult{}
	if d.disabled {
		return agg, nil
	}
	defs, ok := d.hooks[hookType]
	if !ok || len(defs) == 0 {
		return agg, nil
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("hooks: marshal input: %w", err)
	}

	for _, def := range defs {
		result, err := runHookCommand(ctx, def, inputJSON)
		if err != nil {
			// Hook execution error is non-fatal; treat as no-op.
			continue
		}
		if result == nil {
			continue
		}
		switch result.Decision {
		case types.HookDecisionBlock:
			agg.Blocked = true
			if result.Reason != "" {
				agg.BlockReasons = append(agg.BlockReasons, result.Reason)
			}
		case types.HookDecisionModify:
			agg.ModifiedContent = result.ModifiedContent
		}
	}
	return agg, nil
}

// runHookCommand executes a single HookDefinition's shell command and parses
// the JSON output as a HookResult.
func runHookCommand(ctx context.Context, def types.HookDefinition, inputJSON []byte) (*types.HookResult, error) {
	timeoutMs := def.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}
	deadline := time.Duration(timeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	//nolint:gosec // command is from user configuration
	cmd := exec.CommandContext(cmdCtx, "sh", "-c", def.Command)
	cmd.Stdin = jsonReader(inputJSON)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("hook command %q: %w", def.Command, err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	var result types.HookResult
	if err := json.Unmarshal(out, &result); err != nil {
		// Non-JSON output: treat as informational (no block/modify).
		return nil, nil
	}
	return &result, nil
}

// jsonReader wraps a byte slice as an io.Reader for cmd.Stdin.
type jsonReaderType struct {
	data []byte
	pos  int
}

func jsonReader(data []byte) *jsonReaderType { return &jsonReaderType{data: data} }

func (r *jsonReaderType) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
