//go:build integration

// Package integration contains CLI end-to-end tests that exercise the full
// bootstrap → headlessRun → engine pipeline without real API calls.
// Run with: go test -race -tags=integration ./test/integration/...
package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/bootstrap"
)

// ─────────────────────────────────────────────────────────────────────────────
// stdout capture helper
// ─────────────────────────────────────────────────────────────────────────────

// captureStdout replaces os.Stdout with an in-process pipe, calls fn, then
// restores os.Stdout and returns everything that was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	// Drain the read-end in a goroutine so the write never blocks.
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		io.Copy(&buf, r) //nolint:errcheck
	}()

	fn()

	// Close the write end so the reader goroutine sees EOF.
	w.Close()
	<-done
	r.Close()

	os.Stdout = origStdout
	return buf.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// container builder helper
// ─────────────────────────────────────────────────────────────────────────────

// buildTestContainer creates an AppContainer wired with the given mock client.
// HomeDir and WorkingDir are set to temporary directories.
func buildTestContainer(t *testing.T, client api.Client) *bootstrap.AppContainer {
	t.Helper()

	opts := bootstrap.ContainerOptions{
		HomeDir:    t.TempDir(),
		WorkingDir: t.TempDir(),
	}
	container, err := bootstrap.BuildContainerWithClient(opts, client)
	if err != nil {
		t.Fatalf("BuildContainerWithClient: %v", err)
	}
	return container
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCLI_PlainText — single end_turn response, no tool calls
// ─────────────────────────────────────────────────────────────────────────────

func TestCLI_PlainText(t *testing.T) {
	const want = "Hello from the mock LLM"

	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(buildEndTurnEvents(want)...), nil
		},
	}

	container := buildTestContainer(t, client)

	output := captureStdout(t, func() {
		if err := bootstrap.RunHeadless(container, "say hello", "text", 0); err != nil {
			t.Errorf("RunHeadless returned error: %v", err)
		}
	})

	if !strings.Contains(output, want) {
		t.Errorf("stdout %q does not contain expected text %q", output, want)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCLI_ToolCall_FileRead — one tool_use (Read), then end_turn
// ─────────────────────────────────────────────────────────────────────────────

func TestCLI_ToolCall_FileRead(t *testing.T) {
	// Write a temp file that the Read tool will read.
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/hello.txt"
	if err := os.WriteFile(tmpFile, []byte("file content XYZ"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	const finalText = "I read the file successfully"

	callN := 0
	var mu sync.Mutex

	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			mu.Lock()
			callN++
			n := callN
			mu.Unlock()

			switch n {
			case 1:
				// First LLM call: request a Read tool use.
				input, _ := json.Marshal(map[string]string{"file_path": tmpFile})
				return newStaticReader(
					buildToolUseEvents("toolu_read_01", "Read", string(input))...,
				), nil
			default:
				// Second LLM call (after tool result): return end_turn.
				return newStaticReader(buildEndTurnEvents(finalText)...), nil
			}
		},
	}

	container := buildTestContainer(t, client)

	output := captureStdout(t, func() {
		if err := bootstrap.RunHeadless(container, "read the file", "text", 5); err != nil {
			t.Errorf("RunHeadless returned error: %v", err)
		}
	})

	if !strings.Contains(output, finalText) {
		t.Errorf("stdout %q does not contain expected final text %q", output, finalText)
	}

	mu.Lock()
	n := callN
	mu.Unlock()
	if n < 2 {
		t.Errorf("expected ≥2 LLM calls (tool_use + end_turn), got %d", n)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCLI_MultiTurn_ToolCall — two consecutive tool calls, then end_turn
// ─────────────────────────────────────────────────────────────────────────────

func TestCLI_MultiTurn_ToolCall(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := tmpDir + "/a.txt"
	file2 := tmpDir + "/b.txt"
	if err := os.WriteFile(file1, []byte("content-a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("content-b"), 0o644); err != nil {
		t.Fatal(err)
	}

	const finalText = "multi-turn complete"

	callN := 0
	var mu sync.Mutex

	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			mu.Lock()
			callN++
			n := callN
			mu.Unlock()

			switch n {
			case 1:
				input, _ := json.Marshal(map[string]string{"file_path": file1})
				return newStaticReader(
					buildToolUseEvents("toolu_mt_01", "Read", string(input))...,
				), nil
			case 2:
				input, _ := json.Marshal(map[string]string{"file_path": file2})
				return newStaticReader(
					buildToolUseEvents("toolu_mt_02", "Read", string(input))...,
				), nil
			default:
				return newStaticReader(buildEndTurnEvents(finalText)...), nil
			}
		},
	}

	container := buildTestContainer(t, client)

	output := captureStdout(t, func() {
		if err := bootstrap.RunHeadless(container, "read both files", "text", 10); err != nil {
			t.Errorf("RunHeadless returned error: %v", err)
		}
	})

	if !strings.Contains(output, finalText) {
		t.Errorf("stdout %q does not contain expected final text %q", output, finalText)
	}

	mu.Lock()
	n := callN
	mu.Unlock()
	if n < 3 {
		t.Errorf("expected ≥3 LLM calls (tool_use×2 + end_turn), got %d", n)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCLI_OutputFormat_JSON — outputFormat="json" produces a single JSON object
// ─────────────────────────────────────────────────────────────────────────────

func TestCLI_OutputFormat_JSON(t *testing.T) {
	const responseText = "json output answer"

	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(buildEndTurnEvents(responseText)...), nil
		},
	}

	container := buildTestContainer(t, client)

	output := captureStdout(t, func() {
		if err := bootstrap.RunHeadless(container, "give me json", "json", 0); err != nil {
			t.Errorf("RunHeadless returned error: %v", err)
		}
	})

	// The entire output should be a single valid JSON object.
	output = strings.TrimSpace(output)
	if !json.Valid([]byte(output)) {
		t.Fatalf("output is not valid JSON: %q", output)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if result["type"] != "result" {
		t.Errorf(`expected "type":"result", got %v`, result["type"])
	}
	if result["result"] == nil {
		t.Error(`expected "result" field to be present`)
	}
	if !strings.Contains(result["result"].(string), responseText) {
		t.Errorf(`"result" field %q does not contain %q`, result["result"], responseText)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCLI_OutputFormat_StreamJSON — outputFormat="stream-json" produces NDJSON
// ─────────────────────────────────────────────────────────────────────────────

func TestCLI_OutputFormat_StreamJSON(t *testing.T) {
	const responseText = "streaming json answer"

	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(buildEndTurnEvents(responseText)...), nil
		},
	}

	container := buildTestContainer(t, client)

	output := captureStdout(t, func() {
		if err := bootstrap.RunHeadless(container, "stream it", "stream-json", 0); err != nil {
			t.Errorf("RunHeadless returned error: %v", err)
		}
	})

	// Each non-empty line must be valid JSON.
	scanner := bufio.NewScanner(strings.NewReader(output))
	lineCount := 0
	streamTextCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineCount++

		if !json.Valid([]byte(line)) {
			t.Errorf("line %d is not valid JSON: %q", lineCount, line)
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d unmarshal: %v", lineCount, err)
			continue
		}

		if obj["Type"] == "stream_text" {
			streamTextCount++
		}
	}

	if err := scanner.Err(); err != nil {
		t.Errorf("scanner error: %v", err)
	}

	if lineCount == 0 {
		t.Error("expected at least one NDJSON line")
	}

	if streamTextCount == 0 {
		t.Errorf("expected at least one stream_text event in NDJSON output; got 0 (total lines: %d)", lineCount)
	}
}
