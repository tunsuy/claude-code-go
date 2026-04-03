package bootstrap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anthropics/claude-code-go/internal/engine"
	"github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// collectHeadlessPrompt builds the prompt string for -p mode.
// If extraArgs is non-empty the args are joined with a single space.
// Otherwise stdin is read until EOF.
func collectHeadlessPrompt(extraArgs []string) (string, error) {
	if len(extraArgs) > 0 {
		return strings.Join(extraArgs, " "), nil
	}

	// Read from stdin.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// headlessRun executes a single query in non-interactive mode and writes
// results to stdout according to the output format specified in f.
func headlessRun(
	container *AppContainer,
	prompt string,
	f *rootFlags,
	sigCh <-chan os.Signal,
) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Honour SIGINT.
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	useCtx := &tool.UseContext{
		Ctx: ctx,
	}

	params := engine.QueryParams{
		Messages:       container.QueryEngine.GetMessages(),
		ToolUseContext: useCtx,
		QuerySource:    "foreground",
		MaxTurns:       f.maxTurns,
		FallbackModel:  f.fallbackModel,
	}

	if f.systemPrompt != "" {
		params.SystemPrompt = engine.SystemPrompt{
			Parts: []engine.SystemPromptPart{
				{Text: f.systemPrompt},
			},
		}
	}

	// Append the user prompt as the first message.
	params.Messages = append(params.Messages, types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr(prompt)},
		},
	})

	msgCh, err := container.QueryEngine.Query(ctx, params)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	switch f.outputFormat {
	case "json":
		return consumeJSON(ctx, msgCh, container)
	case "stream-json":
		return consumeStreamJSON(ctx, msgCh)
	default: // "text"
		return consumeText(ctx, msgCh)
	}
}

// consumeText prints assistant text tokens to stdout as they arrive.
func consumeText(ctx context.Context, msgCh <-chan engine.Msg) error {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush() //nolint:errcheck

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgCh:
			if !ok {
				return lastErr
			}
			switch msg.Type {
			case engine.MsgTypeStreamText:
				fmt.Fprint(w, msg.TextDelta) //nolint:errcheck
				w.Flush()                    //nolint:errcheck
			case engine.MsgTypeError:
				lastErr = msg.Err
			case engine.MsgTypeTurnComplete:
				// Add final newline if the response didn't end with one.
				fmt.Fprintln(w) //nolint:errcheck
			}
		}
	}
}

// consumeStreamJSON writes each engine.Msg as a JSON object followed by a
// newline (newline-delimited JSON / NDJSON).
func consumeStreamJSON(ctx context.Context, msgCh <-chan engine.Msg) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgCh:
			if !ok {
				return lastErr
			}
			if msg.Type == engine.MsgTypeError {
				lastErr = msg.Err
			}
			if encErr := enc.Encode(msg); encErr != nil {
				return encErr
			}
		}
	}
}

// collectResult consumes all messages and returns the accumulated assistant text.
func collectResult(ctx context.Context, msgCh <-chan engine.Msg, container *AppContainer) (string, error) {
	var sb strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case msg, ok := <-msgCh:
			if !ok {
				return sb.String(), nil
			}
			switch msg.Type {
			case engine.MsgTypeStreamText:
				sb.WriteString(msg.TextDelta)
			case engine.MsgTypeError:
				return sb.String(), msg.Err
			}
		}
	}
}

// consumeJSON collects the full result and writes a single JSON object to stdout.
func consumeJSON(ctx context.Context, msgCh <-chan engine.Msg, container *AppContainer) error {
	text, err := collectResult(ctx, msgCh, container)
	if err != nil {
		return err
	}

	result := map[string]any{
		"type":   "result",
		"result": text,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(result)
}

// strPtr is a helper that returns a pointer to a string.
func strPtr(s string) *string { return &s }
