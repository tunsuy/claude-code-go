package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// WebSearchInput is the input schema for the WebSearch tool.
type WebSearchInput struct {
	// Query is the search query (required).
	Query string `json:"query"`
	// AllowedDomains filters results to these domains (optional).
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	// BlockedDomains excludes results from these domains (optional).
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

// WebSearchResult represents a single search result.
type WebSearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// WebSearchOutput is the structured output of WebSearch.
type WebSearchOutput struct {
	Query   string            `json:"query"`
	Results []WebSearchResult `json:"results"`
}

// braveSearchAPIURL is the Brave Search API endpoint.
const braveSearchAPIURL = "https://api.search.brave.com/res/v1/web/search"

// ── Tool implementation ───────────────────────────────────────────────────────

type webSearchTool struct{ tool.BaseTool }

// WebSearchTool is the exported singleton instance.
var WebSearchTool tool.Tool = &webSearchTool{}

func (t *webSearchTool) Name() string { return "WebSearch" }

func (t *webSearchTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Searches the web using the Brave Search API and returns formatted results.

Usage notes:
- Requires BRAVE_API_KEY environment variable to be set
- Returns search result titles, URLs, and descriptions
- Supports domain filtering via allowed_domains and blocked_domains
- Web search is read-only and concurrency-safe`
}

func (t *webSearchTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"query": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The search query",
			}),
			"allowed_domains": tool.PropSchema(map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Only include results from these domains",
			}),
			"blocked_domains": tool.PropSchema(map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Never include results from these domains",
			}),
		},
		[]string{"query"},
	)
}

func (t *webSearchTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *webSearchTool) IsReadOnly(_ tool.Input) bool         { return true }
func (t *webSearchTool) SearchHint() string {
	return "web search internet query find browse"
}

func (t *webSearchTool) IsEnabled() bool {
	// Tool is enabled regardless; it will return an error at call time if
	// the API key is missing, giving the model a clear error message.
	return true
}

func (t *webSearchTool) UserFacingName(input tool.Input) string {
	var in WebSearchInput
	if json.Unmarshal(input, &in) == nil && in.Query != "" {
		return fmt.Sprintf("WebSearch(%s)", in.Query)
	}
	return "WebSearch"
}

func (t *webSearchTool) ValidateInput(input tool.Input, _ *tool.UseContext) (tool.ValidationResult, error) {
	var in WebSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.ValidationResult{OK: false, Reason: "invalid JSON: " + err.Error()}, nil
	}
	if strings.TrimSpace(in.Query) == "" {
		return tool.ValidationResult{OK: false, Reason: "query is required and must be non-empty"}, nil
	}
	return tool.ValidationResult{OK: true}, nil
}

// Call executes the WebSearch tool.
func (t *webSearchTool) Call(input tool.Input, ctx *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	var in WebSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tool.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}

	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		return &tool.Result{
			IsError: true,
			Content: "BRAVE_API_KEY environment variable is not set. WebSearch requires a Brave Search API key.",
		}, nil
	}

	results, err := callBraveSearchAPI(apiKey, in.Query, in.AllowedDomains, in.BlockedDomains, ctx)
	if err != nil {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("search failed: %v", err)}, nil
	}

	out := WebSearchOutput{
		Query:   in.Query,
		Results: results,
	}
	return &tool.Result{Content: out}, nil
}

// callBraveSearchAPI calls the Brave Search API and returns parsed results.
func callBraveSearchAPI(
	apiKey, query string,
	allowedDomains, blockedDomains []string,
	ctx *tool.UseContext,
) ([]WebSearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", "10")

	reqURL := braveSearchAPIURL + "?" + params.Encode()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	if ctx != nil && ctx.Ctx != nil {
		req = req.WithContext(ctx.Ctx)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid BRAVE_API_KEY (HTTP 401)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}

	return parseBraveResponse(body, allowedDomains, blockedDomains)
}

// parseBraveResponse parses a Brave Search API response.
func parseBraveResponse(body []byte, allowed, blocked []string) ([]WebSearchResult, error) {
	var raw struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("cannot parse API response: %w", err)
	}

	var results []WebSearchResult
	for _, r := range raw.Web.Results {
		if !domainAllowed(r.URL, allowed, blocked) {
			continue
		}
		results = append(results, WebSearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		})
	}
	return results, nil
}

// domainAllowed returns true if the URL passes the domain filters.
func domainAllowed(rawURL string, allowed, blocked []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := strings.ToLower(parsed.Hostname())

	for _, d := range blocked {
		d = strings.ToLower(d)
		if host == d || strings.HasSuffix(host, "."+d) {
			return false
		}
	}
	if len(allowed) == 0 {
		return true
	}
	for _, d := range allowed {
		d = strings.ToLower(d)
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (t *webSearchTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	out, ok := output.(WebSearchOutput)
	if !ok {
		return t.BaseTool.MapResultToToolResultBlock(output, toolUseID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", out.Query))
	for i, r := range out.Results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description))
	}
	if len(out.Results) == 0 {
		sb.WriteString("No results found.")
	}

	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     sb.String(),
	}
	return json.Marshal(block)
}
