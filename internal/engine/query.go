package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/compact"
	"github.com/tunsuy/claude-code-go/internal/msgqueue"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// defaultMsgBufSize is the default channel buffer for Msg events.
const defaultMsgBufSize = 256

// msgBufSize reads the buffer size from env or returns the default.
func msgBufSize() int {
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_ENGINE_MSG_BUF_SIZE")); err == nil && v > 0 {
		return v
	}
	return defaultMsgBufSize
}

// Query implements QueryEngine.Query.
// It starts the query loop in a background goroutine and returns the event channel.
func (e *engineImpl) Query(ctx context.Context, params QueryParams) (<-chan Msg, error) {
	ctx, cancel := context.WithCancel(ctx)
	e.abortMu.Lock()
	e.abortFn = cancel
	e.abortMu.Unlock()

	msgCh := make(chan Msg, msgBufSize())

	go func() {
		defer close(msgCh)
		defer cancel()
		e.runQueryLoop(ctx, params, msgCh)
	}()

	return msgCh, nil
}

// runQueryLoop executes the main LLM conversation loop until termination.
func (e *engineImpl) runQueryLoop(ctx context.Context, params QueryParams, msgCh chan<- Msg) {
	budget := NewBudgetTracker()

	// Initialise the working message list from params (don't mutate engine state).
	messages := make([]types.Message, len(params.Messages))
	copy(messages, params.Messages)

	// Write the final message history back to the engine when the loop exits,
	// under mu so GetMessages/SetMessages callers see a consistent snapshot.
	// If NoWriteBack is set (forked agents), skip the write-back to avoid
	// polluting the parent conversation.
	defer func() {
		if params.NoWriteBack {
			return
		}
		e.mu.Lock()
		e.messages = make([]types.Message, len(messages))
		copy(e.messages, messages)
		e.mu.Unlock()
	}()

	maxTurns := params.MaxTurns
	turns := 0

	for {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Respect MaxTurns limit.
		if maxTurns > 0 && turns >= maxTurns {
			sendSystem(ctx, msgCh, "Maximum turns reached.")
			return
		}
		turns++

		// ── Compaction pipeline ─────────────────────────────────────────────
		// Step 1: Snip compaction — remove old tool result content locally
		// (no LLM call) to stay within the context budget.
		snipResult := compact.SnipCompactIfNeeded(messages)
		messages = snipResult.Messages
		if snipResult.BoundaryMessage != nil {
			messages = append(messages, *snipResult.BoundaryMessage)
		}

		// Step 2: Micro compaction — truncate any single oversized tool result.
		if e.microCompactor != nil {
			extra := compact.CompactionExtra{SnipTokensFreed: snipResult.TokensFreed}
			if e.microCompactor.NeedsCompaction(messages, e.model, extra) {
				compactParams := compact.CompactionParams{
					SystemPromptParts: systemPromptParts(params),
					UserContext:       params.UserContext,
					SystemContext:     params.SystemContext,
					QuerySource:       params.QuerySource,
				}
				if result, err := e.microCompactor.Compact(ctx, messages, compactParams); err == nil {
					messages = result.SummaryMessages
				}
			}
		}

		// Step 3: Auto compaction — LLM-based summarisation when context is
		// near the model's context window limit.
		if e.autoCompactor != nil {
			extra := compact.CompactionExtra{SnipTokensFreed: snipResult.TokensFreed}
			if e.autoCompactor.NeedsCompaction(messages, e.model, extra) {
				compactParams := compact.CompactionParams{
					SystemPromptParts: systemPromptParts(params),
					UserContext:       params.UserContext,
					SystemContext:     params.SystemContext,
					QuerySource:       params.QuerySource,
				}
				if result, err := e.autoCompactor.Compact(ctx, messages, compactParams); err == nil {
					messages = result.SummaryMessages
				}
			}
		}
		// ────────────────────────────────────────────────────────────────────

		// Build the API request using the primary model.
		req, err := e.buildRequestWithModel(params, messages, e.model)
		if err != nil {
			sendError(ctx, msgCh, fmt.Errorf("engine: build request: %w", err))
			return
		}

		// Emit request-start event.
		select {
		case msgCh <- Msg{Type: MsgTypeRequestStart, Model: req.Model}:
		case <-ctx.Done():
			return
		}

		// Stream the LLM response.
		assistantMsg, toolCalls, stopReason, usage, streamErr := e.streamResponse(ctx, req, msgCh)
		if streamErr != nil {
			// If a context-window overflow error occurred and a fallback model is set,
			// retry the same turn with the fallback model.
			var apiErr *api.APIError
			if params.FallbackModel != "" && errors.As(streamErr, &apiErr) && apiErr.Kind == api.ErrKindContextWindow {
				fallbackReq, fbErr := e.buildRequestWithModel(params, messages, params.FallbackModel)
				if fbErr != nil {
					sendError(ctx, msgCh, fmt.Errorf("engine: build fallback request: %w", fbErr))
					return
				}
				select {
				case msgCh <- Msg{Type: MsgTypeRequestStart, Model: fallbackReq.Model}:
				case <-ctx.Done():
					return
				}
				assistantMsg, toolCalls, stopReason, usage, streamErr = e.streamResponse(ctx, fallbackReq, msgCh)
			}
			if streamErr != nil {
				sendError(ctx, msgCh, streamErr)
				return
			}
		}

		// Append the assistant message to the conversation.
		messages = append(messages, assistantMsg)

		// Update budget tracker.
		budgetDecision := budget.Check(usage.OutputTokens, req.MaxTokens)

		// Emit turn-complete event.
		select {
		case msgCh <- Msg{
			Type:               MsgTypeTurnComplete,
			StopReason:         stopReason,
			InputTokens:        usage.InputTokens,
			OutputTokens:       usage.OutputTokens,
			CacheReadTokens:    usage.CacheReadInputTokens,
			CacheCreatedTokens: usage.CacheCreationInputTokens,
		}:
		case <-ctx.Done():
			return
		}

		// Handle stop reasons.
		switch stopReason {
		case "end_turn", "stop_sequence", "":
			// Normal completion — fire stop hooks (non-blocking background goroutines).
			e.fireStopHooks(params, messages)
			return

		case "max_tokens":
			// Budget nudge logic.
			if budgetDecision.Action == "stop" {
				return
			}
			if budgetDecision.NudgeMessage != "" {
				// Inject a nudge user message and continue.
				nudgeMsg := types.Message{
					Role: types.RoleUser,
					Content: []types.ContentBlock{
						{Type: types.ContentTypeText, Text: strPtr(budgetDecision.NudgeMessage)},
					},
				}
				messages = append(messages, nudgeMsg)
			}
			continue

		case "tool_use":
			if len(toolCalls) == 0 {
				return
			}
			// Execute tools.
			batches := partitionToolCalls(toolCalls, e.registry)
			results, toolErr := runTools(ctx, batches, e.registry, params.ToolUseContext, msgCh, e.permChecker)
			if toolErr != nil {
				sendError(ctx, msgCh, toolErr)
				return
			}

			// Build tool_result user message.
			resultMsg := types.Message{
				Role:    types.RoleUser,
				Content: results,
			}
			messages = append(messages, resultMsg)

			select {
			case msgCh <- Msg{Type: MsgTypeUserMessage, UserMsg: &resultMsg}:
			case <-ctx.Done():
				return
			}

			// P1: Mid-turn drain checkpoint — inject queued user messages
			// between tool execution and the next LLM call.
			e.drainMidTurn(ctx, &messages, msgCh)

			// Continue the loop so the LLM can process tool results.
			continue

		default:
			// Unknown stop reason — treat as terminal.
			return
		}
	}
}

// streamResponse calls the LLM via streaming and emits Msg events.
// It returns the assistant message, parsed tool calls, and the final stop reason.
func (e *engineImpl) streamResponse(
	ctx context.Context,
	req *api.MessageRequest,
	msgCh chan<- Msg,
) (types.Message, []toolCall, string, api.Usage, error) {
	reader, err := e.client.Stream(ctx, req)
	if err != nil {
		return types.Message{}, nil, "", api.Usage{}, fmt.Errorf("engine: stream: %w", err)
	}
	defer reader.Close()

	// Emit stream-request-start.
	select {
	case msgCh <- Msg{Type: MsgTypeStreamRequestStart}:
	case <-ctx.Done():
		return types.Message{}, nil, "", api.Usage{}, ctx.Err()
	}

	acc := &api.Accumulator{}
	// Track tool_use blocks so we can emit streaming events.
	activeToolUseIDs := make(map[int]string)  // index → toolUseID
	activeToolNames := make(map[int]string)   // index → tool name

	for {
		ev, readErr := reader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return types.Message{}, nil, "", api.Usage{}, fmt.Errorf("engine: read stream: %w", readErr)
		}
		if ev == nil {
			continue
		}

		// Feed into accumulator.
		if accErr := acc.Process(ev); accErr != nil {
			return types.Message{}, nil, "", api.Usage{}, accErr
		}

		// Emit per-event Msg updates to the TUI.
		switch ev.Type {
		case api.EventContentBlockStart:
			var bs struct {
				Index        int              `json:"index"`
				ContentBlock api.ContentBlock `json:"content_block"`
			}
			if ev.Data != nil {
				if merr := json.Unmarshal(ev.Data, &bs); merr == nil {
					if bs.ContentBlock.Type == "tool_use" {
						activeToolUseIDs[bs.Index] = bs.ContentBlock.ID
						activeToolNames[bs.Index] = bs.ContentBlock.Name
						select {
						case msgCh <- Msg{
							Type:      MsgTypeToolUseStart,
							ToolUseID: bs.ContentBlock.ID,
							ToolName:  bs.ContentBlock.Name,
						}:
						case <-ctx.Done():
							return types.Message{}, nil, "", api.Usage{}, ctx.Err()
						}
					}
				}
			}

		case api.EventContentBlockDelta:
			if ev.ContentBlockDelta == nil {
				break
			}
			d := ev.ContentBlockDelta
			switch d.Delta.Type {
			case "text_delta":
				select {
				case msgCh <- Msg{Type: MsgTypeStreamText, TextDelta: d.Delta.Text}:
				case <-ctx.Done():
					return types.Message{}, nil, "", api.Usage{}, ctx.Err()
				}
			case "thinking_delta":
				select {
				case msgCh <- Msg{Type: MsgTypeThinkingDelta, TextDelta: d.Delta.Thinking}:
				case <-ctx.Done():
					return types.Message{}, nil, "", api.Usage{}, ctx.Err()
				}
			case "input_json_delta":
				if id, ok := activeToolUseIDs[d.Index]; ok {
					select {
					case msgCh <- Msg{
						Type:       MsgTypeToolUseInputDelta,
						ToolUseID:  id,
						ToolName:   activeToolNames[d.Index],
						InputDelta: d.Delta.PartialJSON,
					}:
					case <-ctx.Done():
						return types.Message{}, nil, "", api.Usage{}, ctx.Err()
					}
				}
			}

		case api.EventContentBlockStop:
			var bs struct {
				Index int `json:"index"`
			}
			if ev.Data != nil {
				if merr := json.Unmarshal(ev.Data, &bs); merr == nil {
					if id, ok := activeToolUseIDs[bs.Index]; ok {
						delete(activeToolUseIDs, bs.Index)
						delete(activeToolNames, bs.Index)
						// We'll emit ToolUseComplete after accumulator builds the input.
						_ = id
					}
				}
			}
		}
	}

	result := acc.Result()

	// Build the assistant message and emit it.
	assistantMsg := buildAssistantMessage(result)
	select {
	case msgCh <- Msg{Type: MsgTypeAssistantMessage, AssistantMsg: &assistantMsg}:
	case <-ctx.Done():
		return types.Message{}, nil, "", api.Usage{}, ctx.Err()
	}

	// Extract tool calls and emit ToolUseComplete events.
	var toolCalls []toolCall
	for _, blk := range result.Content {
		if blk.Type == "tool_use" {
			tc := toolCall{
				id:    blk.ID,
				name:  blk.Name,
				input: blk.Input,
			}
			toolCalls = append(toolCalls, tc)
			select {
			case msgCh <- Msg{
				Type:      MsgTypeToolUseComplete,
				ToolUseID: blk.ID,
				ToolName:  blk.Name,
				ToolInput: string(blk.Input),
			}:
			case <-ctx.Done():
				return types.Message{}, nil, "", api.Usage{}, ctx.Err()
			}
		}
	}

	return assistantMsg, toolCalls, result.StopReason, result.Usage, nil
}

// buildRequestWithModel constructs an api.MessageRequest using the given model.
func (e *engineImpl) buildRequestWithModel(params QueryParams, messages []types.Message, model string) (*api.MessageRequest, error) {
	maxTokens := e.maxTokens
	if params.MaxOutputTokensOverride > 0 {
		maxTokens = params.MaxOutputTokensOverride
	}

	// Convert types.Message to api.MessageParam.
	apiMessages := make([]api.MessageParam, 0, len(messages))
	for _, m := range messages {
		raw, err := json.Marshal(m.Content)
		if err != nil {
			return nil, fmt.Errorf("marshal message content: %w", err)
		}
		apiMessages = append(apiMessages, api.MessageParam{
			Role:    string(m.Role),
			Content: raw,
		})
	}

	// Build system prompt string.
	var systemParts []string
	for _, p := range params.SystemPrompt.Parts {
		if p.Text != "" {
			systemParts = append(systemParts, p.Text)
		}
	}
	systemStr := joinStrings(systemParts, "\n\n")

	// Build tool schemas, excluding any tools listed in ExcludeTools.
	allTools := e.registry.All()
	toolSchemas := make([]api.ToolSchema, 0, len(allTools))
	for _, t := range allTools {
		if len(params.ExcludeTools) > 0 && params.ExcludeTools[t.Name()] {
			continue
		}
		schema := t.InputSchema()
		schemaRaw, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("marshal tool schema for %s: %w", t.Name(), err)
		}
		var permCtx tools.PermissionContext
		if params.ToolUseContext != nil {
			permCtx = params.ToolUseContext.PermCtx
		}
		toolSchemas = append(toolSchemas, api.ToolSchema{
			Name:        t.Name(),
			Description: t.Description(nil, permCtx),
			InputSchema: schemaRaw,
		})
	}

	return &api.MessageRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Messages:    apiMessages,
		System:      systemStr,
		Tools:       toolSchemas,
		Stream:      true,
		QuerySource: params.QuerySource,
	}, nil
}

// buildAssistantMessage converts an api.MessageResponse into a types.Message.
func buildAssistantMessage(resp *api.MessageResponse) types.Message {
	content := make([]types.ContentBlock, 0, len(resp.Content))
	for _, blk := range resp.Content {
		cb := types.ContentBlock{Type: types.ContentBlockType(blk.Type)}
		switch blk.Type {
		case "text":
			cb.Text = strPtr(blk.Text)
		case "tool_use":
			cb.ID = strPtr(blk.ID)
			cb.Name = strPtr(blk.Name)
			// Unmarshal Input to map[string]any.
			if len(blk.Input) > 0 {
				var inputMap map[string]any
				if err := json.Unmarshal(blk.Input, &inputMap); err == nil {
					cb.Input = inputMap
				}
			}
		case "thinking":
			cb.Thinking = strPtr(blk.Thinking)
			cb.Signature = strPtr(blk.Signature)
		}
		content = append(content, cb)
	}
	return types.Message{
		Role:    types.RoleAssistant,
		Content: content,
	}
}

// sendError sends an error Msg on msgCh (non-blocking if ctx done).
func sendError(ctx context.Context, msgCh chan<- Msg, err error) {
	select {
	case msgCh <- Msg{Type: MsgTypeError, Err: err}:
	case <-ctx.Done():
	}
}

// sendSystem sends a system-text Msg on msgCh.
func sendSystem(ctx context.Context, msgCh chan<- Msg, text string) {
	select {
	case msgCh <- Msg{Type: MsgTypeSystemMessage, SystemText: text}:
	case <-ctx.Done():
	}
}

// joinStrings concatenates non-empty strings with sep.
func joinStrings(parts []string, sep string) string {
	result := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if result != "" {
			result += sep
		}
		result += p
	}
	return result
}

// systemPromptParts extracts the text parts from a QueryParams system prompt.
func systemPromptParts(params QueryParams) []string {
	parts := make([]string, 0, len(params.SystemPrompt.Parts))
	for _, p := range params.SystemPrompt.Parts {
		if p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return parts
}

// drainMidTurn checks the message queue for pending user messages and injects
// them into the conversation before the next LLM call. This lets the LLM see
// user corrections/additions between tool calls within the same turn.
//
// Only ModePrompt commands are consumed mid-turn; slash commands require TUI
// routing and are left in the queue for between-turn processing.
func (e *engineImpl) drainMidTurn(
	ctx context.Context,
	messages *[]types.Message,
	msgCh chan<- Msg,
) {
	if e.msgQueue == nil {
		return
	}

	agentID := "" // main session; sub-agent support deferred to P2
	pending := e.msgQueue.GetByMaxPriority(msgqueue.PriorityNext, agentID)
	if len(pending) == 0 {
		return
	}

	// Only drain ModePrompt commands mid-turn.
	var toInject []msgqueue.QueuedCommand
	for _, cmd := range pending {
		if cmd.Mode == msgqueue.ModePrompt {
			toInject = append(toInject, cmd)
		}
	}
	if len(toInject) == 0 {
		return
	}

	// Remove from queue before injecting.
	ids := make([]string, len(toInject))
	for i, cmd := range toInject {
		ids[i] = cmd.ID
	}
	e.msgQueue.RemoveByIDs(ids)

	// Combine into one user message.
	var parts []string
	for _, cmd := range toInject {
		parts = append(parts, cmd.Value)
	}
	combined := strings.Join(parts, "\n\n")

	userMsg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr(combined)},
		},
	}
	*messages = append(*messages, userMsg)

	// Notify TUI that a user message was injected.
	select {
	case msgCh <- Msg{Type: MsgTypeUserMessage, UserMsg: &userMsg}:
	case <-ctx.Done():
	}
}
