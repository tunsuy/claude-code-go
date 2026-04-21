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
}

// accumulatedToolCall tracks tool call data being streamed incrementally.
type accumulatedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

// newOpenAISSEReader creates a new SSE reader for OpenAI streaming responses.
func newOpenAISSEReader(resp *http.Response) *openaiSSEReader {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &openaiSSEReader{
		resp:             resp,
		scanner:          scanner,
		toolCalls:        make(map[int]*accumulatedToolCall),
		toolBlockStarted: make(map[int]bool),
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
			r.done = true
			// Send content_block_stop for text if we started one
			if r.textBlockStarted {
				r.pendingEvents = append(r.pendingEvents, r.createContentBlockStopEvent(0))
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
			continue // Skip malformed chunks
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
			return r.createMessageStartEvent(&chunk), nil
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
				// Queue message_delta event
				r.pendingEvents = append(r.pendingEvents, r.createMessageDeltaEvent(&choice))
				// Return first pending event
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}

			// Process delta content
			if choice.Delta.Content != "" {
				// Send content_block_start first if not done yet
				if !r.textBlockStarted {
					r.textBlockStarted = true
					r.pendingEvents = append(r.pendingEvents, r.createContentDeltaEvent(choice.Delta.Content))
					return r.createContentBlockStartEvent(0, "text"), nil
				}
				return r.createContentDeltaEvent(choice.Delta.Content), nil
			}

			// Process tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				return r.processToolCallDelta(&choice.Delta.ToolCalls[0], choice.Index)
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
func (r *openaiSSEReader) processToolCallDelta(tc *openaiToolCall, index int) (*StreamEvent, error) {
	// Get or create accumulated tool call
	acc, exists := r.toolCalls[index]
	if !exists {
		acc = &accumulatedToolCall{}
		r.toolCalls[index] = acc
	}

	// Update accumulated data
	if tc.ID != "" {
		acc.ID = tc.ID
	}
	if tc.Function.Name != "" {
		acc.Name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		acc.Arguments.WriteString(tc.Function.Arguments)
	}

	// Return a partial JSON delta (similar to Anthropic's input_json_delta)
	delta := &ContentBlockDeltaData{
		Index: index + 1, // Offset by 1 since index 0 is text
		Delta: Delta{
			Type:        "input_json_delta",
			PartialJSON: tc.Function.Arguments,
		},
	}
	data, _ := json.Marshal(delta)
	return &StreamEvent{
		Type:              EventContentBlockDelta,
		Data:              data,
		ContentBlockDelta: delta,
	}, nil
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
