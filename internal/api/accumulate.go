package api

import (
	"fmt"
	"sync"
)

// Accumulator aggregates a stream of SSE events into a complete MessageResponse.
// Thread-safe: Process and Result may be called from different goroutines.
type Accumulator struct {
	mu      sync.Mutex
	message MessageResponse
	// Track per-index content blocks for delta accumulation
	blocks map[int]*ContentBlock
}

// Process incorporates a single SSE event into the accumulated message.
func (a *Accumulator) Process(ev *StreamEvent) error {
	if ev == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	switch ev.Type {
	case EventMessageStart:
		if ev.MessageStart != nil {
			a.message = ev.MessageStart.Message
			a.blocks = make(map[int]*ContentBlock)
		}

	case EventContentBlockStart:
		// Parse the block start: {"index": N, "content_block": {...}}
		var bs struct {
			Index        int          `json:"index"`
			ContentBlock ContentBlock `json:"content_block"`
		}
		if ev.Data != nil {
			if err := jsonUnmarshal(ev.Data, &bs); err == nil {
				if a.blocks == nil {
					a.blocks = make(map[int]*ContentBlock)
				}
				cb := bs.ContentBlock
				a.blocks[bs.Index] = &cb
			}
		}

	case EventContentBlockDelta:
		if ev.ContentBlockDelta == nil {
			break
		}
		delta := ev.ContentBlockDelta
		if a.blocks == nil {
			break
		}
		block, ok := a.blocks[delta.Index]
		if !ok {
			break
		}
		switch delta.Delta.Type {
		case "text_delta":
			block.Text += delta.Delta.Text
		case "input_json_delta":
			// Accumulate partial JSON for tool_use input
			if block.Input == nil {
				block.Input = []byte(`""`)
			}
			// Accumulate raw string parts; finalisation is caller's responsibility
			block.Text += delta.Delta.PartialJSON
		case "thinking_delta":
			block.Thinking += delta.Delta.Thinking
		}

	case EventContentBlockStop:
		// Finalise the block: move from blocks map to message.Content
		var bs struct {
			Index int `json:"index"`
		}
		if ev.Data != nil {
			if err := jsonUnmarshal(ev.Data, &bs); err == nil {
				if block, ok := a.blocks[bs.Index]; ok {
					// For tool_use blocks, block.Text holds accumulated partial JSON
					if block.Type == "tool_use" && len(block.Text) > 0 {
						block.Input = []byte(block.Text)
						block.Text = ""
					}
					a.message.Content = append(a.message.Content, *block)
					delete(a.blocks, bs.Index)
				}
			}
		}

	case EventMessageDelta:
		if ev.MessageDelta != nil {
			a.message.StopReason = ev.MessageDelta.Delta.StopReason
			// Merge usage delta
			a.message.Usage.OutputTokens += ev.MessageDelta.Usage.OutputTokens
		}

	case EventMessageStop:
		// Nothing extra needed; stream termination is handled by sseReader.Next()

	case EventError:
		if ev.Error != nil {
			return fmt.Errorf("api: stream error: %s", ev.Error.Message)
		}
	}
	return nil
}

// Result returns the accumulated MessageResponse. Safe to call after the stream ends.
func (a *Accumulator) Result() *MessageResponse {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Flush any remaining blocks (shouldn't happen in normal flow)
	if a.blocks != nil {
		for _, block := range a.blocks {
			a.message.Content = append(a.message.Content, *block)
		}
		a.blocks = nil
	}
	msg := a.message
	return &msg
}
