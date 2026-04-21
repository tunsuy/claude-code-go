package api

import (
	"encoding/json"
	"testing"
)

// helper: build a StreamEvent from raw event type and JSON data string.
func makeEvent(evType StreamEventType, data string) *StreamEvent {
	ev := &StreamEvent{Type: evType}
	if data != "" {
		ev.Data = json.RawMessage(data)
	}
	// Parse typed fields the same way parseSSEEvent does
	parsed, err := parseSSEEvent(string(evType), data)
	if err != nil {
		return ev
	}
	return parsed
}

func TestAccumulator_NilEvent(t *testing.T) {
	var a Accumulator
	if err := a.Process(nil); err != nil {
		t.Errorf("Process(nil) returned error: %v", err)
	}
}

func TestAccumulator_MessageStart(t *testing.T) {
	var a Accumulator
	data := `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`
	ev := makeEvent(EventMessageStart, data)
	if err := a.Process(ev); err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	result := a.Result()
	if result.ID != "msg_1" {
		t.Errorf("unexpected ID: %q", result.ID)
	}
	if result.Model != "claude-3" {
		t.Errorf("unexpected model: %q", result.Model)
	}
}

func TestAccumulator_TextDelta(t *testing.T) {
	var a Accumulator

	// Start a message
	startData := `{"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":5,"output_tokens":0}}}`
	_ = a.Process(makeEvent(EventMessageStart, startData))

	// Start content block index 0 (text block)
	cbStartData := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
	_ = a.Process(makeEvent(EventContentBlockStart, cbStartData))

	// Two text deltas
	delta1 := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`
	delta2 := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" World"}}`
	_ = a.Process(makeEvent(EventContentBlockDelta, delta1))
	_ = a.Process(makeEvent(EventContentBlockDelta, delta2))

	// Stop content block
	cbStopData := `{"type":"content_block_stop","index":0}`
	_ = a.Process(makeEvent(EventContentBlockStop, cbStopData))

	result := a.Result()
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", result.Content[0].Text)
	}
}

func TestAccumulator_ToolUseDelta(t *testing.T) {
	var a Accumulator

	startData := `{"type":"message_start","message":{"id":"msg_3","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":5,"output_tokens":0}}}`
	_ = a.Process(makeEvent(EventMessageStart, startData))

	// Tool use block
	cbStartData := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"bash"}}`
	_ = a.Process(makeEvent(EventContentBlockStart, cbStartData))

	// Partial JSON deltas
	delta1 := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\""}}`
	delta2 := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":":\"ls\"}"}}`
	_ = a.Process(makeEvent(EventContentBlockDelta, delta1))
	_ = a.Process(makeEvent(EventContentBlockDelta, delta2))

	cbStopData := `{"type":"content_block_stop","index":0}`
	_ = a.Process(makeEvent(EventContentBlockStop, cbStopData))

	result := a.Result()
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	block := result.Content[0]
	if block.Type != "tool_use" {
		t.Errorf("expected tool_use, got %q", block.Type)
	}
	// Input should be the accumulated JSON
	if string(block.Input) == "" {
		t.Error("expected non-empty Input")
	}
}

func TestAccumulator_MessageDelta(t *testing.T) {
	var a Accumulator

	startData := `{"type":"message_start","message":{"id":"msg_4","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":5,"output_tokens":0}}}`
	_ = a.Process(makeEvent(EventMessageStart, startData))

	// message_delta carries stop_reason and usage
	msgDelta := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":25}}`
	_ = a.Process(makeEvent(EventMessageDelta, msgDelta))

	result := a.Result()
	if result.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %q", result.StopReason)
	}
	if result.Usage.OutputTokens != 25 {
		t.Errorf("expected 25 output tokens, got %d", result.Usage.OutputTokens)
	}
}

func TestAccumulator_ErrorEvent(t *testing.T) {
	var a Accumulator
	errData := `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	ev := makeEvent(EventError, errData)
	err := a.Process(ev)
	if err == nil {
		t.Error("expected error from EventError, got nil")
	}
}

func TestAccumulator_MessageStop(t *testing.T) {
	var a Accumulator
	// message_stop should be a no-op without error
	ev := &StreamEvent{Type: EventMessageStop}
	if err := a.Process(ev); err != nil {
		t.Errorf("Process(message_stop) returned error: %v", err)
	}
}

func TestAccumulator_ResultFlushesRemainingBlocks(t *testing.T) {
	var a Accumulator
	startData := `{"type":"message_start","message":{"id":"msg_5","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":5,"output_tokens":0}}}`
	_ = a.Process(makeEvent(EventMessageStart, startData))

	// Start block but never stop it
	cbStartData := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"incomplete"}}`
	_ = a.Process(makeEvent(EventContentBlockStart, cbStartData))

	// Result() should flush remaining blocks
	result := a.Result()
	if len(result.Content) != 1 {
		t.Errorf("expected flushed block, got %d blocks", len(result.Content))
	}
}
