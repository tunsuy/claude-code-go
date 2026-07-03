package memdir_test

import (
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

func TestLoadScopedAllMemory_IncludesIndex(t *testing.T) {
	dir := t.TempDir()
	store := memdir.NewMemoryStoreWithPath(dir)
	if err := store.WriteMemory(&memdir.MemoryFile{
		Header: memdir.MemoryHeader{Title: "Pref", Type: memdir.MemoryTypeUser},
		Body:   "Use tabs",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.BuildIndex(); err != nil {
		t.Fatal(err)
	}

	files := []memdir.DiscoveredFile{{Path: "ignored", Scope: memdir.ScopeProject}}
	out := memdir.LoadScopedAllMemory(files, store)
	if !strings.Contains(out, "# Auto Memory") {
		t.Fatalf("expected auto memory section, got %q", out)
	}
}
