package memdir_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

// --- MemoryAge ---

func TestMemoryAge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		daysAgo   int
		wantMatch string // substring that must appear in result
	}{
		{"today", 0, "today"},
		{"yesterday", 1, "yesterday"},
		{"3 days ago", 3, "3 days ago"},
		{"6 days ago", 6, "6 days ago"},
		{"1 week ago", 7, "1 weeks ago"},
		{"2 weeks ago", 14, "2 weeks ago"},
		{"1 month ago", 30, "1 months ago"},
		{"3 months ago", 90, "3 months ago"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			updatedAt := time.Now().Add(-time.Duration(tt.daysAgo) * 24 * time.Hour)
			got := memdir.MemoryAge(updatedAt)
			if !strings.Contains(got, tt.wantMatch) {
				t.Errorf("MemoryAge(%d days ago) = %q, want substring %q", tt.daysAgo, got, tt.wantMatch)
			}
		})
	}
}

// --- MemoryFreshnessText ---

func TestMemoryFreshnessText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		daysAgo   int
		wantEmpty bool
	}{
		{"today_is_fresh", 0, true},
		{"yesterday_is_fresh", 1, true},
		{"2_days_ago_is_stale", 2, false},
		{"30_days_ago_is_stale", 30, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			updatedAt := time.Now().Add(-time.Duration(tt.daysAgo) * 24 * time.Hour)
			got := memdir.MemoryFreshnessText(updatedAt)
			if tt.wantEmpty && got != "" {
				t.Errorf("MemoryFreshnessText(%d days ago) = %q, want empty", tt.daysAgo, got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("MemoryFreshnessText(%d days ago) = empty, want non-empty", tt.daysAgo)
			}
			if !tt.wantEmpty && !strings.Contains(got, "outdated") {
				t.Errorf("MemoryFreshnessText(%d days ago) = %q, want 'outdated' substring", tt.daysAgo, got)
			}
		})
	}
}

// --- DefaultRelevanceConfig ---

func TestDefaultRelevanceConfig(t *testing.T) {
	t.Parallel()
	cfg := memdir.DefaultRelevanceConfig()
	if cfg.MaxMemoriesPerTurn != 5 {
		t.Errorf("MaxMemoriesPerTurn = %d, want 5", cfg.MaxMemoriesPerTurn)
	}
	if cfg.MaxMemoryBytes != 4096 {
		t.Errorf("MaxMemoryBytes = %d, want 4096", cfg.MaxMemoryBytes)
	}
	if cfg.MaxSessionBytes != 60_000 {
		t.Errorf("MaxSessionBytes = %d, want 60000", cfg.MaxSessionBytes)
	}
}

// --- SurfaceRelevantMemories ---

func TestSurfaceRelevantMemories_NoMemories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)
	cfg := memdir.DefaultRelevanceConfig()

	results, err := memdir.SurfaceRelevantMemories(store, "test query", nil, 0, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSurfaceRelevantMemories_NilStore(t *testing.T) {
	t.Parallel()
	cfg := memdir.DefaultRelevanceConfig()

	results, err := memdir.SurfaceRelevantMemories(nil, "test query", nil, 0, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSurfaceRelevantMemories_AllAlreadySurfaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)
	writeTestMemory(t, store, "Go Testing", "project", []string{"go", "testing"},
		"Always use table-driven tests in Go.")

	// List memories to get the path.
	memories, err := store.ListMemories()
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	already := map[string]bool{memories[0].Path: true}
	cfg := memdir.DefaultRelevanceConfig()

	results, err := memdir.SurfaceRelevantMemories(store, "go testing", already, 0, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 (all already surfaced)", len(results))
	}
}

func TestSurfaceRelevantMemories_SessionBytesLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)
	writeTestMemory(t, store, "Go Testing", "project", []string{"go", "testing"},
		"Always use table-driven tests in Go.")

	cfg := memdir.RelevanceConfig{
		MaxMemoriesPerTurn: 5,
		MaxMemoryBytes:     4096,
		MaxSessionBytes:    100, // Very small limit
	}

	// Exhaust the session byte budget.
	results, err := memdir.SurfaceRelevantMemories(store, "go testing", nil, 100, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 (session bytes exhausted)", len(results))
	}
}

func TestSurfaceRelevantMemories_ReturnsRelevant(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	writeTestMemory(t, store, "Go Testing Patterns", "project", []string{"go", "testing"},
		"Always use table-driven tests. Run with -race flag.")
	writeTestMemory(t, store, "Python Style Guide", "project", []string{"python", "style"},
		"Follow PEP 8 conventions for Python code.")
	writeTestMemory(t, store, "Docker Setup", "reference", []string{"docker", "infra"},
		"Use multi-stage builds for production images.")

	cfg := memdir.DefaultRelevanceConfig()
	results, err := memdir.SurfaceRelevantMemories(store, "how to write go tests", nil, 0, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The "Go Testing Patterns" memory should be the most relevant.
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Title != "Go Testing Patterns" {
		t.Errorf("top result = %q, want %q", results[0].Title, "Go Testing Patterns")
	}
}

func TestSurfaceRelevantMemories_TruncatesLargeMemory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	// Create a large memory body.
	largeBody := strings.Repeat("go testing pattern details. ", 200) // >5000 bytes
	writeTestMemory(t, store, "Go Testing", "project", []string{"go", "testing"}, largeBody)

	cfg := memdir.RelevanceConfig{
		MaxMemoriesPerTurn: 5,
		MaxMemoryBytes:     100, // Very small per-memory limit.
		MaxSessionBytes:    60_000,
	}

	results, err := memdir.SurfaceRelevantMemories(store, "go testing", nil, 0, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if !strings.Contains(results[0].Content, "truncated") {
		t.Errorf("expected content to contain 'truncated', got length %d", len(results[0].Content))
	}
}

func TestSurfaceRelevantMemories_MaxPerTurn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	// Create more memories than MaxMemoriesPerTurn, all with "go" keyword.
	for i := 0; i < 10; i++ {
		writeTestMemory(t, store, "Go "+string(rune('A'+i)), "project",
			[]string{"go"}, "Go content for keyword matching.")
	}

	cfg := memdir.RelevanceConfig{
		MaxMemoriesPerTurn: 3,
		MaxMemoryBytes:     4096,
		MaxSessionBytes:    60_000,
	}

	results, err := memdir.SurfaceRelevantMemories(store, "go", nil, 0, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("got %d results, want at most 3", len(results))
	}
}

// --- FormatRelevantMemoriesPrompt ---

func TestFormatRelevantMemoriesPrompt_Empty(t *testing.T) {
	t.Parallel()
	got := memdir.FormatRelevantMemoriesPrompt(nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFormatRelevantMemoriesPrompt_WithMemories(t *testing.T) {
	t.Parallel()
	memories := []memdir.RelevantMemory{
		{
			Path:    "/path/to/go_testing.md",
			Title:   "Go Testing",
			Content: "Use table-driven tests.",
		},
		{
			Path:          "/path/to/old_memory.md",
			Title:         "Old Memory",
			Content:       "Some old content.",
			FreshnessNote: "This memory was last updated 2 weeks ago. It may be outdated.",
		},
	}

	got := memdir.FormatRelevantMemoriesPrompt(memories)
	if !strings.Contains(got, "<system-reminder>") {
		t.Error("missing <system-reminder> tag")
	}
	if !strings.Contains(got, "</system-reminder>") {
		t.Error("missing </system-reminder> tag")
	}
	if !strings.Contains(got, "## Relevant Memories") {
		t.Error("missing '## Relevant Memories' header")
	}
	if !strings.Contains(got, "### Go Testing") {
		t.Error("missing '### Go Testing' header")
	}
	if !strings.Contains(got, "Use table-driven tests.") {
		t.Error("missing memory content")
	}
	if !strings.Contains(got, "### Old Memory") {
		t.Error("missing '### Old Memory' header")
	}
	if !strings.Contains(got, "outdated") {
		t.Error("missing freshness note")
	}
}

func TestFormatRelevantMemoriesPrompt_UntitledMemory(t *testing.T) {
	t.Parallel()
	memories := []memdir.RelevantMemory{
		{Path: "/path/to/untitled.md", Title: "", Content: "Some content."},
	}
	got := memdir.FormatRelevantMemoriesPrompt(memories)
	if !strings.Contains(got, "### Untitled Memory") {
		t.Error("missing '### Untitled Memory' fallback title")
	}
}

// --- scoreRelevance (tested indirectly via SurfaceRelevantMemories ordering) ---

func TestSurfaceRelevantMemories_NoMatchReturnsEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	writeTestMemory(t, store, "Quantum Physics Notes", "reference",
		[]string{"physics", "quantum"}, "Schrodinger equation and wave functions.")

	cfg := memdir.DefaultRelevanceConfig()
	results, err := memdir.SurfaceRelevantMemories(store, "go testing patterns", nil, 0, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results for unrelated query, want 0", len(results))
	}
}

// --- Helper ---

func writeTestMemory(t *testing.T, store *memdir.MemoryStore, title, typ string, tags []string, body string) {
	t.Helper()
	if err := store.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	mt := memdir.MemoryType(typ)
	mf := &memdir.MemoryFile{
		Header: memdir.MemoryHeader{
			Title:     title,
			Type:      mt,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Tags:      tags,
			Source:    "test",
		},
		Body: body,
	}
	if err := store.WriteMemory(mf); err != nil {
		t.Fatalf("WriteMemory(%q): %v", title, err)
	}
	// Verify the file was written.
	dir := store.Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no .md file found in %q after WriteMemory", dir)
	}
}
