package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// openaiSSEReader implements StreamReader for OpenAI streaming responses.
// It translates OpenAI SSE events to the Anthropic StreamEvent format.
type openaiSSEReader struct {
	resp             *http.Response
	scanner          *bufio.Scanner
	done             bool
	messageID        string
	model            string
	started          bool
	textBlockStarted bool                         // whether we've sent content_block_start for text
	toolCalls        map[int]*accumulatedToolCall // index -> accumulated tool call
	toolBlockStarted map[int]bool                 // whether we've sent content_block_start for each tool
	inputUsage       int                          // accumulated from usage chunks
	pendingEvents    []*StreamEvent               // events to return before processing more chunks
	debugLogger      *DebugLogger                 // debug logger for SSE event tracing
}

// accumulatedToolCall tracks tool call data being streamed incrementally.
type accumulatedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

// newOpenAISSEReader creates a new SSE reader for OpenAI streaming responses.
func newOpenAISSEReader(resp *http.Response, dl *DebugLogger) *openaiSSEReader {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &openaiSSEReader{
		resp:             resp,
		scanner:          scanner,
		toolCalls:        make(map[int]*accumulatedToolCall),
		toolBlockStarted: make(map[int]bool),
		debugLogger:      dl,
	}
}

// Next returns the next SSE event from the OpenAI stream, converted to Anthropic format.
// Returns (nil, io.EOF) when the stream ends normally.
func (r *openaiSSEReader) Next() (*StreamEvent, error) {
	// Return any pending events first (even if done)
	if len(r.pendingEvents) > 0 {
		ev := r.pendingEvents[0]
		r.pendingEvents = r.pendingEvents[1:]
		return ev, nil
	}

	if r.done {
		return nil, io.EOF
	}

	for r.scanner.Scan() {
		line := r.scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for data prefix
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end sentinel
		if data == "[DONE]" {
			r.debugLogger.LogSSEEvent("DONE", "[DONE]")
			r.done = true
			// Send content_block_stop for text if we started one
			if r.textBlockStarted {
				r.pendingEvents = append(r.pendingEvents, r.createContentBlockStopEvent(0))
			}
			// Send content_block_stop for each tool block
			for toolIndex := range r.toolBlockStarted {
				blockIndex := toolIndex + 1
				if !r.textBlockStarted {
					blockIndex = toolIndex
				}
				r.pendingEvents = append(r.pendingEvents, r.createContentBlockStopEvent(blockIndex))
			}
			// Send message_stop event
			r.pendingEvents = append(r.pendingEvents, &StreamEvent{
				Type: EventMessageStop,
			})
			// Return first pending event
			if len(r.pendingEvents) > 0 {
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}
			return nil, io.EOF
		}

		// Parse the chunk
		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			r.debugLogger.LogError("SSE chunk parse error", err)
			continue // Skip malformed chunks
		}

		// Log the SSE event data.
		r.debugLogger.LogSSEEvent("chunk", data)

		// Debug: log tool calls in chunk.
		if r.debugLogger.Enabled() && len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			tc := chunk.Choices[0].Delta.ToolCalls[0]
			r.debugLogger.Logf("Stream tool call chunk: Index=%d, ID=%q, Name=%q, Args=%q",
				tc.Index, tc.ID, tc.Function.Name, tc.Function.Arguments)
		}

		// Store message metadata
		if chunk.ID != "" {
			r.messageID = chunk.ID
		}
		if chunk.Model != "" {
			r.model = chunk.Model
		}

		// Track usage if provided
		if chunk.Usage != nil {
			r.inputUsage = chunk.Usage.PromptTokens
		}

		// Send message_start on first chunk
		if !r.started {
			r.started = true
			// Queue message_start event and continue processing the same chunk
			r.pendingEvents = append(r.pendingEvents, r.createMessageStartEvent(&chunk))
		}

		// Process choices
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]

			// Check for finish
			if choice.FinishReason != "" {
				// Send content_block_stop for text if we started one
				if r.textBlockStarted {
					r.pendingEvents = append(r.pendingEvents, r.createContentBlockStopEvent(0))
				}
				// Send content_block_stop for each tool block
				for toolIndex := range r.toolBlockStarted {
					blockIndex := toolIndex + 1
					if !r.textBlockStarted {
						blockIndex = toolIndex
					}
					r.pendingEvents = append(r.pendingEvents, r.createContentBlockStopEvent(blockIndex))
				}
				// Queue message_delta event
				r.pendingEvents = append(r.pendingEvents, r.createMessageDeltaEvent(&choice))
				// Return first pending event
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}

			// Process delta content
			if choice.Delta.Content != "" {
				if !r.textBlockStarted {
					r.textBlockStarted = true
					r.pendingEvents = append(r.pendingEvents, r.createContentBlockStartEvent(0, "text"))
				}
				r.pendingEvents = append(r.pendingEvents, r.createContentDeltaEvent(choice.Delta.Content))
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}

			// Process tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				r.processToolCallDelta(&choice.Delta.ToolCalls[0])
				if len(r.pendingEvents) > 0 {
					retEv := r.pendingEvents[0]
					r.pendingEvents = r.pendingEvents[1:]
					return retEv, nil
				}
				continue
			}

			// No content or tool calls, but we may have pending events (e.g., message_start)
			if len(r.pendingEvents) > 0 {
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// createMessageStartEvent creates the initial message_start event.
func (r *openaiSSEReader) createMessageStartEvent(_ *openaiStreamChunk) *StreamEvent {
	msgStart := &MessageStartData{
		Message: MessageResponse{
			ID:    r.messageID,
			Type:  "message",
			Role:  "assistant",
			Model: r.model,
			Usage: Usage{
				InputTokens: r.inputUsage,
			},
		},
	}
	data, _ := json.Marshal(msgStart)
	return &StreamEvent{
		Type:         EventMessageStart,
		Data:         data,
		MessageStart: msgStart,
	}
}

// createContentBlockStartEvent creates a content_block_start event.
func (r *openaiSSEReader) createContentBlockStartEvent(index int, blockType string) *StreamEvent {
	blockData := struct {
		Index        int          `json:"index"`
		ContentBlock ContentBlock `json:"content_block"`
	}{
		Index: index,
		ContentBlock: ContentBlock{
			Type: blockType,
		},
	}
	data, _ := json.Marshal(blockData)
	return &StreamEvent{
		Type: EventContentBlockStart,
		Data: data,
	}
}

// createContentBlockStopEvent creates a content_block_stop event.
func (r *openaiSSEReader) createContentBlockStopEvent(index int) *StreamEvent {
	stopData := struct {
		Index int `json:"index"`
	}{
		Index: index,
	}
	data, _ := json.Marshal(stopData)
	return &StreamEvent{
		Type: EventContentBlockStop,
		Data: data,
	}
}

// createContentDeltaEvent creates a content_block_delta event for text content.
func (r *openaiSSEReader) createContentDeltaEvent(text string) *StreamEvent {
	delta := &ContentBlockDeltaData{
		Index: 0,
		Delta: Delta{
			Type: "text_delta",
			Text: text,
		},
	}
	data, _ := json.Marshal(delta)
	return &StreamEvent{
		Type:              EventContentBlockDelta,
		Data:              data,
		ContentBlockDelta: delta,
	}
}

// processToolCallDelta processes incremental tool call data.
func (r *openaiSSEReader) processToolCallDelta(tc *openaiToolCall) {
	toolIndex := tc.Index

	acc, exists := r.toolCalls[toolIndex]
	if !exists {
		acc = &accumulatedToolCall{}
		r.toolCalls[toolIndex] = acc
	}

	if tc.ID != "" {
		acc.ID = tc.ID
	}
	if tc.Function.Name != "" {
		acc.Name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		acc.Arguments.WriteString(tc.Function.Arguments)
	}

	blockIndex := toolIndex + 1
	if !r.textBlockStarted {
		blockIndex = toolIndex
	}

	if !r.toolBlockStarted[toolIndex] {
		r.toolBlockStarted[toolIndex] = true
		r.pendingEvents = append(r.pendingEvents, r.createToolUseBlockStartEvent(blockIndex, acc.ID, acc.Name))
	}

	if tc.Function.Arguments != "" {
		r.pendingEvents = append(r.pendingEvents, r.createToolCallDeltaEvent(blockIndex, tc.Function.Arguments))
	}
}

// createToolCallDeltaEvent creates a content_block_delta event for tool input JSON.
func (r *openaiSSEReader) createToolCallDeltaEvent(index int, partialJSON string) *StreamEvent {
	delta := &ContentBlockDeltaData{
		Index: index,
		Delta: Delta{
			Type:        "input_json_delta",
			PartialJSON: partialJSON,
		},
	}
	data, _ := json.Marshal(delta)
	return &StreamEvent{
		Type:              EventContentBlockDelta,
		Data:              data,
		ContentBlockDelta: delta,
	}
}

// createToolUseBlockStartEvent creates a content_block_start event for tool use.
func (r *openaiSSEReader) createToolUseBlockStartEvent(index int, id, name string) *StreamEvent {
	blockData := struct {
		Index        int          `json:"index"`
		ContentBlock ContentBlock `json:"content_block"`
	}{
		Index: index,
		ContentBlock: ContentBlock{
			Type: "tool_use",
			ID:   id,
			Name: name,
		},
	}
	data, _ := json.Marshal(blockData)
	return &StreamEvent{
		Type: EventContentBlockStart,
		Data: data,
	}
}

// createMessageDeltaEvent creates the final message_delta event with stop reason.
func (r *openaiSSEReader) createMessageDeltaEvent(choice *openaiStreamChoice) *StreamEvent {
	stopReason := r.convertFinishReason(choice.FinishReason)
	msgDelta := &MessageDeltaData{
		Usage: Usage{
			InputTokens: r.inputUsage,
		},
	}
	msgDelta.Delta.StopReason = stopReason
	data, _ := json.Marshal(msgDelta)
	return &StreamEvent{
		Type:         EventMessageDelta,
		Data:         data,
		MessageDelta: msgDelta,
	}
}

// convertFinishReason converts OpenAI finish_reason to Anthropic stop_reason.
func (r *openaiSSEReader) convertFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn" // Map content filter to end_turn
	default:
		return reason
	}
}

// Close closes the underlying HTTP response body.
func (r *openaiSSEReader) Close() error {
	r.done = true
	return r.resp.Body.Close()
}
