package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// clearFetchCacheForTest resets the in-memory cache between test cases.
func clearFetchCacheForTest() {
	fetchCacheMu.Lock()
	fetchCache = make(map[string]cacheEntry)
	fetchCacheMu.Unlock()
}

// ── upgradeToHTTPS ────────────────────────────────────────────────────────────

func TestUpgradeToHTTPS_HTTP(t *testing.T) {
	got := upgradeToHTTPS("http://example.com/path")
	if got != "https://example.com/path" {
		t.Errorf("expected https upgrade, got %q", got)
	}
}

func TestUpgradeToHTTPS_AlreadyHTTPS(t *testing.T) {
	url := "https://example.com"
	if got := upgradeToHTTPS(url); got != url {
		t.Errorf("expected no change, got %q", got)
	}
}

// ── htmlToText ────────────────────────────────────────────────────────────────

func TestHtmlToText_StripsScript(t *testing.T) {
	html := `<html><body><script>alert('x')</script><p>Hello</p></body></html>`
	out := htmlToText(html)
	if strings.Contains(out, "alert") {
		t.Errorf("script content should be stripped, got %q", out)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("body text missing: %q", out)
	}
}

func TestHtmlToText_StripsStyle(t *testing.T) {
	html := `<html><head><style>.foo{color:red}</style></head><body>World</body></html>`
	out := htmlToText(html)
	if strings.Contains(out, "color") {
		t.Errorf("style content should be stripped, got %q", out)
	}
	if !strings.Contains(out, "World") {
		t.Errorf("body text missing: %q", out)
	}
}

func TestHtmlToText_BlockElements(t *testing.T) {
	html := `<div>Line1</div><div>Line2</div>`
	out := htmlToText(html)
	if !strings.Contains(out, "Line1") || !strings.Contains(out, "Line2") {
		t.Errorf("expected both lines: %q", out)
	}
}

func TestHtmlToText_CollapsesBlanks(t *testing.T) {
	html := "<p>A</p>\n\n\n\n\n<p>B</p>"
	out := htmlToText(html)
	lines := strings.Split(out, "\n")
	// Check no run of consecutive blank lines exceeds 2.
	maxConsecutiveBlanks := 0
	currentRun := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			currentRun++
			if currentRun > maxConsecutiveBlanks {
				maxConsecutiveBlanks = currentRun
			}
		} else {
			currentRun = 0
		}
	}
	if maxConsecutiveBlanks > 2 {
		t.Errorf("expected ≤2 consecutive blank lines in any run, got %d in %q", maxConsecutiveBlanks, out)
	}
}

// ── removeTagBlock ────────────────────────────────────────────────────────────

func TestRemoveTagBlock_Simple(t *testing.T) {
	html := `before<script>bad</script>after`
	out := removeTagBlock(html, "script")
	if strings.Contains(out, "bad") {
		t.Errorf("expected tag block removed: %q", out)
	}
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Errorf("surrounding content removed: %q", out)
	}
}

func TestRemoveTagBlock_Multiple(t *testing.T) {
	html := `a<script>s1</script>b<script>s2</script>c`
	out := removeTagBlock(html, "script")
	if strings.Contains(out, "s1") || strings.Contains(out, "s2") {
		t.Errorf("expected both blocks removed: %q", out)
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") || !strings.Contains(out, "c") {
		t.Errorf("surrounding content removed: %q", out)
	}
}

func TestRemoveTagBlock_NoMatch(t *testing.T) {
	html := `<p>hello</p>`
	out := removeTagBlock(html, "script")
	if out != html {
		t.Errorf("expected unchanged output, got %q", out)
	}
}

// ── domainAllowed ─────────────────────────────────────────────────────────────

func TestDomainAllowed_NoFilters(t *testing.T) {
	if !domainAllowed("https://example.com/foo", nil, nil) {
		t.Error("expected allowed with no filters")
	}
}

func TestDomainAllowed_BlockedExact(t *testing.T) {
	if domainAllowed("https://evil.com/", nil, []string{"evil.com"}) {
		t.Error("expected blocked for exact match")
	}
}

func TestDomainAllowed_BlockedSubdomain(t *testing.T) {
	if domainAllowed("https://sub.evil.com/", nil, []string{"evil.com"}) {
		t.Error("expected blocked for subdomain")
	}
}

func TestDomainAllowed_BlockedDoesNotMatchSuperDomain(t *testing.T) {
	// "notevil.com" should NOT be blocked when blocked=[evil.com]
	if !domainAllowed("https://notevil.com/", nil, []string{"evil.com"}) {
		t.Error("expected allowed: notevil.com is not a subdomain of evil.com")
	}
}

func TestDomainAllowed_AllowedMatch(t *testing.T) {
	if !domainAllowed("https://good.com/", []string{"good.com"}, nil) {
		t.Error("expected allowed for matching allowed domain")
	}
}

func TestDomainAllowed_AllowedSubdomain(t *testing.T) {
	if !domainAllowed("https://api.good.com/", []string{"good.com"}, nil) {
		t.Error("expected allowed for subdomain of allowed domain")
	}
}

func TestDomainAllowed_AllowedMismatch(t *testing.T) {
	if domainAllowed("https://other.com/", []string{"good.com"}, nil) {
		t.Error("expected blocked when not in allowed list")
	}
}

func TestDomainAllowed_InvalidURL(t *testing.T) {
	// Invalid URL should pass through (fail open)
	if !domainAllowed("not-a-url", nil, nil) {
		t.Error("expected fail-open for invalid URL")
	}
}

// ── WebFetch tool ─────────────────────────────────────────────────────────────

func TestWebFetchTool_Name(t *testing.T) {
	if WebFetchTool.Name() != "WebFetch" {
		t.Errorf("expected WebFetch, got %q", WebFetchTool.Name())
	}
}

func TestWebFetchTool_IsConcurrencySafe_True(t *testing.T) {
	if !WebFetchTool.IsConcurrencySafe(nil) {
		t.Error("WebFetchTool should be concurrency-safe")
	}
}

func TestWebFetchTool_IsReadOnly_True(t *testing.T) {
	if !WebFetchTool.IsReadOnly(nil) {
		t.Error("WebFetchTool should be read-only")
	}
}

func TestWebFetchTool_InputSchema(t *testing.T) {
	schema := WebFetchTool.InputSchema()
	if _, ok := schema.Properties["url"]; !ok {
		t.Error("schema missing 'url'")
	}
	if _, ok := schema.Properties["prompt"]; !ok {
		t.Error("schema missing 'prompt'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "url" {
		t.Errorf("expected Required=[url], got %v", schema.Required)
	}
}

func TestWebFetchTool_ValidateInput_EmptyURL(t *testing.T) {
	in, _ := json.Marshal(WebFetchInput{URL: ""})
	vr, err := WebFetchTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for empty URL")
	}
}

func TestWebFetchTool_ValidateInput_InvalidURL(t *testing.T) {
	in, _ := json.Marshal(WebFetchInput{URL: "not a url"})
	vr, err := WebFetchTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for invalid URL")
	}
}

func TestWebFetchTool_ValidateInput_ValidURL(t *testing.T) {
	in, _ := json.Marshal(WebFetchInput{URL: "https://example.com"})
	vr, err := WebFetchTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.OK {
		t.Errorf("expected validation OK, got reason: %q", vr.Reason)
	}
}

func TestWebFetchTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(WebFetchInput{URL: "https://example.com"})
	name := WebFetchTool.UserFacingName(in)
	if name != "WebFetch(https://example.com)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestWebFetchTool_Call_Success(t *testing.T) {
	clearFetchCacheForTest()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><p>Hello World</p></body></html>")
	}))
	defer srv.Close()

	oldClient := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	defer func() { defaultHTTPClient = oldClient }()

	in, _ := json.Marshal(WebFetchInput{URL: srv.URL})
	result, err := WebFetchTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	out, ok := result.Content.(WebFetchOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if !strings.Contains(out.Content, "Hello World") {
		t.Errorf("expected body content: %q", out.Content)
	}
	if out.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", out.StatusCode)
	}
}

func TestWebFetchTool_Call_Cache(t *testing.T) {
	clearFetchCacheForTest()
	callCount := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "cached response")
	}))
	defer srv.Close()

	oldClient := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	defer func() { defaultHTTPClient = oldClient }()

	in, _ := json.Marshal(WebFetchInput{URL: srv.URL})

	// First call
	_, _ = WebFetchTool.Call(in, nil, nil)
	// Second call should use cache
	_, _ = WebFetchTool.Call(in, nil, nil)

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (cache hit), got %d", callCount)
	}
}

func TestWebFetchTool_Call_HTTPError(t *testing.T) {
	clearFetchCacheForTest()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer srv.Close()

	oldClient := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	defer func() { defaultHTTPClient = oldClient }()

	in, _ := json.Marshal(WebFetchInput{URL: srv.URL})
	result, err := WebFetchTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for 404")
	}
}

func TestWebFetchTool_MapResultToToolResultBlock(t *testing.T) {
	out := WebFetchOutput{URL: "https://example.com", Content: "hello", StatusCode: 200}
	raw, err := WebFetchTool.MapResultToToolResultBlock(out, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	if block["type"] != "tool_result" {
		t.Error("expected type=tool_result")
	}
	if block["content"] != "hello" {
		t.Errorf("unexpected content: %v", block["content"])
	}
}

func TestWebFetchTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = WebFetchTool
}

// ── WebSearch tool ────────────────────────────────────────────────────────────

func TestWebSearchTool_Name(t *testing.T) {
	if WebSearchTool.Name() != "WebSearch" {
		t.Errorf("expected WebSearch, got %q", WebSearchTool.Name())
	}
}

func TestWebSearchTool_IsConcurrencySafe_True(t *testing.T) {
	if !WebSearchTool.IsConcurrencySafe(nil) {
		t.Error("WebSearchTool should be concurrency-safe")
	}
}

func TestWebSearchTool_IsReadOnly_True(t *testing.T) {
	if !WebSearchTool.IsReadOnly(nil) {
		t.Error("WebSearchTool should be read-only")
	}
}

func TestWebSearchTool_InputSchema(t *testing.T) {
	schema := WebSearchTool.InputSchema()
	if _, ok := schema.Properties["query"]; !ok {
		t.Error("schema missing 'query'")
	}
	if _, ok := schema.Properties["allowed_domains"]; !ok {
		t.Error("schema missing 'allowed_domains'")
	}
	if _, ok := schema.Properties["blocked_domains"]; !ok {
		t.Error("schema missing 'blocked_domains'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Errorf("expected Required=[query], got %v", schema.Required)
	}
}

func TestWebSearchTool_ValidateInput_EmptyQuery(t *testing.T) {
	in, _ := json.Marshal(WebSearchInput{Query: "  "})
	vr, err := WebSearchTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for empty query")
	}
}

func TestWebSearchTool_ValidateInput_Valid(t *testing.T) {
	in, _ := json.Marshal(WebSearchInput{Query: "golang testing"})
	vr, err := WebSearchTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.OK {
		t.Errorf("expected validation OK, got reason: %q", vr.Reason)
	}
}

func TestWebSearchTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(WebSearchInput{Query: "go unit testing"})
	name := WebSearchTool.UserFacingName(in)
	if name != "WebSearch(go unit testing)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestWebSearchTool_Call_MissingAPIKey(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "")
	in, _ := json.Marshal(WebSearchInput{Query: "test"})
	result, err := WebSearchTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when API key is missing")
	}
}

func TestWebSearchTool_Call_WithMockServer(t *testing.T) {
	braveResp := `{"web":{"results":[{"title":"Go Testing","url":"https://go.dev/testing","description":"The testing package"},{"title":"Excluded","url":"https://blocked.com/x","description":"Should be filtered"}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, braveResp)
	}))
	defer srv.Close()

	oldClient := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	defer func() { defaultHTTPClient = oldClient }()

	// Patch the API URL for the test by overriding the client behavior
	// The tool will call braveSearchAPIURL which is the real URL, but our
	// test client routes all requests to the test server.
	// We need to use a transport that redirects all calls to srv.
	defaultHTTPClient = &redirectClient{target: srv.URL, inner: srv.Client()}

	t.Setenv("BRAVE_API_KEY", "test-key")
	in, _ := json.Marshal(WebSearchInput{
		Query:          "go testing",
		BlockedDomains: []string{"blocked.com"},
	})
	result, err := WebSearchTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	out, ok := result.Content.(WebSearchOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if out.Query != "go testing" {
		t.Errorf("unexpected query: %q", out.Query)
	}
	// "blocked.com" result should be filtered out
	for _, r := range out.Results {
		if strings.Contains(r.URL, "blocked.com") {
			t.Errorf("blocked domain appeared in results: %s", r.URL)
		}
	}
}

// redirectClient redirects all HTTP requests to a fixed target server.
type redirectClient struct {
	target string
	inner  *http.Client
}

func (c *redirectClient) Do(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = strings.TrimPrefix(c.target, "http://")
	return c.inner.Do(req2)
}

func TestWebSearchTool_MapResultToToolResultBlock_WithResults(t *testing.T) {
	out := WebSearchOutput{
		Query: "test",
		Results: []WebSearchResult{
			{Title: "Result 1", URL: "https://a.com", Description: "Desc A"},
		},
	}
	raw, err := WebSearchTool.MapResultToToolResultBlock(out, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	if block["type"] != "tool_result" {
		t.Error("expected type=tool_result")
	}
	content := block["content"].(string)
	if !strings.Contains(content, "Result 1") {
		t.Errorf("expected result title in content: %q", content)
	}
	if !strings.Contains(content, "test") {
		t.Errorf("expected query in content: %q", content)
	}
}

func TestWebSearchTool_MapResultToToolResultBlock_Empty(t *testing.T) {
	out := WebSearchOutput{Query: "nothing", Results: nil}
	raw, err := WebSearchTool.MapResultToToolResultBlock(out, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	json.Unmarshal(raw, &block)
	content := block["content"].(string)
	if !strings.Contains(content, "No results found") {
		t.Errorf("expected 'No results found' for empty results: %q", content)
	}
}

func TestParseBraveResponse_AllowedDomains(t *testing.T) {
	body := []byte(`{"web":{"results":[
		{"title":"A","url":"https://allowed.com/page","description":"desc a"},
		{"title":"B","url":"https://other.com/page","description":"desc b"}
	]}}`)
	results, err := parseBraveResponse(body, []string{"allowed.com"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "A" {
		t.Errorf("expected 1 allowed result, got %v", results)
	}
}

func TestParseBraveResponse_BlockedDomains(t *testing.T) {
	body := []byte(`{"web":{"results":[
		{"title":"A","url":"https://good.com/page","description":"desc a"},
		{"title":"B","url":"https://bad.com/page","description":"desc b"}
	]}}`)
	results, err := parseBraveResponse(body, nil, []string{"bad.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "A" {
		t.Errorf("expected 1 result after blocking, got %v", results)
	}
}

func TestParseBraveResponse_InvalidJSON(t *testing.T) {
	_, err := parseBraveResponse([]byte("not-json"), nil, nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWebSearchTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = WebSearchTool
}
