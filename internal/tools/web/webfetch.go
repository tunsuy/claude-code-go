package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// WebFetchInput is the input schema for the WebFetch tool.
type WebFetchInput struct {
	// URL is the target URL (HTTP is upgraded to HTTPS automatically).
	URL string `json:"url"`
	// Prompt is an optional instruction describing what to extract from the page.
	Prompt string `json:"prompt,omitempty"`
}

// WebFetchOutput is the structured output of WebFetch.
type WebFetchOutput struct {
	// URL is the final URL after any redirects.
	URL string `json:"url"`
	// Content is the page content as plain text / Markdown.
	Content string `json:"content"`
	// StatusCode is the HTTP response status.
	StatusCode int `json:"status_code"`
}

// ── Cache ─────────────────────────────────────────────────────────────────────

const webFetchCacheTTL = 15 * time.Minute
const maxWebFetchBodyBytes = 5 * 1024 * 1024 // 5 MiB

type cacheEntry struct {
	out       WebFetchOutput
	expiresAt time.Time
}

var (
	fetchCacheMu sync.Mutex
	fetchCache   = make(map[string]cacheEntry)
)

func getCached(rawURL string) (WebFetchOutput, bool) {
	fetchCacheMu.Lock()
	defer fetchCacheMu.Unlock()
	e, ok := fetchCache[rawURL]
	if !ok || time.Now().After(e.expiresAt) {
		return WebFetchOutput{}, false
	}
	return e.out, true
}

func setCached(rawURL string, out WebFetchOutput) {
	fetchCacheMu.Lock()
	defer fetchCacheMu.Unlock()
	fetchCache[rawURL] = cacheEntry{out: out, expiresAt: time.Now().Add(webFetchCacheTTL)}
}

// ── HTTP client ───────────────────────────────────────────────────────────────

var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects (max 5)")
		}
		return nil
	},
}

// ── Tool implementation ───────────────────────────────────────────────────────

type webFetchTool struct{ tool.BaseTool }

// WebFetchTool is the exported singleton instance.
var WebFetchTool tool.Tool = &webFetchTool{}

func (t *webFetchTool) Name() string { return "WebFetch" }

func (t *webFetchTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Fetches content from a specified URL and returns it as text/Markdown.

Usage notes:
- The URL must be a fully-formed valid URL
- HTTP URLs are automatically upgraded to HTTPS
- The prompt parameter optionally describes what information you want to extract
- This tool is read-only and does not modify any files
- Results may be summarised if the content is very large
- Includes a 15-minute cache for repeated access to the same URL`
}

func (t *webFetchTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"url": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The URL to fetch content from",
			}),
			"prompt": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "A prompt describing what information to extract from the page",
			}),
		},
		[]string{"url"},
	)
}

func (t *webFetchTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *webFetchTool) IsReadOnly(_ tool.Input) bool         { return true }
func (t *webFetchTool) SearchHint() string                   { return "web fetch url http download browse" }

func (t *webFetchTool) UserFacingName(input tool.Input) string {
	var in WebFetchInput
	if json.Unmarshal(input, &in) == nil && in.URL != "" {
		return fmt.Sprintf("WebFetch(%s)", in.URL)
	}
	return "WebFetch"
}

func (t *webFetchTool) ValidateInput(input tool.Input, _ *tool.UseContext) (tool.ValidationResult, error) {
	var in WebFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.ValidationResult{OK: false, Reason: "invalid JSON: " + err.Error()}, nil
	}
	if strings.TrimSpace(in.URL) == "" {
		return tool.ValidationResult{OK: false, Reason: "url is required"}, nil
	}
	if _, err := url.ParseRequestURI(in.URL); err != nil {
		return tool.ValidationResult{OK: false, Reason: "url is not a valid URI: " + err.Error()}, nil
	}
	return tool.ValidationResult{OK: true}, nil
}

// Call executes the WebFetch tool.
func (t *webFetchTool) Call(input tool.Input, ctx *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	var in WebFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tool.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}

	// Upgrade HTTP → HTTPS.
	fetchURL := upgradeToHTTPS(in.URL)

	// Check cache.
	if cached, ok := getCached(fetchURL); ok {
		return &tool.Result{Content: cached}, nil
	}

	// Build request.
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("cannot build request: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "claude-code-go/1.0 (WebFetch tool)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.5")

	if ctx != nil && ctx.Ctx != nil {
		req = req.WithContext(ctx.Ctx)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("request failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	// Read with size cap.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWebFetchBodyBytes))
	if err != nil {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("error reading response: %v", err)}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	content := convertToText(body, contentType)

	out := WebFetchOutput{
		URL:        resp.Request.URL.String(),
		Content:    content,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		setCached(fetchURL, out)
	}

	if resp.StatusCode >= 400 {
		return &tool.Result{
			IsError: true,
			Content: fmt.Sprintf("HTTP %d: %s\n\n%s", resp.StatusCode, resp.Status, content),
		}, nil
	}

	return &tool.Result{Content: out}, nil
}

// upgradeToHTTPS replaces http:// with https://.
func upgradeToHTTPS(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") {
		return "https://" + rawURL[len("http://"):]
	}
	return rawURL
}

// convertToText converts HTTP response body to plain text.
// For HTML content it strips tags; for other content types it returns as-is.
func convertToText(body []byte, contentType string) string {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		return htmlToText(string(body))
	}
	return string(body)
}

// htmlToText is a simple HTML-to-text converter that strips tags.
// TODO(dep): Replace with github.com/JohannesKaufmann/html-to-markdown for
// better fidelity once the dependency is added to go.mod.
func htmlToText(html string) string {
	// Remove <script> and <style> blocks entirely.
	result := removeTagBlock(html, "script")
	result = removeTagBlock(result, "style")

	// Replace block-level elements with newlines.
	for _, tag := range []string{"p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "td", "th"} {
		result = strings.ReplaceAll(result, "<"+tag, "\n<"+tag)
		result = strings.ReplaceAll(result, "</"+tag+">", "\n")
	}

	// Strip all remaining tags.
	var sb strings.Builder
	inTag := false
	for _, ch := range result {
		switch {
		case ch == '<':
			inTag = true
		case ch == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(ch)
		}
	}

	// Collapse excessive blank lines.
	lines := strings.Split(sb.String(), "\n")
	var cleaned []string
	blankRun := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blankRun++
			if blankRun <= 2 {
				cleaned = append(cleaned, "")
			}
		} else {
			blankRun = 0
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, "\n")
}

// removeTagBlock removes entire <tag>...</tag> blocks from an HTML string.
func removeTagBlock(html, tag string) string {
	lower := strings.ToLower(html)
	open := "<" + tag
	close := "</" + tag + ">"
	var sb strings.Builder
	for {
		start := strings.Index(lower, open)
		if start < 0 {
			sb.WriteString(html)
			break
		}
		sb.WriteString(html[:start])
		rest := lower[start:]
		end := strings.Index(rest, close)
		if end < 0 {
			sb.WriteString(html)
			break
		}
		html = html[start+end+len(close):]
		lower = strings.ToLower(html)
	}
	return sb.String()
}

func (t *webFetchTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	out, ok := output.(WebFetchOutput)
	if !ok {
		return t.BaseTool.MapResultToToolResultBlock(output, toolUseID)
	}
	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     out.Content,
	}
	return json.Marshal(block)
}
