package bootstrap

import (
	"testing"
)

// ── HandleFastPath ────────────────────────────────────────────────────────────

func TestHandleFastPath_VersionLong(t *testing.T) {
	if !HandleFastPath([]string{"claude", "--version"}) {
		t.Error("expected true for --version")
	}
}

func TestHandleFastPath_VersionShort(t *testing.T) {
	if !HandleFastPath([]string{"claude", "-v"}) {
		t.Error("expected true for -v")
	}
}

func TestHandleFastPath_NoFastPath(t *testing.T) {
	if HandleFastPath([]string{"claude", "--print", "hello"}) {
		t.Error("expected false for non-version flag")
	}
}

func TestHandleFastPath_DoubleDashStops(t *testing.T) {
	// "--" terminates flag scanning; "--version" after it is not a fast-path.
	if HandleFastPath([]string{"claude", "--", "--version"}) {
		t.Error("expected false: --version after -- should not trigger fast path")
	}
}

func TestHandleFastPath_OnlyBinary(t *testing.T) {
	if HandleFastPath([]string{"claude"}) {
		t.Error("expected false for no args")
	}
}

// ── isPrintMode ───────────────────────────────────────────────────────────────

func TestIsPrintMode_ShortFlag(t *testing.T) {
	if !isPrintMode([]string{"claude", "-p"}) {
		t.Error("expected true for -p")
	}
}

func TestIsPrintMode_LongFlag(t *testing.T) {
	if !isPrintMode([]string{"claude", "--print"}) {
		t.Error("expected true for --print")
	}
}

func TestIsPrintMode_NotPresent(t *testing.T) {
	if isPrintMode([]string{"claude", "--model", "opus"}) {
		t.Error("expected false when -p is absent")
	}
}

func TestIsPrintMode_AfterDoubleDash(t *testing.T) {
	if isPrintMode([]string{"claude", "--", "-p"}) {
		t.Error("expected false: -p after -- should not trigger print mode")
	}
}

// ── longDesc ─────────────────────────────────────────────────────────────────

func TestLongDesc_TrimsLeadingNewline(t *testing.T) {
	in := `
hello world`
	got := longDesc(in)
	want := "hello world"
	if got != want {
		t.Errorf("longDesc(%q) = %q, want %q", in, got, want)
	}
}

func TestLongDesc_TrimsTrailingNewline(t *testing.T) {
	in := "hello world\n"
	got := longDesc(in)
	want := "hello world"
	if got != want {
		t.Errorf("longDesc(%q) = %q, want %q", in, got, want)
	}
}

func TestLongDesc_Empty(t *testing.T) {
	got := longDesc("")
	if got != "" {
		t.Errorf("longDesc(\"\") = %q, want \"\"", got)
	}
}

func TestLongDesc_NoOp(t *testing.T) {
	in := "already trimmed"
	got := longDesc(in)
	if got != in {
		t.Errorf("longDesc(%q) = %q, want %q", in, got, in)
	}
}

// ── buildRootCmd ──────────────────────────────────────────────────────────────

func TestBuildRootCmd_Headless_HasNoSubcommands(t *testing.T) {
	cmd := buildRootCmd(true)
	if len(cmd.Commands()) != 0 {
		names := make([]string, 0, len(cmd.Commands()))
		for _, c := range cmd.Commands() {
			names = append(names, c.Name())
		}
		t.Errorf("headless mode: expected no subcommands, got %v", names)
	}
}

func TestBuildRootCmd_Interactive_HasSubcommands(t *testing.T) {
	cmd := buildRootCmd(false)
	if len(cmd.Commands()) == 0 {
		t.Error("interactive mode: expected at least one subcommand, got none")
	}
}

func TestBuildRootCmd_Name(t *testing.T) {
	cmd := buildRootCmd(true)
	if cmd.Name() != "claude" {
		t.Errorf("expected root command name %q, got %q", "claude", cmd.Name())
	}
}

// ── Run (via buildRootCmd help) ───────────────────────────────────────────────

// TestRun_Help verifies that Run with --help does not return an error.
// cobra exits with code 0 for --help and Run returns nil.
func TestRun_Help(t *testing.T) {
	err := Run([]string{"claude", "--help"})
	if err != nil {
		t.Errorf("Run([\"claude\", \"--help\"]) returned unexpected error: %v", err)
	}
}

// TestRun_Version verifies that HandleFastPath intercepts --version before Run.
func TestRun_Version(t *testing.T) {
	// Run is NOT called for --version (main.go checks HandleFastPath first).
	// We verify HandleFastPath returns true so Run is skipped.
	if !HandleFastPath([]string{"claude", "--version"}) {
		t.Error("expected HandleFastPath to return true for --version")
	}
}
