package api

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// DebugLogger provides structured debug logging for API request/response flows.
// It writes to a file (if configured) or stderr, and is safe for concurrent use.
type DebugLogger struct {
	mu      sync.Mutex
	file    *os.File
	enabled bool
	// ownsFile indicates whether the logger should close the file on Close().
	ownsFile bool
}

// NewDebugLogger creates a DebugLogger from the given options.
// If debugFile is non-empty, logs are written to that file.
// If debug is true but debugFile is empty, logs are written to stderr.
// If neither is set, the logger is disabled (no-op).
func NewDebugLogger(debug bool, debugFile string) (*DebugLogger, error) {
	dl := &DebugLogger{}

	// Also check the CLAUDE_DEBUG environment variable for backward compatibility.
	if !debug && debugFile == "" {
		if os.Getenv("CLAUDE_DEBUG") != "" {
			debug = true
		}
	}

	if !debug && debugFile == "" {
		return dl, nil
	}

	dl.enabled = true

	if debugFile != "" {
		f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, fmt.Errorf("api: open debug log file %q: %w", debugFile, err)
		}
		dl.file = f
		dl.ownsFile = true
	} else {
		dl.file = os.Stderr
	}

	return dl, nil
}

// Enabled reports whether debug logging is active.
func (dl *DebugLogger) Enabled() bool {
	return dl != nil && dl.enabled
}

// Close releases any resources held by the logger.
func (dl *DebugLogger) Close() error {
	if dl == nil || !dl.ownsFile || dl.file == nil {
		return nil
	}
	return dl.file.Close()
}

// LogRequest logs the outgoing HTTP request details.
func (dl *DebugLogger) LogRequest(method, url string, headers map[string]string, body []byte) {
	if !dl.Enabled() {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(dl.timestamp())
	sb.WriteString(" ══════════════════════ REQUEST ══════════════════════\n")
	fmt.Fprintf(&sb, "  Method: %s\n", method)
	fmt.Fprintf(&sb, "  URL:    %s\n", url)
	sb.WriteString("  Headers:\n")
	for k, v := range headers {
		// Mask sensitive headers.
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "x-api-key") {
			if len(v) > 10 {
				v = v[:10] + "..."
			}
		}
		fmt.Fprintf(&sb, "    %s: %s\n", k, v)
	}
	sb.WriteString("  Body:\n")
	sb.WriteString(indentJSON(body))
	sb.WriteString("\n══════════════════════════════════════════════════════\n\n")

	_, _ = fmt.Fprint(dl.file, sb.String())
}

// LogResponse logs the HTTP response status and headers.
func (dl *DebugLogger) LogResponse(statusCode int, status string, headers map[string][]string) {
	if !dl.Enabled() {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(dl.timestamp())
	sb.WriteString(" ══════════════════════ RESPONSE ═════════════════════\n")
	fmt.Fprintf(&sb, "  Status: %d %s\n", statusCode, status)
	sb.WriteString("  Headers:\n")
	for k, vals := range headers {
		for _, v := range vals {
			fmt.Fprintf(&sb, "    %s: %s\n", k, v)
		}
	}
	sb.WriteString("══════════════════════════════════════════════════════\n\n")

	_, _ = fmt.Fprint(dl.file, sb.String())
}

// LogResponseBody logs the full non-streaming response body.
func (dl *DebugLogger) LogResponseBody(body []byte) {
	if !dl.Enabled() {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(dl.timestamp())
	sb.WriteString(" ───────────────── RESPONSE BODY ─────────────────────\n")
	sb.WriteString(indentJSON(body))
	sb.WriteString("\n─────────────────────────────────────────────────────\n\n")

	_, _ = fmt.Fprint(dl.file, sb.String())
}

// LogSSEEvent logs a single SSE event from the stream.
func (dl *DebugLogger) LogSSEEvent(eventType, data string) {
	if !dl.Enabled() {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(dl.timestamp())
	fmt.Fprintf(&sb, " [SSE] type=%s", eventType)
	if len(data) > 500 {
		fmt.Fprintf(&sb, " data=%s...(truncated, total %d bytes)", data[:500], len(data))
	} else {
		fmt.Fprintf(&sb, " data=%s", data)
	}
	sb.WriteString("\n")

	_, _ = fmt.Fprint(dl.file, sb.String())
}

// LogSSERawLine logs a raw SSE line from the stream for detailed debugging.
func (dl *DebugLogger) LogSSERawLine(line string) {
	if !dl.Enabled() {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(dl.timestamp())
	fmt.Fprintf(&sb, " [SSE-RAW] %s\n", line)

	_, _ = fmt.Fprint(dl.file, sb.String())
}

// LogError logs an error that occurred during API interaction.
func (dl *DebugLogger) LogError(context string, err error) {
	if !dl.Enabled() {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(dl.timestamp())
	fmt.Fprintf(&sb, " [ERROR] %s: %v\n", context, err)

	_, _ = fmt.Fprint(dl.file, sb.String())
}

// Logf logs a formatted debug message.
func (dl *DebugLogger) Logf(format string, args ...any) {
	if !dl.Enabled() {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(dl.timestamp())
	sb.WriteString(" [DEBUG] ")
	fmt.Fprintf(&sb, format, args...)
	sb.WriteString("\n")

	_, _ = fmt.Fprint(dl.file, sb.String())
}

// timestamp returns a formatted timestamp for log entries.
func (dl *DebugLogger) timestamp() string {
	return time.Now().Format("2006-01-02T15:04:05.000")
}

// indentJSON returns the JSON body as a string with 4-space indent prefix on each line.
func indentJSON(body []byte) string {
	s := string(body)
	lines := strings.Split(s, "\n")
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString("    ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}
