package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// buildFakeHTTPResponse constructs a minimal *http.Response with a string body.
func buildFakeHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestParseSSEEvent_MessageStart(t *testing.T) {
	data := `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`
	ev, err := parseSSEEvent("message_start", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventMessageStart {
		t.Errorf("expected EventMessageStart, got %q", ev.Type)
	}
	if ev.MessageStart == nil {
		t.Fatal("MessageStart should be parsed")
	}
	if ev.MessageStart.Message.ID != "msg_1" {
		t.Errorf("unexpected message ID: %q", ev.MessageStart.Message.ID)
	}
}

func TestParseSSEEvent_ContentBlockDelta_TextDelta(t *testing.T) {
	data := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`
	ev, err := parseSSEEvent("content_block_delta", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ContentBlockDelta == nil {
		t.Fatal("ContentBlockDelta should be parsed")
	}
	if ev.ContentBlockDelta.Delta.Text != "Hello" {
		t.Errorf("unexpected text: %q", ev.ContentBlockDelta.Delta.Text)
	}
}

func TestParseSSEEvent_MessageStop(t *testing.T) {
	data := `{"type":"message_stop"}`
	ev, err := parseSSEEvent("message_stop", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventMessageStop {
		t.Errorf("expected EventMessageStop, got %q", ev.Type)
	}
}

func TestParseSSEEvent_Ping(t *testing.T) {
	data := `{"type":"ping"}`
	ev, err := parseSSEEvent("ping", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventPing {
		t.Errorf("expected EventPing, got %q", ev.Type)
	}
}

func TestParseSSEEvent_Error(t *testing.T) {
	data := `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	ev, err := parseSSEEvent("error", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventError {
		t.Errorf("expected EventError, got %q", ev.Type)
	}
}

func TestParseSSEEvent_EmptyData(t *testing.T) {
	ev, err := parseSSEEvent("ping", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventPing {
		t.Errorf("expected type from event header, got %q", ev.Type)
	}
}

// ─── sseReader.Next ──────────────────────────────────────────────────────────

func TestSSEReader_Next_SimpleStream(t *testing.T) {
	stream := `event: message_start
data: {"type":"message_start","message":{"id":"msg_x","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":5,"output_tokens":0}}}

event: message_stop
data: {"type":"message_stop"}

`
	resp := buildFakeHTTPResponse(stream)
	r := newSSEReader(resp)
	defer r.Close()

	ev1, err := r.Next()
	if err != nil {
		t.Fatalf("expected first event, got error: %v", err)
	}
	if ev1.Type != EventMessageStart {
		t.Errorf("expected message_start, got %q", ev1.Type)
	}

	ev2, err := r.Next()
	if err != nil {
		t.Fatalf("expected second event, got error: %v", err)
	}
	if ev2.Type != EventMessageStop {
		t.Errorf("expected message_stop, got %q", ev2.Type)
	}
}

func TestSSEReader_Next_EOF(t *testing.T) {
	stream := `event: message_stop
data: {"type":"message_stop"}

`
	resp := buildFakeHTTPResponse(stream)
	r := newSSEReader(resp)
	defer r.Close()

	_, _ = r.Next() // consume message_stop (sets done=true)
	_, err := r.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF after message_stop, got %v", err)
	}
}

func TestSSEReader_Next_PingSkipped(t *testing.T) {
	stream := `event: ping
data: {"type":"ping"}

event: message_stop
data: {"type":"message_stop"}

`
	resp := buildFakeHTTPResponse(stream)
	r := newSSEReader(resp)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ping events may be returned or skipped; just ensure we eventually get message_stop
	if ev.Type == EventPing {
		ev, err = r.Next()
		if err != nil {
			t.Fatalf("unexpected error after ping: %v", err)
		}
	}
	if ev.Type != EventMessageStop {
		t.Errorf("expected message_stop, got %q", ev.Type)
	}
}

func TestSSEReader_Next_ContentBlockDelta(t *testing.T) {
	stream := `event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}

event: message_stop
data: {"type":"message_stop"}

`
	resp := buildFakeHTTPResponse(stream)
	r := newSSEReader(resp)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventContentBlockDelta {
		t.Errorf("expected content_block_delta, got %q", ev.Type)
	}
	if ev.ContentBlockDelta == nil || ev.ContentBlockDelta.Delta.Text != "world" {
		t.Errorf("unexpected delta: %+v", ev.ContentBlockDelta)
	}
}
