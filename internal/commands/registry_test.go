package commands_test

import (
	"testing"

	"github.com/tunsuy/claude-code-go/internal/commands"
)

func TestRegistryLookup(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	cmd := r.Lookup("clear")
	if cmd == nil {
		t.Fatal("expected /clear to be registered")
	}
	if cmd.Name != "clear" {
		t.Errorf("expected name 'clear', got %q", cmd.Name)
	}
}

func TestRegistryCompletePrefix(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	matches := r.CompletePrefix("/cl")
	if len(matches) == 0 {
		t.Fatal("expected completions for '/cl'")
	}
	found := false
	for _, m := range matches {
		if m.Name == "clear" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'clear' in completions for '/cl'")
	}
}

func TestRegistryAll(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	all := r.All()
	if len(all) == 0 {
		t.Fatal("expected built-in commands to be registered")
	}
}

func TestCommandExecute(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	cmd := r.Lookup("exit")
	if cmd == nil {
		t.Fatal("expected /exit to be registered")
	}

	ctx := commands.CommandContext{Model: "claude-opus-4-5"}
	result := cmd.Execute(ctx, "")
	if !result.ShouldExit {
		t.Error("expected /exit to set ShouldExit = true")
	}
}

func TestHelpCommand(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	cmd := r.Lookup("help")
	if cmd == nil {
		t.Fatal("expected /help to be registered")
	}

	ctx := commands.CommandContext{}
	result := cmd.Execute(ctx, "")
	if result.Text == "" {
		t.Error("expected /help to return non-empty text")
	}
}
