package main

import (
	"os"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/bootstrap"
)

// TestMain restores os.Args after the test suite completes so that any
// parallel test infrastructure is not polluted.
func TestMain(m *testing.M) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Exit(m.Run())
}

// TestMain_HelpFlag verifies that passing --help to bootstrap.Run does not
// panic.  Cobra prints usage and returns an error (or nil) for --help; both
// outcomes are acceptable here — the important invariant is no panic.
func TestMain_HelpFlag(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("bootstrap.Run panicked with --help: %v", r)
		}
	}()

	// bootstrap.Run uses the supplied args slice directly; it does NOT call
	// os.Exit, so this is safe to call from a test.
	_ = bootstrap.Run([]string{"claude", "--help"})
}

// TestMain_VersionFlag verifies that HandleFastPath returns true when
// --version is supplied, indicating the process should exit cleanly.
func TestMain_VersionFlag(t *testing.T) {
	os.Args = []string{"claude", "--version"}

	handled := bootstrap.HandleFastPath(os.Args)
	if !handled {
		t.Error("HandleFastPath returned false for --version, expected true")
	}
}

// TestMain_NoFastPath verifies that HandleFastPath returns false for regular flags.
func TestMain_NoFastPath(t *testing.T) {
	handled := bootstrap.HandleFastPath([]string{"claude", "--print", "hello"})
	if handled {
		t.Error("HandleFastPath returned true for non-version flag, expected false")
	}
}

// TestMain_ShortVersionFlag verifies that -v is treated as the version flag.
func TestMain_ShortVersionFlag(t *testing.T) {
	handled := bootstrap.HandleFastPath([]string{"claude", "-v"})
	if !handled {
		t.Error("HandleFastPath returned false for -v, expected true")
	}
}

// TestMain_RunUnknownSubcommand verifies that Run with an unknown subcommand
// returns an error (and does not panic).
func TestMain_RunUnknownSubcommand(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("bootstrap.Run panicked: %v", r)
		}
	}()

	// An unknown subcommand should return an error without panicking.
	err := bootstrap.Run([]string{"claude", "unknown-subcommand-xyz"})
	if err == nil {
		t.Error("expected error for unknown subcommand, got nil")
	}
}

// TestMainFunc_Help exercises main() directly with --help to get coverage.
// We set os.Args before the call so cobra prints help and returns nil.
// main() will NOT call os.Exit in this path.
func TestMainFunc_Help(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	os.Args = []string{"claude", "--help"}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("main() panicked: %v", r)
		}
	}()

	main()
}
