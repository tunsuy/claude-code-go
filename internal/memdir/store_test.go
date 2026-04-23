package memdir_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

func TestParseMemoryFile_WithFrontmatter(t *testing.T) {
	content := `---
title: Go Coding Style
type: user
created_at: 2025-01-15T10:30:00Z
updated_at: 2025-03-20T14:00:00Z
tags: [go, style, conventions]
source: assistant
---

The user prefers table-driven tests and short variable names.
`
	mf, err := memdir.ParseMemoryFile(content, "/tmp/test.md")
	if err != nil {
		t.Fatalf("ParseMemoryFile: %v", err)
	}
	if mf.Header.Title != "Go Coding Style" {
		t.Errorf("Title: got %q, want %q", mf.Header.Title, "Go Coding Style")
	}
	if mf.Header.Type != memdir.MemoryTypeUser {
		t.Errorf("Type: got %q, want %q", mf.Header.Type, memdir.MemoryTypeUser)
	}
	if len(mf.Header.Tags) != 3 {
		t.Errorf("Tags: got %d, want 3", len(mf.Header.Tags))
	}
	if mf.Header.Source != "assistant" {
		t.Errorf("Source: got %q, want %q", mf.Header.Source, "assistant")
	}
	if !strings.Contains(mf.Body, "table-driven tests") {
		t.Errorf("Body: expected to contain 'table-driven tests', got %q", mf.Body)
	}
	if mf.Path != "/tmp/test.md" {
		t.Errorf("Path: got %q, want %q", mf.Path, "/tmp/test.md")
	}
}

func TestParseMemoryFile_WithoutFrontmatter(t *testing.T) {
	content := "This is plain text without frontmatter."
	mf, err := memdir.ParseMemoryFile(content, "/tmp/plain.md")
	if err != nil {
		t.Fatalf("ParseMemoryFile: %v", err)
	}
	if mf.Header.Type != memdir.MemoryTypeProject {
		t.Errorf("Type: got %q, want default %q", mf.Header.Type, memdir.MemoryTypeProject)
	}
	if mf.Body != content {
		t.Errorf("Body: got %q, want %q", mf.Body, content)
	}
}

func TestParseMemoryFile_UnclosedFrontmatter(t *testing.T) {
	content := "---\ntitle: Broken\nNo closing separator"
	_, err := memdir.ParseMemoryFile(content, "/tmp/broken.md")
	if err == nil {
		t.Fatal("expected error for unclosed frontmatter, got nil")
	}
}

func TestFormatAndParseRoundTrip(t *testing.T) {
	original := &memdir.MemoryFile{
		Header: memdir.MemoryHeader{
			Title:     "Architecture Notes",
			Type:      memdir.MemoryTypeProject,
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			Tags:      []string{"architecture", "patterns"},
			Source:    "user",
		},
		Body: "Use Clean Architecture with six layers.",
		Path: "/tmp/arch.md",
	}

	formatted := memdir.FormatMemoryFile(original)
	parsed, err := memdir.ParseMemoryFile(formatted, "/tmp/arch.md")
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}

	if parsed.Header.Title != original.Header.Title {
		t.Errorf("Title: got %q, want %q", parsed.Header.Title, original.Header.Title)
	}
	if parsed.Header.Type != original.Header.Type {
		t.Errorf("Type: got %q, want %q", parsed.Header.Type, original.Header.Type)
	}
	if !strings.Contains(parsed.Body, "Clean Architecture") {
		t.Errorf("Body: expected 'Clean Architecture', got %q", parsed.Body)
	}
	if len(parsed.Header.Tags) != 2 {
		t.Errorf("Tags: got %d, want 2", len(parsed.Header.Tags))
	}
}

func TestMemoryStore_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	mf := &memdir.MemoryFile{
		Header: memdir.MemoryHeader{
			Title: "Test Memory",
			Type:  memdir.MemoryTypeUser,
			Tags:  []string{"test"},
		},
		Body: "This is a test memory for unit tests.",
	}

	if err := store.WriteMemory(mf); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	read, err := store.ReadMemory("test_memory")
	if err != nil {
		t.Fatalf("ReadMemory: %v", err)
	}
	if read.Header.Title != "Test Memory" {
		t.Errorf("Title: got %q, want %q", read.Header.Title, "Test Memory")
	}
	if !strings.Contains(read.Body, "unit tests") {
		t.Errorf("Body: expected 'unit tests', got %q", read.Body)
	}
}

func TestMemoryStore_ListMemories(t *testing.T) {
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	// Write memories with explicit time control by writing files directly.
	// WriteMemory always sets UpdatedAt=now(), so we write raw files.
	titles := []string{"First", "Second", "Third"}
	for i, title := range titles {
		slug := strings.ToLower(title)
		h := memdir.MemoryHeader{
			Title:     title,
			Type:      memdir.MemoryTypeProject,
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 1, 1, i, 0, 0, 0, time.UTC), // i=0,1,2
		}
		mf := &memdir.MemoryFile{
			Header: h,
			Body:   "Content for " + title,
			Path:   filepath.Join(dir, slug+".md"),
		}
		content := memdir.FormatMemoryFile(mf)
		if err := os.WriteFile(mf.Path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", title, err)
		}
	}

	memories, err := store.ListMemories()
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) != 3 {
		t.Fatalf("ListMemories: got %d, want 3", len(memories))
	}

	// Should be sorted by UpdatedAt descending.
	if memories[0].Header.Title != "Third" {
		t.Errorf("First memory should be 'Third' (most recent), got %q", memories[0].Header.Title)
	}
}

func TestMemoryStore_DeleteMemory(t *testing.T) {
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	mf := &memdir.MemoryFile{
		Header: memdir.MemoryHeader{Title: "To Delete", Type: memdir.MemoryTypeProject},
		Body:   "Will be deleted.",
	}
	if err := store.WriteMemory(mf); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	// Verify it exists.
	memories, _ := store.ListMemories()
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory before delete, got %d", len(memories))
	}

	// Delete it.
	if err := store.DeleteMemory("to_delete"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}

	// Verify it's gone.
	memories, _ = store.ListMemories()
	if len(memories) != 0 {
		t.Fatalf("expected 0 memories after delete, got %d", len(memories))
	}
}

func TestMemoryStore_BuildIndex(t *testing.T) {
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)

	mf := &memdir.MemoryFile{
		Header: memdir.MemoryHeader{
			Title: "Go Conventions",
			Type:  memdir.MemoryTypeProject,
		},
		Body: "Always use gofmt and goimports.",
	}
	if err := store.WriteMemory(mf); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	if err := store.BuildIndex(); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	indexContent, err := store.LoadMemoryIndex()
	if err != nil {
		t.Fatalf("LoadMemoryIndex: %v", err)
	}

	if !strings.Contains(indexContent, "Project Memories") {
		t.Error("index should contain 'Project Memories' header")
	}
	if !strings.Contains(indexContent, "Go Conventions") {
		t.Error("index should contain 'Go Conventions'")
	}
	// Verify the index uses Markdown link format with filename.
	if !strings.Contains(indexContent, "[Go Conventions](go_conventions.md)") {
		t.Errorf("index should contain Markdown link '[Go Conventions](go_conventions.md)', got:\n%s", indexContent)
	}
	// Verify summary is included after the em dash.
	if !strings.Contains(indexContent, "— Always use gofmt and goimports.") {
		t.Errorf("index should contain summary after em dash, got:\n%s", indexContent)
	}
}

func TestMemoryStore_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	nonExistent := filepath.Join(dir, "nonexistent")
	store := memdir.NewMemoryStoreWithPath(nonExistent)

	memories, err := store.ListMemories()
	if err != nil {
		t.Fatalf("ListMemories on nonexistent dir: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories for nonexistent dir, got %d", len(memories))
	}
}

func TestLoadAndTruncate_UTF8Safe(t *testing.T) {
	dir := t.TempDir()

	// Write a CLAUDE.md with multi-byte UTF-8 characters.
	content := "Hello 你好世界！🌍 This is a test."
	p := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Truncate to a small size that would cut a multi-byte character.
	result := memdir.LoadAndTruncate([]string{p}, 30)

	// The result should be valid UTF-8 and not contain broken characters.
	for i := 0; i < len(result); {
		r, size := []rune(result[i:])[0], len(string([]rune(result[i:])[0]))
		if r == 0xFFFD && result[i] != 0xEF { // replacement character check
			t.Errorf("found broken UTF-8 at position %d", i)
		}
		i += size
	}
}

func TestLoadAllMemory(t *testing.T) {
	dir := t.TempDir()

	// Create a CLAUDE.md file.
	claudePath := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# Project Instructions"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a MemoryStore with a memory file.
	memDir := filepath.Join(dir, "memory")
	store := memdir.NewMemoryStoreWithPath(memDir)
	mf := &memdir.MemoryFile{
		Header: memdir.MemoryHeader{
			Title: "User Preference",
			Type:  memdir.MemoryTypeUser,
		},
		Body: "Prefers Go with clean architecture.",
	}
	if err := store.WriteMemory(mf); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	if err := store.BuildIndex(); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	result := memdir.LoadAllMemory([]string{claudePath}, store)

	if !strings.Contains(result, "Project Instructions") {
		t.Error("expected CLAUDE.md content in result")
	}
	if !strings.Contains(result, "Auto Memory") {
		t.Error("expected Auto Memory section in result")
	}
	// Index should contain the title with filename link.
	if !strings.Contains(result, "[User Preference](user_preference.md)") {
		t.Errorf("expected Markdown link in index, got:\n%s", result)
	}
}
