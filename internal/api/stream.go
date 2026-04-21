package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamEventType corresponds to the Anthropic SSE event type field.
type StreamEventType string

const (
	EventMessageStart      StreamEventType = "message_start"
	EventContentBlockStart StreamEventType = "content_block_start"
	EventContentBlockDelta StreamEventType = "content_block_delta"
	EventContentBlockStop  StreamEventType = "content_block_stop"
	EventMessageDelta      StreamEventType = "message_delta"
	EventMessageStop       StreamEventType = "message_stop"
	EventPing              StreamEventType = "ping"
	EventError             StreamEventType = "error"
)

// StreamEvent is a parsed SSE event.
// Data is kept as json.RawMessage for deferred parsing.
type StreamEvent struct {
	Type StreamEventType `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
	// Parsed typed fields (mutually exclusive)
	MessageStart      *MessageStartData      `json:"-"`
	ContentBlockDelta *ContentBlockDeltaData `json:"-"`
	MessageDelta      *MessageDeltaData      `json:"-"`
	Error             *APIErrorData          `json:"-"`
}

// MessageStartData carries the initial message metadata.
type MessageStartData struct {
	Message MessageResponse `json:"message"`
}

// ContentBlockDeltaData carries an incremental content delta.
type ContentBlockDeltaData struct {
	Index int   `json:"index"`
	Delta Delta `json:"delta"`
}

// Delta is an incremental content update.
type Delta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
}

// MessageDeltaData carries final message metadata (stop_reason, usage).
type MessageDeltaData struct {
	Delta struct {
		StopReason   string `json:"stop_reason"`
		StopSequence string `json:"stop_sequence"`
	} `json:"delta"`
	Usage Usage `json:"usage"`
}

// sseReader implements StreamReader via bufio.Scanner over an HTTP response.
type sseReader struct {
	resp    *http.Response
	scanner *bufio.Scanner
	done    bool
}

func newSSEReader(resp *http.Response) *sseReader {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &sseReader{resp: resp, scanner: scanner}
}

// Next returns the next SSE event from the stream.
// Returns (nil, io.EOF) when the stream ends normally (event: message_stop).
// Does NOT check for "[DONE]" sentinel (that is OpenAI convention, not Anthropic).
func (r *sseReader) Next() (*StreamEvent, error) {
	if r.done {
		return nil, io.EOF
	}

	var eventType string
	var dataLine string

	for r.scanner.Scan() {
		line := r.scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			continue
		}

		if line == "" && (eventType != "" || dataLine != "") {
			// End of event block — parse and return
			ev, err := parseSSEEvent(eventType, dataLine)
			if err != nil {
				return nil, err
			}
			if ev.Type == EventMessageStop {
				r.done = true
				return ev, nil
			}
			return ev, nil
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, fmt.Errorf("sse: scan error: %w", err)
	}
	return nil, io.EOF
}

// parseSSEEvent converts raw event type + data strings into a *StreamEvent.
func parseSSEEvent(eventType, data string) (*StreamEvent, error) {
	ev := &StreamEvent{}

	// If the event type is embedded in the data JSON (Anthropic sends `"type"` in data)
	if data != "" {
		if err := json.Unmarshal([]byte(data), ev); err != nil {
			// Fallback: set type from event: header
			if eventType != "" {
				ev.Type = StreamEventType(eventType)
			}
			ev.Data = json.RawMessage(data)
		}
	} else if eventType != "" {
		ev.Type = StreamEventType(eventType)
	}

	// Override type from event: header if data type is missing
	if ev.Type == "" && eventType != "" {
		ev.Type = StreamEventType(eventType)
	}

	ev.Data = json.RawMessage(data)

	// Parse typed sub-fields
	switch ev.Type {
	case EventMessageStart:
		ev.MessageStart = &MessageStartData{}
		if err := json.Unmarshal([]byte(data), ev.MessageStart); err != nil {
			ev.MessageStart = nil
		}
	case EventContentBlockDelta:
		ev.ContentBlockDelta = &ContentBlockDeltaData{}
		if err := json.Unmarshal([]byte(data), ev.ContentBlockDelta); err != nil {
			ev.ContentBlockDelta = nil
		}
	case EventMessageDelta:
		ev.MessageDelta = &MessageDeltaData{}
		if err := json.Unmarshal([]byte(data), ev.MessageDelta); err != nil {
			ev.MessageDelta = nil
		}
	case EventError:
		ev.Error = &APIErrorData{}
		if err := json.Unmarshal([]byte(data), ev.Error); err != nil {
			ev.Error = nil
		}
	}

	return ev, nil
}

// Close closes the underlying HTTP response body.
func (r *sseReader) Close() error {
	r.done = true
	return r.resp.Body.Close()
}
