package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// DefaultMaxToolUseConcurrency is the maximum number of concurrently-executing
// read-only tools per batch. Override with CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY.
const DefaultMaxToolUseConcurrency = 10

// toolCall bundles a single tool_use content block from the LLM response with
// the parsed Input for execution.
type toolCall struct {
	id    string
	name  string
	input json.RawMessage
}

// toolBatch is a group of tool calls that should be executed together.
// concurrent=true means all calls in the batch may run in parallel.
type toolBatch struct {
	calls      []toolCall
	concurrent bool
}

// partitionToolCalls splits a flat list of tool calls into consecutive batches
// according to concurrency safety.  Consecutive IsConcurrencySafe=true calls
// are merged into one concurrent batch; a single IsConcurrencySafe=false call
// forms its own serial batch.
func partitionToolCalls(calls []toolCall, registry *tool.Registry) []toolBatch {
	var batches []toolBatch
	var currentBatch []toolCall
	currentConcurrent := false

	flush := func() {
		if len(currentBatch) == 0 {
			return
		}
		batches = append(batches, toolBatch{
			calls:      currentBatch,
			concurrent: currentConcurrent,
		})
		currentBatch = nil
	}

	for _, tc := range calls {
		t, ok := registry.Get(tc.name)
		safe := ok && t.IsConcurrencySafe(tc.input)

		if len(currentBatch) == 0 {
			currentBatch = append(currentBatch, tc)
			currentConcurrent = safe
			continue
		}

		if safe && currentConcurrent {
			// Append to the current concurrent batch.
			currentBatch = append(currentBatch, tc)
		} else {
			// Different safety class — start a new batch.
			flush()
			currentBatch = append(currentBatch, tc)
			currentConcurrent = safe
		}
	}
	flush()
	return batches
}

// maxToolConcurrency returns the configured concurrency limit.
func maxToolConcurrency() int {
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY")); err == nil && v > 0 {
		return v
	}
	return DefaultMaxToolUseConcurrency
}

// runTools executes all tool batches in order, emitting Msg events on msgCh.
// It returns the list of tool_result content blocks to append to the conversation,
// as well as any UseContext modifications applied by serial tools.
func runTools(
	ctx context.Context,
	batches []toolBatch,
	registry *tool.Registry,
	uctx *tool.UseContext,
	msgCh chan<- Msg,
) ([]types.ContentBlock, error) {
	var allResults []types.ContentBlock

	for _, batch := range batches {
		results, err := executeBatch(ctx, batch, registry, uctx, msgCh)
		if err != nil {
			return allResults, err
		}
		allResults = append(allResults, results...)
	}
	return allResults, nil
}

// executeBatch runs one toolBatch (concurrent or serial).
func executeBatch(
	ctx context.Context,
	batch toolBatch,
	registry *tool.Registry,
	uctx *tool.UseContext,
	msgCh chan<- Msg,
) ([]types.ContentBlock, error) {
	if batch.concurrent {
		return executeConcurrentBatch(ctx, batch.calls, registry, uctx, msgCh)
	}
	return executeSerialBatch(ctx, batch.calls, registry, uctx, msgCh)
}

// executeConcurrentBatch runs all calls in parallel up to the concurrency limit.
func executeConcurrentBatch(
	ctx context.Context,
	calls []toolCall,
	registry *tool.Registry,
	uctx *tool.UseContext,
	msgCh chan<- Msg,
) ([]types.ContentBlock, error) {
	limit := maxToolConcurrency()
	sem := make(chan struct{}, limit)

	type indexedResult struct {
		idx    int
		result types.ContentBlock
	}

	results := make([]types.ContentBlock, len(calls))
	errCh := make(chan error, len(calls))
	resCh := make(chan indexedResult, len(calls))

	var wg sync.WaitGroup
	for i, tc := range calls {
		wg.Add(1)
		go func(idx int, tc toolCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := executeOneTool(ctx, tc, registry, uctx, msgCh)
			if err != nil {
				errCh <- err
				return
			}
			resCh <- indexedResult{idx: idx, result: result}
		}(i, tc)
	}

	go func() {
		wg.Wait()
		close(errCh)
		close(resCh)
	}()

	for r := range resCh {
		results[r.idx] = r.result
	}

	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// executeSerialBatch runs calls one after another, applying any ContextModifier
// returned by each call before starting the next.
func executeSerialBatch(
	ctx context.Context,
	calls []toolCall,
	registry *tool.Registry,
	uctx *tool.UseContext,
	msgCh chan<- Msg,
) ([]types.ContentBlock, error) {
	var results []types.ContentBlock
	for _, tc := range calls {
		result, err := executeOneTool(ctx, tc, registry, uctx, msgCh)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// executeOneTool runs a single tool call and emits the appropriate Msg events.
func executeOneTool(
	ctx context.Context,
	tc toolCall,
	registry *tool.Registry,
	uctx *tool.UseContext,
	msgCh chan<- Msg,
) (types.ContentBlock, error) {
	t, ok := registry.Get(tc.name)
	if !ok {
		// Unknown tool — return an error result block so the LLM can recover.
		errMsg := fmt.Sprintf("unknown tool: %q", tc.name)
		trueVal := true
		return types.ContentBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: &tc.id,
			Content:   []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr(errMsg)}},
			IsError:   &trueVal,
		}, nil
	}

	// Validate input.
	vr, verr := t.ValidateInput(tc.input, uctx)
	if verr != nil || !vr.OK {
		reason := "invalid input"
		if verr != nil {
			reason = verr.Error()
		} else if vr.Reason != "" {
			reason = vr.Reason
		}
		trueVal := true
		return types.ContentBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: &tc.id,
			Content:   []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr(reason)}},
			IsError:   &trueVal,
		}, nil
	}

	// Progress callback: forward to engine msg channel.
	onProgress := func(data any) {
		select {
		case msgCh <- Msg{Type: MsgTypeProgress, ProgressData: data, ToolUseID: tc.id}:
		default:
		}
	}

	// Execute.
	callResult, callErr := t.Call(tc.input, uctx, onProgress)

	var contentStr string
	isError := false
	if callErr != nil {
		contentStr = callErr.Error()
		isError = true
	} else if callResult != nil {
		// Apply context modifier for serial tools.
		if callResult.ContextModifier != nil && !t.IsConcurrencySafe(tc.input) {
			callResult.ContextModifier(uctx)
		}
		// Serialize content.
		switch v := callResult.Content.(type) {
		case string:
			contentStr = v
		case []byte:
			contentStr = string(v)
		case nil:
			contentStr = ""
		default:
			raw, merr := json.Marshal(v)
			if merr != nil {
				contentStr = fmt.Sprintf("error marshalling result: %v", merr)
				isError = true
			} else {
				contentStr = string(raw)
			}
		}
		isError = callResult.IsError
	}

	// Emit ToolResult msg.
	select {
	case msgCh <- Msg{
		Type: MsgTypeToolResult,
		ToolResult: &ToolResultMsg{
			ToolUseID: tc.id,
			Content:   contentStr,
			IsError:   isError,
		},
	}:
	case <-ctx.Done():
	}

	isErrorVal := isError
	return types.ContentBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: &tc.id,
		Content:   []types.ContentBlock{{Type: types.ContentTypeText, Text: &contentStr}},
		IsError:   &isErrorVal,
	}, nil
}

func strPtr(s string) *string { return &s }
